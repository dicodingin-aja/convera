package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/core"
	"github.com/dicodingin-aja/convera/backend/internal/meta"
	"github.com/dicodingin-aja/convera/backend/internal/secure"
	"github.com/dicodingin-aja/convera/backend/internal/wa"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var digitsRe = regexp.MustCompile(`^[0-9]{5,25}$`)

// ---------- Koneksi Meta ----------

func (s *Server) getMetaConnection(c *gin.Context) {
	rows, err := query(c, s.DB, `
SELECT c.mode, c.pixel_id, c.access_token_hint, (c.access_token_enc IS NOT NULL) AS has_token,
       c.test_event_code, c.waba_id, c.phone_number_id, c.app_secret_hint, (c.app_secret_enc IS NOT NULL) AS has_app_secret,
       c.verify_token, c.status, c.last_error_code, c.last_check_at, c.token_expires_at, w.webhook_slug
FROM meta_connections c JOIN workspaces w ON w.id = c.workspace_id WHERE c.workspace_id = $1`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if len(rows) == 0 {
		fail(c, http.StatusNotFound, "no_connection", "Koneksi belum dibuat.")
		return
	}
	r := rows[0]
	r["webhook_url"] = s.Cfg.WebhookBase + "/cloud/" + r["webhook_slug"].(string)
	delete(r, "webhook_slug")
	r["allowed_events"] = meta.AllowedEvents(r["mode"].(string))
	ok(c, r)
}

type metaInput struct {
	Mode          *string `json:"mode"`
	PixelID       *string `json:"pixel_id"`
	AccessToken   *string `json:"access_token"` // kosong/tidak ada = tidak diubah
	TestEventCode *string `json:"test_event_code"`
	WABAID        *string `json:"waba_id"`
	PhoneNumberID *string `json:"phone_number_id"`
	AppSecret     *string `json:"app_secret"` // kosong/tidak ada = tidak diubah
	// TokenExpiresAt: tanggal berlaku token (YYYY-MM-DD atau RFC3339) bila Meta menyebutnya; "" = tanpa kedaluwarsa.
	// Dipakai untuk pengingat sebelum token habis.
	TokenExpiresAt *string `json:"token_expires_at"`
}

func (s *Server) putMetaConnection(c *gin.Context) {
	var in metaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	if in.Mode != nil {
		if *in.Mode != meta.ModeWABA && *in.Mode != meta.ModeQR {
			fail(c, http.StatusUnprocessableEntity, "validation_error", "Mode harus 'waba' atau 'qr'.")
			return
		}
		if *in.Mode == meta.ModeQR {
			var on bool
			_ = s.DB.QueryRow(c, `SELECT COALESCE((SELECT enabled FROM feature_flags WHERE key = 'qr'), false)`).Scan(&on)
			if !on {
				fail(c, http.StatusForbidden, "feature_disabled", "Mode WA Bisnis (scan QR) belum diaktifkan.")
				return
			}
		}
	}
	if in.PixelID != nil && *in.PixelID != "" && !digitsRe.MatchString(*in.PixelID) {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Pixel/Dataset ID hanya angka (5-25 digit).")
		return
	}
	for _, v := range []*string{in.WABAID, in.PhoneNumberID} {
		if v != nil && *v != "" && !digitsRe.MatchString(*v) {
			fail(c, http.StatusUnprocessableEntity, "validation_error", "WABA ID dan Phone Number ID hanya angka.")
			return
		}
	}

	var tokEnc, secEnc []byte
	var tokHint, secHint *string
	var err error
	if in.AccessToken != nil && strings.TrimSpace(*in.AccessToken) != "" {
		t := strings.TrimSpace(*in.AccessToken)
		if tokEnc, err = s.Box.SealString(t); err != nil {
			serverError(c, err)
			return
		}
		h := secure.Hint(t)
		tokHint = &h
	}
	if in.AppSecret != nil && strings.TrimSpace(*in.AppSecret) != "" {
		t := strings.TrimSpace(*in.AppSecret)
		if secEnc, err = s.Box.SealString(t); err != nil {
			serverError(c, err)
			return
		}
		h := secure.Hint(t)
		secHint = &h
	}

	var expAt *time.Time
	clearExp := false
	if in.TokenExpiresAt != nil {
		v := strings.TrimSpace(*in.TokenExpiresAt)
		if v == "" {
			clearExp = true
		} else {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				// YYYY-MM-DD dianggap akhir hari di Asia/Jakarta
				d, derr := time.ParseInLocation("2006-01-02", v, time.FixedZone("WIB", 7*3600))
				if derr != nil {
					fail(c, http.StatusUnprocessableEntity, "validation_error", "token_expires_at harus YYYY-MM-DD atau ISO 8601.")
					return
				}
				t = d.Add(24*time.Hour - time.Second)
			}
			if t.Before(time.Now()) || t.After(time.Now().AddDate(5, 0, 0)) {
				fail(c, http.StatusUnprocessableEntity, "validation_error", "token_expires_at harus di masa depan (maks. 5 tahun).")
				return
			}
			expAt = &t
		}
	}

	// Semua kolom opsional: NULL = biarkan nilai lama. Kredensial baru mereset status ke 'pending'.
	tag, err := s.DB.Exec(c, `
UPDATE meta_connections SET
  mode             = COALESCE($2, mode),
  pixel_id         = COALESCE(NULLIF($3, ''), pixel_id),
  access_token_enc = COALESCE($4, access_token_enc),
  access_token_hint= COALESCE($5, access_token_hint),
  test_event_code  = CASE WHEN $6::text IS NULL THEN test_event_code ELSE NULLIF($6, '') END,
  waba_id          = COALESCE(NULLIF($7, ''), waba_id),
  phone_number_id  = COALESCE(NULLIF($8, ''), phone_number_id),
  app_secret_enc   = COALESCE($9, app_secret_enc),
  app_secret_hint  = COALESCE($10, app_secret_hint),
  status           = CASE WHEN $4 IS NOT NULL
                          OR ($3::text IS NOT NULL AND NULLIF($3, '') IS DISTINCT FROM pixel_id)
                          OR ($2::text IS NOT NULL AND $2 <> mode) THEN 'pending' ELSE status END,
  last_error_code  = CASE WHEN $4 IS NOT NULL THEN NULL ELSE last_error_code END,
  token_expires_at = CASE WHEN $12 THEN NULL WHEN $11::timestamptz IS NULL THEN token_expires_at ELSE $11::timestamptz END
WHERE workspace_id = $1`,
		wsID(c), in.Mode, in.PixelID, nullBytes(tokEnc), tokHint, in.TestEventCode, in.WABAID, in.PhoneNumberID,
		nullBytes(secEnc), secHint, expAt, clearExp)
	if err != nil {
		serverError(c, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(c, http.StatusNotFound, "no_connection", "Koneksi belum dibuat.")
		return
	}
	s.getMetaConnection(c)
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// testMetaConnection mengirim satu event uji (wajib dengan test_event_code agar tidak mencemari data asli).
func (s *Server) testMetaConnection(c *gin.Context) {
	if !s.limiter("testcapi|"+wsID(c), 5.0/60, 5).Allow() { // bab 5.1: maks. 5 per menit
		fail(c, http.StatusTooManyRequests, "rate_limited", "Terlalu sering mengetes koneksi. Coba lagi sebentar.")
		return
	}
	var (
		mode                  string
		pixel, testCode, waba *string
		tokEnc                []byte
		prevStatus            string
	)
	err := s.DB.QueryRow(c, `SELECT mode, pixel_id, access_token_enc, test_event_code, waba_id, status
		FROM meta_connections WHERE workspace_id = $1`, wsID(c)).Scan(&mode, &pixel, &tokEnc, &testCode, &waba, &prevStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(c, http.StatusNotFound, "no_connection", "Koneksi belum dibuat.")
		return
	} else if err != nil {
		serverError(c, err)
		return
	}
	switch {
	case pixel == nil || len(tokEnc) == 0:
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Isi Pixel/Dataset ID dan access token dulu.")
		return
	case testCode == nil || *testCode == "":
		fail(c, http.StatusUnprocessableEntity, "validation_error",
			"Isi Test Event Code (Pengaturan lanjutan) agar event uji tidak masuk ke data asli. Ambil dari tab Test Events di Events Manager.")
		return
	case s.Cfg.GraphAPIVersion == "":
		fail(c, http.StatusServiceUnavailable, "graph_version_missing", "GRAPH_API_VERSION belum diisi di server.")
		return
	}
	token, err := s.Box.OpenString(tokEnc)
	if err != nil {
		fail(c, http.StatusUnprocessableEntity, "decrypt_failed", "Token tersimpan tidak bisa dibaca. Simpan ulang token.")
		return
	}

	// Data uji murni sintetis; tidak memakai data lead.
	eid, _ := secure.RandomToken(8)
	in := meta.Input{Mode: mode, EventID: "cv_test_" + eid, EventTime: time.Now()}
	if mode == meta.ModeWABA {
		in.EventName, in.CtwaClid = "LeadSubmitted", "convera-test-ctwa-clid"
		if waba != nil {
			in.WABAID = *waba
		}
	} else {
		in.EventName, in.Phone, in.Name = "Lead", "6280000000000", "Convera Test"
	}
	ev, err := meta.BuildEvent(in)
	if err != nil {
		serverError(c, err)
		return
	}
	payload, _ := json.Marshal(ev)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	res, err := s.Meta.Send(ctx, *pixel, token, *testCode, []map[string]any{ev})
	if err != nil {
		fail(c, http.StatusBadGateway, "meta_unreachable", "Tidak bisa menghubungi Meta. Coba lagi.")
		return
	}

	status, msg, code := "sent", "Event uji terkirim. Cek tab Test Events di Events Manager.", ""
	conn := "connected"
	if !res.OK() {
		v := meta.Classify(res)
		status, msg, conn = "failed", v.Message, "error"
		if v.InvalidAuth {
			conn = "invalid"
		}
		code = "meta_" + strconv.Itoa(res.ErrCode)
	}
	_, _ = s.DB.Exec(c, `
INSERT INTO events (workspace_id, event_id, event_name, source, status, mode, request_payload, meta_response,
                    fbtrace_id, error_code, error_message, sent_at, next_retry_at)
VALUES ($1,$2,$3,'test',$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),CASE WHEN $4='failed' THEN $10 END,
        CASE WHEN $4='sent' THEN now() END, NULL)`,
		wsID(c), in.EventID, in.EventName, status, mode, payload, res.Body, res.FBTraceID, code, msg)
	_, _ = s.DB.Exec(c, `UPDATE meta_connections SET status=$2, last_error_code=NULLIF($3,''), last_check_at=now() WHERE workspace_id=$1`,
		wsID(c), conn, strings.TrimPrefix(code, "meta_"))
	if conn == "invalid" && prevStatus != "invalid" {
		_, _ = s.DB.Exec(c, `INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
			SELECT owner_user_id, id, 'token_invalid', 'Token Meta tidak valid', $2, '/koneksi' FROM workspaces WHERE id = $1`, wsID(c), msg)
	}
	ok(c, gin.H{"ok": res.OK(), "message": msg, "fbtrace_id": res.FBTraceID, "status": conn})
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// ---------- Mode QR: proxy ke layanan sesi WhatsApp Web ----------

var errQRUnavailable = errors.New("layanan sesi WhatsApp belum tersedia")

func (s *Server) qrCall(c *gin.Context, method, path string, body any) (map[string]any, error) {
	if s.Cfg.QRServiceURL == "" {
		return nil, errQRUnavailable
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(mustJSON(body))
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), method, s.Cfg.QRServiceURL+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Service-Token", s.Cfg.QRServiceToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, errQRUnavailable
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode >= 300 {
		return nil, errQRUnavailable
	}
	return out, nil
}

func (s *Server) qrGate(c *gin.Context) bool {
	var mode string
	var on bool
	_ = s.DB.QueryRow(c, `SELECT COALESCE((SELECT enabled FROM feature_flags WHERE key='qr'), false)`).Scan(&on)
	if !on {
		fail(c, http.StatusForbidden, "feature_disabled", "Mode WA Bisnis (scan QR) belum diaktifkan.")
		return false
	}
	_ = s.DB.QueryRow(c, `SELECT mode FROM meta_connections WHERE workspace_id=$1`, wsID(c)).Scan(&mode)
	if mode != meta.ModeQR {
		fail(c, http.StatusUnprocessableEntity, "validation_error", "Ubah mode koneksi ke 'qr' dulu.")
		return false
	}
	return true
}

func (s *Server) qrStart(c *gin.Context) {
	if !s.qrGate(c) {
		return
	}
	// Pixel ID dan token CAPI harus sudah tersimpan sebelum QR dibuat, agar chat pertama langsung bisa dikirim ke Meta.
	var ready bool
	_ = s.DB.QueryRow(c, `SELECT COALESCE(pixel_id <> '' AND access_token_enc IS NOT NULL, false) FROM meta_connections WHERE workspace_id=$1`, wsID(c)).Scan(&ready)
	if !ready {
		fail(c, http.StatusUnprocessableEntity, "connection_incomplete", "Simpan Pixel ID dan Access Token CAPI dulu di halaman Koneksi, baru hubungkan WhatsApp.")
		return
	}
	out, err := s.qrCall(c, http.MethodPost, "/sessions/"+wsID(c)+"/start", nil)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "qr_unavailable", err.Error())
		return
	}
	_, _ = s.DB.Exec(c, `INSERT INTO wa_sessions (workspace_id, status) VALUES ($1,'pending')
		ON CONFLICT (workspace_id) DO UPDATE SET status='pending'`, wsID(c))
	st, _ := out["status"].(string)
	if st == "" {
		st = "pending"
	}
	if st == "connected" {
		_, _ = s.DB.Exec(c, `UPDATE wa_sessions SET status='connected' WHERE workspace_id=$1`, wsID(c))
	}
	ok(c, gin.H{"qr": out["qr"], "qr_image": out["qr_image"], "status": st})
}

func (s *Server) qrStatus(c *gin.Context) {
	rows, err := query(c, s.DB, `SELECT status, phone_masked, last_seen_at FROM wa_sessions WHERE workspace_id=$1`, wsID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	if len(rows) == 0 {
		ok(c, gin.H{"status": "disconnected", "phone_masked": nil, "last_seen_at": nil})
		return
	}
	// Selalu baca status layanan sesi. Status database bisa tertinggal ketika koneksi
	// putus sebelum layanan sempat mengirim laporan ke API.
	if out, err := s.qrCall(c, http.MethodGet, "/sessions/"+wsID(c), nil); err == nil {
		if qr, ok := out["qr"]; ok {
			rows[0]["qr"] = qr
		}
		if image, ok := out["qr_image"]; ok {
			rows[0]["qr_image"] = image
		}
		if st, _ := out["status"].(string); st == "connected" || st == "disconnected" || st == "pending" {
			rows[0]["status"] = st
			if st == "disconnected" {
				_, _ = s.DB.Exec(c, `UPDATE wa_sessions SET status='disconnected', last_seen_at=now() WHERE workspace_id=$1`, wsID(c))
			}
		}
	}
	ok(c, rows[0])
}

func (s *Server) qrDisconnect(c *gin.Context) {
	if _, err := s.qrCall(c, http.MethodDelete, "/sessions/"+wsID(c), nil); err != nil && !errors.Is(err, errQRUnavailable) {
		serverError(c, err)
		return
	}
	_, _ = s.DB.Exec(c, `UPDATE wa_sessions SET status='disconnected', session_data_enc=NULL, phone_masked=NULL, phone_enc=NULL WHERE workspace_id=$1`, wsID(c))
	ok(c, gin.H{"status": "disconnected"})
}

// serviceAuth: layanan sesi QR memakai header X-Service-Token (dibandingkan constant-time).
func (s *Server) serviceAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := c.GetHeader("X-Service-Token")
		if s.Cfg.QRServiceToken == "" || subtle.ConstantTimeCompare([]byte(t), []byte(s.Cfg.QRServiceToken)) != 1 {
			fail(c, http.StatusUnauthorized, "unauthorized", "Token layanan tidak valid.")
			return
		}
		var id string
		err := s.DB.QueryRow(c, `SELECT id::text FROM workspaces WHERE id::text = $1 AND deleted_at IS NULL`, c.Param("workspace_id")).Scan(&id)
		if err != nil {
			fail(c, http.StatusNotFound, "workspace_not_found", "Workspace tidak ditemukan.")
			return
		}
		c.Set("workspace_id", id)
		c.Next()
	}
}

