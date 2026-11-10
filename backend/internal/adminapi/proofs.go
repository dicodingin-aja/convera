package adminapi

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const proofCols = `pp.id::text AS id, pp.workspace_id::text AS workspace_id, w.name AS customer, u.name AS owner, u.phone, w.city,
  pp.kind, pp.purchase, pp.ad_slots, pl.code AS plan, pl.name AS plan_name, pp.months, pp.bank, pp.sender, pp.ref,
  pp.billed::float8 AS billed, pp.received::float8 AS received,
  (pp.received IS NOT NULL AND pp.received <> pp.billed) AS amount_mismatch,
  ba.bank || ' ' || ba.number AS to_bank, pp.message, pp.status, pp.reject_reason,
  pp.file_mime, pp.file_size, pp.created_at AS uploaded_at, pp.decided_at`

const proofFrom = `
FROM payment_proofs pp
JOIN workspaces w ON w.id = pp.workspace_id
JOIN users u ON u.id = w.owner_user_id
LEFT JOIN plans pl ON pl.id = pp.plan_id
LEFT JOIN bank_accounts ba ON ba.id = pp.to_bank_account_id
WHERE pp.deleted_at IS NULL`

func (s *Server) listProofs(c *gin.Context) {
	w := &httpx.Where{}
	switch st := c.DefaultQuery("status", "pending"); st {
	case "all":
	case "pending", "approved", "rejected":
		w.Add("pp.status = ?", st)
	default:
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "status harus pending, approved, rejected, atau all.")
		return
	}
	page, per, off := httpx.Paging(c)
	var total int
	if err := s.DB.QueryRow(c, `SELECT count(*) `+proofFrom+` AND `+w.SQL(), w.Args...).Scan(&total); err != nil {
		httpx.ServerError(c, err)
		return
	}
	rows, err := httpx.Query(c, s.DB, fmt.Sprintf(`SELECT %s %s AND %s ORDER BY pp.created_at %s LIMIT %d OFFSET %d`,
		proofCols, proofFrom, w.SQL(), orderFor(c.DefaultQuery("status", "pending")), per, off), w.Args...)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.Page(c, rows, page, per, total)
}

// Antrean pending diurutkan terlama dulu (yang menunggu paling lama diverifikasi duluan); riwayat terbaru dulu.
func orderFor(status string) string {
	if status == "pending" {
		return "ASC"
	}
	return "DESC"
}

