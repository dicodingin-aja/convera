// Package meta membangun payload Conversions API sesuai mode koneksi, mengirimnya ke Graph API,
// dan menerjemahkan error Meta menjadi pesan untuk pengguna.
package meta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dicodingin-aja/convera/backend/internal/wa"
)

const (
	ModeWABA = "waba"
	ModeQR   = "qr"
)

// WABAEvents adalah satu-satunya event yang boleh dikirim di mode WABA (meta-integration.md bagian 1).
var WABAEvents = []string{"LeadSubmitted", "Purchase", "AddToCart", "InitiateCheckout", "ViewContent"}

// QREvents adalah event standar Meta yang diizinkan di mode QR.
var QREvents = []string{
	"Purchase", "Lead", "Contact", "Schedule", "CompleteRegistration", "AddToCart", "InitiateCheckout",
	"ViewContent", "SubmitApplication", "StartTrial", "Subscribe", "AddPaymentInfo", "Search",
	"FindLocation", "CustomizeProduct", "Donate", "AddToWishlist",
}

// AllowedEvents mengembalikan daftar event untuk mode, atau nil bila mode tidak dikenal.
func AllowedEvents(mode string) []string {
	switch mode {
	case ModeWABA:
		return WABAEvents
	case ModeQR:
		return QREvents
	}
	return nil
}

