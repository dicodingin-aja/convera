// Package waqr adalah layanan sesi WhatsApp Web (mode QR): menautkan nomor WhatsApp Bisnis pelanggan sebagai
// "perangkat tertaut", membaca pesan masuk/keluar, lalu meneruskannya ke API Convera (/internal/qr/*).
//
// Catatan: ini memakai protokol WhatsApp Web tidak resmi (library whatsmeow). Layanan ini hanya MEMBACA pesan;
// tidak pernah mengirim pesan. Isi pesan tidak disimpan di sini, hanya diteruskan untuk pencocokan aturan.
package waqr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Manager memegang satu klien WhatsApp per workspace.
type Manager struct {
	DB        *pgxpool.Pool
	Store     *sqlstore.Container
	APIURL    string // basis API Convera, mis. http://api:8080
	Token     string // X-Service-Token
	HTTP      *http.Client
	Log       waLog.Logger
	mu        sync.Mutex
	sessions  map[string]*session
	ctx       context.Context
	startedAt time.Time
}

type session struct {
	ws     string
	client *whatsmeow.Client
	mu     sync.Mutex
	qr     string // kode QR terbaru (mentah)
	status string // pending | connected | disconnected
	cancel context.CancelFunc
}

func NewManager(ctx context.Context, db *pgxpool.Pool, store *sqlstore.Container, apiURL, token string) *Manager {
	return &Manager{DB: db, Store: store, APIURL: strings.TrimRight(apiURL, "/"), Token: token,
		HTTP: &http.Client{Timeout: 15 * time.Second}, Log: waLog.Stdout("wa", "WARN", false),
		sessions: map[string]*session{}, ctx: ctx, startedAt: time.Now()}
}

// ---------- Siklus hidup sesi ----------

// Resume memulihkan sesi yang sudah tertaut (setelah restart layanan).
func (m *Manager) Resume(ctx context.Context) {
	rows, err := m.DB.Query(ctx, `SELECT workspace_id::text FROM wa_sessions WHERE device_jid IS NOT NULL AND status <> 'disconnected'`)
	if err != nil {
		log.Printf("waqr resume: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		if _, err := m.open(ctx, id, false); err != nil {
			// Perangkat hilang/rusak: jangan biarkan dashboard menampilkan "Terhubung" palsu.
			log.Printf("waqr resume %s: %v", id, err)
			if strings.Contains(err.Error(), "belum tertaut") { // perangkat sudah tidak ada di penyimpanan
				_, _ = m.DB.Exec(ctx, `UPDATE wa_sessions SET device_jid = NULL WHERE workspace_id::text = $1`, id)
			}
			m.report(id, "disconnected", "")
		}
		time.Sleep(300 * time.Millisecond)
	}
	log.Printf("waqr: %d sesi dipulihkan", len(ids))
}

// Start memulai penautan: mengembalikan kode QR pertama (atau status connected bila sudah tertaut).
func (m *Manager) Start(ctx context.Context, ws string) (qr, status string, err error) {
	s, err := m.open(ctx, ws, true)
	if err != nil {
		return "", "", err
	}
	// Tunggu kode QR pertama (koneksi ke server WhatsApp butuh beberapa detik).
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		qr, status = s.snapshot()
		if qr != "" || status == "connected" {
			return qr, status, nil
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	qr, status = s.snapshot()
	if qr == "" && status != "connected" {
		return "", status, errors.New("kode QR belum tersedia, coba lagi")
	}
	return qr, status, nil
}

// Status melaporkan keadaan sesi saat ini beserta kode QR terbaru (untuk di-refresh klien tiap ~20 detik).
func (m *Manager) Status(ws string) (qr, status string) {
	m.mu.Lock()
	s := m.sessions[ws]
	m.mu.Unlock()
	if s == nil {
		return "", "disconnected"
	}
	return s.snapshot()
}

func (s *session) snapshot() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.qr, s.status
}

func (s *session) set(qr, status string) {
	s.mu.Lock()
	s.qr, s.status = qr, status
	s.mu.Unlock()
}

