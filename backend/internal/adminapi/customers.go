package adminapi

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/billing"
	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const effectiveStatus = billing.EffectiveStatusSQL

const customerFrom = `
FROM workspaces w
JOIN users u ON u.id = w.owner_user_id
LEFT JOIN subscriptions sub ON sub.workspace_id = w.id
LEFT JOIN plans p ON p.id = sub.plan_id
LEFT JOIN sales_people sp ON sp.id = sub.sales_id
CROSS JOIN LATERAL (SELECT ` + effectiveStatus + ` AS status) eff
WHERE w.deleted_at IS NULL`

const customerCols = `w.id::text AS id, w.slug, w.name, u.name AS owner, w.city, u.phone, u.username::text AS username, u.email::text AS email,
  eff.status AS status, p.code AS plan, p.name AS plan_name, p.price_month::float8 AS plan_price,
  CASE WHEN eff.status IN ('active', 'expired', 'stopped') THEN sub.current_period_end END AS until,
  sub.trial_until, sp.name AS sales, u.last_seen_at,
  COALESCE(u.last_seen_at > now() - interval '5 minutes', false) AS online, w.created_at,
  (SELECT count(*) FROM workspaces x WHERE x.owner_user_id = u.id AND x.deleted_at IS NULL) AS owner_ws_count,
  (SELECT count(*) FROM subscriptions sb JOIN workspaces x ON x.id = sb.workspace_id WHERE x.owner_user_id = u.id AND sb.trial_until IS NOT NULL) AS owner_trial_count`

// ---------- Ringkasan ----------

