package server

// Uji slot kampanye iklan: isi otomatis, slot penuh, chat organik, penggabungan iklan ke kampanye, masa kunci
// 30 hari, beli langganan/tambah slot, dan approve lewat API admin sungguhan.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/adminapi"
	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/dispatch"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type slotState struct {
	Quota   int `json:"quota"`
	Used    int `json:"used"`
	Free    int `json:"free"`
	Tracked []struct {
		Kind      string `json:"kind"`
		ID        string `json:"id"`
		Auto      bool   `json:"auto"`
		Effective bool   `json:"effective"`
	} `json:"tracked"`
	Candidates []struct {
		Kind    string `json:"kind"`
		ID      string `json:"id"`
		Tracked bool   `json:"tracked"`
	} `json:"candidates"`
}

func (c *customer) slots(ws string) slotState {
	c.t.Helper()
	w := c.call("GET", "/api/v1/ad-slots", ws, nil)
	if w.Code != http.StatusOK {
		c.t.Fatalf("GET /ad-slots: %d %s", w.Code, w.Body)
	}
	var out struct {
		Data slotState `json:"data"`
	}
	decode(c.t, w, &out)
	return out.Data
}

func (st slotState) candidate(kind, id string) (found, tracked bool) {
	for _, x := range st.Candidates {
		if x.Kind == kind && x.ID == id {
			return true, x.Tracked
		}
	}
	return false, false
}

// slotFixture: workspace mode QR dengan Pixel + token dan satu aturan Purchase bernilai tetap, plus Meta palsu.
type slotFixture struct {
	s     *Server
	h     http.Handler
	db    *pgxpool.Pool
	a     *customer
	ws    createdWS
	calls *atomic.Int32
	d     *dispatch.Dispatcher
	seq   int
}

func newSlotFixture(t *testing.T) *slotFixture {
	t.Helper()
	s, h := newIntegrationServer(t)
	f := &slotFixture{s: s, h: h, db: s.DB, calls: &atomic.Int32{}}
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events_received":1,"fbtrace_id":"TRACE"}`))
	}))
	t.Cleanup(graph.Close)
	f.d = &dispatch.Dispatcher{DB: s.DB, Box: s.Box, Meta: &meta.Client{BaseURL: graph.URL, Version: "v23.0"}, Batch: 50}

	execSQL(t, s.DB, `INSERT INTO feature_flags (key, enabled) VALUES ('qr', true) ON CONFLICT (key) DO UPDATE SET enabled = true`)
	f.a = newCustomer(t, h, "slot_owner")
	f.ws = f.a.createWS("Toko Slot")
	if w := f.a.call("PUT", "/api/v1/connections/meta", f.ws.Slug, map[string]any{"mode": "qr", "pixel_id": "123456789", "access_token": "EAAtokenrahasiauji"}); w.Code != http.StatusOK {
		t.Fatalf("koneksi meta: %d %s", w.Code, w.Body)
	}
	rule := map[string]any{"name": "Bayar", "who": "customer", "keywords": []string{"transfer"}, "event_name": "Purchase", "value_mode": "fixed", "fixed_value": 100000}
	if w := f.a.call("POST", "/api/v1/event-rules", f.ws.Slug, rule); w.Code != http.StatusCreated {
		t.Fatalf("aturan: %d %s", w.Code, w.Body)
	}
	return f
}

// chat mengirim pesan "sudah transfer" (memicu aturan) dari nomor baru, opsional dari iklan adID, lalu
// menjalankan worker satu putaran. Mengembalikan status event yang dibuat.
func (f *slotFixture) chat(t *testing.T, adID string) string {
	t.Helper()
	f.seq++
	var ad map[string]any
	if adID != "" {
		ad = map[string]any{"ad_id": adID, "click_id": fmt.Sprintf("CLICK-%d", f.seq), "headline": "Iklan " + adID}
	}
	phone := fmt.Sprintf("0813%08d", f.seq)
	if w := qrMessageIn(t, f.h, f.ws.ID, fmt.Sprintf("SLOT-MSG-%d", f.seq), phone, "kak sudah transfer ya", ad); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan: %d %s", w.Code, w.Body)
	}
	if _, err := f.d.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	var code *string
	if err := f.db.QueryRow(context.Background(), `SELECT e.status, e.error_code FROM events e JOIN leads l ON l.id = e.lead_id
		WHERE l.workspace_id = $1::uuid AND l.phone = $2`, f.ws.ID, "62"+phone[1:]).Scan(&status, &code); err != nil {
		t.Fatalf("event untuk pesan %d: %v", f.seq, err)
	}
	if code != nil && status != "sent" {
		return status + ":" + *code
	}
	return status
}