// open membuat/menemukan klien untuk workspace. pair=true dan belum tertaut -> mulai alur QR.
func (m *Manager) open(ctx context.Context, ws string, pair bool) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[ws]; s != nil {
		if _, st := s.snapshot(); st == "connected" || (st == "pending" && s.client.IsConnected()) {
			return s, nil
		}
		s.client.Disconnect() // sesi macet: mulai ulang bersih
		if s.cancel != nil {
			s.cancel()
		}
		delete(m.sessions, ws)
	}

	var jidStr *string
	if err := m.DB.QueryRow(ctx, `SELECT device_jid FROM wa_sessions WHERE workspace_id::text = $1`, ws).Scan(&jidStr); err != nil && !strings.Contains(err.Error(), "no rows") {
		return nil, err
	}
	var dev = m.Store.NewDevice()
	if jidStr != nil && *jidStr != "" {
		jid, err := types.ParseJID(*jidStr)
		if err != nil {
			return nil, err
		}
		d, err := m.Store.GetDevice(ctx, jid)
		if err != nil {
			return nil, err
		}
		if d != nil {
			dev = d
		}
	}
	cli := whatsmeow.NewClient(dev, m.Log)
	cli.EnableAutoReconnect = true
	cli.AutoTrustIdentity = true
	sctx, cancel := context.WithCancel(m.ctx)
	s := &session{ws: ws, client: cli, status: "pending", cancel: cancel}
	cli.AddEventHandler(func(evt any) { m.handle(s, evt) })

	if cli.Store.ID == nil { // belum tertaut: butuh QR
		if !pair {
			cancel()
			return nil, errors.New("sesi belum tertaut")
		}
		ch, err := cli.GetQRChannel(sctx)
		if err != nil {
			cancel()
			return nil, err
		}
		if err := cli.Connect(); err != nil {
			cancel()
			return nil, err
		}
		go func() {
			for item := range ch {
				switch item.Event {
				case "code":
					s.set(item.Code, "pending")
				case "success":
					s.set("", "connected")
				default: // timeout / error: QR habis atau penautan gagal
					s.set("", "disconnected")
					m.report(ws, "disconnected", "")
					m.forget(ws, s)
					return
				}
			}
		}()
	} else {
		if err := cli.Connect(); err != nil {
			cancel()
			return nil, err
		}
	}
	m.sessions[ws] = s
	return s, nil
}

func (m *Manager) forget(ws string, s *session) {
	m.mu.Lock()
	if m.sessions[ws] == s {
		delete(m.sessions, ws)
	}
	m.mu.Unlock()
	s.client.Disconnect()
	if s.cancel != nil {
		s.cancel()
	}
}

// Delete memutuskan dan membatalkan tautan perangkat di WhatsApp (Logout), lalu melupakan sesi.
func (m *Manager) Delete(ctx context.Context, ws string) error {
	m.mu.Lock()
	s := m.sessions[ws]
	m.mu.Unlock()
	if s != nil {
		if s.client.Store.ID != nil {
			lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = s.client.Logout(lctx) // gagal (mis. sudah dicabut dari ponsel) tidak masalah
			cancel()
		}
		m.forget(ws, s)
	} else {
		// Tidak aktif di memori: hapus perangkat tersimpan bila ada.
		var jidStr *string
		_ = m.DB.QueryRow(ctx, `SELECT device_jid FROM wa_sessions WHERE workspace_id::text = $1`, ws).Scan(&jidStr)
		if jidStr != nil && *jidStr != "" {
			if jid, err := types.ParseJID(*jidStr); err == nil {
				if d, err := m.Store.GetDevice(ctx, jid); err == nil && d != nil {
					_ = d.Delete(ctx)
				}
			}
		}
	}
	_, _ = m.DB.Exec(ctx, `UPDATE wa_sessions SET device_jid = NULL, status = 'disconnected' WHERE workspace_id::text = $1`, ws)
	return nil
}

// ---------- Event WhatsApp ----------

func (m *Manager) handle(s *session, evt any) {
	// Satu workspace bermasalah tidak boleh menjatuhkan sesi workspace lain: tangkap panic per event.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("waqr ws=%s: panic di handler event: %v", s.ws, r)
		}
	}()
	switch e := evt.(type) {
	case *events.PairSuccess:
		_, _ = m.DB.Exec(m.ctx, `UPDATE wa_sessions SET device_jid = $2 WHERE workspace_id::text = $1`, s.ws, e.ID.String())
	case *events.Connected:
		if s.client.Store.ID == nil {
			return
		}
		s.set("", "connected")
		// Pastikan JID tersimpan (juga untuk sesi hasil pemulihan).
		_, _ = m.DB.Exec(m.ctx, `UPDATE wa_sessions SET device_jid = $2 WHERE workspace_id::text = $1`, s.ws, s.client.Store.ID.String())
		m.report(s.ws, "connected", s.client.Store.ID.User)
	case *events.LoggedOut: // dicabut dari ponsel (Perangkat tertaut -> Keluar), juga terdeteksi saat menyambung ulang
		log.Printf("waqr ws=%s: dicabut dari ponsel (on_connect=%v reason=%v)", s.ws, e.OnConnect, e.Reason)
		m.lost(s, true)
	case *events.StreamReplaced: // sesi yang sama dibuka di tempat lain; perangkat masih sah, bisa disambung ulang
		log.Printf("waqr ws=%s: sesi digantikan koneksi lain", s.ws)
		m.lost(s, false)
	case *events.TemporaryBan:
		log.Printf("waqr ws=%s: dibatasi sementara oleh WhatsApp: %v", s.ws, e)
		m.lost(s, false)
	case *events.ClientOutdated:
		log.Printf("waqr ws=%s: versi klien WhatsApp usang, perlu memperbarui library", s.ws)
		m.lost(s, false)
	case *events.ConnectFailure:
		// Alasan 401/403 (sesi dicabut) sudah diikuti event LoggedOut oleh library; selainnya bisa dicoba lagi otomatis.
		log.Printf("waqr ws=%s: gagal menyambung (reason=%v)", s.ws, e.Reason)
		// Jangan biarkan dashboard menampilkan "Terhubung" ketika reconnect sedang berlangsung.
		// Client tetap dipertahankan agar EnableAutoReconnect dapat memulihkan sesi.
		s.set("", "disconnected")
		m.report(s.ws, "disconnected", "")
	case *events.Message:
		m.onMessage(s, e)
	}
}

