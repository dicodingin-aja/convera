package adminapi

import (
	"fmt"
	"math"
	"net/http"
	"regexp"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/billing"
	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

var monthRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

type person struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	IsFounder bool       `json:"is_founder"`
	Share     int64      `json:"share"`  // bagian rata 80%
	Komisi    int64      `json:"komisi"` // komisi sebagai sales
	Total     int64      `json:"total"`
	Paid      bool       `json:"paid"`
	PaidAt    *time.Time `json:"paid_at"`
}

type shareLine struct {
	Customer string    `json:"customer"`
	Sales    string    `json:"sales"`
	PaidAt   time.Time `json:"paid_at"`
	Months   int       `json:"months"`
	Amount   int64     `json:"amount"`
	Komisi   int64     `json:"komisi"`
	Kas      int64     `json:"kas"`
	Each     int64     `json:"each"`
}

type shareReport struct {
	Month    string      `json:"month"`
	Revenue  int64       `json:"revenue"`
	Kas      int64       `json:"kas"`
	Count    int         `json:"payments_count"`
	People   []*person   `json:"people"`
	Payments []shareLine `json:"payments"`
}

// currentMonth mengembalikan bulan berjalan (zona Asia/Jakarta) berformat YYYY-MM.
func currentMonth() string { return time.Now().In(jakarta()).Format("2006-01") }

// computeShare menghitung bagi hasil satu bulan dari tabel payments. Persentase tiap pembayaran memakai
// snapshot yang disimpan saat aktivasi, sehingga mengubah aturan tidak mengubah riwayat.
// Daftar founder yang ikut bagian rata adalah founder aktif SAAT INI.
func (s *Server) computeShare(c *gin.Context, month string) (*shareReport, error) {
	people, err := httpx.Query(c, s.DB, `SELECT id::text AS id, name::text AS name, is_founder FROM sales_people ORDER BY is_founder DESC, sort, name`)
	if err != nil {
		return nil, err
	}
	founders := 0
	byID := map[string]*person{}
	var order []*person
	for _, p := range people {
		pp := &person{ID: p["id"].(string), Name: p["name"].(string), IsFounder: p["is_founder"].(bool)}
		byID[pp.ID] = pp
		order = append(order, pp)
	}
	active, err := httpx.Query(c, s.DB, `SELECT id::text AS id FROM sales_people WHERE is_founder AND active`)
	if err != nil {
		return nil, err
	}
	isActiveFounder := map[string]bool{}
	for _, a := range active {
		isActiveFounder[a["id"].(string)] = true
		founders++
	}

	pays, err := httpx.Query(c, s.DB, `
SELECT w.name AS customer, pay.sales_id::text AS sales_id, pay.paid_at, pay.months, pay.amount::float8 AS amount,
       pay.komisi_pct::float8 AS komisi_pct, pay.kas_pct::float8 AS kas_pct
FROM payments pay JOIN workspaces w ON w.id = pay.workspace_id
WHERE to_char(pay.paid_at AT TIME ZONE 'Asia/Jakarta', 'YYYY-MM') = $1 ORDER BY pay.paid_at`, month)
	if err != nil {
		return nil, err
	}
	rep := &shareReport{Month: month, Payments: []shareLine{}}
	for _, p := range pays {
		amount := int64(math.Round(p["amount"].(float64)))
		sh := billing.Split(amount, p["komisi_pct"].(float64), p["kas_pct"].(float64), founders)
		sid := p["sales_id"].(string)
		if sp := byID[sid]; sp != nil {
			sp.Komisi += sh.Komisi
		}
		for id := range isActiveFounder {
			byID[id].Share += sh.Each
		}
		rep.Revenue += amount
		rep.Kas += sh.Kas
		rep.Count++
		name := ""
		if sp := byID[sid]; sp != nil {
			name = sp.Name
		}
		rep.Payments = append(rep.Payments, shareLine{Customer: p["customer"].(string), Sales: name, PaidAt: p["paid_at"].(time.Time),
			Months: int(p["months"].(int32)), Amount: amount, Komisi: sh.Komisi, Kas: sh.Kas, Each: sh.Each})
	}

	paid, err := httpx.Query(c, s.DB, `SELECT person_id::text AS person_id, transferred_at FROM payouts WHERE month = ($1 || '-01')::date`, month)
	if err != nil {
		return nil, err
	}
	for _, p := range paid {
		if pp := byID[p["person_id"].(string)]; pp != nil {
			t := p["transferred_at"].(time.Time)
			pp.Paid, pp.PaidAt = true, &t
		}
	}
	for _, p := range order {
		p.Total = p.Share + p.Komisi
		// Tampilkan founder aktif dan siapa pun yang punya nominal; sembunyikan sales luar tanpa komisi.
		if p.Total > 0 || isActiveFounder[p.ID] {
			rep.People = append(rep.People, p)
		}
	}
	if rep.People == nil {
		rep.People = []*person{}
	}
	return rep, nil
}