type qrMessage struct {
	ID         string `json:"id"`
	Direction  string `json:"direction"` // in | out
	Contact    string `json:"contact"`
	PushName   string `json:"push_name"`
	Timestamp  int64  `json:"timestamp"`
	Text       string `json:"text"`
	IsTemplate bool   `json:"is_template"`
	// externalAdReply (tidak resmi): semua field boleh kosong.
	Ad *struct {
		AdID      string `json:"ad_id"`
		Headline  string `json:"headline"`
		ClickID   string `json:"click_id"`
		SourceURL string `json:"source_url"`
	} `json:"ad"`
}

// qrIngestMessage menerima pesan dari layanan sesi QR. Isi pesan hanya dipakai untuk mencocokkan aturan.
func (s *Server) qrIngestMessage(c *gin.Context) {
	var in qrMessage
	if err := c.ShouldBindJSON(&in); err != nil || in.ID == "" {
		fail(c, http.StatusBadRequest, "bad_request", "Body tidak valid.")
		return
	}
	phone, err := wa.NormalizePhone(in.Contact)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"skipped": "nomor tidak valid (mis. grup)"}})
		return
	}
	if in.Direction != "in" && in.Direction != "out" {
		fail(c, http.StatusBadRequest, "bad_request", "direction harus 'in' atau 'out'.")
		return
	}
	at := time.Now().UTC()
	if in.Timestamp > 0 {
		at = time.Unix(in.Timestamp, 0).UTC()
	}
	m := core.Message{Provider: "qr", ID: in.ID, Direction: in.Direction, Contact: phone, PushName: in.PushName, At: at,
		Text: in.Text, IsTemplate: in.IsTemplate, LeadSource: "qr"}
	if in.Direction == "in" && in.Ad != nil {
		m.CtwaClid, m.AdID, m.Headline, m.SourceURL = in.Ad.ClickID, in.Ad.AdID, in.Ad.Headline, in.Ad.SourceURL
	}
	processed, err := s.processTx(c, c.GetString("workspace_id"), m)
	if err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{"processed": processed})
}

