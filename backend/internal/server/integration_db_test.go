package server

// Penyiapan uji integrasi ke PostgreSQL sungguhan.
//
// Database uji diambil dari TEST_DATABASE_URL, atau dari DATABASE_URL di backend/.env dengan nama database
// ditambah akhiran "_test" (mis. db_convera -> db_convera_test). Database dibuat bila belum ada lalu
// dimigrasi. Nama yang tidak berakhiran _test ditolak agar data development tidak terhapus.
// Bila tidak ada URL sama sekali, uji integrasi dilewati (go test tetap lulus tanpa database).

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/migrations"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

var (
	itOnce sync.Once
	itPool *pgxpool.Pool
	itSkip string
	itErr  error
)

func testDatabaseURL() (string, error) {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u, nil
	}
	config.LoadDotEnv("../../.env")
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		return "", nil
	}
	u, err := neturl.Parse(base)
	if err != nil {
		return "", fmt.Errorf("DATABASE_URL tidak valid: %w", err)
	}
	u.Path = "/" + strings.TrimPrefix(u.Path, "/") + "_test"
	return u.String(), nil
}

func setupIntegrationDB() {
	ctx := context.Background()
	raw, err := testDatabaseURL()
	if err != nil {
		itErr = err
		return
	}
	if raw == "" {
		itSkip = "uji integrasi dilewati: isi TEST_DATABASE_URL atau DATABASE_URL di backend/.env"
		return
	}
	u, err := neturl.Parse(raw)
	if err != nil {
		itErr = err
		return
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !strings.HasSuffix(name, "_test") {
		itErr = fmt.Errorf("database uji harus berakhiran _test, bukan %q", name)
		return
	}
	admin := *u
	admin.Path = "/postgres"
	conn, err := pgx.Connect(ctx, admin.String())
	if err != nil {
		itErr = fmt.Errorf("tidak bisa terhubung ke PostgreSQL: %w", err)
		return
	}
	var exists bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err == nil && !exists {
		_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	}
	conn.Close(ctx)
	if err != nil {
		itErr = err
		return
	}

	sqldb, err := sql.Open("pgx", raw)
	if err != nil {
		itErr = err
		return
	}
	defer sqldb.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err = goose.SetDialect("postgres"); err == nil {
		err = goose.Up(sqldb, ".")
	}
	if err != nil {
		itErr = fmt.Errorf("migrasi database uji: %w", err)
		return
	}
	itPool, itErr = pgxpool.New(ctx, raw)
}

// integrationDB mengembalikan pool ke database uji yang sudah dikosongkan dari data akun dan workspace.
// Data awal dari migrasi (paket, pengaturan, sales, rekening) tetap ada.
func integrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	itOnce.Do(setupIntegrationDB)
	if itSkip != "" {
		t.Skip(itSkip)
	}
	if itErr != nil {
		t.Fatal(itErr)
	}
	if _, err := itPool.Exec(context.Background(), `TRUNCATE users, admins, webhook_inbox CASCADE`); err != nil {
		t.Fatal(err)
	}
	return itPool
}

const testServiceToken = "svc-test-token"

// newIntegrationServer membuat Server dengan database uji dan kunci enkripsi acak.
func newIntegrationServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := integrationDB(t)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, err := secure.NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{WebhookBase: "https://api.test/webhook", QRServiceToken: testServiceToken, UploadDir: t.TempDir()}
	s := New(cfg, db, box)
	return s, s.Router()
}

// ---------- Klien customer ----------

type customer struct {
	t        *testing.T
	h        http.Handler
	ip       string
	cookies  map[string]string
	csrfName string // nama cookie CSRF; kosong = cookie customer
}

var ipSeq struct {
	sync.Mutex
	n int
}

