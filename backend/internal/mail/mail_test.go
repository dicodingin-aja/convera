package mail

import (
	"strings"
	"testing"
)

func TestMessageRejectsHeaderInjection(t *testing.T) {
	if _, err := Message("Convera <no-reply@x.id>", "a@b.id\r\nBcc: korban@x.id", "Halo", "isi"); err == nil {
		t.Error("CRLF di alamat tujuan harus ditolak")
	}
	if _, err := Message("Convera <no-reply@x.id>", "a@b.id", "Halo\nBcc: x@y.id", "isi"); err == nil {
		t.Error("newline di subjek harus ditolak")
	}
}

func TestMessageFormat(t *testing.T) {
	b, err := Message("Convera <no-reply@x.id>", "a@b.id", "Reset password Convera — penting", "baris1\nbaris2")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"To: a@b.id", "Content-Type: text/plain; charset=utf-8", "Subject: =?utf-8?q?", "baris1\r\nbaris2"} {
		if !strings.Contains(s, want) {
			t.Errorf("tidak memuat %q:\n%s", want, s)
		}
	}
}

func TestNewPicksLogWithoutHost(t *testing.T) {
	if _, ok := New(Config{}).(LogSender); !ok {
		t.Error("tanpa host harus LogSender")
	}
	if _, ok := New(Config{Host: "smtp.x.id"}).(*SMTPSender); !ok {
		t.Error("dengan host harus SMTPSender")
	}
}