// ValidateEvent memeriksa event_name, nilai, dan mata uang terhadap aturan mode (bagian 8 "Validasi").
func ValidateEvent(mode, name string, value float64, currency string) error {
	ok := false
	for _, e := range AllowedEvents(mode) {
		if e == name {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("event %q tidak didukung pada mode %s", name, mode)
	}
	if name == "Purchase" {
		if value <= 0 {
			return errors.New("Purchase wajib punya value > 0")
		}
		if len(currency) != 3 {
			return errors.New("Purchase wajib punya currency (3 huruf, mis. IDR)")
		}
	}
	return nil
}

// EventID membuat ID dedup. Bila externalID ada: workspace+external+event. Selain itu:
// workspace+lead+event+value+tanggal (UTC), sehingga event yang sama di hari yang sama dianggap duplikat.
func EventID(workspaceID, leadID, externalID, eventName string, value float64, day time.Time) string {
	var raw string
	if externalID != "" {
		raw = strings.Join([]string{workspaceID, externalID, eventName}, "|")
	} else {
		raw = strings.Join([]string{workspaceID, leadID, eventName, fmt.Sprintf("%.2f", value), day.UTC().Format("2006-01-02")}, "|")
	}
	h := sha256.Sum256([]byte(raw))
	return "cv_" + hex.EncodeToString(h[:])[:32]
}

// Input adalah data yang dibutuhkan untuk membangun satu event CAPI.
type Input struct {
	Mode      string
	EventName string
	EventID   string
	EventTime time.Time // waktu konversi sebenarnya, bukan waktu kirim
	Value     *float64
	Currency  string

	// Mode WABA
	WABAID   string
	CtwaClid string

	// Mode QR
	Phone  string // 62xxxxxxxxxx
	Name   string
	City   string
	LeadID string
}

// BuildEvent membuat satu elemen "data" CAPI. Token TIDAK ada di sini.
func BuildEvent(in Input) (map[string]any, error) {
	ev := map[string]any{
		"event_name": in.EventName,
		"event_time": in.EventTime.Unix(),
		"event_id":   in.EventID,
	}
	if in.Value != nil {
		cur := in.Currency
		if cur == "" {
			cur = "IDR"
		}
		ev["custom_data"] = map[string]any{"currency": cur, "value": *in.Value}
	}
	switch in.Mode {
	case ModeWABA:
		if in.CtwaClid == "" {
			return nil, ErrNotFromAd
		}
		ud := map[string]any{"ctwa_clid": in.CtwaClid}
		if in.WABAID != "" {
			ud["whatsapp_business_account_id"] = in.WABAID
		}
		ev["action_source"] = "business_messaging"
		ev["messaging_channel"] = "whatsapp"
		ev["user_data"] = ud
	case ModeQR:
		// "chat", BUKAN "website": konversi terjadi di chat dan tidak ada data browser.
		ev["action_source"] = "chat"
		ud := map[string]any{"country": []string{wa.Hash256("id")}}
		if in.Phone != "" {
			ud["ph"] = []string{wa.Hash256(in.Phone)}
		}
		if fn := wa.FirstName(in.Name); fn != "" {
			ud["fn"] = []string{wa.Hash256(fn)}
		}
		if ct := wa.NormalizeCity(in.City); ct != "" {
			ud["ct"] = []string{wa.Hash256(ct)}
		}
		if in.LeadID != "" {
			ud["external_id"] = []string{wa.Hash256(in.LeadID)}
		}
		ev["user_data"] = ud
	default:
		return nil, fmt.Errorf("mode %q tidak dikenal", in.Mode)
	}
	return ev, nil
}

// ErrNotFromAd: lead mode WABA tanpa ctwa_clid tidak boleh dikirim.
var ErrNotFromAd = errors.New("bukan dari iklan: lead tidak punya ctwa_clid")

// Client mengirim event ke Graph API. Versi Graph API selalu dari konfigurasi.
type Client struct {
	BaseURL string // default https://graph.facebook.com
	Version string // mis. "v23.0"; wajib diisi dari config
	HTTP    *http.Client
}

// Result adalah ringkasan respons Meta.
type Result struct {
	HTTPStatus int
	Body       map[string]any
	FBTraceID  string
	ErrCode    int
	ErrSubcode int
	ErrMessage string
}

// OK benar bila Meta menerima event.
func (r *Result) OK() bool { return r.HTTPStatus >= 200 && r.HTTPStatus < 300 && r.ErrCode == 0 }

// Send memanggil POST {base}/{version}/{pixelID}/events. Error jaringan dikembalikan sebagai error biasa
// (boleh di-retry); respons Meta (termasuk error) dikembalikan lewat Result.
func (c *Client) Send(ctx context.Context, pixelID, token, testEventCode string, events []map[string]any) (*Result, error) {
	if c.Version == "" {
		return nil, errors.New("GRAPH_API_VERSION belum diisi")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://graph.facebook.com"
	}
	body := map[string]any{"data": events, "access_token": token}
	if testEventCode != "" {
		body["test_event_code"] = testEventCode
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s/%s/events", base, c.Version, pixelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		// Jangan sertakan URL/body (berisi token) di pesan error.
		return nil, errors.New("gagal menghubungi Meta (jaringan)")
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	res := &Result{HTTPStatus: resp.StatusCode, Body: map[string]any{}}
	_ = json.Unmarshal(data, &res.Body)
	if e, ok := res.Body["error"].(map[string]any); ok {
		res.ErrCode = intOf(e["code"])
		if res.ErrCode == 0 {
			res.ErrCode = -1
		}
		res.ErrSubcode = intOf(e["error_subcode"])
		res.ErrMessage, _ = e["message"].(string)
		res.FBTraceID, _ = e["fbtrace_id"].(string)
	}
	if v, ok := res.Body["fbtrace_id"].(string); ok && res.FBTraceID == "" {
		res.FBTraceID = v
	}
	return res, nil
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// Verdict adalah hasil klasifikasi error Meta.
type Verdict struct {
	Message     string // pesan untuk pengguna (bagian 7 "Terjemahan error")
	Retry       bool   // boleh di-retry
	InvalidAuth bool   // token tidak valid → status koneksi menjadi 'invalid' + notifikasi
}

// Classify menerjemahkan Result gagal. Pencocokan teks bersifat heuristik karena format pesan Meta
// belum diverifikasi dengan respons asli; selalu sertakan pesan asli + fbtrace_id pada kasus lain.
func Classify(r *Result) Verdict {
	msg := strings.ToLower(r.ErrMessage)
	switch {
	case r.ErrCode == 190:
		return Verdict{Message: "Token Meta tidak valid atau sudah dicabut. Buat token baru di Events Manager.", InvalidAuth: true}
	case r.HTTPStatus >= 500, r.ErrCode == 1, r.ErrCode == 2, r.ErrCode == 4, r.ErrCode == 17, r.ErrCode == 32, r.ErrCode == 613:
		return Verdict{Message: other(r), Retry: true}
	case strings.Contains(msg, "ctwa_clid"), strings.Contains(msg, "whatsapp_business_account"):
		return Verdict{Message: "Dataset belum terhubung dengan akun WhatsApp Business yang dipakai iklan."}
	case strings.Contains(msg, "event_name"), strings.Contains(msg, "event name"):
		return Verdict{Message: "Jenis event ini tidak didukung Meta untuk WhatsApp resmi."}
	case r.ErrCode == 100 && (r.ErrSubcode == 33 || strings.Contains(msg, "does not exist") || strings.Contains(msg, "permission")),
		r.ErrCode == 803, r.ErrCode == 10, r.ErrCode == 200:
		return Verdict{Message: "Pixel/Dataset ID salah atau token tidak punya akses ke dataset ini."}
	}
	return Verdict{Message: other(r)}
}

func other(r *Result) string {
	m := r.ErrMessage
	if m == "" {
		m = fmt.Sprintf("Meta mengembalikan HTTP %d", r.HTTPStatus)
	}
	if r.FBTraceID != "" {
		m += " (fbtrace_id: " + r.FBTraceID + ")"
	}
	return m
}

// RetryDelay mengembalikan jeda sebelum percobaan berikutnya: 1m, 5m, 30m, 2j, 6j. ok=false setelah 5 kali.
func RetryDelay(attempts int) (time.Duration, bool) {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}
	if attempts < 1 || attempts > len(steps) {
		return 0, false
	}
	return steps[attempts-1], true
}
