package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/internal/wa"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(s)
}

func validPassword(p string) bool { return len(p) >= 8 && len(p) <= 72 }

// ---------- Sesi: refresh & CSRF ----------

// refresh memutar token sesi (token lama dicabut) dan memperpanjang masa berlakunya.
func (s *Server) refresh(c *gin.Context) {
	old, err := c.Cookie(sessionCookie)
	if err != nil {
		fail(c, http.StatusUnauthorized, "unauthenticated", "Silakan login.")
		return
	}
	tok, e1 := secure.RandomToken(32)
	csrf, e2 := secure.RandomToken(16)
	if e1 != nil || e2 != nil {
		serverError(c, errors.Join(e1, e2))
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	tag, err := tx.Exec(c, `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND user_id::text = $2 AND revoked_at IS NULL`, secure.HashToken(old), c.GetString("user_id"))
	if err != nil || tag.RowsAffected() == 0 {
		if err != nil {
			serverError(c, err)
		} else {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir. Silakan login lagi.")
		}
		return
	}
	if _, err := tx.Exec(c, `INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1::uuid,$2,$3,NULLIF($4,'')::inet,NULLIF($5,''))`, c.GetString("user_id"), secure.HashToken(tok), time.Now().Add(sessionTTL), c.ClientIP(), c.Request.UserAgent()); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	max := int(sessionTTL.Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, tok, max, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, true)
	c.SetCookie(csrfCookie, csrf, max, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, false)
	ok(c, gin.H{"csrf_token": csrf})
}

// csrfToken mengembalikan token CSRF yang sedang berlaku (nilai cookie csrf_token).
func (s *Server) csrfToken(c *gin.Context) {
	t, err := c.Cookie(csrfCookie)
	if err != nil || t == "" {
		t, _ = secure.RandomToken(16)
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(csrfCookie, t, int(sessionTTL.Seconds()), "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, false)
	}
	ok(c, gin.H{"csrf_token": t})
}

// heartbeat memperbarui last_seen_at (kehadiran untuk "online sekarang" di Convera Admin).
func (s *Server) heartbeat(c *gin.Context) {
	if _, err := s.DB.Exec(c, `UPDATE users SET last_seen_at = now() WHERE id::text = $1`, c.GetString("user_id")); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"ok": true})
}

// ---------- Profil & password ----------