func (s *Server) getProof(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT `+proofCols+proofFrom+` AND pp.id::text = $1`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if len(rows) == 0 {
		httpx.Fail(c, http.StatusNotFound, "proof_not_found", "Bukti transfer tidak ditemukan.")
		return
	}
	// Berkas gambar/PDF disimpan di penyimpanan privat (belum dibangun); di sini hanya metadata.
	httpx.OK(c, rows[0])
}

// approveProof: admin memilih paket, periode, nominal, dan sales (wajib), lalu langganan diaktifkan.
func (s *Server) approveProof(c *gin.Context) {
	var in extendInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	amount, ok := in.validate(c)
	if !ok {
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)

	var ws, status, purchase string
	var proofSlots int
	err = tx.QueryRow(c, `SELECT workspace_id::text, status, purchase, ad_slots FROM payment_proofs WHERE id::text = $1 AND deleted_at IS NULL FOR UPDATE`, c.Param("id")).
		Scan(&ws, &status, &purchase, &proofSlots)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, http.StatusNotFound, "proof_not_found", "Bukti transfer tidak ditemukan.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if status != "pending" {
		httpx.Fail(c, http.StatusConflict, "already_decided", "Bukti transfer ini sudah diputuskan.")
		return
	}
	// Jumlah slot dan jenis pembelian mengikuti bukti transfer, kecuali admin mengubah jumlah slotnya.
	if in.AdSlots < 1 {
		in.AdSlots = proofSlots
	}
	if in.Purchase == "" {
		in.Purchase = purchase
	}
	end, err := s.activate(c, tx, activation{WorkspaceID: ws, PlanCode: in.Plan, Months: in.Months, Amount: amount, SalesID: in.SalesID, ProofID: c.Param("id"),
		AdSlots: in.AdSlots, Purchase: in.Purchase})
	if activationError(c, err) {
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if _, err := tx.Exec(c, `UPDATE payment_proofs SET status = 'approved', received = $2, decided_by = $3::uuid, decided_at = now() WHERE id::text = $1`,
		c.Param("id"), amount, adminID(c)); err != nil {
		httpx.ServerError(c, err)
		return
	}
	body := fmt.Sprintf("Langganan kamu aktif sampai %s (%d slot kampanye).", end.In(jakarta()).Format("2 Jan 2006"), in.AdSlots)
	if in.Purchase == "slots" {
		body = fmt.Sprintf("%d slot kampanye tambahan aktif sampai %s.", in.AdSlots, end.In(jakarta()).Format("2 Jan 2006"))
	}
	if err := s.notifyOwner(c, tx, ws, "payment_approved", "Pembayaran diterima", body, "/langganan"); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "payment_proof.approve", ws, "payment_proof", c.Param("id"),
		map[string]any{"plan": in.Plan, "months": in.Months, "amount": amount, "sales_id": in.SalesID, "period_end": end, "ad_slots": in.AdSlots, "purchase": in.Purchase}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"status": "approved", "current_period_end": end})
}

// rejectProof: alasan wajib; customer diberi tahu dan bisa mengunggah ulang.
func (s *Server) rejectProof(c *gin.Context) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len(in.Reason) > 300 {
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "Alasan penolakan wajib diisi (maks. 300 karakter).")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var ws, status string
	err = tx.QueryRow(c, `SELECT workspace_id::text, status FROM payment_proofs WHERE id::text = $1 AND deleted_at IS NULL FOR UPDATE`, c.Param("id")).Scan(&ws, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, http.StatusNotFound, "proof_not_found", "Bukti transfer tidak ditemukan.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if status != "pending" {
		httpx.Fail(c, http.StatusConflict, "already_decided", "Bukti transfer ini sudah diputuskan.")
		return
	}
	if _, err := tx.Exec(c, `UPDATE payment_proofs SET status = 'rejected', reject_reason = $2, decided_by = $3::uuid, decided_at = now() WHERE id::text = $1`,
		c.Param("id"), in.Reason, adminID(c)); err != nil {
		httpx.ServerError(c, err)
		return
	}
	// Bila langganan "pending" hanya karena bukti ini, kembalikan ke status sebelumnya (aktif / trial / berakhir).
	if _, err := tx.Exec(c, `
UPDATE subscriptions SET status = CASE
    WHEN current_period_end > now() THEN 'active'
    WHEN trial_until > now() THEN 'trial'
    ELSE 'expired' END
WHERE workspace_id::text = $1 AND status = 'pending'
  AND NOT EXISTS (SELECT 1 FROM payment_proofs WHERE workspace_id::text = $1 AND status = 'pending' AND deleted_at IS NULL)`, ws); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.notifyOwner(c, tx, ws, "payment_rejected", "Bukti transfer ditolak", in.Reason+" Silakan unggah ulang.", "/langganan"); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "payment_proof.reject", ws, "payment_proof", c.Param("id"), map[string]any{"reason": in.Reason}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"status": "rejected", "decided_at": time.Now().UTC()})
}

// proofFile menyajikan berkas bukti transfer dari penyimpanan privat. Hanya lewat sesi admin; tiap pembukaan dicatat
// di audit log. Header dibuat ketat: tipe dari database (bukan dari berkas), tanpa sniffing, tanpa cache, tanpa skrip.
func (s *Server) proofFile(c *gin.Context) {
	var key, mime, ws string
	err := s.DB.QueryRow(c, `SELECT file_key, file_mime, workspace_id::text FROM payment_proofs WHERE id::text = $1 AND deleted_at IS NULL`, c.Param("id")).Scan(&key, &mime, &ws)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, http.StatusNotFound, "proof_not_found", "Bukti transfer tidak ditemukan.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	f, err := s.Files.Open(key)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, "file_missing", "Berkas tidak ditemukan di penyimpanan.")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, s.DB, "payment_proof.view_file", ws, "payment_proof", c.Param("id"), nil); err != nil {
		httpx.ServerError(c, err)
		return
	}
	c.DataFromReader(http.StatusOK, fi.Size(), mime, f, map[string]string{
		"Content-Disposition":     `inline; filename="bukti-` + c.Param("id")[:8] + `.` + strings.TrimPrefix(filepath.Ext(key), ".") + `"`,
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
		"Cache-Control":           "private, no-store",
	})
}
