package server

// Uji multi-workspace: trial hanya untuk workspace pertama, workspace berikutnya wajib dibayar, workspace yang
// belum dibayar hanya bisa dilihat (402 untuk aksi fitur), dan data antar workspace / antar akun tidak bocor.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dicodingin-aja/convera/backend/internal/dispatch"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
)

func TestWorkspaceCreation_TrialOnlyFirst_NewOnesMustBePaid(t *testing.T) {
	s, h := newIntegrationServer(t)
	db := s.DB
	a := newCustomer(t, h, "toko_a")

	first := a.createWS("Toko Pertama")
	if first.Status != "trial" {
		t.Fatalf("workspace pertama = %q; want trial", first.Status)
	}
	second := a.createWS("Toko Kedua")
	if second.Status != "expired" {
		t.Fatalf("workspace kedua = %q; want expired (wajib bayar)", second.Status)
	}

	// Workspace kedua belum dibayar: tidak boleh membuat workspace ketiga.
	w := a.call("POST", "/api/v1/workspaces", "", map[string]any{"name": "Toko Ketiga"})
	if w.Code != http.StatusConflict || errCode(t, w) != "unpaid_workspace" || !strings.Contains(w.Body.String(), "Toko Kedua") {
		t.Fatalf("workspace ketiga saat kedua belum bayar: %d %s; want 409 unpaid_workspace", w.Code, w.Body)
	}

	// Bukti transfer yang masih menunggu verifikasi belum dihitung sebagai bayar.
	if w := a.uploadProof(second.Slug); w.Code != http.StatusCreated {
		t.Fatalf("unggah bukti workspace belum bayar: %d %s", w.Code, w.Body)
	}
	if w := a.call("POST", "/api/v1/workspaces", "", map[string]any{"name": "Toko Ketiga"}); w.Code != http.StatusConflict {
		t.Fatalf("workspace ketiga saat bukti masih pending: %d; want 409", w.Code)
	}

	// Setelah admin mengaktifkan workspace kedua, workspace ketiga boleh dibuat (tetap wajib bayar).
	activateWS(t, db, second.ID)
	third := a.createWS("Toko Ketiga")
	if third.Status != "expired" {
		t.Fatalf("workspace ketiga = %q; want expired", third.Status)
	}
	if w := a.call("POST", "/api/v1/workspaces", "", map[string]any{"name": "Toko Keempat"}); w.Code != http.StatusConflict {
		t.Fatalf("workspace keempat saat ketiga belum bayar: %d; want 409", w.Code)
	}

	// Menghapus workspace yang belum dibayar membuka lagi pembuatan workspace (juga tanpa trial).
	if w := a.call("DELETE", "/api/v1/workspaces/"+third.Slug, "", map[string]any{"confirm": third.Slug}); w.Code != http.StatusOK {
		t.Fatalf("hapus workspace ketiga: %d %s", w.Code, w.Body)
	}
	if fourth := a.createWS("Toko Keempat"); fourth.Status != "expired" {
		t.Fatalf("workspace keempat = %q; want expired", fourth.Status)
	}

	// Menghapus workspace trial lalu membuat lagi tidak memberi trial kedua.
	b := newCustomer(t, h, "toko_b")
	bt := b.createWS("Trial B")
	if w := b.call("DELETE", "/api/v1/workspaces/"+bt.Slug, "", map[string]any{"confirm": bt.Slug}); w.Code != http.StatusOK {
		t.Fatalf("hapus workspace trial: %d %s", w.Code, w.Body)
	}
	if again := b.createWS("Trial B lagi"); again.Status != "expired" {
		t.Fatalf("workspace setelah trial dihapus = %q; want expired", again.Status)
	}

	// Trial yang habis tanpa bayar juga dihitung belum dibayar.
	c := newCustomer(t, h, "toko_c")
	ct := c.createWS("Trial C")
	execSQL(t, db, `UPDATE subscriptions SET trial_until = now() - interval '1 day' WHERE workspace_id = $1::uuid`, ct.ID)
	if w := c.call("POST", "/api/v1/workspaces", "", map[string]any{"name": "C kedua"}); w.Code != http.StatusConflict {
		t.Fatalf("workspace baru saat trial habis tanpa bayar: %d; want 409", w.Code)
	}
}

