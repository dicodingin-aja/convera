package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/billing"
	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/upload"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// maxPendingProofs membatasi antrean bukti yang belum diputuskan per workspace (mencegah banjir unggahan).
const maxPendingProofs = 3

// ---------- Info untuk halaman bayar ----------

// mySubscription: status langganan workspace + apakah ada bukti yang sedang menunggu verifikasi.
func (s *Server) mySubscription(c *gin.Context) {
	rows, err := query(c, s.DB, `
SELECT `+billing.EffectiveStatusSQL+` AS status, p.code::text AS plan, p.name AS plan_name, sub.trial_until, sub.current_period_end,
  EXISTS (SELECT 1 FROM payment_proofs pp WHERE pp.workspace_id = sub.workspace_id AND pp.status = 'pending' AND pp.deleted_at IS NULL) AS has_pending_proof
FROM subscriptions sub LEFT JOIN plans p ON p.id = sub.plan_id WHERE sub.workspace_id = $1`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if len(rows) == 0 {
		fail(c, http.StatusNotFound, "no_subscription", "Langganan tidak ditemukan.")
		return
	}
	quota, err := core.SlotQuota(c, s.DB, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	used, err := core.SlotUsage(c, s.DB, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	rows[0]["ad_slots"] = quota
	rows[0]["ad_slots_used"] = used
	ok(c, rows[0])
}

// customerBanks: rekening tujuan transfer yang aktif.
func (s *Server) customerBanks(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT id::text AS id, bank, number, holder FROM bank_accounts WHERE active AND deleted_at IS NULL ORDER BY sort, created_at`)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rows)
}

// listMyProofs: riwayat bukti transfer workspace ini (tanpa nama berkas internal).
func (s *Server) listMyProofs(c *gin.Context) {
	rows, err := query(c, s.DB, `
SELECT pp.id::text AS id, pp.kind, pp.purchase, pp.ad_slots, pl.name AS plan_name, pp.months, pp.billed::float8 AS billed, pp.received::float8 AS received,
       pp.status, pp.reject_reason, pp.created_at, pp.decided_at
FROM payment_proofs pp LEFT JOIN plans pl ON pl.id = pp.plan_id
WHERE pp.workspace_id = $1 AND pp.deleted_at IS NULL ORDER BY pp.created_at DESC LIMIT 50`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rows)
}

// ---------- Unggah bukti transfer ----------

// uploadProof menerima multipart/form-data: file (JPG/PNG/PDF, maks. 5 MB) + plan, months, bank, sender,
// ref?, amount?, bank_account_id?, message?, ad_slots? (jumlah slot kampanye, default 1), purchase? ('subscription'
// default, atau 'slots' = tambah slot 30 hari di tengah langganan yang berjalan). Jenis berkas ditentukan dari ISINYA, nama file acak, disimpan privat.
// Tagihan (billed) dihitung server dari harga paket; nominal dari customer (amount) hanya klaim untuk dicocokkan admin.
func (s *Server) uploadProof(c *gin.Context) {
	if !s.limiter("proof|"+c.GetString("user_id"), 10.0/3600, 5).Allow() { // maks. 10 unggahan/jam, burst 5
		fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak unggahan. Coba lagi nanti.")
		return
	}
	mr, err := c.Request.MultipartReader()
	if err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Kirim sebagai multipart/form-data.")
		return
	}

	fields := map[string]string{}
	var key, mime string
	var size int64
	committed := false
	defer func() {
		if !committed && key != "" {
			s.Files.Remove(key) // jangan tinggalkan berkas yatim bila request gagal
		}
	}()

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			tooBig(c, err)
			return
		}
		name := part.FormName()
		if name != "file" {
			b, _ := io.ReadAll(io.LimitReader(part, 2049))
			if len(b) > 2048 {
				bad422(c, "Isian "+name+" terlalu panjang.")
				return
			}
			fields[name] = strings.TrimSpace(string(b))
			continue
		}
		if key != "" {
			bad422(c, "Hanya satu berkas per unggahan.")
			return
		}
		head := make([]byte, 512)
		n, _ := io.ReadFull(part, head)
		kind, okType := upload.Detect(head[:n])
		if !okType {
			fail(c, http.StatusUnsupportedMediaType, "unsupported_file", "Berkas harus berupa JPG, PNG, atau PDF.")
			return
		}
		k, err := upload.NewKey(kind)
		if err != nil {
			serverError(c, err)
			return
		}
		sz, err := s.Files.Save(k, io.MultiReader(bytes.NewReader(head[:n]), part), upload.MaxSize)
		if errors.Is(err, upload.ErrTooLarge) {
			fail(c, http.StatusRequestEntityTooLarge, "file_too_large", "Ukuran berkas maksimal 5 MB.")
			return
		} else if err != nil {
			if !tooBig(c, err) {
				serverError(c, err)
			}
			return
		}
		key, mime, size = k, kind.Mime, sz
	}

	// Bank dan nama pengirim opsional (admin mencocokkan lewat nominal dan bukti); kolom DB wajib terisi, jadi pakai "-".
	for _, k := range []string{"bank", "sender"} {
		if strings.TrimSpace(fields[k]) == "" {
			fields[k] = "-"
		}
	}

	// ----- validasi isian -----
	months, _ := strconv.Atoi(fields["months"])
	slots := 1
	if v := fields["ad_slots"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > billing.MaxAdSlots {
			bad422(c, fmt.Sprintf("Jumlah slot kampanye harus 1 sampai %d.", billing.MaxAdSlots))
			return
		}
		slots = n
	}
	purchase := fields["purchase"]
	switch purchase {
	case "", "subscription":
		purchase = "subscription"
	case "slots":
		months = 1 // tambah slot selalu 30 hari
	default:
		bad422(c, "purchase harus 'subscription' atau 'slots'.")
		return
	}
	var declared *float64
	if v := fields["amount"]; v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f != math.Trunc(f) || f > 1e12 {
			bad422(c, "amount harus bilangan bulat Rupiah.")
			return
		}
		declared = &f
	}
	msg := ""
	switch {
	case key == "":
		msg = "Berkas bukti transfer wajib dilampirkan."
	case fields["plan"] == "":
		msg = "Pilih paket."
	case !billing.ValidMonths(months):
		msg = "Periode harus 1, 3, atau 12 bulan."
	case len(fields["bank"]) > 40:
		msg = "Bank pengirim maksimal 40 karakter."
	case len(fields["sender"]) > 120:
		msg = "Nama pemilik rekening pengirim maksimal 120 karakter."
	case len(fields["ref"]) > 60:
		msg = "Nomor referensi maksimal 60 karakter."
	case len(fields["message"]) > 1000:
		msg = "Pesan maksimal 1000 karakter."
	}
	if msg != "" {
		bad422(c, msg)
		return
	}

	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)

	var planID string
	var pm, p12 float64
	if err := tx.QueryRow(c, `SELECT id::text, price_month::float8, price_12m::float8 FROM plans WHERE code = $1 AND deleted_at IS NULL AND shown`, fields["plan"]).
		Scan(&planID, &pm, &p12); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			bad422(c, "Paket tidak ditemukan.")
		} else {
			serverError(c, err)
		}
		return
	}
	billed := billing.ProofPrice(pm, p12, months, slots)
	if purchase == "slots" {
		// Tambah slot hanya untuk langganan yang sedang berjalan; selain itu bayar langganan biasa.
		access, err := core.WorkspaceAccess(c, tx, wsID(c))
		if err != nil {
			serverError(c, err)
			return
		}
		if !access {
			bad422(c, "Tambah slot hanya untuk langganan yang sedang aktif. Bayar langganan dulu.")
			return
		}
	}
	if id := fields["bank_account_id"]; id != "" {
		var okBank bool
		_ = tx.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM bank_accounts WHERE id::text = $1 AND active AND deleted_at IS NULL)`, id).Scan(&okBank)
		if !okBank {
			bad422(c, "Rekening tujuan tidak valid.")
			return
		}
	}

	// Kunci baris langganan agar hitungan antrean dan perubahan status konsisten.
	var subStatus string
	if err := tx.QueryRow(c, `SELECT status FROM subscriptions WHERE workspace_id = $1 FOR UPDATE`, wsID(c)).Scan(&subStatus); err != nil {
		serverError(c, err)
		return
	}
	var pending, paid int
	_ = tx.QueryRow(c, `SELECT count(*) FROM payment_proofs WHERE workspace_id = $1 AND status = 'pending' AND deleted_at IS NULL`, wsID(c)).Scan(&pending)
	if pending >= maxPendingProofs {
		fail(c, http.StatusConflict, "too_many_pending", fmt.Sprintf("Masih ada %d bukti transfer yang menunggu verifikasi. Tunggu hingga diperiksa.", pending))
		return
	}
	_ = tx.QueryRow(c, `SELECT count(*) FROM payments WHERE workspace_id = $1`, wsID(c)).Scan(&paid)
	kind := "new"
	switch {
	case paid > 0:
		kind = "renew"
	case subStatus == "trial":
		kind = "trial"
	}

	var id string
	if err := tx.QueryRow(c, `
INSERT INTO payment_proofs (workspace_id, uploaded_by, kind, plan_id, months, bank, sender, ref, billed, received, to_bank_account_id,
                            file_key, file_mime, file_size, message, ad_slots, purchase)
VALUES ($1, $2::uuid, $3, $4::uuid, $5, $6, $7, NULLIF($8,''), $9, $10, NULLIF($11,'')::uuid, $12, $13, $14, NULLIF($15,''), $16, $17)
RETURNING id::text`,
		wsID(c), c.GetString("user_id"), kind, planID, months, fields["bank"], fields["sender"], fields["ref"], billed, declared,
		fields["bank_account_id"], key, mime, size, fields["message"], slots, purchase).Scan(&id); err != nil {
		serverError(c, err)
		return
	}
	// Langganan jadi 'pending' (menunggu verifikasi), kecuali sudah dihentikan admin atau masih aktif dengan sisa waktu
	// (perpanjang lebih awal tidak boleh membuat langganan yang masih berjalan tampak tertunda).
	// Tambah slot tidak mengubah status langganan.
	if _, err := tx.Exec(c, `UPDATE subscriptions SET status = 'pending'
		WHERE workspace_id = $1 AND $2 = 'subscription' AND status NOT IN ('stopped', 'pending') AND NOT (status = 'active' AND current_period_end > now())`, wsID(c), purchase); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.Exec(c, `INSERT INTO notifications (admin_id, type, title, body, link)
		SELECT a.id, 'payment_proof_new', 'Bukti transfer baru', w.name || ' · Rp' || to_char($2::numeric, 'FM999G999G999G999'), '/verifikasi'
		FROM admins a, workspaces w WHERE a.status = 'active' AND w.id = $1`, wsID(c), billed); err != nil {
		serverError(c, err)
		return
	}
	detail, _ := json.Marshal(map[string]any{"months": months, "plan": fields["plan"], "kind": kind, "purchase": purchase, "ad_slots": slots, "file_mime": mime, "file_size": size})
	if _, err := tx.Exec(c, `INSERT INTO audit_logs (actor_type, actor_id, actor_label, workspace_id, action, target_type, target_id, detail, ip)
		SELECT 'user', u.id, u.name, $2::uuid, 'payment_proof.upload', 'payment_proof', $3, $4, NULLIF($5,'')::inet FROM users u WHERE u.id = $1::uuid`,
		c.GetString("user_id"), wsID(c), id, detail, c.ClientIP()); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	committed = true
	okStatus(c, http.StatusCreated, gin.H{"id": id, "status": "pending", "kind": kind, "purchase": purchase, "ad_slots": slots, "months": months, "billed": billed, "file_size": size})
}

func bad422(c *gin.Context, msg string) {
	fail(c, http.StatusUnprocessableEntity, "validation_error", msg)
}

// tooBig menjawab 413 bila err berasal dari batas ukuran body; mengembalikan true bila ditangani.
func tooBig(c *gin.Context, err error) bool {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		fail(c, http.StatusRequestEntityTooLarge, "file_too_large", "Ukuran unggahan terlalu besar (berkas maksimal 5 MB).")
		return true
	}
	fail(c, http.StatusBadRequest, "bad_request", "Unggahan tidak valid.")
	return true
}
