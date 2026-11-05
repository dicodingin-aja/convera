package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/jobs"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// where membangun klausa WHERE dengan placeholder bernomor.
type where struct {
	parts []string
	args  []any
}

func (w *where) add(cond string, val any) {
	w.args = append(w.args, val)
	w.parts = append(w.parts, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(w.args))))
}

func (w *where) sql() string { return strings.Join(w.parts, " AND ") }

// dateRange membaca ?from dan ?to (YYYY-MM-DD, zona Asia/Jakarta). `to` inklusif.
func dateRange(c *gin.Context, defDays int) (from, to time.Time, err error) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	today := time.Now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	from, to = today.AddDate(0, 0, -(defDays-1)), today
	if v := c.Query("from"); v != "" {
		if from, err = time.ParseInLocation("2006-01-02", v, loc); err != nil {
			return from, to, fmt.Errorf("from harus YYYY-MM-DD")
		}
	}
	if v := c.Query("to"); v != "" {
		if to, err = time.ParseInLocation("2006-01-02", v, loc); err != nil {
			return from, to, fmt.Errorf("to harus YYYY-MM-DD")
		}
	}
	if to.Before(from) {
		return from, to, fmt.Errorf("to tidak boleh sebelum from")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return from, to, fmt.Errorf("rentang maksimal 366 hari")
	}
	return from, to.AddDate(0, 0, 1), nil // to dijadikan batas atas eksklusif
}

// ---------- Overview ----------

