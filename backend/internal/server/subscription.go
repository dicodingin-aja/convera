package server

import (
	"net/http"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/gin-gonic/gin"
)

// requireAccess menolak aksi yang mengubah atau memakai fitur bila workspace belum dibayar atau langganannya
// sudah berakhir (402 subscription_required). Lihat data, unggah bukti transfer, ubah/hapus workspace, dan
// menghapus data tetap boleh. Harus dipasang setelah withWorkspace() atau apiKeyAuth().
func (s *Server) requireAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, err := core.WorkspaceAccess(c, s.DB, wsID(c))
		if err != nil {
			serverError(c, err)
			return
		}
		if !ok {
			fail(c, http.StatusPaymentRequired, "subscription_required",
				"Langganan workspace ini belum aktif. Upgrade dulu untuk memakai fitur ini.")
			return
		}
		c.Next()
	}
}