func (s *Server) getProfile(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT id::text AS id, username::text AS username, email::text AS email, name, phone, created_at
		FROM users WHERE id::text = $1`, c.GetString("user_id"))
	if err != nil || len(rows) == 0 {
		serverError(c, errors.Join(err, errors.New("profil tidak ditemukan")))
		return
	}
	ok(c, rows[0])
}

func (s *Server) patchProfile(c *gin.Context) {
	var in struct {
		Name  *string `json:"name"`
		Phone *string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	var name *string
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 120 {
			bad422(c, "Nama wajib diisi (maks. 120 karakter).")
			return
		}
		name = &n
	}
	// phone: nil = tidak diubah, "" = hapus, selain itu dinormalisasi ke 62xxxxxxxxxx.
	var phone *string
	clearPhone := false
	if in.Phone != nil {
		if strings.TrimSpace(*in.Phone) == "" {
			clearPhone = true
		} else {
			p, err := wa.NormalizePhone(*in.Phone)
			if err != nil {
				bad422(c, "Nomor telepon tidak valid.")
				return
			}
			phone = &p
		}
	}
	if name == nil && phone == nil && !clearPhone {
		fail(c, http.StatusBadRequest, "bad_request", "Tidak ada yang diubah.")
		return
	}
	if _, err := s.DB.Exec(c, `UPDATE users SET name = COALESCE($2, name), phone = CASE WHEN $4 THEN NULL ELSE COALESCE($3, phone) END WHERE id::text = $1`,
		c.GetString("user_id"), name, phone, clearPhone); err != nil {
		serverError(c, err)
		return
	}
	s.getProfile(c)
}

// changePassword: POST {current, new}. Sesi lain dicabut; sesi ini tetap berlaku.
func (s *Server) changePassword(c *gin.Context) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if !s.limiter("chpw|"+c.GetString("user_id"), 5.0/900, 5).Allow() { // 5 percobaan / 15 menit
		fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan. Coba lagi nanti.")
		return
	}
	switch {
	case !validPassword(in.New):
		bad422(c, "Password baru 8 sampai 72 karakter.")
		return
	case in.New == in.Current:
		bad422(c, "Password baru tidak boleh sama dengan yang lama.")
		return
	}
	var hash string
	if err := s.DB.QueryRow(c, `SELECT password_hash FROM users WHERE id::text = $1`, c.GetString("user_id")).Scan(&hash); err != nil {
		serverError(c, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Current)) != nil {
		fail(c, http.StatusForbidden, "wrong_password", "Password saat ini salah.")
		return
	}
	nh, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		serverError(c, err)
		return
	}
	cur, _ := c.Cookie(sessionCookie)
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	if _, err := tx.Exec(c, `UPDATE users SET password_hash = $2 WHERE id::text = $1`, c.GetString("user_id"), string(nh)); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.Exec(c, `UPDATE sessions SET revoked_at = now() WHERE user_id::text = $1 AND revoked_at IS NULL AND token_hash <> $2`, c.GetString("user_id"), secure.HashToken(cur)); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"changed": true})
}

// ---------- Workspace per slug & penghapusan data (UU PDP) ----------

// ownedWorkspace mengembalikan id workspace milik user (slug dari path); selain itu 404.
func (s *Server) ownedWorkspace(c *gin.Context) (string, bool) {
	var id string
	err := s.DB.QueryRow(c, `SELECT id::text FROM workspaces WHERE slug = $1 AND owner_user_id::text = $2 AND deleted_at IS NULL AND ($3 = '' OR id::text = $3)`, c.Param("slug"), c.GetString("user_id"), c.GetString("support_ws_id")).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, http.StatusNotFound, "workspace_not_found", "Workspace tidak ditemukan.")
		return "", false
	} else if err != nil {
		serverError(c, err)
		return "", false
	}
	return id, true
}

func (s *Server) getWorkspace(c *gin.Context) {
	id, found := s.ownedWorkspace(c)
	if !found {
		return
	}
	rows, err := query(c, s.DB, `
SELECT w.id::text AS id, w.slug, w.name, w.category, w.city, w.webhook_slug, w.created_at,
       sub.status AS subscription_status, sub.trial_until, sub.current_period_end
FROM workspaces w LEFT JOIN subscriptions sub ON sub.workspace_id = w.id WHERE w.id::text = $1`, id)
	if err != nil || len(rows) == 0 {
		serverError(c, errors.Join(err, errors.New("workspace tidak terbaca")))
		return
	}
	r := rows[0]
	r["webhook_url"] = s.Cfg.WebhookBase + "/cloud/" + r["webhook_slug"].(string)
	delete(r, "webhook_slug")
	ok(c, r)
}

func (s *Server) patchWorkspace(c *gin.Context) {
	id, found := s.ownedWorkspace(c)
	if !found {
		return
	}
	var in struct {
		Name     *string `json:"name"`
		Category *string `json:"category"`
		City     *string `json:"city"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 120 {
			bad422(c, "Nama bisnis wajib diisi (maks. 120 karakter).")
			return
		}
		in.Name = &n
	}
	for _, p := range []*string{in.Category, in.City} {
		if p != nil && len(strings.TrimSpace(*p)) > 80 {
			bad422(c, "Kategori dan kota maksimal 80 karakter.")
			return
		}
	}
	if _, err := s.DB.Exec(c, `UPDATE workspaces SET name = COALESCE($2, name),
		category = CASE WHEN $3::text IS NULL THEN category ELSE NULLIF(trim($3), '') END,
		city = CASE WHEN $4::text IS NULL THEN city ELSE NULLIF(trim($4), '') END WHERE id::text = $1`, id, in.Name, in.Category, in.City); err != nil {
		serverError(c, err)
		return
	}
	s.getWorkspace(c)
}

