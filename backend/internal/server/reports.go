package server

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	monthRe  = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)
	filterRe = regexp.MustCompile(`^[a-z_]{1,20}$`)
)

// reportRange membaca ?month=YYYY-MM (satu bulan penuh, zona Asia/Jakarta) atau ?from&to seperti endpoint lain.
// Batas atas eksklusif, sama dengan dateRange.
func reportRange(c *gin.Context) (from, to time.Time, err error) {
	m := c.Query("month")
	if m == "" {
		return dateRange(c, 30)
	}
	if !monthRe.MatchString(m) {
		return from, to, fmt.Errorf("month harus YYYY-MM")
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	from, err = time.ParseInLocation("2006-01", m, loc)
	if err != nil {
		return from, to, fmt.Errorf("month harus YYYY-MM")
	}
	return from, from.AddDate(0, 1, 0), nil
}

// dailyReport: GET /reports/daily?month=YYYY-MM | ?from=&to=  [&source=qr|webhook|organic|api][&status=new|lead|checkout|purchase|lost]
// Rekap per tanggal (percakapan, purchase, revenue, event terkirim/gagal), total, dan rincian per jenis event.
// source dan status memfilter lead; event ikut terfilter lewat lead-nya (event tanpa lead hanya muncul bila tanpa filter).
// Event uji koneksi tidak dihitung. Purchase dan revenue hanya event yang berstatus terkirim ke Meta.
func (s *Server) dailyReport(c *gin.Context) {
	from, to, err := reportRange(c)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	source, status := c.Query("source"), c.Query("status")
	if source != "" && !filterRe.MatchString(source) {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "source tidak valid.")
		return
	}
	switch status {
	case "", "new", "lead", "checkout", "purchase", "lost":
	default:
		fail(c, http.StatusUnprocessableEntity, "validation_error", "status tidak valid.")
		return
	}
	id := wsID(c)

	days, err := query(c, s.DB, `
WITH d AS (
  SELECT g::date AS day FROM generate_series(($2::timestamptz AT TIME ZONE 'Asia/Jakarta')::date, (($3::timestamptz AT TIME ZONE 'Asia/Jakarta')::date - 1), interval '1 day') g),
lc AS (
  SELECT (first_message_at AT TIME ZONE 'Asia/Jakarta')::date AS day, count(*) AS conversations, (count(*) FILTER (WHERE ctwa_clid IS NOT NULL)) AS from_ads
  FROM leads WHERE workspace_id = $1 AND deleted_at IS NULL AND first_message_at >= $2 AND first_message_at < $3
    AND ($4 = '' OR source = $4) AND ($5 = '' OR status = $5) GROUP BY 1),
ec AS (
  SELECT (e.event_time AT TIME ZONE 'Asia/Jakarta')::date AS day,
         (count(*) FILTER (WHERE e.event_name = 'Purchase' AND e.status = 'sent')) AS purchases,
         COALESCE(sum(e.value) FILTER (WHERE e.event_name = 'Purchase' AND e.status = 'sent'), 0)::float8 AS revenue,
         (count(*) FILTER (WHERE e.status = 'sent')) AS events_sent, (count(*) FILTER (WHERE e.status = 'failed')) AS events_failed
  FROM events e LEFT JOIN leads l ON l.id = e.lead_id
  WHERE e.workspace_id = $1 AND e.source <> 'test' AND e.event_time >= $2 AND e.event_time < $3
    AND ($4 = '' OR l.source = $4) AND ($5 = '' OR l.status = $5) GROUP BY 1)
SELECT to_char(d.day, 'YYYY-MM-DD') AS date, COALESCE(lc.conversations, 0) AS conversations, COALESCE(lc.from_ads, 0) AS from_ads,
       COALESCE(ec.purchases, 0) AS purchases, COALESCE(ec.revenue, 0)::float8 AS revenue,
       COALESCE(ec.events_sent, 0) AS events_sent, COALESCE(ec.events_failed, 0) AS events_failed
FROM d LEFT JOIN lc ON lc.day = d.day LEFT JOIN ec ON ec.day = d.day ORDER BY d.day`, id, from, to, source, status)
	if err != nil {
		serverError(c, err)
		return
	}

	byEvent, err := query(c, s.DB, `
SELECT e.event_name, count(*) AS total, (count(*) FILTER (WHERE e.status = 'sent')) AS sent, (count(*) FILTER (WHERE e.status = 'failed')) AS failed,
       (count(*) FILTER (WHERE e.status NOT IN ('sent', 'failed'))) AS other,
       COALESCE(sum(e.value) FILTER (WHERE e.status = 'sent'), 0)::float8 AS value
FROM events e LEFT JOIN leads l ON l.id = e.lead_id
WHERE e.workspace_id = $1 AND e.source <> 'test' AND e.event_time >= $2 AND e.event_time < $3
  AND ($4 = '' OR l.source = $4) AND ($5 = '' OR l.status = $5)
GROUP BY e.event_name ORDER BY total DESC, e.event_name`, id, from, to, source, status)
	if err != nil {
		serverError(c, err)
		return
	}

	srcRows, err := query(c, s.DB, `SELECT DISTINCT source FROM leads WHERE workspace_id = $1 AND deleted_at IS NULL AND source IS NOT NULL ORDER BY 1`, id)
	if err != nil {
		serverError(c, err)
		return
	}
	sources := make([]any, 0, len(srcRows))
	for _, r := range srcRows {
		sources = append(sources, r["source"])
	}

	total := gin.H{"conversations": int64(0), "from_ads": int64(0), "purchases": int64(0), "revenue": float64(0), "events_sent": int64(0), "events_failed": int64(0)}
	for _, d := range days {
		for _, k := range []string{"conversations", "from_ads", "purchases", "events_sent", "events_failed"} {
			total[k] = total[k].(int64) + asInt(d[k])
		}
		total["revenue"] = total["revenue"].(float64) + asFloat(d["revenue"])
	}
	ok(c, gin.H{
		"range":    gin.H{"from": from.Format("2006-01-02"), "to": to.AddDate(0, 0, -1).Format("2006-01-02")},
		"filters":  gin.H{"source": source, "status": status},
		"sources":  sources,
		"days":     days,
		"total":    total,
		"by_event": byEvent,
	})
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int16:
		return int64(n)
	}
	return 0
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	}
	return 0
}
