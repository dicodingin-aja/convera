package server

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/dicodingin-aja/convera/backend/internal/wa"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type webhookConn struct {
	workspaceID string
	mode        string
	verifyToken *string
	secretEnc   []byte
}

func (s *Server) loadWebhookConn(c *gin.Context) (*webhookConn, bool) {
	var w webhookConn
	err := s.DB.QueryRow(c, `
SELECT w.id::text, mc.mode, mc.verify_token, mc.app_secret_enc
FROM workspaces w JOIN meta_connections mc ON mc.workspace_id = w.id
WHERE w.webhook_slug = $1 AND w.deleted_at IS NULL`, c.Param("slug")).Scan(&w.workspaceID, &w.mode, &w.verifyToken, &w.secretEnc)
	if errors.Is(err, pgx.ErrNoRows) {
		c.Status(http.StatusNotFound)
		return nil, false
	} else if err != nil {
		_ = c.Error(err)
		c.Status(http.StatusInternalServerError)
		return nil, false
	}
	return &w, true
}

// webhookVerify: GET verifikasi Meta. hub.mode=subscribe dan hub.verify_token cocok → balas hub.challenge (plain text).
func (s *Server) webhookVerify(c *gin.Context) {
	w, found := s.loadWebhookConn(c)
	if !found {
		return
	}
	tok := c.Query("hub.verify_token")
	if c.Query("hub.mode") != "subscribe" || w.verifyToken == nil || *w.verifyToken == "" ||
		subtle.ConstantTimeCompare([]byte(tok), []byte(*w.verifyToken)) != 1 {
		c.Status(http.StatusForbidden)
		return
	}
	c.String(http.StatusOK, "%s", c.Query("hub.challenge"))
}

// webhookReceive: POST pesan masuk (mode WABA).
// 1) verifikasi X-Hub-Signature-256 terhadap RAW body, 2) proses cepat di memori, 3) jawab 200.
// Isi pesan tidak disimpan maupun di-log. Kegagalan DB dijawab 500 agar Meta mengulang; pesan yang sama
// tidak diproses dua kali (webhook_inbox).
func (s *Server) webhookReceive(c *gin.Context) {
	w, found := s.loadWebhookConn(c)
	if !found {
		return
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	if w.mode != meta.ModeWABA {
		c.Status(http.StatusConflict) // workspace ini tidak memakai webhook WABA
		return
	}
	// App Secret opsional. Bila diisi (webhook langsung dari Meta), tanda tangan wajib valid.
	// Bila kosong (webhook diteruskan CRM/penyedia WABA seperti kirim.chat yang tidak menandatangani
	// dengan app secret kita), kerahasiaan URL (UUID acak 122 bit) yang menjadi pengaman.
	if len(w.secretEnc) > 0 {
		secret, err := s.Box.OpenString(w.secretEnc)
		if err != nil {
			_ = c.Error(errors.New("app secret tidak bisa didekripsi"))
			c.Status(http.StatusInternalServerError)
			return
		}
		if !wa.VerifySignature(raw, c.GetHeader("X-Hub-Signature-256"), secret) {
			c.Status(http.StatusUnauthorized)
			return
		}
	}
	msgs, err := wa.ParseCloudWebhook(raw)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	for _, m := range msgs {
		phone, perr := wa.NormalizePhone(m.From)
		if perr != nil {
			continue
		}
		cm := core.Message{Provider: "cloud_api", ID: m.ID, Direction: "in", Contact: phone, Name: m.Name,
			At: m.Timestamp, Text: m.Text, LeadSource: "webhook"}
		if m.Referral.IsAd() {
			cm.CtwaClid, cm.AdID, cm.Headline, cm.SourceURL = m.Referral.CtwaClid, m.Referral.SourceID, m.Referral.Headline, m.Referral.SourceURL
		}
		if _, err := s.processTx(c, w.workspaceID, cm); err != nil {
			_ = c.Error(err)
			c.Status(http.StatusInternalServerError)
			return
		}
	}
	c.Status(http.StatusOK)
}

// ---------- API publik (Bearer API key) ----------

func (s *Server) apiKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := bearer(c)
		if key == "" {
			fail(c, http.StatusUnauthorized, "unauthenticated", "Sertakan header Authorization: Bearer <api_key>.")
			return
		}
		var keyID, workspaceID string
		err := s.DB.QueryRow(c, `
SELECT k.id::text, k.workspace_id::text FROM api_keys k JOIN workspaces w ON w.id = k.workspace_id
WHERE k.key_hash = $1 AND k.revoked_at IS NULL AND w.deleted_at IS NULL`, hashKey(key)).Scan(&keyID, &workspaceID)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(c, http.StatusUnauthorized, "unauthenticated", "API key tidak valid atau sudah dicabut.")
			return
		} else if err != nil {
			serverError(c, err)
			return
		}
		// 10 permintaan/detik per key, burst 30.
		if !s.limiter("key|"+keyID, 10, 30).Allow() {
			c.Header("Retry-After", "1")
			fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak permintaan.")
			return
		}
		_, _ = s.DB.Exec(c, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, keyID)
		c.Set("workspace_id", workspaceID)
		c.Set("api_key_id", keyID)
		c.Next()
	}
}

