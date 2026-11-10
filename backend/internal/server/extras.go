package server

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/wa"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ---------- Notifikasi (lonceng customer) ----------

func (s *Server) listNotifications(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT n.id::text AS id, n.type, n.title, n.body, n.link, n.read_at, n.created_at, w.slug AS workspace
		FROM notifications n LEFT JOIN workspaces w ON w.id = n.workspace_id
		WHERE n.user_id::text = $1 ORDER BY n.created_at DESC LIMIT 50`, c.GetString("user_id"))
	if err != nil {
		serverError(c, err)
		return
	}
	var unread int
	_ = s.DB.QueryRow(c, `SELECT count(*) FROM notifications WHERE user_id::text = $1 AND read_at IS NULL`, c.GetString("user_id")).Scan(&unread)
	c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"unread": unread}})
}

func (s *Server) readAllNotifications(c *gin.Context) {
	if _, err := s.DB.Exec(c, `UPDATE notifications SET read_at = now() WHERE user_id::text = $1 AND read_at IS NULL`, c.GetString("user_id")); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"read": true})
}

func (s *Server) readNotification(c *gin.Context) {
	tag, err := s.DB.Exec(c, `UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id::text = $1 AND user_id::text = $2`, c.Param("id"), c.GetString("user_id"))
	if err != nil {
		serverError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, http.StatusNotFound, "not_found", "Notifikasi tidak ditemukan.")
		return
	}
	ok(c, gin.H{"read": true})
}

// ---------- Tandai purchase (nama resmi di dokumen: leads/{id}/mark-purchase) ----------

func (s *Server) markPurchase(c *gin.Context) {
	var in struct {
		Value    *float64 `json:"value"`
		Currency string   `json:"currency"`
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
	out, err := core.CreateConversion(c, s.DB, core.Params{
		WorkspaceID: wsID(c), LeadID: leadID, EventName: "Purchase", Value: in.Value, Currency: in.Currency,
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

// ---------- Ekspor Event Log (CSV) ----------

// csvSafe mencegah injeksi rumus saat CSV dibuka di Excel/Sheets: sel berawalan = + - @ diberi apostrof.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// exportEvents: GET /events.csv dengan filter yang sama seperti /events (status, event_name, from, to). Maks. 50.000 baris.
func (s *Server) exportEvents(c *gin.Context) {
	w, okFilter := s.eventsWhere(c)
	if !okFilter {
		return
	}
	rows, err := query(c, s.DB, fmt.Sprintf(`
SELECT e.created_at, e.event_name, e.status, e.source, e.value::float8 AS value, e.currency,
       COALESCE(l.phone, '') AS phone,
       COALESCE(l.name, l.push_name, '') AS lead_name, COALESCE(e.error_message, '') AS error_message, COALESCE(e.fbtrace_id, '') AS fbtrace_id
FROM events e LEFT JOIN leads l ON l.id = e.lead_id WHERE %s ORDER BY e.created_at DESC LIMIT 50000`, w.sql()), w.args...)
	if err != nil {
		serverError(c, err)
		return
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="event-log.csv"`)
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF")) // BOM agar Excel membaca UTF-8
	out := csv.NewWriter(c.Writer)
	_ = out.Write([]string{"Waktu (WIB)", "Event", "Status", "Sumber", "Nilai", "Mata uang", "Nomor", "Nama lead", "Pesan error", "fbtrace_id"})
	for _, r := range rows {
		val := ""
		if v, okv := r["value"].(float64); okv {
			val = fmt.Sprintf("%.0f", v)
		}
		t, _ := r["created_at"].(time.Time)
		_ = out.Write([]string{t.In(loc).Format("2006-01-02 15:04:05"), csvSafe(fmt.Sprint(r["event_name"])), fmt.Sprint(r["status"]),
			fmt.Sprint(r["source"]), val, fmt.Sprint(r["currency"]), fmt.Sprint(r["phone"]),
			csvSafe(fmt.Sprint(r["lead_name"])), csvSafe(fmt.Sprint(r["error_message"])), fmt.Sprint(r["fbtrace_id"])})
	}
	out.Flush()
}

// ---------- Ubah status lead (tandai Hilang / pulihkan) ----------

// bulkLeadStatus: POST /leads/bulk-status {ids, status}. Status lain (lead, checkout, purchase) berubah otomatis
// lewat event, jadi di sini hanya 'lost' (dari status selain purchase) dan 'new' (memulihkan lead yang hilang).
func (s *Server) bulkLeadStatus(c *gin.Context) {
	var in struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || len(in.IDs) == 0 || len(in.IDs) > 200 {
		fail(c, http.StatusBadRequest, "bad_request", "Kirim ids (1-200) dan status.")
		return
	}
	if in.Status != "lost" && in.Status != "new" {
		bad422(c, "status hanya boleh 'lost' atau 'new'.")
		return
	}
	tag, err := s.DB.Exec(c, `UPDATE leads SET status = $3 WHERE workspace_id = $1 AND deleted_at IS NULL AND id::text = ANY($2)
		AND (($3 = 'lost' AND status <> 'purchase') OR ($3 = 'new' AND status = 'lost'))`, wsID(c), in.IDs, in.Status)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"updated": tag.RowsAffected()})
}

// ---------- Hapus satu lead ----------

// deleteLead: DELETE /leads/:id. Menghapus permanen satu lead beserta event dan pesannya di workspace ini.
// Dicatat di audit log dengan nomor tersamarkan.
func (s *Server) deleteLead(c *gin.Context) {
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	var phone string
	err = tx.QueryRow(c, `SELECT phone FROM leads WHERE id::text = $1 AND workspace_id = $2 AND deleted_at IS NULL FOR UPDATE`, c.Param("id"), wsID(c)).Scan(&phone)
	if err != nil {
		fail(c, http.StatusNotFound, "lead_not_found", "Lead tidak ditemukan.")
		return
	}
	ev, err := tx.Exec(c, `DELETE FROM events WHERE workspace_id = $1 AND lead_id::text = $2`, wsID(c), c.Param("id"))
	if err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.Exec(c, `DELETE FROM leads WHERE workspace_id = $1 AND id::text = $2`, wsID(c), c.Param("id")); err != nil {
		serverError(c, err)
		return
	}
	if err := auditUser(c, tx, c.GetString("user_id"), wsID(c), "lead.delete", wa.MaskPhone(phone)); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"deleted": true, "events": ev.RowsAffected()})
}

// ---------- Hapus data kontak (UU PDP) ----------

// eraseContact: POST /leads/erase {phone}. Menghapus permanen semua lead dengan nomor itu di workspace ini beserta
// event-nya. Dicatat di audit log tanpa menyimpan nomor lengkap.
func (s *Server) eraseContact(c *gin.Context) {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	phone, err := wa.NormalizePhone(in.Phone)
	if err != nil {
		bad422(c, "Nomor HP tidak valid.")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	ev, err := tx.Exec(c, `DELETE FROM events WHERE workspace_id = $1 AND lead_id IN (SELECT id FROM leads WHERE workspace_id = $1 AND phone = $2)`, wsID(c), phone)
	if err != nil {
		serverError(c, err)
		return
	}
	ld, err := tx.Exec(c, `DELETE FROM leads WHERE workspace_id = $1 AND phone = $2`, wsID(c), phone)
	if err != nil {
		serverError(c, err)
		return
	}
	if err := auditUser(c, tx, c.GetString("user_id"), wsID(c), "contact.erase", wa.MaskPhone(phone)); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"leads": ld.RowsAffected(), "events": ev.RowsAffected()})
}
