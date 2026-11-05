package adminapi

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// errGuard menandai bahwa pengaman sudah menjawab request; withAudit tidak boleh menjawab lagi.
var errGuard = errors.New("guard")

var planCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,29}$`)

func bad(c *gin.Context, msg string) {
	httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", msg)
}

func wholeRp(v float64) bool { return v >= 0 && v == math.Trunc(v) && v <= 1e12 }

// ---------- Paket & harga ----------

type planInput struct {
	Code       string   `json:"code"`
	Name       string   `json:"name"`
	PriceMonth float64  `json:"price_month"`
	Price12m   float64  `json:"price_12m"`
	Features   []string `json:"features"`
	Shown      *bool    `json:"shown"`
	Sort       int      `json:"sort"`
}

func (in *planInput) validate(c *gin.Context, creating bool) bool {
	in.Name = strings.TrimSpace(in.Name)
	var kept []string
	for _, f := range in.Features {
		if f = strings.TrimSpace(f); f != "" {
			kept = append(kept, f)
		}
	}
	in.Features = kept
	switch {
	case creating && !planCodeRe.MatchString(in.Code):
		bad(c, "code 2-30 karakter: huruf kecil, angka, - atau _.")
	case in.Name == "" || len(in.Name) > 80:
		bad(c, "Nama paket wajib diisi (maks. 80 karakter).")
	case !wholeRp(in.PriceMonth) || !wholeRp(in.Price12m):
		bad(c, "Harga harus bilangan bulat Rupiah >= 0.")
	case len(in.Features) > 20:
		bad(c, "Maksimal 20 fitur.")
	default:
		return true
	}
	return false
}

const planSelect = `SELECT p.id::text AS id, p.code::text AS code, p.name, p.price_month::float8 AS price_month, p.price_12m::float8 AS price_12m,
  p.features, p.shown, p.sort, p.created_at,
  (SELECT count(*) FROM subscriptions s WHERE s.plan_id = p.id AND s.status = 'active') AS active_customers,
  (SELECT count(*) FROM subscriptions s WHERE s.plan_id = p.id) AS total_customers
FROM plans p WHERE p.deleted_at IS NULL`

func (s *Server) listPlans(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, planSelect+` ORDER BY p.sort, p.created_at`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (s *Server) createPlan(c *gin.Context) {
	var in planInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if !in.validate(c, true) {
		return
	}
	feats, _ := json.Marshal(nonNil(in.Features))
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var id string
	err = tx.QueryRow(c, `INSERT INTO plans (code, name, price_month, price_12m, features, shown, sort) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
		in.Code, in.Name, in.PriceMonth, in.Price12m, feats, in.Shown == nil || *in.Shown, in.Sort).Scan(&id)
	if isUnique(err) {
		httpx.Fail(c, http.StatusConflict, "code_taken", "Kode paket sudah dipakai.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "plan.create", "", "plan", id, map[string]any{"code": in.Code, "price_month": in.PriceMonth, "price_12m": in.Price12m}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.Status(c, http.StatusCreated, gin.H{"id": id})
}

// updatePlan tidak mengubah code (dipakai frontend sebagai id paket).
func (s *Server) updatePlan(c *gin.Context) {
	var in planInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if !in.validate(c, false) {
		return
	}
	feats, _ := json.Marshal(nonNil(in.Features))
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var before struct{ M, Y float64 }
	if err := tx.QueryRow(c, `SELECT price_month::float8, price_12m::float8 FROM plans WHERE id::text = $1 AND deleted_at IS NULL FOR UPDATE`, c.Param("id")).Scan(&before.M, &before.Y); err != nil {
		notFoundOr500(c, err, "plan_not_found", "Paket tidak ditemukan.")
		return
	}
	if _, err := tx.Exec(c, `UPDATE plans SET name=$2, price_month=$3, price_12m=$4, features=$5, shown=COALESCE($6, shown), sort=$7 WHERE id::text=$1`,
		c.Param("id"), in.Name, in.PriceMonth, in.Price12m, feats, in.Shown, in.Sort); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "plan.update", "", "plan", c.Param("id"), map[string]any{
		"price_month": []float64{before.M, in.PriceMonth}, "price_12m": []float64{before.Y, in.Price12m}}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"updated": true})
}

