package adminapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Dukungan customer: tim Convera bisa melihat event tiap customer (status, error, saran perbaikan) untuk membantu
// bila ada kendala. Nomor HP selalu tersamar; payload hanya dibuka di halaman detail dan pembukaannya dicatat di audit log.

const adminEventCols = `e.id::text AS id, e.workspace_id::text AS workspace_id, w.name AS customer, u.name AS owner,
  e.event_name, e.status, e.source, e.mode, e.value::float8 AS value, e.currency, e.attempts, e.next_retry_at,
  e.error_code, e.error_message, e.fbtrace_id, e.event_time, e.sent_at, e.created_at,
  CASE WHEN l.phone IS NULL THEN NULL ELSE left(l.phone,4) || '****' || right(l.phone,4) END AS phone_masked,
  l.name AS lead_name, r.name AS rule_name`

const adminEventFrom = `
FROM events e
JOIN workspaces w ON w.id = e.workspace_id
JOIN users u ON u.id = w.owner_user_id
LEFT JOIN leads l ON l.id = e.lead_id
LEFT JOIN event_rules r ON r.id = e.rule_id`

var eventStatuses = map[string]bool{"queued": true, "sent": true, "failed": true, "skipped": true, "pending_value": true}

// eventFilter membangun WHERE dari query (status, event_name, q, from, to, include_test) + workspace tetap bila ada.
// withStatus=false dipakai untuk menghitung jumlah per status tanpa filter status.
func (s *Server) eventFilter(c *gin.Context, workspaceID string, withStatus bool) (*httpx.Where, bool) {
	w := &httpx.Where{}
	if workspaceID == "" {
		workspaceID = c.Query("workspace_id")
	}
	if workspaceID != "" {
		w.Add("e.workspace_id::text = ?", workspaceID)
	}
	if c.Query("include_test") != "1" {
		w.Raw("e.source <> 'test'")
	}
	if withStatus {
		if st := c.Query("status"); st != "" && st != "all" {
			if !eventStatuses[st] {
				bad(c, "status tidak dikenal.")
				return nil, false
			}
			w.Add("e.status = ?", st)
		}
	}
	if v := c.Query("event_name"); v != "" {
		w.Add("e.event_name = ?", v)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add("(w.name ILIKE '%' || ? || '%' OR u.name ILIKE '%' || ? || '%' OR l.name ILIKE '%' || ? || '%')", httpx.EscapeLike(q))
	}
	// Tanpa rentang tanggal: 30 hari terakhir (log bisa sangat besar).
	from, to := c.Query("from"), c.Query("to")
	if from == "" && to == "" {
		w.Raw("e.created_at > now() - interval '30 days'")
	}
	if from != "" {
		t, err := time.ParseInLocation("2006-01-02", from, jakarta())
		if err != nil {
			bad(c, "from harus YYYY-MM-DD.")
			return nil, false
		}
		w.Add("e.created_at >= ?", t)
	}
	if to != "" {
		t, err := time.ParseInLocation("2006-01-02", to, jakarta())
		if err != nil {
			bad(c, "to harus YYYY-MM-DD.")
			return nil, false
		}
		w.Add("e.created_at < ?", t.AddDate(0, 0, 1))
	}
	return w, true
}

// listEvents melayani GET /events (semua customer) dan GET /customers/{id}/events (satu customer).
func (s *Server) listEvents(c *gin.Context) {
	w, ok := s.eventFilter(c, c.Param("id"), true)
	if !ok {
		return
	}
	page, per, off := httpx.Paging(c)
	var total int
	if err := s.DB.QueryRow(c, `SELECT count(*) `+adminEventFrom+` WHERE `+w.SQL(), w.Args...).Scan(&total); err != nil {
		httpx.ServerError(c, err)
		return
	}
	rows, err := httpx.Query(c, s.DB, fmt.Sprintf(`SELECT %s %s WHERE %s ORDER BY e.created_at DESC LIMIT %d OFFSET %d`,
		adminEventCols, adminEventFrom, w.SQL(), per, off), w.Args...)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	for _, r := range rows {
		if code, _ := r["error_code"].(string); code != "" {
			r["hint"] = supportHint(code)
		}
	}
	// Jumlah per status (mengikuti filter lain, kecuali status) untuk chip.
	cw, ok := s.eventFilter(c, c.Param("id"), false)
	if !ok {
		return
	}
	counts, err := httpx.Query(c, s.DB, `SELECT count(*) AS "all",
	  count(*) FILTER (WHERE e.status = 'sent') AS sent, count(*) FILTER (WHERE e.status = 'failed') AS failed,
	  count(*) FILTER (WHERE e.status = 'queued') AS queued, count(*) FILTER (WHERE e.status = 'pending_value') AS pending_value,
	  count(*) FILTER (WHERE e.status = 'skipped') AS skipped `+adminEventFrom+` WHERE `+cw.SQL(), cw.Args...)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"page": page, "per_page": per, "total": total, "counts": counts[0]}})
}

