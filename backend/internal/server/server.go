// Package server berisi REST API (Gin): auth, koneksi Meta, webhook, API publik, dan endpoint dashboard.
package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/ads"
	"github.com/dicodingin-aja/convera/backend/internal/config"
	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/internal/upload"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

type Server struct {
	Cfg    *config.Config
	DB     *pgxpool.Pool
	Box    *secure.Box
	Meta   *meta.Client
	Files  *upload.Store
	Syncer *ads.Syncer

	limMu    sync.Mutex
	limiters map[string]*rate.Limiter
}

func New(cfg *config.Config, db *pgxpool.Pool, box *secure.Box) *Server {
	return &Server{
		Cfg: cfg, DB: db, Box: box,
		Meta:     &meta.Client{BaseURL: cfg.GraphBaseURL, Version: cfg.GraphAPIVersion},
		Files:    &upload.Store{Dir: cfg.UploadDir},
		Syncer:   &ads.Syncer{DB: db, Box: box, Client: &ads.Client{BaseURL: cfg.GraphBaseURL, Version: cfg.GraphAPIVersion}},
		limiters: map[string]*rate.Limiter{},
	}
}

// Router membangun semua route.
func (s *Server) Router() *gin.Engine {
	r := gin.New()
	if err := r.SetTrustedProxies(s.Cfg.TrustedProxies); err != nil {
		log.Printf("TRUSTED_PROXIES tidak valid: %v", err)
	}
	r.Use(gin.Recovery(), s.cors(), limitBody(1<<20))
	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil {
			fail(c, http.StatusServiceUnavailable, "db_down", "database tidak terjangkau")
			return
		}
		ok(c, gin.H{"status": "ok"})
	})

	// Webhook WhatsApp (tanpa sesi; diamankan signature). Dua alias URL.
	for _, p := range []string{"/webhook/cloud/:slug", "/webhooks/whatsapp/:slug"} {
		r.GET(p, s.webhookVerify)
		r.POST(p, s.webhookReceive)
	}

	// Layanan sesi WhatsApp Web (mode QR) memanggil ini dengan X-Service-Token.
	in := r.Group("/internal/qr/:workspace_id", s.serviceAuth())
	in.POST("/messages", s.qrIngestMessage)
	in.POST("/status", s.qrStatusUpdate)

	// API publik untuk sistem klien (Bearer API key), dibatasi per key.
	pub := r.Group("/v1", s.apiKeyAuth())
	pub.POST("/leads", s.publicCreateLead)
	pub.POST("/events", s.publicCreateEvent)

	api := r.Group("/api/v1")
	api.GET("/plans", s.publicPlans)
	api.GET("/public/status", s.publicStatus)
	api.GET("/status", s.publicStatus)
	api.POST("/auth/support-login", s.rateLimitIP(10), s.supportLogin)
	api.POST("/auth/register", s.rateLimitIP(5), s.register)
	api.POST("/auth/login", s.rateLimitIP(10), s.login)

	authed := api.Group("", s.requireUser(), s.supportGuard(), s.csrf())
	authed.POST("/auth/logout", s.logout)
	authed.GET("/auth/session", s.session)
	authed.POST("/auth/refresh", s.refresh)
	authed.GET("/auth/csrf", s.csrfToken)
	authed.GET("/me/profile", s.getProfile)
	authed.PATCH("/me/profile", s.patchProfile)
	authed.POST("/me/password", s.changePassword)
	authed.GET("/me/sessions", s.listSessions)
	authed.DELETE("/me/sessions/:id", s.revokeSession)
	authed.GET("/me/audit-logs", s.listUserAuditLogs)
	authed.DELETE("/me", s.deleteAccount)
	authed.POST("/presence/heartbeat", s.heartbeat)
	authed.GET("/notifications", s.listNotifications)
	authed.POST("/notifications/read-all", s.readAllNotifications)
	authed.POST("/notifications/:id/read", s.readNotification)
	authed.GET("/workspaces/:slug", s.getWorkspace)
	authed.PATCH("/workspaces/:slug", s.patchWorkspace)
	authed.DELETE("/workspaces/:slug", s.deleteWorkspace)
	authed.GET("/workspaces", s.listWorkspaces)
	authed.POST("/workspaces", s.createWorkspace)

	ws := authed.Group("", s.withWorkspace())
	ws.GET("/subscription", s.mySubscription)
	ws.GET("/bank-accounts", s.customerBanks)
	ws.GET("/payment-proofs", s.listMyProofs)
	ws.POST("/payment-proofs", s.uploadProof)
	ws.GET("/overview", s.overview)
	ws.GET("/performance", s.performance)
	ws.GET("/reports/daily", s.dailyReport)
	ws.GET("/leads", s.listLeads)
	ws.GET("/leads/:id", s.getLead)
	ws.POST("/leads/:id/conversions", s.manualConversion)
	ws.POST("/leads/bulk-status", s.bulkLeadStatus)
	ws.POST("/leads/erase", s.eraseContact)
	ws.POST("/leads/:id/mark-purchase", s.markPurchase)
	ws.GET("/conversions", s.listEvents) // sama dengan /events tetapi tanpa event uji
	ws.GET("/events", s.listEvents)
	ws.GET("/events.csv", s.exportEvents)
	ws.GET("/events/:id", s.getEvent)
	ws.POST("/events/:id/retry", s.retryEvent)
	ws.PUT("/events/:id/value", s.setEventValue)
	ws.GET("/connections/meta", s.getMetaConnection)
	ws.PUT("/connections/meta", s.putMetaConnection)
	ws.POST("/connections/meta/test", s.testMetaConnection)
	ws.POST("/connections/test-capi", s.testMetaConnection)
	ws.GET("/connections/ads", s.getAdsConnection)
	ws.PUT("/connections/ads", s.putAdsConnection)
	ws.POST("/connections/ads/sync", s.syncAds)
	ws.DELETE("/connections/ads", s.deleteAdsConnection)
	ws.POST("/connections/whatsapp/qr", s.qrStart)
	ws.GET("/connections/whatsapp/status", s.qrStatus)
	ws.DELETE("/connections/whatsapp", s.qrDisconnect)
	ws.GET("/event-mappings", s.listRules)
	ws.POST("/event-mappings", s.createRule)
	ws.PUT("/event-mappings/:id", s.updateRule)
	ws.DELETE("/event-mappings/:id", s.deleteRule)
	ws.POST("/event-mappings/test", s.testRule)
	// Nama resmi di dokumen (bab 5.1): event-rules
	ws.GET("/event-rules", s.listRules)
	ws.POST("/event-rules", s.createRule)
	ws.PUT("/event-rules/:id", s.updateRule)
	ws.DELETE("/event-rules/:id", s.deleteRule)
	ws.POST("/event-rules/simulate", s.testRule)
	ws.GET("/api-keys", s.listAPIKeys)
	ws.POST("/api-keys", s.createAPIKey)
	ws.DELETE("/api-keys/:id", s.revokeAPIKey)
	return r
}