func (s *Server) overview(c *gin.Context) {
	from, to, err := dateRange(c, 30)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	id := wsID(c)
	// Rekap hari ini dan kemarin dihitung ulang untuk workspace ini agar angka selalu segar (murah: satu workspace, 2 hari).
	if err := jobs.RecomputeStats(c, s.DB, 1, id); err != nil {
		_ = c.Error(err)
	}

	kpi, err := query(c, s.DB, `
SELECT
 (SELECT count(*) FROM leads WHERE workspace_id=$1 AND deleted_at IS NULL AND ctwa_clid IS NOT NULL AND first_message_at >= $2 AND first_message_at < $3) AS chats_from_ads,
 (SELECT count(*) FROM leads WHERE workspace_id=$1 AND deleted_at IS NULL AND first_message_at >= $2 AND first_message_at < $3) AS leads,
 (SELECT count(*) FROM events WHERE workspace_id=$1 AND event_name='Purchase' AND status='sent' AND source<>'test' AND event_time >= $2 AND event_time < $3) AS purchases,
 (SELECT COALESCE(sum(value),0)::float8 FROM events WHERE workspace_id=$1 AND event_name='Purchase' AND status='sent' AND source<>'test' AND event_time >= $2 AND event_time < $3) AS purchase_value,
 (SELECT count(*) FROM events WHERE workspace_id=$1 AND status='sent' AND source<>'test' AND created_at >= $2 AND created_at < $3) AS events_sent,
 (SELECT count(*) FROM events WHERE workspace_id=$1 AND status='failed' AND source<>'test' AND created_at >= $2 AND created_at < $3) AS events_failed`, id, from, to)
	if err != nil {
		serverError(c, err)
		return
	}
	k := kpi[0]
	// match_rate = event terkirim / (terkirim + gagal). Definisi sementara; belum ditetapkan di dokumen.
	sent, failed := k["events_sent"].(int64), k["events_failed"].(int64)
	if sent+failed > 0 {
		k["match_rate"] = float64(sent) / float64(sent+failed)
	} else {
		k["match_rate"] = nil
	}

	// Periode pembanding: rentang yang sama panjangnya tepat sebelum rentang ini (untuk persentase naik/turun).
	span := to.Sub(from)
	prev, err := query(c, s.DB, `
SELECT
 (SELECT count(*) FROM leads WHERE workspace_id=$1 AND deleted_at IS NULL AND ctwa_clid IS NOT NULL AND first_message_at >= $2 AND first_message_at < $3) AS chats_from_ads,
 (SELECT count(*) FROM leads WHERE workspace_id=$1 AND deleted_at IS NULL AND first_message_at >= $2 AND first_message_at < $3) AS leads,
 (SELECT count(*) FROM events WHERE workspace_id=$1 AND event_name='Purchase' AND status='sent' AND source<>'test' AND event_time >= $2 AND event_time < $3) AS purchases,
 (SELECT COALESCE(sum(value),0)::float8 FROM events WHERE workspace_id=$1 AND event_name='Purchase' AND status='sent' AND source<>'test' AND event_time >= $2 AND event_time < $3) AS purchase_value`, id, from.Add(-span), from)
	if err != nil {
		serverError(c, err)
		return
	}

	// Data iklan dari Meta (bila tersambung) untuk rentang ini dan periode pembanding.
	pf, pt := from.Add(-span).Format("2006-01-02"), from.AddDate(0, 0, -1).Format("2006-01-02")
	adsQ, err := query(c, s.DB, `
SELECT EXISTS (SELECT 1 FROM ad_connections WHERE workspace_id = $1) AS connected,
  COALESCE((SELECT currency FROM ad_connections WHERE workspace_id = $1), '') AS currency,
  COALESCE(sum(spend) FILTER (WHERE day BETWEEN $2::date AND $3::date), 0)::float8 AS spend,
  COALESCE(sum(clicks) FILTER (WHERE day BETWEEN $2::date AND $3::date), 0)::bigint AS clicks,
  COALESCE(sum(impressions) FILTER (WHERE day BETWEEN $2::date AND $3::date), 0)::bigint AS impressions,
  COALESCE(sum(conversations) FILTER (WHERE day BETWEEN $2::date AND $3::date), 0)::bigint AS conversations,
  COALESCE(sum(spend) FILTER (WHERE day BETWEEN $4::date AND $5::date), 0)::float8 AS previous_spend
FROM ad_insights_daily WHERE workspace_id = $1`, id, from.Format("2006-01-02"), to.AddDate(0, 0, -1).Format("2006-01-02"), pf, pt)
	if err != nil {
		serverError(c, err)
		return
	}

	series, err := query(c, s.DB, `
SELECT to_char(g.d, 'YYYY-MM-DD') AS date, COALESCE(ds.chats_from_ads, 0) AS chats_from_ads, COALESCE(ds.leads, 0) AS leads,
       COALESCE(ds.checkouts, 0) AS checkouts, COALESCE(ds.purchases, 0) AS purchases, COALESCE(ds.purchase_value, 0)::float8 AS purchase_value
FROM generate_series(($2::timestamptz AT TIME ZONE 'Asia/Jakarta')::date, (($3::timestamptz AT TIME ZONE 'Asia/Jakarta')::date - 1), interval '1 day') AS g(d)
LEFT JOIN daily_stats ds ON ds.workspace_id = $1 AND ds.day = g.d::date
ORDER BY g.d`, id, from, to)
	if err != nil {
		serverError(c, err)
		return
	}

	// Funnel: posisi lead saat ini, untuk lead yang pertama kali chat dalam rentang.
	funnel, err := query(c, s.DB, `
SELECT count(*) AS leads, (count(*) FILTER (WHERE ctwa_clid IS NOT NULL)) AS from_ads,
       (count(*) FILTER (WHERE status = 'new')) AS new, (count(*) FILTER (WHERE status = 'lead')) AS lead,
       (count(*) FILTER (WHERE status = 'checkout')) AS checkout, (count(*) FILTER (WHERE status = 'purchase')) AS purchase,
       (count(*) FILTER (WHERE status = 'lost')) AS lost
FROM leads WHERE workspace_id = $1 AND deleted_at IS NULL AND first_message_at >= $2 AND first_message_at < $3`, id, from, to)
	if err != nil {
		serverError(c, err)
		return
	}

	top, err := query(c, s.DB, `
SELECT l.ad_id, max(l.ad_headline) AS headline, count(DISTINCT l.id) AS leads,
       count(DISTINCT e.id) FILTER (WHERE e.event_name='Purchase' AND e.status='sent') AS purchases,
       COALESCE(sum(e.value) FILTER (WHERE e.event_name='Purchase' AND e.status='sent'),0)::float8 AS purchase_value
FROM leads l LEFT JOIN events e ON e.lead_id = l.id AND e.source <> 'test'
WHERE l.workspace_id=$1 AND l.deleted_at IS NULL AND l.ad_id IS NOT NULL AND l.first_message_at >= $2 AND l.first_message_at < $3
GROUP BY l.ad_id ORDER BY purchases DESC, leads DESC LIMIT 10`, id, from, to)
	if err != nil {
		serverError(c, err)
		return
	}

	// Kesehatan koneksi: apakah event benar-benar mengalir ke Meta.
	health, err := query(c, s.DB, `
SELECT mc.mode, mc.status AS meta_status, mc.last_check_at, mc.token_expires_at,
  (mc.access_token_enc IS NOT NULL AND mc.pixel_id IS NOT NULL) AS configured,
  (SELECT count(*) FROM events WHERE workspace_id = $1 AND status = 'failed' AND source <> 'test' AND created_at > now() - interval '24 hours') AS failed_24h,
  (SELECT count(*) FROM events WHERE workspace_id = $1 AND status = 'queued') AS queued,
  (SELECT count(*) FROM events WHERE workspace_id = $1 AND status = 'pending_value') AS pending_value,
  (SELECT max(received_at) FROM webhook_inbox WHERE workspace_id = $1) AS last_message_at,
  (SELECT status FROM wa_sessions WHERE workspace_id = $1) AS qr_status
FROM meta_connections mc WHERE mc.workspace_id = $1`, id)
	if err != nil {
		serverError(c, err)
		return
	}
	var h any
	if len(health) > 0 {
		h = health[0]
	}
	ok(c, gin.H{"kpi": k, "previous": prev[0], "ads": adsQ[0], "series": series, "funnel": funnel[0], "top_ads": top, "health": h,
		"range": gin.H{"from": from.Format("2006-01-02"), "to": to.AddDate(0, 0, -1).Format("2006-01-02")}})
}

