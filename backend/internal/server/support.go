package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// supportSessionTTL: sesi dukungan singkat dan tidak bisa diperpanjang (refresh diblokir).
const supportSessionTTL = 2 * time.Hour

// supportBlocked: tindakan yang tidak boleh dilakukan admin atas nama customer, apa pun alasannya:
// mengubah kredensial/profil, menghapus data, membuat workspace baru, mengunggah bukti bayar, dan membuat API key.
var supportBlocked = map[string]bool{
	"POST /api/v1/auth/refresh":       true,
	"PATCH /api/v1/me/profile":        true,
	"POST /api/v1/me/password":        true,
	"DELETE /api/v1/me":               true,
	"POST /api/v1/workspaces":         true,
	"DELETE /api/v1/workspaces/:slug": true,
	"POST /api/v1/payment-proofs":     true,
	"POST /api/v1/leads/erase":        true,
	"POST /api/v1/api-keys":           true,
	"DELETE /api/v1/api-keys/:id":     true,
}

func isSupport(c *gin.Context) bool { return c.GetString("support_admin_id") != "" }

// supportGuard membatasi sesi dukungan dan mencatat setiap permintaan yang mengubah data (metode, pola path, status;
// tanpa isi body dan tanpa nilai parameter path) ke audit_logs atas nama admin.
func (s *Server) supportGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isSupport(c) {
			c.Next()
			return
		}
		if supportBlocked[c.Request.Method+" "+c.FullPath()] {
			fail(c, http.StatusForbidden, "support_mode_blocked", "Tindakan ini tidak diizinkan dalam mode dukungan.")
		} else {
			c.Next()
		}
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			return
		}
		detail, _ := json.Marshal(map[string]any{"method": c.Request.Method, "path": c.FullPath(), "status": c.Writer.Status()})
		_, _ = s.DB.Exec(c.Request.Context(), `INSERT INTO audit_logs (actor_type, actor_id, actor_label, workspace_id, action, target_type, detail, ip)
			VALUES ('admin', $1::uuid, $2, $3::uuid, 'support.request', 'http', $4, NULLIF($5,'')::inet)`,
			c.GetString("support_admin_id"), c.GetString("support_admin_name")+" (dukungan)", c.GetString("support_ws_id"), detail, c.ClientIP())
	}
}

// supportLogin menukar kode sekali pakai dari Convera Admin menjadi sesi customer yang terbatas.
func (s *Server) supportLogin(c *gin.Context) {
	var in struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Code == "" {
		fail(c, http.StatusBadRequest, "bad_request", "Kode wajib diisi.")
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
	var codeID, adminID, wsID, userID, reason, adminName, wsSlug, wsName, username, email, name string
	err = tx.QueryRow(c, `
SELECT sc.id::text, sc.admin_id::text, sc.workspace_id::text, sc.user_id::text, sc.reason, a.name, w.slug, w.name, u.username::text, u.email::text, u.name
FROM support_access_codes sc
JOIN admins a ON a.id = sc.admin_id AND a.status = 'active'
JOIN workspaces w ON w.id = sc.workspace_id AND w.deleted_at IS NULL
JOIN users u ON u.id = sc.user_id AND u.deleted_at IS NULL AND u.status = 'active'
WHERE sc.token_hash = $1 AND sc.used_at IS NULL AND sc.expires_at > now() FOR UPDATE OF sc`, secure.HashToken(in.Code)).
		Scan(&codeID, &adminID, &wsID, &userID, &reason, &adminName, &wsSlug, &wsName, &username, &email, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, http.StatusBadRequest, "invalid_code", "Tautan dukungan tidak valid, sudah dipakai, atau kedaluwarsa. Buat tautan baru dari Convera Admin.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	expires := time.Now().Add(supportSessionTTL)
	steps := []struct {
		sql  string
		args []any
	}{
		{`UPDATE support_access_codes SET used_at = now() WHERE id::text = $1`, []any{codeID}},
		{`INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent, impersonator_admin_id, support_workspace_id)
		  VALUES ($1::uuid, $2, $3, NULLIF($4,'')::inet, NULLIF($5,''), $6::uuid, $7::uuid)`,
			[]any{userID, secure.HashToken(tok), expires, c.ClientIP(), c.Request.UserAgent(), adminID, wsID}},
		{`INSERT INTO audit_logs (actor_type, actor_id, actor_label, workspace_id, action, target_type, target_id, detail, ip)
		  VALUES ('admin', $1::uuid, $2, $3::uuid, 'support.access.login', 'workspace', $3, $4, NULLIF($5,'')::inet)`,
			[]any{adminID, adminName + " (dukungan)", wsID, mustJSON(map[string]any{"reason": reason}), c.ClientIP()}},
		// Transparansi: pemilik akun diberi tahu bahwa tim dukungan masuk.
		{`INSERT INTO notifications (user_id, workspace_id, type, title, body, link) VALUES ($1::uuid, $2::uuid, 'support_access',
		  'Tim dukungan Convera mengakses akun Anda', $3, '/koneksi')`,
			[]any{userID, wsID, "Tim Convera membuka dashboard " + oneLine(wsName) + " untuk membantu Anda. Keperluan: " + oneLine(reason) + "."}},
	}
	for _, st := range steps {
		if _, err := tx.Exec(c, st.sql, st.args...); err != nil {
			serverError(c, err)
			return
		}
	}
	if err := tx.Commit(c); err != nil {
		serverError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, tok, int(supportSessionTTL.Seconds()), "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, true)
	c.SetCookie(csrfCookie, csrf, int(supportSessionTTL.Seconds()), "/", s.Cfg.CookieDomain, s.Cfg.CookieSecure, false)
	ok(c, gin.H{
		"user":       gin.H{"username": username, "email": email, "name": name},
		"csrf_token": csrf,
		"support":    gin.H{"admin": adminName, "workspace": wsSlug, "workspace_name": wsName, "expires_at": expires},
	})
}