// newCustomer mendaftarkan akun baru. Setiap akun memakai IP berbeda agar tidak terkena rate limit daftar.
func newCustomer(t *testing.T, h http.Handler, username string) *customer {
	t.Helper()
	ipSeq.Lock()
	ipSeq.n++
	ip := fmt.Sprintf("10.20.%d.%d", ipSeq.n/250, ipSeq.n%250+1)
	ipSeq.Unlock()
	c := &customer{t: t, h: h, ip: ip, cookies: map[string]string{}}
	w := c.call("POST", "/api/v1/auth/register", "", map[string]any{
		"username": username, "name": "Pemilik " + username, "email": username + "@contoh.test",
		"phone": "0812" + fmt.Sprintf("%08d", ipSeq.n), "password": "rahasia-123",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("daftar %s: %d %s", username, w.Code, w.Body)
	}
	return c
}

// call mengirim request JSON (body bisa nil) dengan cookie sesi, token CSRF, dan header X-Workspace bila ws diisi.
func (c *customer) call(method, path, ws string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = c.ip + ":40000"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req, ws)
}

func (c *customer) send(req *http.Request, ws string) *httptest.ResponseRecorder {
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	name := c.csrfName
	if name == "" {
		name = csrfCookie
	}
	if v := c.cookies[name]; v != "" {
		req.Header.Set("X-CSRF-Token", v)
	}
	if ws != "" {
		req.Header.Set("X-Workspace", ws)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	for _, ck := range w.Result().Cookies() {
		c.cookies[ck.Name] = ck.Value
	}
	return w
}

type createdWS struct {
	ID, Slug, Status string
}

// createWS membuat workspace dan mengharapkan 201.
func (c *customer) createWS(name string) createdWS {
	c.t.Helper()
	w := c.call("POST", "/api/v1/workspaces", "", map[string]any{"name": name})
	if w.Code != http.StatusCreated {
		c.t.Fatalf("buat workspace %q: %d %s", name, w.Code, w.Body)
	}
	var out struct {
		Data struct {
			ID     string `json:"id"`
			Slug   string `json:"slug"`
			Status string `json:"subscription_status"`
		} `json:"data"`
	}
	decode(c.t, w, &out)
	return createdWS{ID: out.Data.ID, Slug: out.Data.Slug, Status: out.Data.Status}
}

// uploadProof mengunggah bukti transfer PNG kecil untuk workspace ws (langganan 1 bulan, 1 slot).
func (c *customer) uploadProof(ws string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.uploadProofWith(ws, nil)
}

// uploadProofWith: seperti uploadProof, dengan isian tambahan/pengganti (mis. months, ad_slots, purchase).
func (c *customer) uploadProofWith(ws string, extra map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields := map[string]string{"plan": "workspace", "months": "1", "bank": "BCA", "sender": "Pemilik"}
	for k, v := range extra {
		fields[k] = v
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", "bukti.png")
	// PNG 1x1 yang valid.
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")
	_, _ = fw.Write(png)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/payment-proofs", &buf)
	req.RemoteAddr = c.ip + ":40000"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.send(req, ws)
}

func decode(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("respons bukan JSON: %v: %s", err, w.Body)
	}
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	return e.Code
}

// ---------- Bantuan data ----------

// activateWS meniru aktivasi admin (adminapi activate): catat pembayaran 1 bulan lalu jadikan langganan aktif.
func activateWS(t *testing.T, db *pgxpool.Pool, workspaceID string) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
WITH sub AS (SELECT id, workspace_id FROM subscriptions WHERE workspace_id = $1::uuid),
pay AS (
  INSERT INTO payments (workspace_id, subscription_id, plan_id, amount, months, period_start, period_end, paid_at, sales_id, komisi_pct, kas_pct)
  SELECT sub.workspace_id, sub.id, (SELECT id FROM plans WHERE code = 'workspace'), 150000, 1, now(), now() + interval '1 month', now(),
         (SELECT id FROM sales_people ORDER BY created_at LIMIT 1), 10, 10 FROM sub
  RETURNING period_end)
UPDATE subscriptions SET status = 'active', plan_id = (SELECT id FROM plans WHERE code = 'workspace'),
  current_period_end = (SELECT period_end FROM pay) WHERE workspace_id = $1::uuid`, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, db *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}

func countSQL(t *testing.T, db *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// qrMessageIn mengirim pesan masuk lewat endpoint layanan QR untuk workspace tertentu.
func qrMessageIn(t *testing.T, h http.Handler, workspaceID, msgID, from, text string, ad map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{"id": msgID, "direction": "in", "contact": from, "push_name": "Pelanggan", "text": text}
	if ad != nil {
		body["ad"] = ad
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/internal/qr/"+workspaceID+"/messages", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Token", testServiceToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
