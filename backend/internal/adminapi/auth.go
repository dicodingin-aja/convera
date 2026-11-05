package adminapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "cv_admin"
	csrfCookie    = "csrf_admin"
	sessionTTL    = 12 * time.Hour
)

// Hash palsu agar login username yang tidak ada memakan waktu yang sama dengan yang ada.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("convera-admin-dummy"), bcrypt.DefaultCost)

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (s *Server) login(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	user := strings.ToLower(strings.TrimSpace(in.Username))
	// Batasi percobaan: per IP (10/menit) dan per username (5/menit).
	if !s.lim.Get("ip|"+c.ClientIP(), 10.0/60, 10).Allow() || !s.lim.Get("user|"+user, 5.0/60, 5).Allow() {
		httpx.Fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan. Coba lagi sebentar.")
		return
	}
	var id, hash, name, role, status string
	err := s.DB.QueryRow(c, `SELECT id::text, password_hash, name, role, status FROM admins WHERE username = $1`, user).
		Scan(&id, &hash, &name, &role, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		httpx.Fail(c, http.StatusUnauthorized, "invalid_credentials", "Username atau password salah.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil || status != "active" {
		httpx.Fail(c, http.StatusUnauthorized, "invalid_credentials", "Username atau password salah.")
		return
	}
	token, e1 := secure.RandomToken(32)
	csrf, e2 := secure.RandomToken(16)
	if e1 != nil || e2 != nil {
		httpx.ServerError(c, errors.Join(e1, e2))
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)
	if _, err := tx.Exec(c, `INSERT INTO sessions (admin_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1,$2,$3,NULLIF($4,'')::inet,NULLIF($5,''))`, id, secure.HashToken(token), time.Now().Add(sessionTTL), c.ClientIP(), c.Request.UserAgent()); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if _, err := tx.Exec(c, `UPDATE admins SET last_login_at = now() WHERE id = $1`, id); err != nil {
		httpx.ServerError(c, err)
		return
	}
	c.Set("admin_id", id)
	c.Set("admin_name", user)
	if err := s.audit(c, tx, "admin.login", "", "admin", id, nil); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	max := int(sessionTTL.Seconds())
	c.SetSameSite(http.SameSiteStrictMode) // admin dan API satu host: tidak perlu cookie lintas situs
	c.SetCookie(sessionCookie, token, max, "/", "", s.Cfg.CookieSecure, true)
	c.SetCookie(csrfCookie, csrf, max, "/", "", s.Cfg.CookieSecure, false)
	httpx.OK(c, gin.H{"admin": gin.H{"id": id, "username": user, "name": name, "role": role}, "csrf_token": csrf})
}

func (s *Server) logout(c *gin.Context) {
	if t, err := c.Cookie(sessionCookie); err == nil {
		_, _ = s.DB.Exec(c, `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1`, secure.HashToken(t))
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", s.Cfg.CookieSecure, true)
	c.SetCookie(csrfCookie, "", -1, "/", "", s.Cfg.CookieSecure, false)
	httpx.OK(c, gin.H{"logged_out": true})
}

func (s *Server) session(c *gin.Context) {
	httpx.OK(c, gin.H{"admin": gin.H{"id": adminID(c), "username": c.GetString("admin_name"), "name": c.GetString("admin_display"), "role": c.GetString("admin_role")}})
}

// requireAdmin: sesi admin yang valid. Sesi customer tidak berlaku di sini (cookie dan tabel sesi dipisah).
func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		t, err := c.Cookie(sessionCookie)
		if err != nil || t == "" {
			httpx.Fail(c, http.StatusUnauthorized, "unauthenticated", "Silakan login.")
			return
		}
		var id, username, name, role string
		err = s.DB.QueryRow(c, `
SELECT a.id::text, a.username::text, a.name, a.role FROM sessions s JOIN admins a ON a.id = s.admin_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND a.status = 'active'`, secure.HashToken(t)).
			Scan(&id, &username, &name, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir. Silakan login lagi.")
			return
		} else if err != nil {
			httpx.ServerError(c, err)
			return
		}
		c.Set("admin_id", id)
		c.Set("admin_name", username)
		c.Set("admin_display", name)
		c.Set("admin_role", role)
		c.Next()
	}
}

// requireSuper membatasi pengaturan sensitif (aturan langganan, maintenance, API key) untuk superadmin.
func (s *Server) requireSuper() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("admin_role") != "superadmin" {
			httpx.Fail(c, http.StatusForbidden, "forbidden", "Hanya superadmin yang boleh mengubah ini.")
			return
		}
		c.Next()
	}
}

// csrf: double-submit cookie untuk POST/PUT/PATCH/DELETE.
func (s *Server) csrf() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		ck, err := c.Cookie(csrfCookie)
		if err != nil || ck == "" || subtle.ConstantTimeCompare([]byte(ck), []byte(c.GetHeader("X-CSRF-Token"))) != 1 {
			httpx.Fail(c, http.StatusForbidden, "csrf", "Token CSRF tidak valid.")
			return
		}
		c.Next()
	}
}