func TestAdSlots_TrialAutoFillFullAndOrganic(t *testing.T) {
	f := newSlotFixture(t)

	// Trial = 1 slot. Iklan pertama yang mendatangkan event mengisi slot otomatis.
	if got := f.chat(t, "111"); got != "sent" {
		t.Fatalf("event iklan pertama = %q; want sent", got)
	}
	st := f.a.slots(f.ws.Slug)
	if st.Quota != 1 || st.Used != 1 || st.Free != 0 || len(st.Tracked) != 1 || st.Tracked[0].ID != "111" || !st.Tracked[0].Auto || !st.Tracked[0].Effective {
		t.Fatalf("slot setelah iklan pertama: %+v", st)
	}
	// Iklan kedua (kampanye lain): slot penuh -> event tidak dikirim.
	if got := f.chat(t, "222"); got != "skipped:ad_not_tracked" {
		t.Fatalf("event iklan kedua = %q; want skipped:ad_not_tracked", got)
	}
	if f.calls.Load() != 1 {
		t.Fatalf("panggilan Meta = %d; want 1", f.calls.Load())
	}
	// Chat organik (tanpa iklan) tidak memakai slot dan tetap dikirim.
	if got := f.chat(t, ""); got != "sent" {
		t.Fatalf("event organik = %q; want sent", got)
	}
	// Iklan yang sama dengan slot terpakai tetap terkirim.
	if got := f.chat(t, "111"); got != "sent" {
		t.Fatalf("event iklan pertama (lagi) = %q; want sent", got)
	}

	st = f.a.slots(f.ws.Slug)
	if found, tracked := st.candidate("ad", "222"); !found || tracked {
		t.Fatalf("iklan 222 harus jadi pilihan yang belum dilacak: %+v", st.Candidates)
	}
	if w := f.a.call("POST", "/api/v1/ad-slots/track", f.ws.Slug, map[string]any{"kind": "ad", "id": "222"}); w.Code != http.StatusConflict || errCode(t, w) != "slots_full" {
		t.Fatalf("lacak saat penuh: %d %s; want 409 slots_full", w.Code, w.Body)
	}
	if w := f.a.call("POST", "/api/v1/ad-slots/track", f.ws.Slug, map[string]any{"kind": "campaign", "id": "999"}); w.Code != http.StatusNotFound {
		t.Fatalf("lacak kampanye tak dikenal: %d; want 404", w.Code)
	}
}