// ---------- Leads ----------

const leadCols = `l.id::text AS id, l.phone, left(l.phone,4) || '****' || right(l.phone,4) AS phone_masked, COALESCE(l.name, l.push_name) AS name, l.city,
  (SELECT m.body FROM lead_messages m WHERE m.lead_id = l.id ORDER BY m.sent_at DESC LIMIT 1) AS last_message,
  l.status, l.source, l.ctwa_clid, l.ad_id, l.ad_headline, ai.ad_name, ai.campaign_name, ai.adset_name, l.campaign_id, l.clicked_at, l.attribution_expires_at,
  (l.attribution_expires_at IS NOT NULL AND l.attribution_expires_at < now()) AS attribution_expired,
  l.first_message_at, l.last_message_at, l.value::float8 AS value, (l.ctwa_clid IS NOT NULL) AS from_ad`

func (s *Server) listLeads(c *gin.Context) {
	from, to, err := dateRange(c, 30)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	w := &where{}
	w.add("l.workspace_id = ?", wsID(c))
	w.parts = append(w.parts, "l.deleted_at IS NULL")
	if c.Query("from") != "" || c.Query("to") != "" {
		w.add("l.first_message_at >= ?", from)
		w.add("l.first_message_at < ?", to)
	}
	for param, col := range map[string]string{"status": "l.status", "ad_id": "l.ad_id", "source": "l.source"} {
		if v := c.Query(param); v != "" {
			w.add(col+" = ?", v)
		}
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		var digits strings.Builder
		for _, r := range q {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			}
		}
		if digits.Len() >= 3 { // cari juga lewat nomor
			w.args = append(w.args, esc, digits.String())
			n := len(w.args)
			w.parts = append(w.parts, fmt.Sprintf(`(l.name ILIKE '%%' || $%d || '%%' OR l.phone LIKE '%%' || $%d || '%%')`, n-1, n))
		} else {
			w.add(`l.name ILIKE '%' || ? || '%'`, esc)
		}
	}
	page, per, off := paging(c)
	var total int
	if err := s.DB.QueryRow(c, `SELECT count(*) FROM leads l WHERE `+w.sql(), w.args...).Scan(&total); err != nil {
		serverError(c, err)
		return
	}
	rows, err := query(c, s.DB, fmt.Sprintf(`SELECT %s FROM leads l LEFT JOIN LATERAL (SELECT x.ad_name, x.campaign_name, x.adset_name FROM ad_insights_daily x WHERE x.workspace_id = l.workspace_id AND x.ad_id = l.ad_id ORDER BY x.day DESC LIMIT 1) ai ON true WHERE %s ORDER BY COALESCE(l.last_message_at, l.first_message_at) DESC LIMIT %d OFFSET %d`,
		leadCols, w.sql(), per, off), w.args...)
	if err != nil {
		serverError(c, err)
		return
	}
	okPage(c, rows, page, per, total)
}