func TestWorkspaceCreation_ConcurrentRequestsGetOneTrial(t *testing.T) {
	s, h := newIntegrationServer(t)
	a := newCustomer(t, h, "toko_paralel")
	const n = 5
	codes := make(chan int, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/v1/workspaces", strings.NewReader(`{"name":"Paralel"}`))
			req.RemoteAddr = a.ip + ":40000"
			req.Header.Set("Content-Type", "application/json")
			for k, v := range a.cookies {
				req.AddCookie(&http.Cookie{Name: k, Value: v})
			}
			req.Header.Set("X-CSRF-Token", a.cookies[csrfCookie])
			h.ServeHTTP(w, req)
			codes <- w.Code
		}(i)
	}
	created := 0
	for i := 0; i < n; i++ {
		if <-codes == http.StatusCreated {
			created++
		}
	}
	// Satu workspace trial + satu workspace belum bayar; sisanya ditolak.
	if created != 2 {
		t.Fatalf("workspace dibuat bersamaan = %d; want 2", created)
	}
	trials := countSQL(t, s.DB, `SELECT count(*) FROM subscriptions sub JOIN workspaces w ON w.id = sub.workspace_id
		JOIN users u ON u.id = w.owner_user_id WHERE u.username = 'toko_paralel' AND sub.trial_until IS NOT NULL`)
	if trials != 1 {
		t.Fatalf("jumlah trial = %d; want 1", trials)
	}
}