// ---------- Format respons: { data, meta } / { message, code } ----------

func ok(c *gin.Context, data any) { c.JSON(http.StatusOK, gin.H{"data": data}) }

func okStatus(c *gin.Context, status int, data any) { c.JSON(status, gin.H{"data": data}) }

func okPage(c *gin.Context, data any, page, per, total int) {
	c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "per_page": per, "total": total}})
}

func fail(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"message": msg, "code": code})
}

func serverError(c *gin.Context, err error) {
	// Jangan kirim detail error internal ke klien (bisa berisi data); cukup tulis ke log server.
	_ = c.Error(err)
	fail(c, http.StatusInternalServerError, "internal_error", "Terjadi kesalahan di server.")
}

// coreError memetakan error domain ke HTTP; mengembalikan false bila bukan error domain.
func coreError(c *gin.Context, err error) bool {
	var ve *core.ValidationError
	switch {
	case err == nil:
		return false
	case asValidation(err, &ve):
		fail(c, http.StatusUnprocessableEntity, "validation_error", ve.Msg)
	case err == core.ErrLeadNotFound:
		fail(c, http.StatusNotFound, "lead_not_found", "Lead tidak ditemukan.")
	case err == core.ErrNoConnection:
		fail(c, http.StatusUnprocessableEntity, "no_connection", "Koneksi Meta belum diatur. Isi dulu di halaman Koneksi.")
	default:
		return false
	}
	return true
}

// ---------- Middleware umum ----------

func (s *Server) cors() gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, o := range s.Cfg.AllowedOrigins {
		allowed[o] = true
	}
	return func(c *gin.Context) {
		if o := c.GetHeader("Origin"); o != "" && allowed[o] {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Workspace, Authorization")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Add("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := n
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/v1/payment-proofs" {
			limit = upload.MaxSize + 1<<20 // berkas bukti transfer + field teks
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// limiter mengembalikan token-bucket untuk kunci tertentu (IP atau API key).
func (s *Server) limiter(key string, perSec float64, burst int) *rate.Limiter {
	s.limMu.Lock()
	defer s.limMu.Unlock()
	l, found := s.limiters[key]
	if !found {
		if len(s.limiters) > 50000 { // batasi memori
			s.limiters = map[string]*rate.Limiter{}
		}
		l = rate.NewLimiter(rate.Limit(perSec), burst)
		s.limiters[key] = l
	}
	return l
}

// rateLimitIP membatasi per IP: perMinute permintaan per menit.
func (s *Server) rateLimitIP(perMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.FullPath() + "|" + c.ClientIP()
		if !s.limiter(key, float64(perMinute)/60, perMinute).Allow() {
			fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan. Coba lagi sebentar.")
			return
		}
		c.Next()
	}
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
