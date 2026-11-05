// Package wa berisi logika murni WhatsApp: normalisasi nomor, parser nominal Rupiah,
// verifikasi signature webhook, dan pembacaan payload Cloud API.
// Tidak ada fungsi di sini yang menyimpan atau mencatat isi pesan.
package wa

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var phoneRe = regexp.MustCompile(`^62[0-9]{7,13}$`)

// NormalizePhone mengubah nomor ke format 62xxxxxxxxxx: hapus non-digit, 08… → 628…, +62… → 62….
func NormalizePhone(raw string) (string, error) {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case strings.HasPrefix(d, "62"):
	case strings.HasPrefix(d, "0"):
		d = "62" + d[1:]
	case strings.HasPrefix(d, "8"):
		d = "62" + d
	}
	if !phoneRe.MatchString(d) {
		return "", errors.New("nomor HP tidak valid")
	}
	return d, nil
}

// MaskPhone menyamarkan nomor: 6281234567890 → 6281****7890.
func MaskPhone(phone string) string {
	if len(phone) < 8 {
		return "••••"
	}
	return phone[:4] + "****" + phone[len(phone)-4:]
}

// Hash256 menormalisasi (trim, lowercase) lalu SHA-256 hex, sesuai syarat user_data Meta.
func Hash256(s string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s))))
	return hex.EncodeToString(h[:])
}

// FirstName mengambil nama depan lowercase untuk user_data.fn.
func FirstName(name string) string {
	f := strings.Fields(strings.ToLower(name))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// NormalizeCity menghapus spasi dan tanda baca serta mengubah ke lowercase untuk user_data.ct.
func NormalizeCity(city string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(city) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// VerifySignature memeriksa header X-Hub-Signature-256 ("sha256=<hex>") terhadap RAW body
// dengan perbandingan constant-time.
func VerifySignature(rawBody []byte, header, appSecret string) bool {
	const prefix = "sha256="
	if appSecret == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(rawBody)
	return hmac.Equal(got, mac.Sum(nil))
}

// Sign membuat nilai header X-Hub-Signature-256 (dipakai di test).
func Sign(rawBody []byte, appSecret string) string {
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(rawBody)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// rupiahRe menangkap nominal dengan awalan "Rp" dan/atau akhiran rb/ribu/k/jt/juta.
var rupiahRe = regexp.MustCompile(`(?i)(rp\.?\s*)?(\d{1,3}(?:[.,]\d{3})+|\d+(?:[.,]\d{1,2})?)\s*(ribu|rb|juta|jt|k)?\b`)
var groupedRe = regexp.MustCompile(`^\d{1,3}([.,]\d{3})+$`)
var decimalRe = regexp.MustCompile(`^\d+[.,]\d{1,2}$`)

// ParseRupiah mengambil nominal Rupiah pertama dari teks: "Rp 350.000", "350rb", "350k", "1,5jt".
// Angka polos tanpa "Rp" atau akhiran (mis. "2 orang") sengaja diabaikan.
func ParseRupiah(text string) (float64, bool) {
	for _, m := range rupiahRe.FindAllStringSubmatch(text, -1) {
		hasPrefix, num, suffix := m[1] != "", m[2], strings.ToLower(m[3])
		if !hasPrefix && suffix == "" {
			continue
		}
		var v float64
		switch {
		case groupedRe.MatchString(num):
			v, _ = strconv.ParseFloat(strings.NewReplacer(".", "", ",", "").Replace(num), 64)
		case decimalRe.MatchString(num):
			v, _ = strconv.ParseFloat(strings.Replace(num, ",", ".", 1), 64)
		default:
			v, _ = strconv.ParseFloat(num, 64)
		}
		switch suffix {
		case "rb", "ribu", "k":
			v *= 1e3
		case "jt", "juta":
			v *= 1e6
		}
		if v > 0 {
			return v, true
		}
	}
	return 0, false
}

// Referral adalah blok "referral" pada pesan dari iklan Click-to-WhatsApp.
type Referral struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	SourceURL  string `json:"source_url"`
	Headline   string `json:"headline"`
	CtwaClid   string `json:"ctwa_clid"`
}

// IsAd benar bila pesan berasal dari iklan.
func (r *Referral) IsAd() bool { return r != nil && r.SourceType == "ad" }

// Message adalah satu pesan masuk hasil parsing. Text hanya dipakai di memori untuk mencocokkan
// aturan, lalu dibuang: JANGAN disimpan, di-log, atau dikirim ke Sentry.
type Message struct {
	ID            string
	From          string
	Name          string
	PhoneNumberID string
	Timestamp     time.Time
	Type          string
	Text          string
	Referral      *Referral
}

type cloudPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Referral *Referral `json:"referral"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// ParseCloudWebhook membaca payload POST webhook WhatsApp Cloud API. Field yang tidak ada ditoleransi.
func ParseCloudWebhook(body []byte) ([]Message, error) {
	var p cloudPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	var out []Message
	for _, e := range p.Entry {
		for _, c := range e.Changes {
			names := map[string]string{}
			for _, ct := range c.Value.Contacts {
				names[ct.WaID] = ct.Profile.Name
			}
			for _, m := range c.Value.Messages {
				if m.ID == "" || m.From == "" {
					continue
				}
				ts := time.Now().UTC()
				if sec, err := strconv.ParseInt(m.Timestamp, 10, 64); err == nil && sec > 0 {
					ts = time.Unix(sec, 0).UTC()
				}
				out = append(out, Message{
					ID: m.ID, From: m.From, Name: names[m.From], PhoneNumberID: c.Value.Metadata.PhoneNumberID,
					Timestamp: ts, Type: m.Type, Text: m.Text.Body, Referral: m.Referral,
				})
			}
		}
	}
	return out, nil
}
