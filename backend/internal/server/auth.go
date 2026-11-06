package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/internal/wa"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "cv_session"
	csrfCookie    = "csrf_token"
	sessionTTL    = 30 * 24 * time.Hour
)

var (
	usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,29}$`)
	emailRe    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	slugRe     = regexp.MustCompile(`[^a-z0-9]+`)
	// Hash palsu agar login untuk username yang tidak ada memakan waktu yang sama dengan yang ada.
	dummyHash, _ = bcrypt.GenerateFromPassword([]byte("convera-dummy"), bcrypt.DefaultCost)
)

func asValidation(err error, target **core.ValidationError) bool { return errors.As(err, target) }

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// ---------- Register / login / logout ----------

type credentials struct {
	Username string `json:"username"`
	Name     string `json:"name"`  // wajib saat daftar
	Email    string `json:"email"` // wajib saat daftar (kontak; bukan untuk login, tanpa verifikasi)
	Phone    string `json:"phone"` // wajib saat daftar
	Password string `json:"password"`
}

func (s *Server) register(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	phone, phoneErr := wa.NormalizePhone(in.Phone)
	switch {
	case !usernameRe.MatchString(in.Username):
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Username 3 sampai 30 karakter: huruf kecil, angka, titik, garis bawah, atau strip; harus diawali huruf atau angka.")
		return
	case !validPassword(in.Password):
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Password 8 sampai 72 karakter.")
		return
	case in.Name == "" || len(in.Name) > 120:
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Nama wajib diisi (maks. 120 karakter).")
		return
	case !emailRe.MatchString(in.Email) || len(in.Email) > 254:
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Format email tidak valid.")
		return
	case phoneErr != nil:
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Nomor telepon tidak valid.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		serverError(c, err)
		return
	}
	var id string
	err = s.DB.QueryRow(c, `INSERT INTO users (username, email, password_hash, name, phone) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
		in.Username, in.Email, string(hash), in.Name, phone).Scan(&id)
	if isUnique(err) {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && strings.Contains(pe.ConstraintName, "email") {
			fail(c, http.StatusConflict, "email_taken", "Email sudah terdaftar.")
		} else {
			fail(c, http.StatusConflict, "username_taken", "Username sudah dipakai.")
		}
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	s.startSession(c, id, http.StatusCreated, in.Username, in.Email, in.Name, phone)
}

func (s *Server) login(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	var id, hash, name, status, email, phone string
	err := s.DB.QueryRow(c, `SELECT id::text, password_hash, name, status, email::text, COALESCE(phone, '') FROM users WHERE username = $1 AND deleted_at IS NULL`, username).
		Scan(&id, &hash, &name, &status, &email, &phone)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		fail(c, http.StatusUnauthorized, "invalid_credentials", "Username atau password salah.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		fail(c, http.StatusUnauthorized, "invalid_credentials", "Username atau password salah.")
		return
	}
	if status != "active" {
		fail(c, http.StatusForbidden, "account_suspended", "Akun ditangguhkan. Hubungi Convera.")
		return
	}
	s.startSession(c, id, http.StatusOK, username, email, name, phone)
}

func (s *Server) startSession(c *gin.Context, userID string, status int, username, email, name, phone string) {
	token, err1 := secure.RandomToken(32)
	csrf, err2 := secure.RandomToken(16)
	if err1 != nil || err2 != nil {
		serverError(c, errors.Join(err1, err2))
		return
	}
	_, err := s.DB.Exec(c, `INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1,$2,$3,NULLIF($4,'')::inet,NULLIF($5,''))`,
		userID, secure.HashToken(token), time.Now().Add(sessionTTL), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		serverError(c, err)
		return
	}
	if _, err := s.DB.Exec(c, `INSERT INTO audit_logs (actor_type, actor_id, actor_label, action, detail, ip)
		VALUES ('user', $1::uuid, $2, 'auth.login', jsonb_build_object('user_agent', $3), NULLIF($4,'')::inet)`,
		userID, name, c.Request.UserAgent(), c.ClientIP()); err != nil {
		serverError(c, err)
		return
	}
	_, _ = s.DB.Exec(c, `UPDATE users SET last_seen_at = now() WHERE id = $1`, userID)
	max := int(sessionTTL.Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, max, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, true) // httpOnly
	c.SetCookie(csrfCookie, csrf, max, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, false)    // dibaca JS untuk X-CSRF-Token
	okStatus(c, status, gin.H{"user": gin.H{"id": userID, "username": username, "email": email, "name": name, "phone": phone}, "csrf_token": csrf})
}