func TestUnpaidWorkspace_IsViewOnly(t *testing.T) {
	s, h := newIntegrationServer(t)
	db := s.DB
	a := newCustomer(t, h, "pemilik")
	trial := a.createWS("Trial")
	unpaid := a.createWS("Belum Bayar")

	metaBody := map[string]any{"pixel_id": "123456789", "access_token": "EAAtokenrahasia"}
	ruleBody := map[string]any{"name": "Beli", "who": "customer", "keywords": []string{"transfer"}, "event_name": "Purchase", "value_mode": "parse"}
	feature := []struct {
		method, path string
		body         any
	}{
		{"PUT", "/api/v1/connections/meta", metaBody},
		{"POST", "/api/v1/connections/meta/test", map[string]any{}},
		{"POST", "/api/v1/connections/whatsapp/qr", nil},
		{"POST", "/api/v1/event-rules", ruleBody},
		{"POST", "/api/v1/event-mappings", ruleBody},
		{"POST", "/api/v1/api-keys", map[string]any{"label": "CRM"}},
		{"PUT", "/api/v1/connections/ads", map[string]any{}},
		{"POST", "/api/v1/leads/bulk-status", map[string]any{"ids": []string{}, "status": "lead"}},
	}
	for _, f := range feature {
		if w := a.call(f.method, f.path, unpaid.Slug, f.body); w.Code != http.StatusPaymentRequired || errCode(t, w) != "subscription_required" {
			t.Errorf("%s %s di workspace belum bayar = %d %s; want 402 subscription_required", f.method, f.path, w.Code, w.Body)
		}
	}
	// Lihat data dan bayar tetap boleh.
	for _, p := range []string{"/api/v1/subscription", "/api/v1/leads", "/api/v1/events", "/api/v1/connections/meta",
		"/api/v1/event-rules", "/api/v1/overview", "/api/v1/bank-accounts", "/api/v1/payment-proofs", "/api/v1/api-keys"} {
		if w := a.call("GET", p, unpaid.Slug, nil); w.Code != http.StatusOK {
			t.Errorf("GET %s di workspace belum bayar = %d %s; want 200", p, w.Code, w.Body)
		}
	}
	if w := a.call("PATCH", "/api/v1/workspaces/"+unpaid.Slug, "", map[string]any{"name": "Belum Bayar (ganti)"}); w.Code != http.StatusOK {
		t.Errorf("ubah nama workspace belum bayar = %d %s; want 200", w.Code, w.Body)
	}

	// Workspace trial di akun yang sama tetap bisa dipakai.
	if w := a.call("PUT", "/api/v1/connections/meta", trial.Slug, metaBody); w.Code != http.StatusOK {
		t.Fatalf("simpan koneksi di workspace trial = %d %s", w.Code, w.Body)
	}
	if w := a.call("POST", "/api/v1/event-rules", trial.Slug, ruleBody); w.Code != http.StatusCreated {
		t.Fatalf("buat aturan di workspace trial = %d %s", w.Code, w.Body)
	}

	// Setelah dibayar, workspace kedua terbuka.
	activateWS(t, db, unpaid.ID)
	if w := a.call("PUT", "/api/v1/connections/meta", unpaid.Slug, metaBody); w.Code != http.StatusOK {
		t.Fatalf("simpan koneksi setelah dibayar = %d %s", w.Code, w.Body)
	}

	// Trial habis -> terkunci lagi.
	execSQL(t, db, `UPDATE subscriptions SET trial_until = now() - interval '1 minute' WHERE workspace_id = $1::uuid`, trial.ID)
	if w := a.call("POST", "/api/v1/event-rules", trial.Slug, ruleBody); w.Code != http.StatusPaymentRequired {
		t.Fatalf("buat aturan setelah trial habis = %d; want 402", w.Code)
	}

	// Langganan aktif yang lewat masa tenggang -> terkunci; masih dalam masa tenggang -> terbuka.
	execSQL(t, db, `UPDATE subscriptions SET current_period_end = now() - interval '1 day' WHERE workspace_id = $1::uuid`, unpaid.ID)
	if w := a.call("POST", "/api/v1/event-rules", unpaid.Slug, ruleBody); w.Code != http.StatusCreated {
		t.Fatalf("buat aturan dalam masa tenggang = %d %s; want 201", w.Code, w.Body)
	}
	execSQL(t, db, `UPDATE subscriptions SET current_period_end = now() - interval '30 days' WHERE workspace_id = $1::uuid`, unpaid.ID)
	if w := a.call("POST", "/api/v1/event-rules", unpaid.Slug, ruleBody); w.Code != http.StatusPaymentRequired {
		t.Fatalf("buat aturan setelah masa tenggang = %d; want 402", w.Code)
	}

	// Dihentikan admin -> terkunci walau periode masih panjang.
	execSQL(t, db, `UPDATE subscriptions SET status = 'stopped', current_period_end = now() + interval '20 days' WHERE workspace_id = $1::uuid`, unpaid.ID)
	if w := a.call("POST", "/api/v1/event-rules", unpaid.Slug, ruleBody); w.Code != http.StatusPaymentRequired {
		t.Fatalf("buat aturan setelah dihentikan = %d; want 402", w.Code)
	}
}

func TestTrialWorkspace_PayingEarlyKeepsAccess(t *testing.T) {
	_, h := newIntegrationServer(t)
	a := newCustomer(t, h, "bayar_awal")
	trial := a.createWS("Trial")
	if w := a.uploadProof(trial.Slug); w.Code != http.StatusCreated {
		t.Fatalf("unggah bukti saat trial: %d %s", w.Code, w.Body)
	}
	var sub struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	decode(t, a.call("GET", "/api/v1/subscription", trial.Slug, nil), &sub)
	if sub.Data.Status != "pending" {
		t.Fatalf("status setelah unggah bukti = %q; want pending", sub.Data.Status)
	}
	// Trial masih berjalan: bukti yang menunggu verifikasi tidak boleh mengunci workspace.
	if w := a.call("PUT", "/api/v1/connections/meta", trial.Slug, map[string]any{"pixel_id": "123456789"}); w.Code != http.StatusOK {
		t.Fatalf("simpan koneksi saat trial + bukti pending = %d %s; want 200", w.Code, w.Body)
	}
}