// deletePlan menyembunyikan paket (hapus lunak). Ditolak bila masih dipakai langganan yang berjalan.
func (s *Server) deletePlan(c *gin.Context) {
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var inUse int
	_ = tx.QueryRow(c, `SELECT count(*) FROM subscriptions WHERE plan_id::text = $1 AND status IN ('active', 'trial', 'pending')`, c.Param("id")).Scan(&inUse)
	if inUse > 0 {
		httpx.Fail(c, http.StatusConflict, "plan_in_use", "Paket masih dipakai langganan aktif. Sembunyikan saja (shown=false).")
		return
	}
	tag, err := tx.Exec(c, `UPDATE plans SET deleted_at = now(), shown = false WHERE id::text = $1 AND deleted_at IS NULL`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(c, http.StatusNotFound, "plan_not_found", "Paket tidak ditemukan.")
		return
	}
	if err := s.audit(c, tx, "plan.delete", "", "plan", c.Param("id"), nil); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func notFoundOr500(c *gin.Context, err error, code, msg string) {
	if err == pgx.ErrNoRows {
		httpx.Fail(c, http.StatusNotFound, code, msg)
		return
	}
	httpx.ServerError(c, err)
}

// ---------- Rekening bank ----------

type bankInput struct {
	Bank   string `json:"bank"`
	Number string `json:"number"`
	Holder string `json:"holder"`
	Active *bool  `json:"active"`
	Sort   int    `json:"sort"`
}

func (in *bankInput) validate(c *gin.Context) bool {
	in.Bank, in.Number, in.Holder = strings.TrimSpace(in.Bank), strings.TrimSpace(in.Number), strings.TrimSpace(in.Holder)
	if in.Bank == "" || in.Number == "" || in.Holder == "" || len(in.Bank) > 40 || len(in.Number) > 40 || len(in.Holder) > 120 {
		bad(c, "Bank, nomor rekening, dan nama pemilik wajib diisi.")
		return false
	}
	return true
}

func (s *Server) listBanks(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT id::text AS id, bank, number, holder, active, sort FROM bank_accounts WHERE deleted_at IS NULL ORDER BY sort, created_at`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows)
}

func (s *Server) createBank(c *gin.Context) {
	var in bankInput
	if err := c.ShouldBindJSON(&in); err != nil || !in.validate(c) {
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		}
		return
	}
	s.withAudit(c, "bank_account.create", "bank_account", func(tx pgx.Tx) (string, map[string]any, error) {
		var id string
		err := tx.QueryRow(c, `INSERT INTO bank_accounts (bank, number, holder, active, sort) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
			in.Bank, in.Number, in.Holder, in.Active == nil || *in.Active, in.Sort).Scan(&id)
		return id, map[string]any{"bank": in.Bank}, err
	}, http.StatusCreated)
}

func (s *Server) updateBank(c *gin.Context) {
	var in bankInput
	if err := c.ShouldBindJSON(&in); err != nil || !in.validate(c) {
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		}
		return
	}
	s.withAudit(c, "bank_account.update", "bank_account", func(tx pgx.Tx) (string, map[string]any, error) {
		tag, err := tx.Exec(c, `UPDATE bank_accounts SET bank=$2, number=$3, holder=$4, active=COALESCE($5, active), sort=$6 WHERE id::text=$1 AND deleted_at IS NULL`,
			c.Param("id"), in.Bank, in.Number, in.Holder, in.Active, in.Sort)
		if err == nil && tag.RowsAffected() == 0 {
			err = pgx.ErrNoRows
		}
		return c.Param("id"), map[string]any{"bank": in.Bank, "active": in.Active}, err
	}, http.StatusOK)
}

func (s *Server) deleteBank(c *gin.Context) {
	s.withAudit(c, "bank_account.delete", "bank_account", func(tx pgx.Tx) (string, map[string]any, error) {
		tag, err := tx.Exec(c, `UPDATE bank_accounts SET deleted_at = now(), active = false WHERE id::text=$1 AND deleted_at IS NULL`, c.Param("id"))
		if err == nil && tag.RowsAffected() == 0 {
			err = pgx.ErrNoRows
		}
		return c.Param("id"), nil, err
	}, http.StatusOK)
}