func (s *Server) logout(c *gin.Context) {
	if t, err := c.Cookie(sessionCookie); err == nil {
		_, _ = s.DB.Exec(c, `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1`, secure.HashToken(t))
	}
	if !isSupport(c) {
		_, _ = s.DB.Exec(c, `INSERT INTO audit_logs (actor_type, actor_id, actor_label, action, ip)
			VALUES ('user', $1::uuid, 'user', 'auth.logout', NULLIF($2,'')::inet)`, c.GetString("user_id"), c.ClientIP())
	}
	if isSupport(c) {
		_, _ = s.DB.Exec(c, `INSERT INTO audit_logs (actor_type, actor_id, actor_label, workspace_id, action, target_type, ip)
			VALUES ('admin', $1::uuid, $2, $3::uuid, 'support.access.end', 'workspace', NULLIF($4,'')::inet)`,
			c.GetString("support_admin_id"), c.GetString("support_admin_name")+" (dukungan)", c.GetString("support_ws_id"), c.ClientIP())
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, true)
	c.SetCookie(csrfCookie, "", -1, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, false)
	ok(c, gin.H{"logged_out": true})
}

func (s *Server) session(c *gin.Context) {
	var username, email, name, phone string
	if err := s.DB.QueryRow(c, `SELECT username::text, email::text, name, COALESCE(phone, '') FROM users WHERE id = $1`, c.GetString("user_id")).Scan(&username, &email, &name, &phone); err != nil {
		serverError(c, err)
		return
	}

	out := gin.H{"user": gin.H{"id": c.GetString("user_id"), "username": username, "email": email, "name": name, "phone": phone}}
	if isSupport(c) {
		var slug, wsName string
		var exp time.Time
		tok, _ := c.Cookie(sessionCookie)
		_ = s.DB.QueryRow(c, `SELECT w.slug, w.name, s.expires_at FROM sessions s JOIN workspaces w ON w.id::text = $2 WHERE s.token_hash = $1`,
			secure.HashToken(tok), c.GetString("support_ws_id")).Scan(&slug, &wsName, &exp)
		out["support"] = gin.H{"admin": c.GetString("support_admin_name"), "workspace": slug, "workspace_name": wsName, "expires_at": exp}
	}
	ok(c, out)
}

func (s *Server) listSessions(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	rows, err := query(c, s.DB, `SELECT id::text, user_agent, host(ip)::text, created_at, last_used_at,
		expires_at, token_hash = $2 AS current
		FROM sessions
		WHERE user_id::text = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY current DESC, last_used_at DESC NULLS LAST, created_at DESC`,
		c.GetString("user_id"), secure.HashToken(token))
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rows)
}

func (s *Server) revokeSession(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	var current bool
	err := s.DB.QueryRow(c, `SELECT token_hash = $3 FROM sessions
		WHERE id = $1::uuid AND user_id::text = $2 AND revoked_at IS NULL`,
		c.Param("id"), c.GetString("user_id"), secure.HashToken(token)).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, http.StatusNotFound, "session_not_found", "Sesi tidak ditemukan.")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if current {
		fail(c, http.StatusBadRequest, "current_session", "Sesi yang sedang digunakan tidak dapat dikeluarkan.")
		return
	}
	if _, err := s.DB.Exec(c, `UPDATE sessions SET revoked_at = now()
		WHERE id = $1::uuid AND user_id::text = $2 AND revoked_at IS NULL`,
		c.Param("id"), c.GetString("user_id")); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"revoked": true})
}