func TestAdSlots_CampaignMappingAndLock(t *testing.T) {
	f := newSlotFixture(t)
	ctx := context.Background()
	if got := f.chat(t, "111"); got != "sent" {
		t.Fatalf("event iklan 111 = %q", got)
	}
	// Sinkron Meta Ads mengetahui: iklan 111 dan 333 milik kampanye C9, iklan 222 milik kampanye C8.
	execSQL(t, f.db, `INSERT INTO ad_campaign_map (workspace_id, ad_id, campaign_id, campaign_name) VALUES
		($1::uuid, '111', '9000000009', 'Promo Oktober'), ($1::uuid, '333', '9000000009', 'Promo Oktober'), ($1::uuid, '222', '8000000008', 'Hampers')`, f.ws.ID)
	if err := core.ConsolidateTracked(ctx, f.db, f.ws.ID); err != nil {
		t.Fatal(err)
	}
	st := f.a.slots(f.ws.Slug)
	if len(st.Tracked) != 1 || st.Tracked[0].Kind != "campaign" || st.Tracked[0].ID != "9000000009" || st.Used != 1 {
		t.Fatalf("setelah digabung ke kampanye: %+v", st)
	}
	// Iklan lain dari kampanye yang sama ikut terlacak tanpa slot tambahan.
	if got := f.chat(t, "333"); got != "sent" {
		t.Fatalf("event iklan 333 (kampanye sama) = %q; want sent", got)
	}
	if got := f.chat(t, "222"); got != "skipped:ad_not_tracked" {
		t.Fatalf("event iklan 222 (kampanye lain) = %q; want skipped", got)
	}

	// Lepas kampanye: slot tetap terpakai 30 hari, jadi kampanye lain belum bisa dilacak.
	if w := f.a.call("POST", "/api/v1/ad-slots/release", f.ws.Slug, map[string]any{"kind": "campaign", "id": "9000000009"}); w.Code != http.StatusOK {
		t.Fatalf("lepas: %d %s", w.Code, w.Body)
	}
	if st := f.a.slots(f.ws.Slug); st.Used != 1 || st.Free != 0 {
		t.Fatalf("slot setelah dilepas (masa kunci): %+v", st)
	}
	if w := f.a.call("POST", "/api/v1/ad-slots/track", f.ws.Slug, map[string]any{"kind": "campaign", "id": "8000000008"}); w.Code != http.StatusConflict {
		t.Fatalf("lacak kampanye lain dalam masa kunci: %d; want 409", w.Code)
	}
	// Kampanye yang baru dilepas bisa dilacak lagi tanpa slot baru.
	if w := f.a.call("POST", "/api/v1/ad-slots/track", f.ws.Slug, map[string]any{"kind": "campaign", "id": "9000000009"}); w.Code != http.StatusOK {
		t.Fatalf("lacak ulang kampanye yang sama: %d %s", w.Code, w.Body)
	}
	// Setelah 30 hari, slot yang dilepas kembali kosong.
	execSQL(t, f.db, `UPDATE tracked_targets SET started_at = now() - interval '31 days' WHERE workspace_id = $1::uuid`, f.ws.ID)
	if w := f.a.call("POST", "/api/v1/ad-slots/release", f.ws.Slug, map[string]any{"kind": "campaign", "id": "9000000009"}); w.Code != http.StatusOK {
		t.Fatalf("lepas setelah 30 hari: %d", w.Code)
	}
	if w := f.a.call("POST", "/api/v1/ad-slots/track", f.ws.Slug, map[string]any{"kind": "campaign", "id": "8000000008"}); w.Code != http.StatusOK {
		t.Fatalf("lacak kampanye lain setelah masa kunci: %d %s", w.Code, w.Body)
	}
	if got := f.chat(t, "222"); got != "sent" {
		t.Fatalf("event iklan 222 setelah kampanyenya dilacak = %q; want sent", got)
	}
}

// adminClient login ke API admin sungguhan.
func adminClient(t *testing.T, s *Server) *customer {
	db := s.DB
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin-rahasia-123"), bcrypt.MinCost)
	execSQL(t, db, `INSERT INTO admins (username, password_hash, name, role) VALUES ('uji_admin', $1, 'Admin Uji', 'superadmin')`, string(hash))
	srv := adminapi.New(s.Cfg, db, nil)
	c := &customer{t: t, h: srv.Router(), ip: "10.99.0.1", cookies: map[string]string{}, csrfName: "csrf_admin"}
	if w := c.call("POST", "/api/admin/v1/auth/login", "", map[string]any{"username": "uji_admin", "password": "admin-rahasia-123"}); w.Code != http.StatusOK {
		t.Fatalf("login admin: %d %s", w.Code, w.Body)
	}
	return c
}

func (c *customer) approve(t *testing.T, db *pgxpool.Pool, proofID string, months int, amount float64) *httptest.ResponseRecorder {
	t.Helper()
	var sales string
	if err := db.QueryRow(context.Background(), `SELECT id::text FROM sales_people WHERE active ORDER BY created_at LIMIT 1`).Scan(&sales); err != nil {
		t.Fatal(err)
	}
	return c.call("POST", "/api/admin/v1/payment-proofs/"+proofID+"/approve", "", map[string]any{"plan": "workspace", "months": months, "amount": amount, "sales_id": sales})
}

func proofResult(t *testing.T, w *httptest.ResponseRecorder) (id string, billed float64) {
	t.Helper()
	if w.Code != http.StatusCreated {
		t.Fatalf("unggah bukti: %d %s", w.Code, w.Body)
	}
	var out struct {
		Data struct {
			ID     string  `json:"id"`
			Billed float64 `json:"billed"`
		} `json:"data"`
	}
	decode(t, w, &out)
	return out.Data.ID, out.Data.Billed
}