// withAudit menjalankan satu perubahan + audit log dalam satu transaksi dan menjawab hasilnya.
// fn mengembalikan (id target, detail audit, error). pgx.ErrNoRows -> 404, pelanggaran unik -> 409.
func (s *Server) withAudit(c *gin.Context, action, targetType string, fn func(tx pgx.Tx) (string, map[string]any, error), okStatus int) {
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	id, detail, err := fn(tx)
	switch {
	case err == errGuard: // respons sudah ditulis oleh pengaman
		return
	case err == pgx.ErrNoRows:
		httpx.Fail(c, http.StatusNotFound, "not_found", "Data tidak ditemukan.")
		return
	case isUnique(err):
		httpx.Fail(c, http.StatusConflict, "duplicate", "Data dengan nilai itu sudah ada.")
		return
	case err != nil:
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, action, "", targetType, id, detail); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.Status(c, okStatus, gin.H{"id": id})
}

// ---------- Sales / penerima komisi ----------

type salesInput struct {
	Name      string `json:"name"`
	IsFounder *bool  `json:"is_founder"`
	Active    *bool  `json:"active"`
	Sort      int    `json:"sort"`
}

func (s *Server) listSales(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `
SELECT sp.id::text AS id, sp.name::text AS name, sp.is_founder, sp.active, sp.sort,
       (SELECT count(*) FROM payments p WHERE p.sales_id = sp.id) AS payments,
       (SELECT count(*) FROM subscriptions s WHERE s.sales_id = sp.id) AS customers
FROM sales_people sp ORDER BY sp.sort, sp.name`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rows)
}

// lastFounderGuard menolak perubahan yang membuat tidak ada founder aktif tersisa (bagian rata 80% tidak punya penerima).
func (s *Server) lastFounderGuard(c *gin.Context, tx pgx.Tx, id string, willBeFounderActive bool) bool {
	if willBeFounderActive {
		return true
	}
	var others int
	_ = tx.QueryRow(c, `SELECT count(*) FROM sales_people WHERE is_founder AND active AND id::text <> $1`, id).Scan(&others)
	if others == 0 {
		httpx.Fail(c, http.StatusConflict, "last_founder", "Harus ada minimal satu founder aktif untuk menerima bagian rata.")
		return false
	}
	return true
}

func (s *Server) createSales(c *gin.Context) {
	var in salesInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		bad(c, "Nama wajib diisi (maks. 80 karakter).")
		return
	}
	s.withAudit(c, "sales_person.create", "sales_person", func(tx pgx.Tx) (string, map[string]any, error) {
		var id string
		err := tx.QueryRow(c, `INSERT INTO sales_people (name, is_founder, active, sort) VALUES ($1,$2,$3,$4) RETURNING id::text`,
			in.Name, in.IsFounder != nil && *in.IsFounder, in.Active == nil || *in.Active, in.Sort).Scan(&id)
		return id, map[string]any{"name": in.Name}, err
	}, http.StatusCreated)
}

func (s *Server) updateSales(c *gin.Context) {
	var in salesInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 {
		bad(c, "Nama wajib diisi (maks. 80 karakter).")
		return
	}
	s.withAudit(c, "sales_person.update", "sales_person", func(tx pgx.Tx) (string, map[string]any, error) {
		var isF, act bool
		if err := tx.QueryRow(c, `SELECT is_founder, active FROM sales_people WHERE id::text = $1 FOR UPDATE`, c.Param("id")).Scan(&isF, &act); err != nil {
			return "", nil, err
		}
		if in.IsFounder != nil {
			isF = *in.IsFounder
		}
		if in.Active != nil {
			act = *in.Active
		}
		if !s.lastFounderGuard(c, tx, c.Param("id"), isF && act) {
			return "", nil, errGuard
		}
		_, err := tx.Exec(c, `UPDATE sales_people SET name=$2, is_founder=$3, active=$4, sort=$5 WHERE id::text=$1`, c.Param("id"), in.Name, isF, act, in.Sort)
		return c.Param("id"), map[string]any{"name": in.Name, "is_founder": isF, "active": act}, err
	}, http.StatusOK)
}

// deleteSales menonaktifkan (tidak menghapus) karena pembayaran lama merujuk ke sales ini.
func (s *Server) deleteSales(c *gin.Context) {
	s.withAudit(c, "sales_person.deactivate", "sales_person", func(tx pgx.Tx) (string, map[string]any, error) {
		if !s.lastFounderGuard(c, tx, c.Param("id"), false) {
			return "", nil, errGuard
		}
		tag, err := tx.Exec(c, `UPDATE sales_people SET active = false WHERE id::text = $1`, c.Param("id"))
		if err == nil && tag.RowsAffected() == 0 {
			err = pgx.ErrNoRows
		}
		return c.Param("id"), nil, err
	}, http.StatusOK)
}