// POST /v1/leads — forward dari sistem klien (BSP/bot sendiri).
func (s *Server) publicCreateLead(c *gin.Context) {
	var in struct {
		Phone     string `json:"phone"`
		Name      string `json:"name"`
		CtwaClid  string `json:"ctwa_clid"`
		AdID      string `json:"ad_id"`
		Headline  string `json:"headline"`
		ClickedAt string `json:"clicked_at"`
		City      string `json:"city"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	phone, err := wa.NormalizePhone(in.Phone)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "phone tidak valid.")
		return
	}
	var clicked time.Time
	if in.ClickedAt != "" {
		if clicked, err = time.Parse(time.RFC3339, in.ClickedAt); err != nil {
			fail(c, http.StatusUnprocessableEntity, "validation_error", "clicked_at harus berformat ISO 8601, mis. 2026-11-05T10:00:00+07:00.")
			return
		}
		clicked = clicked.UTC()
	}
	id, err := core.UpsertLead(c, s.DB, core.LeadInput{
		WorkspaceID: wsID(c), Phone: phone, Name: in.Name, City: in.City, CtwaClid: in.CtwaClid, AdID: in.AdID,
		Headline: in.Headline, Source: "api", ClickedAt: clicked, MessageAt: time.Now().UTC(),
	})
	if isUnique(err) {
		fail(c, http.StatusConflict, "ctwa_clid_taken", "ctwa_clid ini sudah dipakai lead lain.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	okStatus(c, http.StatusCreated, gin.H{"id": id})
}

// POST /v1/events — pemicu konversi dari sistem klien.
func (s *Server) publicCreateEvent(c *gin.Context) {
	var in struct {
		EventName  string   `json:"event_name"`
		Phone      string   `json:"phone"`
		CtwaClid   string   `json:"ctwa_clid"`
		Name       string   `json:"name"`
		City       string   `json:"city"`
		Value      *float64 `json:"value"`
		Currency   string   `json:"currency"`
		ExternalID string   `json:"external_id"`
		EventTime  string   `json:"event_time"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if in.EventName == "" || (in.Phone == "" && in.CtwaClid == "") {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "event_name dan salah satu dari phone / ctwa_clid wajib diisi.")
		return
	}
	var evTime time.Time
	if in.EventTime != "" {
		t, err := time.Parse(time.RFC3339, in.EventTime)
		if err != nil {
			fail(c, http.StatusUnprocessableEntity, "validation_error", "event_time harus berformat ISO 8601.")
			return
		}
		evTime = t.UTC()
	}

	// Cari lead: ctwa_clid dulu (jika dikirim), lalu nomor; bila banyak pilih yang terbaru.
	phone := ""
	if in.Phone != "" {
		p, err := wa.NormalizePhone(in.Phone)
		if err != nil {
			fail(c, http.StatusUnprocessableEntity, "validation_error", "phone tidak valid.")
			return
		}
		phone = p
	}
	var leadID string
	err := s.DB.QueryRow(c, `
SELECT id::text FROM leads
WHERE workspace_id = $1 AND deleted_at IS NULL
  AND (($2 <> '' AND ctwa_clid = $2) OR ($3 <> '' AND phone = $3))
ORDER BY (ctwa_clid = NULLIF($2, '')) DESC NULLS LAST, COALESCE(last_message_at, first_message_at) DESC LIMIT 1`,
		wsID(c), in.CtwaClid, phone).Scan(&leadID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, http.StatusNotFound, "lead_not_found", "Lead tidak ditemukan. Kirim lead lewat POST /v1/leads atau pastikan nomor/ctwa_clid benar.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	if in.Name != "" || in.City != "" {
		_, _ = s.DB.Exec(c, `UPDATE leads SET name = COALESCE(name, NULLIF($2,'')), city = COALESCE(NULLIF($3,''), city) WHERE id = $1`,
			leadID, in.Name, in.City)
	}
	out, err := core.CreateConversion(c, s.DB, core.Params{
		WorkspaceID: wsID(c), LeadID: leadID, EventName: in.EventName, Value: in.Value, Currency: in.Currency,
		Source: "api", ExternalID: in.ExternalID, EventTime: evTime,
	})
	if coreError(c, err) {
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	okStatus(c, http.StatusAccepted, out)
}
