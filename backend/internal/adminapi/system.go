package adminapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ---------- API key admin (untuk server / otomasi) ----------
// Catatan: key ini baru dikelola (dibuat, dicabut, dilihat prefix-nya). Belum dipakai sebagai cara autentikasi API admin.

func (s *Server) listKeys(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT id::text AS id, name, prefix, created_at, last_used_at, revoked_at
		FROM api_keys WHERE workspace_id IS NULL ORDER BY created_at DESC`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (s *Server) createKey(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		bad(c, "Nama key wajib diisi (maks. 80 karakter).")
		return
	}
	rnd, err := secure.RandomToken(24)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	key := "cvr_admin_" + rnd
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var id string
	if err := tx.QueryRow(c, `INSERT INTO api_keys (name, prefix, key_hash, created_by) VALUES ($1,$2,$3,$4::uuid) RETURNING id::text`,
		in.Name, key[:14], secure.HashToken(key), adminID(c)).Scan(&id); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "api_key.create", "", "api_key", id, map[string]any{"name": in.Name}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.Status(c, http.StatusCreated, gin.H{"id": id, "name": in.Name, "key": key, "prefix": key[:14],
		"note": "Simpan key ini sekarang. Key lengkap tidak akan ditampilkan lagi."})
}

func (s *Server) revokeKey(c *gin.Context) {
	s.withAudit(c, "api_key.revoke", "api_key", func(tx pgx.Tx) (string, map[string]any, error) {
		tag, err := tx.Exec(c, `UPDATE api_keys SET revoked_at = now() WHERE id::text = $1 AND workspace_id IS NULL AND revoked_at IS NULL`, c.Param("id"))
		if err == nil && tag.RowsAffected() == 0 {
			err = pgx.ErrNoRows
		}
		return c.Param("id"), nil, err
	}, http.StatusOK)
}

// ---------- Audit log ----------

func (s *Server) listAudit(c *gin.Context) {
	w := &httpx.Where{}
	if v := c.Query("action"); v != "" {
		w.Add("a.action LIKE ? || '%'", httpx.EscapeLike(v))
	}
	if v := c.Query("workspace_id"); v != "" {
		w.Add("a.workspace_id::text = ?", v)
	}
	if v := c.Query("from"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, jakarta())
		if err != nil {
			bad(c, "from harus YYYY-MM-DD.")
			return
		}
		w.Add("a.at >= ?", t)
	}
	if v := c.Query("to"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, jakarta())
		if err != nil {
			bad(c, "to harus YYYY-MM-DD.")
			return
		}
		w.Add("a.at < ?", t.AddDate(0, 0, 1))
	}
	page, per, off := httpx.Paging(c)
	var total int
	if err := s.DB.QueryRow(c, `SELECT count(*) FROM audit_logs a WHERE `+w.SQL(), w.Args...).Scan(&total); err != nil {
		httpx.ServerError(c, err)
		return
	}
	args := append(w.Args, per, off)
	rows, err := httpx.Query(c, s.DB, `SELECT a.id::text AS id, a.at, a.actor_type, a.actor_label, a.action, a.target_type, a.target_id,
	  a.workspace_id::text AS workspace_id, ws.name AS workspace, a.detail
	FROM audit_logs a LEFT JOIN workspaces ws ON ws.id = a.workspace_id WHERE `+w.SQL()+
		` ORDER BY a.at DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.Page(c, rows, page, per, total)
}

// ---------- Status sistem ----------

func (s *Server) systemStatus(c *gin.Context) {
	t0 := time.Now()
	if err := s.DB.Ping(c); err != nil {
		httpx.Fail(c, http.StatusServiceUnavailable, "db_down", "database tidak terjangkau")
		return
	}
	dbMs := time.Since(t0).Milliseconds()

	ev, err := httpx.Query(c, s.DB, `
SELECT count(*) FILTER (WHERE status = 'sent') AS sent,
  count(*) FILTER (WHERE status = 'failed') AS failed,
  count(*) FILTER (WHERE status = 'skipped') AS skipped
FROM events WHERE source <> 'test' AND created_at > now() - interval '24 hours'`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	q, err := httpx.Query(c, s.DB, `
SELECT count(*) FILTER (WHERE status = 'queued') AS queued,
  count(*) FILTER (WHERE status = 'queued' AND attempts > 0) AS retrying,
  count(*) FILTER (WHERE status = 'pending_value') AS pending_value,
  COALESCE(max(EXTRACT(EPOCH FROM now() - next_retry_at)) FILTER (WHERE status = 'queued' AND next_retry_at < now()), 0)::float8 AS oldest_wait_seconds
FROM events`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	other, err := httpx.Query(c, s.DB, `
SELECT (SELECT count(*) FROM payment_proofs WHERE status = 'pending' AND deleted_at IS NULL) AS pending_proofs,
  (SELECT count(*) FROM webhook_inbox WHERE received_at > now() - interval '24 hours') AS webhooks_24h,
  (SELECT count(*) FROM webhook_inbox WHERE received_at > now() - interval '24 hours' AND error IS NOT NULL) AS webhook_errors_24h,
  (SELECT count(*) FROM meta_connections WHERE status IN ('invalid', 'expired')) AS connections_need_attention`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	sent, failed := ev[0]["sent"].(int64), ev[0]["failed"].(int64)
	var errRate any
	if sent+failed > 0 {
		errRate = float64(failed) / float64(sent+failed)
	}
	httpx.OK(c, gin.H{
		"uptime_seconds": int64(time.Since(s.Started).Seconds()),
		"db_ping_ms":     dbMs,
		"events_24h":     ev[0],
		"error_rate_24h": errRate,
		"queue":          q[0],
		"other":          other[0],
		"server_time":    time.Now().UTC(),
	})
}

// ---------- Notifikasi admin ----------

func (s *Server) listNotifications(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT id::text AS id, type, title, body, link, read_at, created_at
		FROM notifications WHERE admin_id::text = $1 ORDER BY created_at DESC LIMIT 50`, adminID(c))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (s *Server) readAllNotifications(c *gin.Context) {
	if _, err := s.DB.Exec(c, `UPDATE notifications SET read_at = now() WHERE admin_id::text = $1 AND read_at IS NULL`, adminID(c)); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"read": true})
}
