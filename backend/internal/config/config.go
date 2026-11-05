// Package config membaca konfigurasi dari environment (atau file .env di folder backend).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dicodingin-aja/convera/backend/internal/secure"
)

type Config struct {
	DatabaseURL string
	Addr        string
	AdminAddr   string // API Convera Admin (proses terpisah)
	// Email transaksional. SMTPHost kosong = mode log (pengembangan). AppURL dipakai untuk tautan di email.
	SMTPHost, SMTPPort, SMTPUser, SMTPPass, MailFrom string
	AppURL                                           string

	// UploadDir: folder privat bukti transfer. API customer menulis, API admin membaca (volume yang sama).
	UploadDir  string
	EncryptKey []byte

	// GraphAPIVersion wajib diisi (mis. "v23.0"); sengaja tidak ada default agar tidak memakai versi kedaluwarsa.
	GraphAPIVersion string
	GraphBaseURL    string

	// WebhookBase adalah awal URL webhook yang ditampilkan ke pengguna, tanpa slash akhir.
	WebhookBase string

	// TrustedProxies: alamat/CIDR proxy yang boleh menentukan IP klien lewat X-Forwarded-For (untuk rate limit).
	// Default: loopback dan jaringan privat (Caddy di Docker). Jangan isi 0.0.0.0/0.
	TrustedProxies []string

	AllowedOrigins []string // CORS untuk frontend customer dan admin
	CookieSecure   bool     // true di produksi (HTTPS)
	// CookieDomain mis. ".convera.id" agar app.convera.id bisa membaca cookie csrf_token dari api.convera.id.
	// Kosong = cookie milik host API saja (cukup untuk localhost).
	CookieDomain string

	// Layanan sesi WhatsApp Web (mode QR), dijalankan terpisah. Kosong = mode QR belum tersedia.
	QRServiceURL   string
	QRServiceToken string // dipakai layanan QR untuk memanggil /internal/qr/*, dan sebaliknya
}

// LoadDotEnv mengisi variabel dari file .env sederhana tanpa menimpa environment yang sudah ada.
func LoadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}

// Load membaca konfigurasi. needGraph=true (worker, dan API yang menjalankan tes koneksi) mewajibkan GRAPH_API_VERSION.
func Load(needGraph bool) (*Config, error) {
	LoadDotEnv(".env")
	c := &Config{
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Addr:            env("API_ADDR", ":8080"),
		AdminAddr:       env("ADMIN_ADDR", ":8081"),
		UploadDir:       env("UPLOAD_DIR", "./data/uploads"),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        os.Getenv("SMTP_PORT"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPass:        os.Getenv("SMTP_PASS"),
		MailFrom:        env("MAIL_FROM", "Convera <no-reply@localhost>"),
		AppURL:          strings.TrimRight(env("APP_URL", "http://localhost:5173"), "/"),
		GraphAPIVersion: os.Getenv("GRAPH_API_VERSION"),
		GraphBaseURL:    env("GRAPH_BASE_URL", "https://graph.facebook.com"),
		WebhookBase:     strings.TrimRight(env("WEBHOOK_BASE", "https://api.convera.id/webhook"), "/"),
		CookieSecure:    os.Getenv("COOKIE_SECURE") == "true",
		CookieDomain:    os.Getenv("COOKIE_DOMAIN"),
		QRServiceURL:    strings.TrimRight(os.Getenv("QR_SERVICE_URL"), "/"),
		QRServiceToken:  os.Getenv("QR_SERVICE_TOKEN"),
	}
	for _, o := range strings.Split(env("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	for _, p := range strings.Split(env("TRUSTED_PROXIES", "127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			c.TrustedProxies = append(c.TrustedProxies, p)
		}
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL belum diisi (lihat .env.example)"))
	}
	key, err := secure.ParseKey(os.Getenv("ENCRYPTION_KEY"))
	if err != nil {
		errs = append(errs, fmt.Errorf("%w (buat dengan: go run ./cmd/genkey)", err))
	}
	c.EncryptKey = key
	if needGraph && c.GraphAPIVersion == "" {
		errs = append(errs, errors.New("GRAPH_API_VERSION belum diisi: cek versi Graph API yang masih didukung di dokumentasi Meta"))
	}
	return c, errors.Join(errs...)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