// qrStatusUpdate: layanan QR melaporkan status sesi (connected/disconnected) dan nomor tersamar.
func (s *Server) qrStatusUpdate(c *gin.Context) {
	var in struct {
		Status string `json:"status"`
		Phone  string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || (in.Status != "connected" && in.Status != "disconnected" && in.Status != "pending") {
		fail(c, http.StatusBadRequest, "bad_request", "status harus connected/disconnected/pending.")
		return
	}
	masked := ""
	var phoneEnc []byte
	if p, err := wa.NormalizePhone(in.Phone); err == nil {
		masked = wa.MaskPhone(p)
		if s.Box != nil {
			phoneEnc, err = s.Box.SealString(p)
			if err != nil {
				serverError(c, err)
				return
			}
		}
	}
	id := c.GetString("workspace_id")
	var prev string
	_ = s.DB.QueryRow(c, `SELECT status FROM wa_sessions WHERE workspace_id=$1`, id).Scan(&prev)
	if _, err := s.DB.Exec(c, `
INSERT INTO wa_sessions (workspace_id, status, phone_masked, phone_enc, last_seen_at) VALUES ($1,$2,NULLIF($3,''),$4,now())
ON CONFLICT (workspace_id) DO UPDATE SET status=$2, phone_masked=COALESCE(NULLIF($3,''), wa_sessions.phone_masked), phone_enc=COALESCE($4, wa_sessions.phone_enc), last_seen_at=now()`,
		id, in.Status, masked, phoneEnc); err != nil {
		serverError(c, err)
		return
	}
	if in.Status == "disconnected" && prev == "connected" {
		_, _ = s.DB.Exec(c, `INSERT INTO notifications (user_id, workspace_id, type, title, body, link)
			SELECT owner_user_id, id, 'wa_disconnected', 'Sesi WhatsApp terputus',
			       'Scan QR ulang di halaman Koneksi agar chat kembali tercatat.', '/koneksi' FROM workspaces WHERE id=$1`, id)
	}
	ok(c, gin.H{"status": in.Status})
}

// processTx memproses satu pesan dalam transaksi sendiri.
func (s *Server) processTx(ctx context.Context, workspaceID string, m core.Message) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	done, err := core.ProcessMessage(ctx, tx, workspaceID, m)
	if err != nil {
		return false, err
	}
	return done, tx.Commit(ctx)
}