func (s *Server) getLead(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT `+leadCols+` FROM leads l LEFT JOIN LATERAL (SELECT x.ad_name, x.campaign_name, x.adset_name FROM ad_insights_daily x WHERE x.workspace_id = l.workspace_id AND x.ad_id = l.ad_id ORDER BY x.day DESC LIMIT 1) ai ON true WHERE l.id::text = $1 AND l.workspace_id = $2 AND l.deleted_at IS NULL`, c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if len(rows) == 0 {
		fail(c, http.StatusNotFound, "lead_not_found", "Lead tidak ditemukan.")
		return
	}
	events, err := query(c, s.DB, `SELECT id::text, event_name, status, source, value::float8 AS value, currency, event_time, sent_at, error_message
		FROM events WHERE lead_id::text = $1 AND workspace_id = $2 ORDER BY created_at DESC LIMIT 50`, c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	rows[0]["events"] = events
	msgs, err := query(c, s.DB, `SELECT direction, body, sent_at FROM (SELECT direction, body, sent_at FROM lead_messages WHERE lead_id::text = $1 AND workspace_id = $2 ORDER BY sent_at DESC LIMIT 100) t ORDER BY sent_at`, c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	rows[0]["messages"] = msgs
	ok(c, rows[0])
}

func (s *Server) manualConversion(c *gin.Context) {
	var in struct {
		EventName string   `json:"event_name"`
		Value     *float64 `json:"value"`
		Currency  string   `json:"currency"`
		City      string   `json:"city"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	var leadID string
	err := s.DB.QueryRow(c, `SELECT id::text FROM leads WHERE id::text=$1 AND workspace_id=$2 AND deleted_at IS NULL`, c.Param("id"), wsID(c)).Scan(&leadID)
	if err == pgx.ErrNoRows {
		fail(c, http.StatusNotFound, "lead_not_found", "Lead tidak ditemukan.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	if in.City != "" {
		_, _ = s.DB.Exec(c, `UPDATE leads SET city = $2 WHERE id = $1`, leadID, in.City)
	}
	out, err := core.CreateConversion(c, s.DB, core.Params{
		WorkspaceID: wsID(c), LeadID: leadID, EventName: in.EventName, Value: in.Value, Currency: in.Currency,
		Source: "manual", CreatedBy: c.GetString("user_id"),
	})
	if coreError(c, err) {
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	okStatus(c, http.StatusAccepted, out)
}

// ---------- Events ----------

const eventCols = `e.id::text AS id, e.event_name, e.status, e.source, e.mode, e.value::float8 AS value, e.currency,
  e.event_time, e.sent_at, e.attempts, e.next_retry_at, e.error_code, e.error_message, e.fbtrace_id, e.created_at,
  e.lead_id::text AS lead_id, l.phone, CASE WHEN l.phone IS NULL THEN NULL ELSE left(l.phone,4) || '****' || right(l.phone,4) END AS phone_masked,
  COALESCE(l.name, l.push_name) AS lead_name, r.name AS rule_name, e.event_id, l.ctwa_clid`

// eventsWhere membangun filter Event Log (status, jenis event, rentang tanggal). /conversions tanpa event uji.
func (s *Server) eventsWhere(c *gin.Context) (*where, bool) {
	from, to, err := dateRange(c, 30)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return nil, false
	}
	w := &where{}
	w.add("e.workspace_id = ?", wsID(c))
	if strings.HasSuffix(c.FullPath(), "/conversions") {
		w.parts = append(w.parts, "e.source <> 'test'")
	}
	if c.Query("from") != "" || c.Query("to") != "" {
		w.add("e.created_at >= ?", from)
		w.add("e.created_at < ?", to)
	}
	if v := c.Query("status"); v != "" {
		w.add("e.status = ?", v)
	}
	if v := c.Query("event_name"); v != "" {
		w.add("e.event_name = ?", v)
	}
	return w, true
}

func (s *Server) listEvents(c *gin.Context) {
	w, okFilter := s.eventsWhere(c)
	if !okFilter {
		return
	}
	page, per, off := paging(c)
	var total int
	if err := s.DB.QueryRow(c, `SELECT count(*) FROM events e WHERE `+w.sql(), w.args...).Scan(&total); err != nil {
		serverError(c, err)
		return
	}
	rows, err := query(c, s.DB, fmt.Sprintf(`SELECT %s FROM events e LEFT JOIN leads l ON l.id = e.lead_id LEFT JOIN event_rules r ON r.id = e.rule_id
		WHERE %s ORDER BY e.created_at DESC LIMIT %d OFFSET %d`, eventCols, w.sql(), per, off), w.args...)
	if err != nil {
		serverError(c, err)
		return
	}
	for _, r := range rows {
		if code, _ := r["error_code"].(string); code != "" {
			r["hint"] = customerHint(code)
		}
	}
	okPage(c, rows, page, per, total)
}

func (s *Server) getEvent(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT `+eventCols+`, e.request_payload, e.meta_response
		FROM events e LEFT JOIN leads l ON l.id = e.lead_id LEFT JOIN event_rules r ON r.id = e.rule_id
		WHERE e.id::text = $1 AND e.workspace_id = $2`, c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if len(rows) == 0 {
		fail(c, http.StatusNotFound, "event_not_found", "Event tidak ditemukan.")
		return
	}
	if code, _ := rows[0]["error_code"].(string); code != "" {
		rows[0]["hint"] = customerHint(code)
	}
	ok(c, rows[0])
}

// retryEvent mengantrekan ulang event berstatus 'failed' saja.
func (s *Server) retryEvent(c *gin.Context) {
	tag, err := s.DB.Exec(c, `UPDATE events SET status='queued', attempts=0, next_retry_at=now(), error_code=NULL, error_message=NULL
		WHERE id::text=$1 AND workspace_id=$2 AND status='failed' AND source <> 'test'`, c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, http.StatusConflict, "not_retryable", "Hanya event berstatus gagal yang bisa dikirim ulang.")
		return
	}
	ok(c, gin.H{"status": "queued"})
}

// setEventValue mengisi nilai event 'pending_value' lalu mengantrekannya.
func (s *Server) setEventValue(c *gin.Context) {
	var in struct {
		Value float64 `json:"value"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Value <= 0 {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "value wajib angka > 0.")
		return
	}
	tag, err := s.DB.Exec(c, `UPDATE events SET value=$3, status='queued', next_retry_at=now()
		WHERE id::text=$1 AND workspace_id=$2 AND status='pending_value'`, c.Param("id"), wsID(c), in.Value)
	if err != nil {
		serverError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, http.StatusConflict, "not_pending_value", "Event ini tidak sedang menunggu nilai.")
		return
	}
	ok(c, gin.H{"status": "queued"})
}