func (s *Server) monthParam(c *gin.Context, v string) (string, bool) {
	if v == "" {
		return currentMonth(), true
	}
	if !monthRe.MatchString(v) {
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "month harus berformat YYYY-MM.")
		return "", false
	}
	return v, true
}

func (s *Server) revenueShare(c *gin.Context) {
	month, ok := s.monthParam(c, c.Query("month"))
	if !ok {
		return
	}
	rep, err := s.computeShare(c, month)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, rep)
}

func (s *Server) exportRevenueShare(c *gin.Context) {
	month, ok := s.monthParam(c, c.Query("month"))
	if !ok {
		return
	}
	rep, err := s.computeShare(c, month)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	rows := [][]string{{"Bulan", "Nama", "Bagian rata", "Komisi", "Total", "Sudah ditransfer"}}
	for _, p := range rep.People {
		paid := "Belum"
		if p.Paid {
			paid = "Sudah"
		}
		rows = append(rows, []string{month, p.Name, fmt.Sprint(p.Share), fmt.Sprint(p.Komisi), fmt.Sprint(p.Total), paid})
	}
	rows = append(rows, []string{month, "Kas bersama", "", "", fmt.Sprint(rep.Kas), ""})
	writeCSV(c, "bagi-hasil-"+month+".csv", rows)
}

// markPaid menandai (atau membatalkan) bagi hasil satu orang sudah ditransfer. Nominal dihitung ulang di
// server dari tabel payments; angka dari klien tidak dipercaya.
func (s *Server) markPaid(c *gin.Context) {
	month, ok := s.monthParam(c, c.Param("month"))
	if !ok || c.Param("month") == "" {
		return
	}
	if month > currentMonth() {
		httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "Bulan belum berjalan.")
		return
	}
	var in struct {
		Paid *bool `json:"paid"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Paid == nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Kirim {\"paid\": true|false}.")
		return
	}
	rep, err := s.computeShare(c, month)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	var target *person
	for _, p := range rep.People {
		if p.ID == c.Param("person") {
			target = p
		}
	}
	if target == nil {
		httpx.Fail(c, http.StatusNotFound, "person_not_found", "Orang ini tidak punya bagian pada bulan tersebut.")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	if *in.Paid {
		if target.Total <= 0 {
			httpx.Fail(c, http.StatusUnprocessableEntity, "validation_error", "Tidak ada nominal untuk ditandai.")
			return
		}
		_, err = tx.Exec(c, `INSERT INTO payouts (month, person_id, amount, marked_by) VALUES (($1 || '-01')::date, $2::uuid, $3, $4::uuid)
			ON CONFLICT (month, person_id) DO UPDATE SET amount = EXCLUDED.amount, transferred_at = now(), marked_by = EXCLUDED.marked_by`,
			month, target.ID, target.Total, adminID(c))
	} else {
		_, err = tx.Exec(c, `DELETE FROM payouts WHERE month = ($1 || '-01')::date AND person_id::text = $2`, month, target.ID)
	}
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "revenue_share.mark_paid", "", "payout", month+"/"+target.ID,
		map[string]any{"month": month, "person": target.Name, "paid": *in.Paid, "amount": target.Total}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"month": month, "person_id": target.ID, "paid": *in.Paid, "amount": target.Total})
}
