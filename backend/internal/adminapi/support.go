package adminapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// supportCodeTTL: kode harus ditukar dalam waktu singkat (tautan dibuka segera setelah dibuat).
const supportCodeTTL = 90 * time.Second

// supportAccess membuat tautan sekali pakai untuk masuk ke dashboard satu customer sebagai bantuan.
// Hanya superadmin; alasan wajib dan dicatat di audit log. Sesi yang dihasilkan: 2 jam, terbatas ke workspace ini,
// tanpa hak mengubah password/profil/langganan/bukti bayar, dan semua tindakannya dicatat (lihat API customer).
func (s *Server) supportAccess(c *gin.Context) {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 3 || len(in.Reason) > 300 {
		bad(c, "Alasan akses wajib diisi (3-300 karakter), mis. \"Bantu atur koneksi Meta\".")
		return
	}
	if !s.lim.Get("support|"+adminID(c), 10.0/60, 5).Allow() {
		httpx.Fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu sering membuat akses dukungan. Coba lagi sebentar.")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)

	var owner, name string
	err = tx.QueryRow(c, `SELECT w.owner_user_id::text, w.name FROM workspaces w JOIN users u ON u.id = w.owner_user_id
		WHERE w.id::text = $1 AND w.deleted_at IS NULL AND u.deleted_at IS NULL AND u.status = 'active'`, c.Param("id")).Scan(&owner, &name)
	if err == pgx.ErrNoRows {
		httpx.Fail(c, http.StatusNotFound, "customer_not_found", "Customer tidak ditemukan atau akunnya tidak aktif.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	code, err := secure.RandomToken(32)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if _, err := tx.Exec(c, `INSERT INTO support_access_codes (admin_id, workspace_id, user_id, token_hash, reason, expires_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6)`, adminID(c), c.Param("id"), owner, secure.HashToken(code), in.Reason, time.Now().Add(supportCodeTTL)); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "support.access.create", c.Param("id"), "workspace", c.Param("id"), map[string]any{"reason": in.Reason, "customer": name}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	httpx.OK(c, gin.H{"url": s.Cfg.AppURL + "/?support=" + code, "expires_in": int(supportCodeTTL.Seconds()), "session_minutes": 120})
}

// resetCustomerPassword membuat password sementara untuk pemilik workspace (tidak ada reset lewat email).
// Hanya superadmin. Password sementara hanya tampil sekali di respons ini, semua sesi customer dicabut,
// dan tindakannya dicatat di audit log (tanpa password).
func (s *Server) resetCustomerPassword(c *gin.Context) {
	if !s.lim.Get("pwreset|"+adminID(c), 10.0/60, 5).Allow() {
		httpx.Fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu sering reset password. Coba lagi sebentar.")
		return
	}
	tx, err := s.DB.Begin(c)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	defer tx.Rollback(c)

	var owner, username, name string
	err = tx.QueryRow(c, `SELECT u.id::text, u.username::text, w.name FROM workspaces w JOIN users u ON u.id = w.owner_user_id
		WHERE w.id::text = $1 AND w.deleted_at IS NULL AND u.deleted_at IS NULL FOR UPDATE OF u`, c.Param("id")).Scan(&owner, &username, &name)
	if err == pgx.ErrNoRows {
		httpx.Fail(c, http.StatusNotFound, "customer_not_found", "Customer tidak ditemukan.")
		return
	} else if err != nil {
		httpx.ServerError(c, err)
		return
	}
	raw, err := secure.RandomToken(8) // 16 karakter hex
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	temp := raw[:4] + "-" + raw[4:8] + "-" + raw[8:12]
	hash, err := bcrypt.GenerateFromPassword([]byte(temp), bcrypt.DefaultCost)
	if err != nil {
		httpx.ServerError(c, err)
		return
	}
	if _, err := tx.Exec(c, `UPDATE users SET password_hash = $2 WHERE id::text = $1`, owner, string(hash)); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if _, err := tx.Exec(c, `UPDATE sessions SET revoked_at = now() WHERE user_id::text = $1 AND revoked_at IS NULL`, owner); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := s.audit(c, tx, "customer.password_reset", c.Param("id"), "user", owner, map[string]any{"customer": name, "username": username}); err != nil {
		httpx.ServerError(c, err)
		return
	}
	if err := tx.Commit(c); err != nil {
		httpx.ServerError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	httpx.OK(c, gin.H{"username": username, "temporary_password": temp})
}