func TestUnpaidWorkspace_IgnoresIncomingDataAndPublicAPI(t *testing.T) {
	s, h := newIntegrationServer(t)
	db := s.DB
	a := newCustomer(t, h, "masuk")
	paid := a.createWS("Aktif")
	unpaid := a.createWS("Belum Bayar")
	// Lead hanya dibuat bila pesan cocok aturan. Aturan workspace belum bayar dipasang langsung di database
	// (lewat API ditolak 402) agar uji benar-benar membuktikan pesan diabaikan karena langganan.
	if w := a.call("POST", "/api/v1/event-rules", paid.Slug, map[string]any{"name": "Beli", "who": "customer", "keywords": []string{"transfer"}, "event_name": "LeadSubmitted"}); w.Code != http.StatusCreated {
		t.Fatalf("aturan workspace aktif: %d %s", w.Code, w.Body)
	}
	execSQL(t, db, `INSERT INTO event_rules (workspace_id, name, who, op, keywords, event_name, value_mode, active, sort)
		VALUES ($1::uuid, 'Beli', 'customer', 'contains', ARRAY['transfer'], 'LeadSubmitted', 'none', true, 0)`, unpaid.ID)

	// Pesan WhatsApp (layanan QR) ke workspace belum bayar: diabaikan, tidak ada lead.
	w := qrMessageIn(t, h, unpaid.ID, "MSG-UNPAID-1", "081234500001", "sudah transfer kak", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"processed":false`) {
		t.Fatalf("pesan ke workspace belum bayar: %d %s; want 200 processed=false", w.Code, w.Body)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM leads WHERE workspace_id = $1::uuid`, unpaid.ID); n != 0 {
		t.Fatalf("lead di workspace belum bayar = %d; want 0", n)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM lead_messages m JOIN leads l ON l.id = m.lead_id WHERE l.workspace_id = $1::uuid`, unpaid.ID); n != 0 {
		t.Fatalf("pesan tersimpan di workspace belum bayar = %d; want 0", n)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM events WHERE workspace_id = $1::uuid`, unpaid.ID); n != 0 {
		t.Fatalf("event di workspace belum bayar = %d; want 0", n)
	}
	// Workspace aktif tetap menerima.
	if w := qrMessageIn(t, h, paid.ID, "MSG-PAID-1", "081234500001", "sudah transfer kak", nil); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan ke workspace aktif: %d %s; want processed=true", w.Code, w.Body)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM leads WHERE workspace_id = $1::uuid`, paid.ID); n != 1 {
		t.Fatalf("lead di workspace aktif = %d; want 1", n)
	}
	// Setelah dibayar, pesan berikutnya diproses.
	activateWS(t, db, unpaid.ID)
	if w := qrMessageIn(t, h, unpaid.ID, "MSG-UNPAID-2", "081234500001", "transfer lagi", nil); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan setelah dibayar: %d %s; want processed=true", w.Code, w.Body)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM leads WHERE workspace_id = $1::uuid`, unpaid.ID); n != 1 {
		t.Fatalf("lead setelah dibayar = %d; want 1", n)
	}

	// API publik (API key) berhenti saat langganan berakhir.
	var key struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	kw := a.call("POST", "/api/v1/api-keys", paid.Slug, map[string]any{"label": "CRM"})
	if kw.Code != http.StatusCreated {
		t.Fatalf("buat API key: %d %s", kw.Code, kw.Body)
	}
	decode(t, kw, &key)
	publicLead := func() int {
		req := httptest.NewRequest("POST", "/v1/leads", strings.NewReader(`{"phone":"081299990000","name":"Budi"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key.Data.Key)
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, req)
		return rw.Code
	}
	if code := publicLead(); code != http.StatusCreated {
		t.Fatalf("POST /v1/leads saat trial = %d; want 201", code)
	}
	execSQL(t, db, `UPDATE subscriptions SET trial_until = now() - interval '1 minute' WHERE workspace_id = $1::uuid`, paid.ID)
	if code := publicLead(); code != http.StatusPaymentRequired {
		t.Fatalf("POST /v1/leads setelah trial habis = %d; want 402", code)
	}
}

func TestDispatcher_SkipsEventsOfUnpaidWorkspace(t *testing.T) {
	s, h := newIntegrationServer(t)
	db := s.DB
	var calls atomic.Int32
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events_received":1,"fbtrace_id":"TRACE"}`))
	}))
	defer graph.Close()

	a := newCustomer(t, h, "kirim")
	ws := a.createWS("Kirim")
	if w := a.call("PUT", "/api/v1/connections/meta", ws.Slug, map[string]any{"pixel_id": "123456789", "access_token": "EAAtokenrahasia"}); w.Code != http.StatusOK {
		t.Fatalf("simpan koneksi: %d %s", w.Code, w.Body)
	}
	rule := map[string]any{"name": "Lead", "who": "customer", "keywords": []string{"minat"}, "event_name": "LeadSubmitted"}
	if w := a.call("POST", "/api/v1/event-rules", ws.Slug, rule); w.Code != http.StatusCreated {
		t.Fatalf("buat aturan: %d %s", w.Code, w.Body)
	}
	ad := map[string]any{"ad_id": "120200000000001", "click_id": "CLICK-1", "headline": "Promo"}
	if w := qrMessageIn(t, h, ws.ID, "MSG-EV-1", "081234500077", "saya minat kak", ad); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan pemicu event: %d %s", w.Code, w.Body)
	}
	if w := qrMessageIn(t, h, ws.ID, "MSG-EV-2", "081234500078", "minat juga", map[string]any{"ad_id": "120200000000001", "click_id": "CLICK-2"}); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan pemicu event kedua: %d %s", w.Code, w.Body)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM events WHERE workspace_id = $1::uuid AND status = 'queued'`, ws.ID); n != 2 {
		t.Fatalf("event antre = %d; want 2", n)
	}

	d := &dispatch.Dispatcher{DB: db, Box: s.Box, Meta: &meta.Client{BaseURL: graph.URL, Version: "v23.0"}, Batch: 1}
	// Event pertama dikirim selagi trial berjalan.
	if _, err := d.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("panggilan ke Meta saat trial = %d; want 1", calls.Load())
	}
	// Trial habis sebelum event kedua dikirim: event dilewati, Meta tidak dipanggil.
	execSQL(t, db, `UPDATE subscriptions SET trial_until = now() - interval '1 minute' WHERE workspace_id = $1::uuid`, ws.ID)
	if _, err := d.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("panggilan ke Meta setelah trial habis = %d; want tetap 1", calls.Load())
	}
	if n := countSQL(t, db, `SELECT count(*) FROM events WHERE workspace_id = $1::uuid AND status = 'skipped' AND error_code = 'subscription_inactive'`, ws.ID); n != 1 {
		t.Fatalf("event dilewati karena langganan = %d; want 1", n)
	}
}