// purgeWorkspace menghapus data operasional & pribadi workspace dan menonaktifkannya. Catatan keuangan
// (payments, payment_proofs) sengaja dipertahankan untuk pembukuan; kebijakan retensinya belum diputuskan (bab 13).
func purgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	for _, q := range []string{
		`DELETE FROM events WHERE workspace_id::text = $1`,
		`DELETE FROM leads WHERE workspace_id::text = $1`,
		`DELETE FROM event_rules WHERE workspace_id::text = $1`,
		`DELETE FROM webhook_inbox WHERE workspace_id::text = $1`,
		`DELETE FROM meta_connections WHERE workspace_id::text = $1`, // termasuk token & app secret terenkripsi
		`DELETE FROM wa_connections WHERE workspace_id::text = $1`,
		`DELETE FROM wa_sessions WHERE workspace_id::text = $1`,
		`DELETE FROM api_keys WHERE workspace_id::text = $1`,
		`DELETE FROM daily_stats WHERE workspace_id::text = $1`,
		`DELETE FROM reminder_log WHERE workspace_id::text = $1`,
		`DELETE FROM notifications WHERE workspace_id::text = $1`,
		`UPDATE subscriptions SET status = 'stopped', current_period_end = LEAST(COALESCE(current_period_end, now()), now()) WHERE workspace_id::text = $1`,
		`UPDATE workspaces SET deleted_at = now() WHERE id::text = $1`,
	} {
		if _, err := tx.Exec(ctx, q, workspaceID); err != nil {
			return err
		}
	}
	return nil
}

// deleteWorkspace: DELETE {confirm: <slug>}. Tidak bisa dibatalkan.
func (s *Server) deleteWorkspace(c *gin.Context) {
	id, found := s.ownedWorkspace(c)
	if !found {
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Confirm != c.Param("slug") {
		bad422(c, "Ketik slug workspace pada field confirm untuk menghapus.")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	if err := purgeWorkspace(c, tx, id); err != nil {
		serverError(c, err)
		return
	}
	if err := auditUser(c, tx, c.GetString("user_id"), id, "workspace.delete", c.Param("slug")); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"deleted": true})
}

// deleteAccount: DELETE /me {password}. Menghapus semua workspace milik user (lihat purgeWorkspace), menonaktifkan
// akun (username bisa dipakai mendaftar lagi), dan mencabut semua sesi.
func (s *Server) deleteAccount(c *gin.Context) {
	var in struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if !s.limiter("delacct|"+c.GetString("user_id"), 5.0/900, 5).Allow() {
		fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan. Coba lagi nanti.")
		return
	}
	var hash string
	if err := s.DB.QueryRow(c, `SELECT password_hash FROM users WHERE id::text = $1`, c.GetString("user_id")).Scan(&hash); err != nil {
		serverError(c, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		fail(c, http.StatusForbidden, "wrong_password", "Password salah.")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback(c)
	rows, err := tx.Query(c, `SELECT id::text FROM workspaces WHERE owner_user_id::text = $1 AND deleted_at IS NULL`, c.GetString("user_id"))
	if err != nil {
		serverError(c, err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			serverError(c, err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := purgeWorkspace(c, tx, id); err != nil {
			serverError(c, err)
			return
		}
	}
	for _, q := range []string{
		`UPDATE sessions SET revoked_at = now() WHERE user_id::text = $1 AND revoked_at IS NULL`,
		`DELETE FROM notifications WHERE user_id::text = $1`,
		// Anonimkan data pribadi; baris dipertahankan karena payments/payment_proofs merujuknya.
		`UPDATE users SET deleted_at = now(), status = 'suspended', name = 'Akun dihapus', phone = NULL, password_hash = '!',
			username = 'deleted-' || substr(replace(id::text, '-', ''), 1, 12),
			email = 'deleted-' || id::text || '@deleted.invalid' WHERE id::text = $1`,
	} {
		if _, err := tx.Exec(c, q, c.GetString("user_id")); err != nil {
			serverError(c, err)
			return
		}
	}
	if err := auditUser(c, tx, c.GetString("user_id"), "", "account.delete", ""); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, true)
	c.SetCookie(csrfCookie, "", -1, "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, false)
	ok(c, gin.H{"deleted": true})
}

// auditUser menulis catatan aktivitas atas nama user.
func auditUser(c *gin.Context, tx pgx.Tx, userID, workspaceID, action, target string) error {
	_, err := tx.Exec(c, `INSERT INTO audit_logs (actor_type, actor_id, actor_label, workspace_id, action, target_id, ip)
		VALUES ('user', $1::uuid, 'user', NULLIF($2,'')::uuid, $3, NULLIF($4,''), NULLIF($5,'')::inet)`, userID, workspaceID, action, target, c.ClientIP())
	return err
}
