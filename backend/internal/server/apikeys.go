package server

import (
	"net/http"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/gin-gonic/gin"
)

func (s *Server) listAPIKeys(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT id::text AS id, name, prefix, created_at, last_used_at, revoked_at
		FROM api_keys WHERE workspace_id=$1 ORDER BY created_at DESC`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rows)
}

// createAPIKey: key lengkap hanya ditampilkan sekali di respons ini; di database hanya hash + prefix.
func (s *Server) createAPIKey(c *gin.Context) {
	var in struct {
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || len(in.Label) > 80 {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Label wajib diisi (maks. 80 karakter).")
		return
	}
	var n int
	_ = s.DB.QueryRow(c, `SELECT count(*) FROM api_keys WHERE workspace_id=$1 AND revoked_at IS NULL`, wsID(c)).Scan(&n)
	if n >= 10 {
		fail(c, http.StatusUnprocessableEntity, "limit_reached", "Maksimal 10 API key aktif. Cabut yang tidak dipakai.")
		return
	}
	rnd, err := secure.RandomToken(24)
	if err != nil {
		serverError(c, err)
		return
	}
	key := "cvr_" + rnd
	var id string
	if err := s.DB.QueryRow(c, `INSERT INTO api_keys (workspace_id, name, prefix, key_hash, created_by_user)
		VALUES ($1,$2,$3,$4,$5) RETURNING id::text`, wsID(c), in.Label, key[:10], hashKey(key), c.GetString("user_id")).Scan(&id); err != nil {
		serverError(c, err)
		return
	}
	okStatus(c, http.StatusCreated, gin.H{"id": id, "label": in.Label, "key": key, "prefix": key[:10],
		"note": "Simpan key ini sekarang. Key lengkap tidak akan ditampilkan lagi."})
}

func (s *Server) revokeAPIKey(c *gin.Context) {
	tag, err := s.DB.Exec(c, `UPDATE api_keys SET revoked_at=now() WHERE id::text=$1 AND workspace_id=$2 AND revoked_at IS NULL`, c.Param("id"), wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, http.StatusNotFound, "key_not_found", "API key tidak ditemukan.")
		return
	}
	ok(c, gin.H{"revoked": true})
}