func TestMultiWorkspace_DataIsolation(t *testing.T) {
	s, h := newIntegrationServer(t)
	db := s.DB
	a := newCustomer(t, h, "agensi")
	a1 := a.createWS("Klien Satu")
	a2 := a.createWS("Klien Dua")
	activateWS(t, db, a2.ID)
	b := newCustomer(t, h, "orang_lain")
	b1 := b.createWS("Punya B")

	// Tanpa X-Workspace saat punya dua workspace: wajib pilih.
	if w := a.call("GET", "/api/v1/leads", "", nil); w.Code != http.StatusBadRequest || errCode(t, w) != "workspace_required" {
		t.Fatalf("tanpa X-Workspace (2 workspace) = %d %s; want 400 workspace_required", w.Code, w.Body)
	}

	// Koneksi Meta per workspace terpisah.
	if w := a.call("PUT", "/api/v1/connections/meta", a1.Slug, map[string]any{"pixel_id": "1111111111", "access_token": "EAAsatu"}); w.Code != http.StatusOK {
		t.Fatalf("koneksi a1: %d %s", w.Code, w.Body)
	}
	if w := a.call("PUT", "/api/v1/connections/meta", a2.Slug, map[string]any{"pixel_id": "2222222222", "access_token": "EAAdua"}); w.Code != http.StatusOK {
		t.Fatalf("koneksi a2: %d %s", w.Code, w.Body)
	}
	if body := a.call("GET", "/api/v1/connections/meta", a2.Slug, nil).Body.String(); !strings.Contains(body, "2222222222") || strings.Contains(body, "1111111111") {
		t.Fatalf("koneksi a2 tercampur: %s", body)
	}
	for _, body := range []string{a.call("GET", "/api/v1/connections/meta", a1.Slug, nil).Body.String(), a.call("GET", "/api/v1/connections/meta", a2.Slug, nil).Body.String()} {
		if strings.Contains(body, "EAAsatu") || strings.Contains(body, "EAAdua") {
			t.Fatalf("token Meta bocor di respons: %s", body)
		}
	}

	// Aturan per workspace.
	rule := map[string]any{"name": "Transfer", "who": "customer", "keywords": []string{"transfer"}, "event_name": "Purchase", "value_mode": "parse"}
	if w := a.call("POST", "/api/v1/event-rules", a1.Slug, rule); w.Code != http.StatusCreated {
		t.Fatalf("aturan a1: %d %s", w.Code, w.Body)
	}
	if body := a.call("GET", "/api/v1/event-rules", a2.Slug, nil).Body.String(); strings.Contains(body, "Transfer") {
		t.Fatalf("aturan a1 terlihat di a2: %s", body)
	}
	if w := a.call("POST", "/api/v1/event-rules", a2.Slug, map[string]any{"name": "Bayar", "who": "customer", "keywords": []string{"transfer"}, "event_name": "Purchase", "value_mode": "parse"}); w.Code != http.StatusCreated {
		t.Fatalf("aturan a2: %d %s", w.Code, w.Body)
	}

	// Pesan ke a1 hanya membuat lead di a1.
	if w := qrMessageIn(t, h, a1.ID, "MSG-ISO-1", "081300000001", "sudah transfer 150rb", nil); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan a1: %d %s", w.Code, w.Body)
	}
	var leadID string
	if err := db.QueryRow(context.Background(), `SELECT id::text FROM leads WHERE workspace_id = $1::uuid`, a1.ID).Scan(&leadID); err != nil {
		t.Fatal(err)
	}
	if body := a.call("GET", "/api/v1/leads", a2.Slug, nil).Body.String(); strings.Contains(body, leadID) || strings.Contains(body, "081300000001") || strings.Contains(body, "6281300000001") {
		t.Fatalf("lead a1 terlihat di a2: %s", body)
	}
	if w := a.call("GET", "/api/v1/leads/"+leadID, a2.Slug, nil); w.Code != http.StatusNotFound {
		t.Fatalf("detail lead a1 lewat a2 = %d; want 404", w.Code)
	}
	if w := a.call("DELETE", "/api/v1/leads/"+leadID, a2.Slug, nil); w.Code != http.StatusNotFound {
		t.Fatalf("hapus lead a1 lewat a2 = %d; want 404", w.Code)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM leads WHERE id = $1::uuid AND deleted_at IS NULL`, leadID); n != 1 {
		t.Fatal("lead a1 terhapus lewat workspace lain")
	}

	// Akun lain tidak bisa memakai workspace a1 (404, bukan 403).
	for _, req := range []struct{ method, path string }{
		{"GET", "/api/v1/leads"}, {"GET", "/api/v1/leads/" + leadID}, {"GET", "/api/v1/connections/meta"},
		{"GET", "/api/v1/subscription"}, {"PUT", "/api/v1/connections/meta"}, {"DELETE", "/api/v1/leads/" + leadID},
	} {
		var body any
		if req.method == "PUT" {
			body = map[string]any{"pixel_id": "999999999"}
		}
		if w := b.call(req.method, req.path, a1.Slug, body); w.Code != http.StatusNotFound {
			t.Errorf("akun B %s %s di workspace A = %d %s; want 404", req.method, req.path, w.Code, w.Body)
		}
	}
	if w := b.call("GET", "/api/v1/workspaces/"+a1.Slug, "", nil); w.Code != http.StatusNotFound {
		t.Errorf("akun B baca workspace A = %d; want 404", w.Code)
	}
	if w := b.call("DELETE", "/api/v1/workspaces/"+a1.Slug, "", map[string]any{"confirm": a1.Slug}); w.Code != http.StatusNotFound {
		t.Errorf("akun B hapus workspace A = %d; want 404", w.Code)
	}
	if w := b.uploadProof(a1.Slug); w.Code != http.StatusNotFound {
		t.Errorf("akun B unggah bukti ke workspace A = %d; want 404", w.Code)
	}
	var list struct {
		Data []struct {
			Slug string `json:"slug"`
		} `json:"data"`
	}
	decode(t, b.call("GET", "/api/v1/workspaces", "", nil), &list)
	if len(list.Data) != 1 || list.Data[0].Slug != b1.Slug {
		t.Errorf("daftar workspace B = %+v; want hanya %s", list.Data, b1.Slug)
	}

	// API key a1 tidak bisa membuat event untuk lead milik a2.
	if w := qrMessageIn(t, h, a2.ID, "MSG-ISO-2", "081300000002", "sudah transfer 150rb", nil); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Fatalf("pesan a2: %d %s", w.Code, w.Body)
	}
	var key struct {
		Data struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	decode(t, a.call("POST", "/api/v1/api-keys", a1.Slug, map[string]any{"label": "CRM"}), &key)
	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{"event_name":"Purchase","phone":"081300000002","value":1000}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key.Data.Key)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusNotFound {
		t.Errorf("API key a1 membuat event untuk lead a2 = %d %s; want 404", rw.Code, rw.Body)
	}

	// Nomor WhatsApp yang sama boleh menjadi lead di dua workspace berbeda, dan ID pesan yang sama di dua
	// workspace tidak saling menahan (dedupe hanya di dalam satu workspace).
	if w := qrMessageIn(t, h, a2.ID, "MSG-ISO-1", "081300000001", "sudah transfer 150rb", nil); !strings.Contains(w.Body.String(), `"processed":true`) {
		t.Errorf("ID pesan yang sama di workspace lain tidak diproses: %d %s", w.Code, w.Body)
	}
	if n := countSQL(t, db, `SELECT count(*) FROM leads WHERE phone = '6281300000001' AND deleted_at IS NULL`); n != 2 {
		t.Errorf("lead nomor yang sama di dua workspace = %d; want 2", n)
	}
	// Pesan ganda di workspace yang sama tetap ditolak.
	if w := qrMessageIn(t, h, a2.ID, "MSG-ISO-1", "081300000001", "sudah transfer 150rb", nil); strings.Contains(w.Body.String(), `"processed":true`) {
		t.Errorf("pesan ganda di workspace yang sama diproses dua kali: %s", w.Body)
	}

	// Satu perangkat WhatsApp tidak boleh tertaut ke dua workspace.
	execSQL(t, db, `INSERT INTO wa_sessions (workspace_id, status, device_jid) VALUES ($1::uuid, 'connected', '628111@s.whatsapp.net')`, a1.ID)
	if _, err := db.Exec(context.Background(), `INSERT INTO wa_sessions (workspace_id, status, device_jid) VALUES ($1::uuid, 'connected', '628111@s.whatsapp.net')`, a2.ID); err == nil || !isUnique(err) {
		t.Errorf("perangkat WA yang sama tertaut ke dua workspace: err=%v; want unique violation", err)
	}
}

// Pastikan pola JSON yang diuji di atas memang ada di respons (menjaga agar uji tidak lulus karena salah format).
func TestQRIngestResponseShape(t *testing.T) {
	_, h := newIntegrationServer(t)
	a := newCustomer(t, h, "bentuk")
	ws := a.createWS("Bentuk")
	w := qrMessageIn(t, h, ws.ID, "MSG-SHAPE", "081200000009", "halo", nil)
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Data["processed"] != true {
		t.Fatalf("bentuk respons ingest: %d %s", w.Code, w.Body)
	}
}
