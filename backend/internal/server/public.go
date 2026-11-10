package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// publicPlans: daftar paket yang tampil di modal Upgrade (dikelola dari Convera Admin).
func (s *Server) publicPlans(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT code::text AS id, name, price_month::float8 AS price, price_12m::float8 AS price12, features
		FROM plans WHERE deleted_at IS NULL AND shown ORDER BY sort, created_at`)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, rows)
}

// publicStatus: mode maintenance + feature flag, dibaca webapp customer (diatur dari Convera Admin).
func (s *Server) publicStatus(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT
	  COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'maintenance'), false) AS maintenance,
	  COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'maintenance_message'), '') AS message`)
	if err != nil {
		serverError(c, err)
		return
	}
	flags, err := query(c, s.DB, `SELECT key, enabled, badge FROM feature_flags ORDER BY key`)
	if err != nil {
		serverError(c, err)
		return
	}
	rows[0]["flags"] = flags
	ok(c, rows[0])
}

// maintenanceGuard menolak aksi pembuatan (workspace baru, event/konversi manual, aturan baru) saat mode maintenance
// aktif. Customer tetap bisa memantau data (GET) dan data yang masuk otomatis tetap tercatat.
func (s *Server) maintenanceGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		var on bool
		if err := s.DB.QueryRow(c, `SELECT COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'maintenance'), false)`).Scan(&on); err != nil {
			serverError(c, err)
			return
		}
		if on {
			fail(c, http.StatusServiceUnavailable, "maintenance", "Maintenance Ongoing")
			return
		}
		c.Next()
	}
}
