package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/gin-gonic/gin"
)

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	// DB nil: hanya route/middleware yang menolak sebelum menyentuh database yang diuji di sini.
	s := New(&config.Config{AllowedOrigins: []string{"http://localhost:5174"}}, nil, nil)
	return s.Router()
}

func do(r http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAuthRequired(t *testing.T) {
	r := testRouter()
	for _, p := range []string{"/api/v1/leads", "/api/v1/events", "/api/v1/connections/meta", "/api/v1/api-keys", "/api/v1/overview"} {
		if w := do(r, "GET", p, "", nil); w.Code != 401 {
			t.Errorf("GET %s tanpa sesi = %d; want 401", p, w.Code)
		}
	}
	if w := do(r, "POST", "/v1/events", `{}`, nil); w.Code != 401 {
		t.Errorf("POST /v1/events tanpa API key = %d; want 401", w.Code)
	}
	if w := do(r, "POST", "/internal/qr/x/messages", `{}`, nil); w.Code != 401 {
		t.Errorf("/internal/qr tanpa service token = %d; want 401", w.Code)
	}
}

func TestCORS(t *testing.T) {
	r := testRouter()
	w := do(r, "OPTIONS", "/api/v1/auth/login", "", map[string]string{"Origin": "http://localhost:5174"})
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5174" ||
		w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("CORS origin diizinkan: %d %v", w.Code, w.Header())
	}
	w = do(r, "OPTIONS", "/api/v1/auth/login", "", map[string]string{"Origin": "https://evil.example"})
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("origin asing tidak boleh diizinkan")
	}
}

func TestResponseShape(t *testing.T) {
	w := do(testRouter(), "GET", "/api/v1/leads", "", nil)
	if !strings.Contains(w.Body.String(), `"message"`) || !strings.Contains(w.Body.String(), `"code":"unauthenticated"`) {
		t.Errorf("format error harus {message, code}: %s", w.Body.String())
	}
}

func TestWhereBuilder(t *testing.T) {
	w := &where{}
	w.add("a = ?", 1)
	w.add("b >= ?", "x")
	if w.sql() != "a = $1 AND b >= $2" || len(w.args) != 2 {
		t.Errorf("where: %q %v", w.sql(), w.args)
	}
}

func TestHashKeyStable(t *testing.T) {
	if string(hashKey("cvr_abc")) != string(hashKey("cvr_abc")) || string(hashKey("a")) == string(hashKey("b")) {
		t.Error("hashKey")
	}
}

func TestUsernameRule(t *testing.T) {
	good := []string{"abc", "toko.kopi", "a_b-c", "0abc", "a23456789012345678901234567890"}
	bad := []string{"", "ab", "Abc", "_abc", ".abc", "a b", "a@b.com", "a234567890123456789012345678901"}
	for _, u := range good {
		if !usernameRe.MatchString(u) {
			t.Errorf("%q seharusnya valid", u)
		}
	}
	for _, u := range bad {
		if usernameRe.MatchString(u) {
			t.Errorf("%q seharusnya ditolak", u)
		}
	}
}

func TestEmailFlowsRemoved(t *testing.T) {
	r := testRouter()
	for _, p := range []string{"/api/v1/auth/forgot-password", "/api/v1/auth/reset-password", "/api/v1/auth/verify-email", "/api/v1/auth/resend-verification"} {
		if w := do(r, "POST", p, `{}`, nil); w.Code != 404 {
			t.Errorf("POST %s = %d; want 404", p, w.Code)
		}
	}
}