func (s *Server) listUserAuditLogs(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT at, COALESCE(actor_label, 'Sistem') AS actor_label,
		action, host(ip)::text AS ip
		FROM audit_logs
		WHERE actor_type = 'user' AND actor_id::text = $1
		ORDER BY at DESC LIMIT 100`, c.GetString("user_id"))
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rows)
}

// ---------- Middleware sesi ----------

func (s *Server) requireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		t, err := c.Cookie(sessionCookie)
		if err != nil || t == "" {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Silakan login.")
			return
		}
		var userID string
		var supAdmin, supWS, supName *string
		err = s.DB.QueryRow(c, `
SELECT u.id::text, s.impersonator_admin_id::text, s.support_workspace_id::text, a.name
FROM sessions s JOIN users u ON u.id = s.user_id LEFT JOIN admins a ON a.id = s.impersonator_admin_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
  AND u.deleted_at IS NULL AND u.status = 'active'`, secure.HashToken(t)).Scan(&userID, &supAdmin, &supWS, &supName)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir. Silakan login lagi.")
			return
		} else if err != nil {
			serverError(c, err)
			return
		}
		if supAdmin != nil && supWS != nil {
			// Sesi dukungan: dibatasi ke satu workspace dan TIDAK menandai customer sebagai "online".
			c.Set("support_admin_id", *supAdmin)
			c.Set("support_ws_id", *supWS)
			if supName != nil {
				c.Set("support_admin_name", *supName)
			}
		} else {
			// Presence untuk "online sekarang" di Convera Admin; paling sering sekali per menit per user.
			_, _ = s.DB.Exec(c, `UPDATE users SET last_seen_at = now() WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '1 minute')`, userID)
		}
		_, _ = s.DB.Exec(c, `UPDATE sessions SET last_used_at = now() WHERE token_hash = $1`, secure.HashToken(t))
		c.Set("user_id", userID)
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
		h := c.GetHeader("X-CSRF-Token")
		if err != nil || ck == "" || subtle.ConstantTimeCompare([]byte(ck), []byte(h)) != 1 {
			fail(c, http.StatusForbidden, "csrf", "Token CSRF tidak valid.")
			return
		}
		c.Next()
	}
}

// withWorkspace menentukan workspace dari header X-Workspace (slug) atau ?workspace=. Tanpa itu dipakai
// workspace tunggal milik user. Workspace milik orang lain dijawab 404 (bukan 403).
func (s *Server) withWorkspace() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.GetHeader("X-Workspace")
		if slug == "" {
			slug = c.Query("workspace")
		}
		rows, err := s.DB.Query(c, `SELECT id::text, slug FROM workspaces
			WHERE owner_user_id = $1 AND deleted_at IS NULL AND ($2 = '' OR slug = $2) AND ($3 = '' OR id::text = $3) ORDER BY created_at LIMIT 2`,
			c.GetString("user_id"), slug, c.GetString("support_ws_id"))
		if err != nil {
			serverError(c, err)
			return
		}
		type w struct{ id, slug string }
		var ws []w
		for rows.Next() {
			var x w
			if err := rows.Scan(&x.id, &x.slug); err != nil {
				rows.Close()
				serverError(c, err)
				return
			}
			ws = append(ws, x)
		}
		rows.Close()
		switch {
		case len(ws) == 0:
			fail(c, http.StatusNotFound, "workspace_not_found", "Workspace tidak ditemukan.")
		case len(ws) > 1 && slug == "":
			fail(c, http.StatusBadRequest, "workspace_required", "Pilih workspace (header X-Workspace).")
		default:
			c.Set("workspace_id", ws[0].id)
			c.Set("workspace_slug", ws[0].slug)
			c.Next()
		}
	}
}

func wsID(c *gin.Context) string { return c.GetString("workspace_id") }

// ---------- Workspace ----------

func (s *Server) listWorkspaces(c *gin.Context) {
	rows, err := query(c, s.DB, `
SELECT w.id::text, w.slug, w.name, w.category, w.city, w.webhook_slug, w.created_at,
       sub.status AS subscription_status, sub.trial_until, sub.current_period_end
FROM workspaces w LEFT JOIN subscriptions sub ON sub.workspace_id = w.id
WHERE w.owner_user_id = $1 AND w.deleted_at IS NULL AND ($2 = '' OR w.id::text = $2) ORDER BY w.created_at`, c.GetString("user_id"), c.GetString("support_ws_id"))
	if err != nil {
		serverError(c, err)
		return
	}
	for _, r := range rows {
		r["webhook_url"] = s.Cfg.WebhookBase + "/cloud/" + r["webhook_slug"].(string)
		delete(r, "webhook_slug")
	}
	ok(c, rows)
}

func (s *Server) createWorkspace(c *gin.Context) {
	var in struct {
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Category string `json:"category"`
		City     string `json:"city"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Nama bisnis wajib diisi (maks. 120 karakter).")
		return
	}
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(firstNonEmpty(in.Slug, in.Name)), "-"), "-")
	if slug == "" {
		slug = "bisnis"
	}
	if len(slug) > 50 {
		slug = strings.Trim(slug[:50], "-")
	}
	verify, err := secure.RandomToken(16)
	if err != nil {
		serverError(c, err)
		return
	}

	for attempt := 0; attempt < 3; attempt++ {
		cand := slug
		if attempt > 0 {
			sfx, _ := secure.RandomToken(2)
			cand = slug + "-" + sfx
		}
		var id string
		err = s.createWorkspaceTx(c, c.GetString("user_id"), cand, in.Name, in.Category, in.City, verify, &id)
		if isUnique(err) {
			if in.Slug != "" && attempt == 0 {
				fail(c, http.StatusConflict, "slug_taken", "Slug sudah dipakai.")
				return
			}
			continue
		} else if err != nil {
			serverError(c, err)
			return
		}
		okStatus(c, http.StatusCreated, gin.H{"id": id, "slug": cand, "name": in.Name})
		return
	}
	fail(c, http.StatusConflict, "slug_taken", "Slug sudah dipakai.")
}

func (s *Server) createWorkspaceTx(ctx context.Context, userID, slug, name, category, city, verify string, id *string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `INSERT INTO workspaces (owner_user_id, slug, name, category, city)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,'')) RETURNING id::text`, userID, slug, name, category, city).Scan(id); err != nil {
		return err
	}
	// Langganan trial: trial_until = sekarang + settings.trial_days (default 14).
	if _, err := tx.Exec(ctx, `
INSERT INTO subscriptions (workspace_id, status, trial_until)
VALUES ($1, 'trial', now() + make_interval(days => COALESCE((SELECT (value #>> '{}')::int FROM settings WHERE key = 'trial_days'), 14)))`, *id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO meta_connections (workspace_id, verify_token) VALUES ($1,$2)`, *id, verify); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}