// lost menandai sesi workspace ini terputus, memberi tahu API (status + notifikasi), dan melepas klien dari memori.
// revoke=true: tautan perangkat sudah tidak sah (dicabut dari ponsel) sehingga JID dibuang dan perlu scan QR ulang.
func (m *Manager) lost(s *session, revoke bool) {
	s.set("", "disconnected")
	if revoke {
		_, _ = m.DB.Exec(m.ctx, `UPDATE wa_sessions SET device_jid = NULL WHERE workspace_id::text = $1`, s.ws)
	}
	// Status 'disconnected' ditulis oleh API (qrStatusUpdate) agar notifikasi "Sesi WhatsApp terputus" ikut terkirim.
	m.report(s.ws, "disconnected", "")
	go m.forget(s.ws, s)
}

// phoneOf menentukan nomor telepon lawan bicara; WhatsApp kini sering memakai alamat LID sehingga perlu dipetakan.
func (m *Manager) phoneOf(s *session, e *events.Message) string {
	chat := e.Info.Chat
	alt := e.Info.SenderAlt
	if e.Info.IsFromMe {
		alt = e.Info.RecipientAlt
	}
	if chat.Server == types.HiddenUserServer { // @lid
		if alt.Server == types.DefaultUserServer {
			return alt.User
		}
		if pn, err := s.client.Store.LIDs.GetPNForLID(m.ctx, chat); err == nil && !pn.IsEmpty() {
			return pn.User
		}
		return ""
	}
	if chat.Server == types.DefaultUserServer {
		return chat.User
	}
	return ""
}

type outMsg struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Contact   string `json:"contact"`
	PushName  string `json:"push_name,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Text      string `json:"text"`
	Ad        *outAd `json:"ad,omitempty"`
}
type outAd struct {
	AdID      string `json:"ad_id,omitempty"`
	Headline  string `json:"headline,omitempty"`
	ClickID   string `json:"click_id,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
}

func (m *Manager) onMessage(s *session, e *events.Message) {
	if e.Info.IsGroup || e.Info.Chat.Server == types.BroadcastServer || e.Message == nil {
		return
	}
	phone := m.phoneOf(s, e)
	if phone == "" {
		return
	}
	msg := e.Message
	text := msg.GetConversation()
	var ctxInfo = msg.GetExtendedTextMessage().GetContextInfo()
	if et := msg.GetExtendedTextMessage(); et != nil {
		text = et.GetText()
	}
	out := outMsg{ID: e.Info.ID, Direction: "in", Contact: phone, PushName: e.Info.PushName, Timestamp: e.Info.Timestamp.Unix(), Text: text}
	if e.Info.IsFromMe {
		out.Direction, out.PushName = "out", ""
	}
	if ad := ctxInfo.GetExternalAdReply(); ad != nil && out.Direction == "in" {
		out.Ad = &outAd{AdID: ad.GetSourceID(), Headline: ad.GetTitle(), ClickID: ad.GetCtwaClid(), SourceURL: ad.GetSourceURL()}
	}
	// Pesan tanpa teks dan tanpa iklan (stiker, reaksi, dsb.) tetap diteruskan agar kontak & iklan tercatat.
	if err := m.post(s.ws, "/messages", out); err != nil {
		log.Printf("waqr ws=%s: gagal meneruskan pesan: %v", s.ws, err)
	}
}

// ---------- Panggilan ke API Convera ----------

func (m *Manager) report(ws, status, phone string) {
	body := map[string]string{"status": status}
	if phone != "" {
		body["phone"] = phone
	}
	if err := m.post(ws, "/status", body); err != nil {
		log.Printf("waqr ws=%s: gagal melapor status: %v", ws, err)
	}
}

func (m *Manager) post(ws, path string, v any) error {
	b, _ := json.Marshal(v)
	var last error
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(m.ctx, http.MethodPost, m.APIURL+"/internal/qr/"+ws+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Service-Token", m.Token)
		resp, err := m.HTTP.Do(req)
		if err == nil {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
				return fmt.Errorf("API menolak (%d)", resp.StatusCode)
			}
			err = fmt.Errorf("API %d", resp.StatusCode)
		}
		last = err
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return last
}

// ---------- HTTP (dipanggil API Convera) ----------

// QRImage mengubah kode QR mentah menjadi data URL PNG.
func QRImage(code string) string {
	if code == "" {
		return ""
	}
	png, err := qrcode.Encode(code, qrcode.Medium, 280)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