// getEvent: detail lengkap termasuk payload yang dikirim (tanpa token) dan respons Meta. Dicatat di audit log.
func (s *Server) getEvent(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT `+adminEventCols+`, e.event_id, e.request_payload, e.meta_response,
	  l.status AS lead_status, l.ad_id, l.ad_headline, l.source AS lead_source, (l.ctwa_clid IS NOT NULL) AS lead_from_ad, l.attribution_expires_at `+
		adminEventFrom+` WHERE e.id::text = $1`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if len(rows) == 0 {
		httpx.Fail(c, http.StatusNotFound, "event_not_found", "Event tidak ditemukan.")
		return
	}
	r := rows[0]
	if code, _ := r["error_code"].(string); code != "" {
		r["hint"] = supportHint(code)
	}
	if err := s.audit(c, s.DB, "event.view", r["workspace_id"].(string), "event", c.Param("id"), nil); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, r)
}

// retryEvent mengantrekan ulang event 'failed' milik customer sebagai bantuan; tercatat di audit log.
func (s *Server) retryEvent(c *gin.Context) {
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var ws string
	err = tx.QueryRow(c, `UPDATE events SET status = 'queued', attempts = 0, next_retry_at = now(), error_code = NULL, error_message = NULL
		WHERE id::text = $1 AND status = 'failed' AND source <> 'test' RETURNING workspace_id::text`, c.Param("id")).Scan(&ws)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, http.StatusConflict, "not_retryable", "Hanya event berstatus gagal yang bisa dikirim ulang.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "event.retry", ws, "event", c.Param("id"), nil); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"status": "queued"})
}

// customerDiagnostics merangkum kondisi pengiriman event satu customer untuk halaman detail.
func (s *Server) customerDiagnostics(c *gin.Context, workspaceID string) (map[string]any, error) {
	rows, err := httpx.Query(c, s.DB, `
SELECT (SELECT count(*) FROM events WHERE workspace_id::text = $1 AND status = 'failed' AND source <> 'test' AND created_at > now() - interval '24 hours') AS failed_24h,
  (SELECT count(*) FROM events WHERE workspace_id::text = $1 AND status = 'failed' AND source <> 'test' AND created_at > now() - interval '7 days') AS failed_7d,
  (SELECT count(*) FROM events WHERE workspace_id::text = $1 AND status = 'queued') AS queued,
  (SELECT count(*) FROM events WHERE workspace_id::text = $1 AND status = 'pending_value') AS pending_value,
  (SELECT max(created_at) FROM events WHERE workspace_id::text = $1 AND source <> 'test') AS last_event_at,
  (SELECT max(received_at) FROM webhook_inbox WHERE workspace_id::text = $1) AS last_message_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	d := rows[0]
	top, err := httpx.Query(c, s.DB, `
SELECT COALESCE(error_code, '') AS code, left(COALESCE(error_message, ''), 200) AS message, count(*) AS total, max(created_at) AS last_at
FROM events WHERE workspace_id::text = $1 AND status = 'failed' AND source <> 'test' AND created_at > now() - interval '7 days'
GROUP BY 1, 2 ORDER BY total DESC, last_at DESC LIMIT 5`, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, t := range top {
		t["hint"] = supportHint(t["code"].(string))
	}
	d["top_errors"] = top
	return d, nil
}