func TestAdSlots_PurchaseAndAdminApproval(t *testing.T) {
	s, h := newIntegrationServer(t)
	db := s.DB
	admin := adminClient(t, s)
	a := newCustomer(t, h, "pembeli_slot")
	ws := a.createWS("Toko Beli")

	// Tambah 2 slot saat trial: Rp150.000 x 2, status langganan tetap trial.
	id, billed := proofResult(t, a.uploadProofWith(ws.Slug, map[string]string{"purchase": "slots", "ad_slots": "2", "months": "3"}))
	if billed != 300000 {
		t.Fatalf("tagihan tambah 2 slot = %v; want 300000 (selalu 30 hari)", billed)
	}
	var sub struct {
		Data struct {
			Status  string `json:"status"`
			AdSlots int    `json:"ad_slots"`
		} `json:"data"`
	}
	decode(t, a.call("GET", "/api/v1/subscription", ws.Slug, nil), &sub)
	if sub.Data.Status != "trial" || sub.Data.AdSlots != 1 {
		t.Fatalf("langganan setelah unggah tambah slot = %+v; want trial, 1 slot", sub.Data)
	}
	if w := admin.approve(t, db, id, 1, 300000); w.Code != http.StatusOK {
		t.Fatalf("approve tambah slot: %d %s", w.Code, w.Body)
	}
	if st := a.slots(ws.Slug); st.Quota != 3 {
		t.Fatalf("kuota setelah tambah 2 slot saat trial = %d; want 3", st.Quota)
	}
	var days float64
	if err := db.QueryRow(context.Background(), `SELECT extract(epoch FROM period_end - period_start) / 86400 FROM payments WHERE workspace_id = $1::uuid AND purchase = 'slots'`, ws.ID).Scan(&days); err != nil || days != 30 {
		t.Fatalf("periode tambah slot = %v hari (err %v); want 30", days, err)
	}

	// Langganan 3 bulan, 3 slot: 150.000 x 3 x 3.
	id, billed = proofResult(t, a.uploadProofWith(ws.Slug, map[string]string{"months": "3", "ad_slots": "3"}))
	if billed != 1350000 {
		t.Fatalf("tagihan 3 bulan x 3 slot = %v; want 1350000", billed)
	}
	if w := admin.approve(t, db, id, 3, 1350000); w.Code != http.StatusOK {
		t.Fatalf("approve langganan: %d %s", w.Code, w.Body)
	}
	var end time.Time
	if err := db.QueryRow(context.Background(), `SELECT current_period_end FROM subscriptions WHERE workspace_id = $1::uuid`, ws.ID).Scan(&end); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(end).Hours() / 24; d < 89.9 || d > 90.1 {
		t.Fatalf("periode 3 bulan = %.2f hari; want 90", d)
	}
	if st := a.slots(ws.Slug); st.Quota != 5 {
		t.Fatalf("kuota langganan 3 slot + tambahan 2 = %d; want 5", st.Quota)
	}

	// Tambahan habis setelah 30 hari: kuota kembali ke slot langganan. Target yang lebih dulu dilacak didahulukan.
	execSQL(t, db, `INSERT INTO ad_campaign_map (workspace_id, ad_id, campaign_id, campaign_name) VALUES
		($1::uuid, '1', '101', 'K1'), ($1::uuid, '2', '102', 'K2'), ($1::uuid, '3', '103', 'K3'), ($1::uuid, '4', '104', 'K4')`, ws.ID)
	for _, cid := range []string{"101", "102", "103", "104"} {
		if w := a.call("POST", "/api/v1/ad-slots/track", ws.Slug, map[string]any{"kind": "campaign", "id": cid}); w.Code != http.StatusOK {
			t.Fatalf("lacak %s: %d %s", cid, w.Code, w.Body)
		}
		time.Sleep(5 * time.Millisecond) // urutan started_at jelas
	}
	execSQL(t, db, `UPDATE payments SET period_start = now() - interval '31 days', period_end = now() - interval '1 day' WHERE workspace_id = $1::uuid AND purchase = 'slots'`, ws.ID)
	st := a.slots(ws.Slug)
	if st.Quota != 3 || st.Used != 4 {
		t.Fatalf("setelah tambahan habis: kuota %d terpakai %d; want 3 dan 4", st.Quota, st.Used)
	}
	effective := map[string]bool{}
	for _, x := range st.Tracked {
		effective[x.ID] = x.Effective
	}
	if !effective["101"] || !effective["102"] || !effective["103"] || effective["104"] {
		t.Fatalf("yang berlaku harus 3 kampanye pertama: %+v", effective)
	}
	if ok, err := core.EnsureAdTracked(context.Background(), db, ws.ID, "4"); err != nil || ok {
		t.Fatalf("iklan kampanye ke-4 setelah kuota turun: ok=%v err=%v; want false", ok, err)
	}
	if ok, err := core.EnsureAdTracked(context.Background(), db, ws.ID, "1"); err != nil || !ok {
		t.Fatalf("iklan kampanye pertama: ok=%v err=%v; want true", ok, err)
	}

	// Tambah slot hanya untuk langganan berjalan.
	b := newCustomer(t, h, "pembeli_lain")
	b.createWS("Trial Lain")
	unpaid := b.createWS("Belum Bayar")
	if w := b.uploadProofWith(unpaid.Slug, map[string]string{"purchase": "slots", "ad_slots": "1"}); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("tambah slot di workspace belum bayar: %d; want 422", w.Code)
	}
	// Slot workspace lain tidak terpengaruh.
	if st := b.slots(unpaid.Slug); st.Quota != 0 || st.Used != 0 {
		t.Fatalf("slot workspace belum bayar: %+v; want 0/0", st)
	}
	if w := b.call("POST", "/api/v1/ad-slots/track", unpaid.Slug, map[string]any{"kind": "campaign", "id": "101"}); w.Code != http.StatusPaymentRequired && w.Code != http.StatusNotFound {
		t.Fatalf("akun lain melacak kampanye workspace A: %d %s", w.Code, w.Body)
	}
	var raw map[string]any
	_ = json.Unmarshal(a.call("GET", "/api/v1/ad-slots", ws.Slug, nil).Body.Bytes(), &raw)
	if raw["data"] == nil {
		t.Fatal("respons /ad-slots kosong")
	}
}