func (s *Server) overview(c *gin.Context) {
	counts, err := httpx.Query(c, s.DB, `
SELECT count(*) AS total,
  count(*) FILTER (WHERE eff.status = 'active') AS active,
  count(*) FILTER (WHERE eff.status = 'trial') AS trial,
  count(*) FILTER (WHERE eff.status IN ('expired', 'stopped')) AS expired,
  count(*) FILTER (WHERE eff.status = 'pending') AS pending,
  count(DISTINCT u.id) FILTER (WHERE (SELECT count(*) FROM subscriptions sb JOIN workspaces x ON x.id = sb.workspace_id WHERE x.owner_user_id = u.id AND sb.trial_until IS NOT NULL) > 1) AS multi_trial_accounts,
  count(*) FILTER (WHERE u.last_seen_at > now() - interval '5 minutes') AS online,
  COALESCE(sum(p.price_month) FILTER (WHERE eff.status = 'active'), 0)::float8 AS mrr,
  COALESCE(sum(p.price_month) FILTER (WHERE eff.status = 'active' AND sub.current_period_end IS NOT NULL
      AND sub.current_period_end <= (date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta') + interval '1 month') AT TIME ZONE 'Asia/Jakarta'), 0)::float8 AS renewals_this_month,
  count(*) FILTER (WHERE eff.status = 'active' AND sub.current_period_end BETWEEN now() AND now() + interval '14 days') AS ending_14d`+customerFrom)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	k := counts[0]

	rev, err := httpx.Query(c, s.DB, `
SELECT COALESCE(sum(amount), 0)::float8 AS revenue, count(*) AS payments FROM payments
WHERE (paid_at AT TIME ZONE 'Asia/Jakarta') >= date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta')`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	k["revenue_month"] = rev[0]["revenue"]
	k["payments_month"] = rev[0]["payments"]
	// Proyeksi akhir bulan = yang sudah masuk + perpanjangan aktif yang jatuh tempo sisa bulan ini.
	k["projected_month"] = rev[0]["revenue"].(float64) + k["renewals_this_month"].(float64)

	series, err := httpx.Query(c, s.DB, `
SELECT to_char(m, 'YYYY-MM') AS month,
  COALESCE((SELECT sum(amount) FROM payments WHERE date_trunc('month', paid_at AT TIME ZONE 'Asia/Jakarta') = m), 0)::float8 AS revenue,
  (SELECT count(*) FROM payments WHERE date_trunc('month', paid_at AT TIME ZONE 'Asia/Jakarta') = m) AS payments
FROM generate_series(date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta') - interval '6 months',
                     date_trunc('month', now() AT TIME ZONE 'Asia/Jakarta'), interval '1 month') AS m ORDER BY m`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	ending, err := httpx.Query(c, s.DB, `SELECT `+customerCols+customerFrom+`
  AND eff.status = 'active' AND sub.current_period_end BETWEEN now() AND now() + interval '14 days'
ORDER BY sub.current_period_end LIMIT 20`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	pending, err := httpx.Query(c, s.DB, `
SELECT pp.id::text AS id, w.name AS customer, pp.kind, pp.billed::float8 AS billed, pp.created_at
FROM payment_proofs pp JOIN workspaces w ON w.id = pp.workspace_id
WHERE pp.status = 'pending' AND pp.deleted_at IS NULL ORDER BY pp.created_at LIMIT 10`)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"kpi": k, "revenue_series": series, "ending_soon": ending, "pending_proofs": pending})
}

// ---------- Customer ----------

var customerFilters = map[string]string{
	"online":  "COALESCE(u.last_seen_at > now() - interval '5 minutes', false)",
	"offline": "NOT COALESCE(u.last_seen_at > now() - interval '5 minutes', false)",
	"active":  "eff.status = 'active'",
	"expired": "eff.status IN ('expired', 'stopped')",
	"trial":   "eff.status = 'trial'",
	"pending": "eff.status = 'pending'",
}

func (s *Server) customerWhere(c *gin.Context) (*httpx.Where, bool) {
	w := &httpx.Where{}
	if f := c.Query("filter"); f != "" && f != "all" {
		cond, ok := customerFilters[f]
		if !ok {
			httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "filter tidak dikenal.")
			return nil, false
		}
		w.Raw(cond)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(w.name ILIKE '%' || ? || '%' OR u.name ILIKE '%' || ? || '%' OR w.city ILIKE '%' || ? || '%' OR u.username::text ILIKE '%' || ? || '%' OR u.email::text ILIKE '%' || ? || '%')`, httpx.EscapeLike(q))
		// keempat "?" diganti placeholder yang sama ($n): satu nilai dipakai untuk empat kolom
	}
	return w, true
}

func (s *Server) listCustomers(c *gin.Context) {
	w, ok := s.customerWhere(c)
	if !ok {
		return
	}
	page, per, off := httpx.Paging(c)
	var total int
	if err := s.DB.QueryRow(c, `SELECT count(*) `+customerFrom+` AND `+w.SQL(), w.Args...).Scan(&total); err != nil {
		httpx.ServerError(c, err)
		return
	}
	rows, err := httpx.Query(c, s.DB, fmt.Sprintf(`SELECT %s %s AND %s ORDER BY w.created_at DESC LIMIT %d OFFSET %d`,
		customerCols, customerFrom, w.SQL(), per, off), w.Args...)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	// Jumlah per filter (untuk chip), tanpa memedulikan filter/pencarian yang aktif.
	counts, err := httpx.Query(c, s.DB, `
SELECT count(*) AS "all",
  count(*) FILTER (WHERE `+customerFilters["online"]+`) AS online,
  count(*) FILTER (WHERE `+customerFilters["offline"]+`) AS offline,
  count(*) FILTER (WHERE `+customerFilters["active"]+`) AS active,
  count(*) FILTER (WHERE `+customerFilters["expired"]+`) AS expired,
  count(*) FILTER (WHERE `+customerFilters["trial"]+`) AS trial,
  count(*) FILTER (WHERE `+customerFilters["pending"]+`) AS pending`+customerFrom)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "meta": gin.H{"page": page, "per_page": per, "total": total, "counts": counts[0]}})
}

func (s *Server) getCustomer(c *gin.Context) {
	rows, err := httpx.Query(c, s.DB, `SELECT `+customerCols+`, w.category, sub.status AS raw_status, sub.current_period_end, u.created_at AS owner_since`+
		customerFrom+` AND w.id::text = $1`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if len(rows) == 0 {
		httpx.Fail(c, http.StatusNotFound, "customer_not_found", "Customer tidak ditemukan.")
		return
	}
	cust := rows[0]
	pays, err := httpx.Query(c, s.DB, `
SELECT pay.id::text AS id, pay.paid_at, pay.amount::float8 AS amount, pay.months, pay.period_start, pay.period_end, pay.ad_slots, pay.purchase,
       p.code AS plan, p.name AS plan_name, sp.name AS sales, pay.payment_proof_id IS NOT NULL AS from_proof
FROM payments pay JOIN plans p ON p.id = pay.plan_id JOIN sales_people sp ON sp.id = pay.sales_id
WHERE pay.workspace_id::text = $1 ORDER BY pay.paid_at DESC LIMIT 100`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	conn, err := httpx.Query(c, s.DB, `
SELECT mode, status, pixel_id, (access_token_enc IS NOT NULL) AS has_token, last_check_at, last_error_code
FROM meta_connections WHERE workspace_id::text = $1`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	stats, err := httpx.Query(c, s.DB, `
SELECT (SELECT count(*) FROM leads WHERE workspace_id::text = $1 AND deleted_at IS NULL) AS leads,
  (SELECT count(*) FROM events WHERE workspace_id::text = $1 AND status = 'sent' AND source <> 'test') AS events_sent,
  (SELECT count(*) FROM events WHERE workspace_id::text = $1 AND status = 'failed' AND source <> 'test') AS events_failed,
  (SELECT max(created_at) FROM events WHERE workspace_id::text = $1) AS last_event_at`, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	cust["payments"] = pays
	if cust["ad_slots"], err = core.SlotQuota(c, s.DB, c.Param("id")); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if cust["ad_slots_used"], err = core.SlotUsage(c, s.DB, c.Param("id")); err != nil {
		httpx.ServerError(c, err)
		return
	}
	diag, err := s.customerDiagnostics(c, c.Param("id"))
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	cust["diagnostics"] = diag
	cust["connection"] = nil
	if len(conn) > 0 {
		cust["connection"] = conn[0] // ringkasan saja; tidak pernah memuat token/secret
	}
	cust["stats"] = stats[0]
	httpx.OK(c, cust)
}

func (s *Server) exportCustomers(c *gin.Context) {
	w, ok := s.customerWhere(c)
	if !ok {
		return
	}
	rows, err := httpx.Query(c, s.DB, `SELECT `+customerCols+customerFrom+` AND `+w.SQL()+` ORDER BY w.name`, w.Args...)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	head := []string{"Bisnis", "Pemilik", "Username", "Email", "Telepon", "Kota", "Status", "Paket", "Sales", "Berlaku s/d"}
	data := [][]string{head}
	for _, r := range rows {
		data = append(data, []string{str(r["name"]), str(r["owner"]), str(r["username"]), str(r["email"]), str(r["phone"]), str(r["city"]), str(r["status"]),
			str(r["plan_name"]), str(r["sales"]), dateStr(r["until"])})
	}
	writeCSV(c, "customer.csv", data)
}

// ---------- Aktivasi / perpanjang / stop ----------

type activation struct {
	WorkspaceID string
	PlanCode    string
	Months      int
	Amount      int64
	SalesID     string
	ProofID     string
	AdSlots     int    // jumlah slot kampanye yang dibeli (min. 1)
	Purchase    string // 'subscription' (default) atau 'slots' (tambah slot 30 hari, langganan tidak berubah)
}

var (
	errNoPlan   = errors.New("paket tidak ditemukan")
	errNoSales  = errors.New("sales tidak ditemukan atau tidak aktif")
	errNoWS     = errors.New("customer tidak ditemukan")
	errNoAccess = errors.New("tambah slot hanya untuk langganan yang sedang aktif")
)

// activate membuat payment dan memperpanjang langganan di dalam transaksi tx:
//  1. periode baru mulai dari current_period_end bila langganan masih aktif, selain itu dari sekarang
//  2. persentase komisi/kas disalin ke payment (snapshot) agar riwayat bagi hasil tidak berubah
//  3. status langganan menjadi active; sales pertama customer dipertahankan
//
// Pemanggil menulis audit log dan notifikasi dalam transaksi yang sama.
func (s *Server) activate(c *gin.Context, tx pgx.Tx, a activation) (end time.Time, err error) {
	var planID string
	if err = tx.QueryRow(c, `SELECT id::text FROM plans WHERE code = $1 AND deleted_at IS NULL`, a.PlanCode).Scan(&planID); err != nil {
		return end, mapNoRows(err, errNoPlan)
	}
	if err = tx.QueryRow(c, `SELECT id::text FROM sales_people WHERE id::text = $1 AND active`, a.SalesID).Scan(&a.SalesID); err != nil {
		return end, mapNoRows(err, errNoSales)
	}
	var komisi, kas float64
	if err = tx.QueryRow(c, `SELECT
	  COALESCE((SELECT (value #>> '{}')::numeric FROM settings WHERE key = 'komisi_pct'), 10)::float8,
	  COALESCE((SELECT (value #>> '{}')::numeric FROM settings WHERE key = 'kas_pct'), 10)::float8`).Scan(&komisi, &kas); err != nil {
		return end, err
	}
	// Kunci baris langganan agar dua aktivasi bersamaan tidak saling menimpa.
	var subID, status string
	var curEnd *time.Time
	if err = tx.QueryRow(c, `SELECT id::text, status, current_period_end FROM subscriptions WHERE workspace_id::text = $1 FOR UPDATE`, a.WorkspaceID).
		Scan(&subID, &status, &curEnd); err != nil {
		return end, mapNoRows(err, errNoWS)
	}
	now := time.Now().UTC()
	if a.AdSlots < 1 {
		a.AdSlots = 1
	}
	if a.Purchase == "" {
		a.Purchase = "subscription"
	}
	var start time.Time
	if a.Purchase == "slots" {
		// Tambah slot: berlaku 30 hari sejak disetujui, berjalan sendiri; langganan utama tidak berubah.
		ok, err := core.WorkspaceAccess(c, tx, a.WorkspaceID)
		if err != nil {
			return end, err
		}
		if !ok {
			return end, errNoAccess
		}
		a.Months = 1
		start, end = now, now.AddDate(0, 0, billing.DaysPerMonth)
	} else {
		base := curEnd
		if status != "active" {
			base = nil
		}
		start, end = billing.NextPeriod(now, base, a.Months)
	}
	if _, err = tx.Exec(c, `
INSERT INTO payments (workspace_id, subscription_id, plan_id, payment_proof_id, amount, months, period_start, period_end,
                      paid_at, sales_id, komisi_pct, kas_pct, approved_by, ad_slots, purchase)
VALUES ($1::uuid, $2::uuid, $3::uuid, NULLIF($4,'')::uuid, $5, $6, $7, $8, $9, $10::uuid, $11, $12, $13::uuid, $14, $15)`,
		a.WorkspaceID, subID, planID, a.ProofID, a.Amount, a.Months, start, end, now, a.SalesID, komisi, kas, adminID(c), a.AdSlots, a.Purchase); err != nil {
		return end, err
	}
	if a.Purchase == "slots" {
		_, err = tx.Exec(c, `UPDATE subscriptions SET sales_id = COALESCE(sales_id, $2::uuid) WHERE id::text = $1`, subID, a.SalesID)
		return end, err
	}
	_, err = tx.Exec(c, `UPDATE subscriptions SET status = 'active', plan_id = $2::uuid, current_period_end = $3,
	  sales_id = COALESCE(sales_id, $4::uuid) WHERE id::text = $1`, subID, planID, end, a.SalesID)
	return end, err
}

func mapNoRows(err, to error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return to
	}
	return err
}

type extendInput struct {
	Plan     string  `json:"plan"`
	Months   int     `json:"months"`
	Amount   float64 `json:"amount"`
	SalesID  string  `json:"sales_id"`
	AdSlots  int     `json:"ad_slots"` // 0 = ikut bukti transfer (approve) atau 1 (perpanjang manual)
	Purchase string  `json:"purchase"` // '' = ikut bukti transfer / 'subscription'; 'slots' = tambah slot 30 hari
}

// validate memeriksa input aktivasi dan menjawab 422 bila salah.
func (in extendInput) validate(c *gin.Context) (int64, bool) {
	msg := ""
	switch {
	case in.Plan == "":
		msg = "plan (kode paket) wajib diisi."
	case !billing.ValidMonths(in.Months):
		msg = "months harus 1, 3, atau 12."
	case in.Amount <= 0 || in.Amount != math.Trunc(in.Amount) || in.Amount > 1e12:
		msg = "amount harus bilangan bulat Rupiah > 0."
	case in.SalesID == "":
		msg = "sales_id wajib diisi (penerima komisi)."
	}
	if msg != "" {
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", msg)
		return 0, false
	}
	return int64(in.Amount), true
}

// activationError memetakan error domain aktivasi ke HTTP.
func activationError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, errNoPlan), errors.Is(err, errNoSales):
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", err.Error()+".")
	case errors.Is(err, errNoAccess):
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "Tambah slot hanya untuk langganan yang sedang aktif. Perpanjang langganannya dulu.")
	case errors.Is(err, errNoWS):
		httpx.Fail(c, http.StatusNotFound, "customer_not_found", "Customer tidak ditemukan.")
	default:
		return false
	}
	return true
}

func (s *Server) extendCustomer(c *gin.Context) {
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
	end, err := s.activate(c, tx, activation{WorkspaceID: c.Param("id"), PlanCode: in.Plan, Months: in.Months, Amount: amount, SalesID: in.SalesID,
		AdSlots: in.AdSlots, Purchase: in.Purchase})
	if activationError(c, err) {
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.notifyOwner(c, tx, c.Param("id"), "subscription_active", "Langganan aktif",
		fmt.Sprintf("Langganan kamu aktif sampai %s.", end.In(jakarta()).Format("2 Jan 2006")), "/langganan"); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "customer.extend", c.Param("id"), "workspace", c.Param("id"),
		map[string]any{"plan": in.Plan, "months": in.Months, "amount": amount, "sales_id": in.SalesID, "period_end": end, "ad_slots": in.AdSlots, "purchase": in.Purchase}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"status": "active", "current_period_end": end})
}

