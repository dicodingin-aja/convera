// Package mail mengirim email transaksional (verifikasi, reset password, pengingat).
// Tanpa SMTP_HOST, email hanya ditulis ke log (mode pengembangan) dan tidak dikirim ke mana pun.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Sender mengirim satu email teks.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Config adalah pengaturan SMTP. Host kosong = mode log.
type Config struct {
	Host, Port, User, Pass, From string
}

// New memilih pengirim sesuai konfigurasi.
func New(c Config) Sender {
	if c.Host == "" {
		return LogSender{}
	}
	if c.Port == "" {
		c.Port = "587"
	}
	return &SMTPSender{c: c}
}

// LogSender menulis email ke log. HANYA untuk pengembangan: isi (termasuk tautan berisi token) tercetak.
type LogSender struct{}

func (LogSender) Send(_ context.Context, to, subject, body string) error {
	log.Printf("[email-dev] kepada=%s subjek=%q\n%s\n[/email-dev]", to, subject, body)
	return nil
}

// SMTPSender mengirim lewat SMTP (STARTTLS di 587/25, TLS langsung di 465).
type SMTPSender struct{ c Config }

// clean menolak CR/LF agar tidak ada injeksi header email.
func clean(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n\x00") {
		return "", errors.New("karakter tidak valid di header email")
	}
	return strings.TrimSpace(s), nil
}

// Message membangun pesan MIME teks UTF-8.
func Message(from, to, subject, body string) ([]byte, error) {
	var err error
	if from, err = clean(from); err != nil {
		return nil, err
	}
	if to, err = clean(to); err != nil {
		return nil, err
	}
	if subject, err = clean(subject); err != nil {
		return nil, err
	}
	h := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(h, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(body, "\n", "\r\n") + "\r\n"), nil
}

func (s *SMTPSender) Send(ctx context.Context, to, subject, body string) error {
	msg, err := Message(s.c.From, to, subject, body)
	if err != nil {
		return err
	}
	to, _ = clean(to)
	addr := net.JoinHostPort(s.c.Host, s.c.Port)
	d := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.c.Port == "465" {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: s.c.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: tidak bisa terhubung: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, s.c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.c.Port != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.c.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else if s.c.User != "" {
			return errors.New("smtp: server tidak menawarkan STARTTLS; menolak mengirim kredensial tanpa enkripsi")
		}
	}
	if s.c.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.c.User, s.c.Pass, s.c.Host)); err != nil {
			return err
		}
	}
	// Alamat pengirim amplop: bagian <...> dari From bila ada.
	envFrom := s.c.From
	if i := strings.LastIndex(envFrom, "<"); i >= 0 {
		envFrom = strings.TrimSuffix(envFrom[i+1:], ">")
	}
	if err := c.Mail(strings.TrimSpace(envFrom)); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// SendAsync mengirim di goroutine (dengan batas waktu) agar request HTTP tidak menunggu SMTP.
// Kegagalan hanya dicatat ke log tanpa alamat atau isi.
func SendAsync(s Sender, to, subject, body string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.Send(ctx, to, subject, body); err != nil {
			log.Printf("email gagal terkirim: %v", err)
		}
	}()
}