func TestAdSlots_MetaAdsSyncGroupsAdsIntoCampaign(t *testing.T) {
	f := newSlotFixture(t)
	ctx := context.Background()
	if got := f.chat(t, "777"); got != "sent" {
		t.Fatalf("event iklan 777 = %q", got)
	}
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v23.0/act_555555":
			_, _ = w.Write([]byte(`{"name":"Toko Slot Ads","currency":"IDR","account_status":1,"id":"act_555555"}`))
		case "/v23.0/act_555555/insights":
			day := time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02")
			_, _ = w.Write([]byte(`{"data":[
{"date_start":"` + day + `","ad_id":"777","ad_name":"Video Natal","adset_id":"55","adset_name":"Jaksel","campaign_id":"4444","campaign_name":"Promo Natal","spend":"250000","impressions":"1000","clicks":"50","account_currency":"IDR"},
{"date_start":"` + day + `","ad_id":"778","ad_name":"Foto Natal","adset_id":"55","adset_name":"Jaksel","campaign_id":"4444","campaign_name":"Promo Natal","spend":"100000","impressions":"500","clicks":"20","account_currency":"IDR"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer graph.Close()
	f.s.Syncer.Client.BaseURL, f.s.Syncer.Client.Version = graph.URL, "v23.0"
	enc, err := f.s.Box.SealString("EAAtokeniklanuji")
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, f.db, `INSERT INTO ad_connections (workspace_id, ad_account_id, access_token_enc, access_token_hint) VALUES ($1::uuid, '555555', $2, 'EAAt…uji')`, f.ws.ID, enc)
	if err := f.s.Syncer.SyncWorkspace(ctx, f.ws.ID, 5); err != nil {
		t.Fatalf("sinkron: %v", err)
	}
	if n := countSQL(t, f.db, `SELECT count(*) FROM ad_campaign_map WHERE workspace_id = $1::uuid AND campaign_id = '4444'`, f.ws.ID); n != 2 {
		t.Fatalf("peta iklan->kampanye = %d; want 2", n)
	}
	st := f.a.slots(f.ws.Slug)
	if len(st.Tracked) != 1 || st.Tracked[0].Kind != "campaign" || st.Tracked[0].ID != "4444" {
		t.Fatalf("target setelah sinkron: %+v; want kampanye 4444", st.Tracked)
	}
	if found, tracked := st.candidate("campaign", "4444"); !found || !tracked {
		t.Fatalf("kampanye 4444 harus tampil sebagai dilacak: %+v", st.Candidates)
	}
	if got := f.chat(t, "778"); got != "sent" {
		t.Fatalf("event iklan 778 (kampanye sama) = %q; want sent", got)
	}
	// Halaman Performa memakai data yang sama.
	body := f.a.call("GET", "/api/v1/performance", f.ws.Slug, nil).Body.String()
	if !strings.Contains(body, `"connected":true`) || !strings.Contains(body, "Promo Natal") {
		t.Fatalf("performa setelah sinkron: %s", body)
	}
}