func (s *Server) stopCustomer(c *gin.Context) {
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	var status string
	err = tx.QueryRow(c, `SELECT status FROM subscriptions WHERE workspace_id::text = $1 FOR UPDATE`, c.Param("id")).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, http.StatusNotFound, "customer_not_found", "Customer tidak ditemukan.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if status == "stopped" {
		httpx.Fail(c, http.StatusConflict, "already_stopped", "Langganan sudah dihentikan.")
		return
	}
	if _, err := tx.Exec(c, `UPDATE subscriptions SET status = 'stopped', current_period_end = LEAST(COALESCE(current_period_end, now()), now()),
	  trial_until = LEAST(COALESCE(trial_until, now()), now()) WHERE workspace_id::text = $1`, c.Param("id")); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "customer.stop", c.Param("id"), "workspace", c.Param("id"), map[string]any{"previous_status": status}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"status": "stopped"})
}

// notifyOwner mengirim notifikasi lonceng ke pemilik workspace.
func (s *Server) notifyOwner(c *gin.Context, tx pgx.Tx, workspaceID, typ, title, body, link string) error {
	_, err := tx.Exec(c, `INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
		SELECT owner_user_id, id, $2, $3, $4, $5 FROM workspaces WHERE id::text = $1`, workspaceID, typ, title, body, link)
	return err
}

// ---------- Util ----------

func jakarta() *time.Location {
	if l, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return l
	}
	return time.FixedZone("WIB", 7*3600)
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func dateStr(v any) string {
	if t, ok := v.(time.Time); ok {
		return t.In(jakarta()).Format("2006-01-02")
	}
	return ""
}

// csvSafe mencegah injeksi rumus saat CSV dibuka di Excel/Sheets: sel yang diawali = + - @ diberi apostrof.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func writeCSV(c *gin.Context, name string, rows [][]string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF")) // BOM agar Excel membaca UTF-8
	w := csv.NewWriter(c.Writer)
	for i, r := range rows {
		if i > 0 {
			out := make([]string, len(r))
			for j, v := range r {
				out[j] = csvSafe(v)
			}
			r = out
		}
		_ = w.Write(r)
	}
	w.Flush()
}
