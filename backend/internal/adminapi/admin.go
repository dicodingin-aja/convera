// Package adminapi adalah API internal Convera Admin (/api/admin/v1). Memakai database yang sama dengan
// API customer tetapi berjalan sebagai proses terpisah dengan sesi, cookie, dan hak akses sendiri.
package adminapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/dicodingin-aja/convera/backend/internal/httpx"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/internal/upload"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	Cfg     *config.Config
	DB      *pgxpool.Pool
	Box     *secure.Box
	Files   *upload.Store
	Started time.Time
	lim     httpx.Limiters
}

func New(cfg *config.Config, db *pgxpool.Pool, box *secure.Box) *Server {
	return &Server{Cfg: cfg, DB: db, Box: box, Files: &upload.Store{Dir: cfg.UploadDir}, Started: time.Now()}
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	if err := r.SetTrustedProxies(s.Cfg.TrustedProxies); err != nil {
		log.Printf("TRUSTED_PROXIES tidak valid: %v", err)
	}
	r.Use(gin.Recovery(), httpx.CORS(s.Cfg.AllowedOrigins), httpx.LimitBody(1<<20))
	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil {
			httpx.Fail(c, http.StatusServiceUnavailable, "db_down", "database tidak terjangkau")
			return
		}
		httpx.OK(c, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/admin/v1")
	v1.POST("/auth/login", s.login)

	a := v1.Group("", s.requireAdmin(), s.csrf())
	a.POST("/auth/logout", s.logout)
	a.GET("/auth/session", s.session)

	a.GET("/overview", s.overview)

	a.GET("/customers", s.listCustomers)
	a.GET("/customers.csv", s.exportCustomers)
	a.GET("/customers/:id", s.getCustomer)
	a.POST("/customers/:id/extend", s.extendCustomer)
	a.POST("/customers/:id/stop", s.stopCustomer)
	a.POST("/customers/:id/support-access", s.requireSuper(), s.supportAccess)
	a.POST("/customers/:id/reset-password", s.requireSuper(), s.resetCustomerPassword)

	a.GET("/events", s.listEvents)
	a.GET("/events/:id", s.getEvent)
	a.POST("/events/:id/retry", s.retryEvent)
	a.GET("/customers/:id/events", s.listEvents)

	a.GET("/payment-proofs", s.listProofs)
	a.GET("/payment-proofs/:id", s.getProof)
	a.GET("/payment-proofs/:id/file", s.proofFile)
	a.POST("/payment-proofs/:id/approve", s.approveProof)
	a.POST("/payment-proofs/:id/reject", s.rejectProof)

	a.GET("/revenue-share", s.revenueShare)
	a.GET("/revenue-share.csv", s.exportRevenueShare)
	a.POST("/revenue-share/:month/:person/mark-paid", s.markPaid)

	a.GET("/plans", s.listPlans)
	a.POST("/plans", s.createPlan)
	a.PUT("/plans/:id", s.updatePlan)
	a.DELETE("/plans/:id", s.deletePlan)
	a.GET("/bank-accounts", s.listBanks)
	a.POST("/bank-accounts", s.createBank)
	a.PUT("/bank-accounts/:id", s.updateBank)
	a.DELETE("/bank-accounts/:id", s.deleteBank)
	a.GET("/sales-people", s.listSales)
	a.POST("/sales-people", s.createSales)
	a.PUT("/sales-people/:id", s.updateSales)
	a.DELETE("/sales-people/:id", s.deleteSales)

	a.GET("/subscription-rules", s.getRules)
	a.PUT("/subscription-rules", s.requireSuper(), s.putRules)
	a.GET("/feature-flags", s.listFlags)
	a.PUT("/feature-flags/:key", s.putFlag)
	a.GET("/maintenance", s.getMaintenance)
	a.PUT("/maintenance", s.requireSuper(), s.putMaintenance)
	a.GET("/integrations", s.getIntegrations)
	a.PUT("/integrations", s.putIntegrations)

	a.GET("/api-keys", s.requireSuper(), s.listKeys)
	a.POST("/api-keys", s.requireSuper(), s.createKey)
	a.DELETE("/api-keys/:id", s.requireSuper(), s.revokeKey)
	a.GET("/audit-logs", s.listAudit)
	a.GET("/system-status", s.systemStatus)
	a.GET("/notifications", s.listNotifications)
	a.POST("/notifications/read-all", s.readAllNotifications)
	return r
}

// audit menulis catatan aktivitas admin. Dipanggil di dalam transaksi bila tindakannya mengubah data,
// supaya perubahan dan catatannya berhasil atau gagal bersama. detail tidak boleh berisi rahasia.
func (s *Server) audit(c *gin.Context, db httpx.DBTX, action, workspaceID, targetType, targetID string, detail map[string]any) error {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, _ := json.Marshal(detail)
	_, err := db.Exec(c, `
INSERT INTO audit_logs (actor_type, actor_id, actor_label, workspace_id, action, target_type, target_id, detail, ip)
VALUES ('admin', $1::uuid, $2, NULLIF($3,'')::uuid, $4, NULLIF($5,''), NULLIF($6,''), $7, NULLIF($8,'')::inet)`,
		c.GetString("admin_id"), c.GetString("admin_name"), workspaceID, action, targetType, targetID, raw, c.ClientIP())
	return err
}

func adminID(c *gin.Context) string { return c.GetString("admin_id") }
