package secure

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func testBox(t *testing.T) *Box {
	t.Helper()
	b, err := NewBox(bytes.Repeat([]byte{7}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSealOpen(t *testing.T) {
	b := testBox(t)
	c1, _ := b.SealString("EAAB-token")
	c2, _ := b.SealString("EAAB-token")
	if bytes.Equal(c1, c2) {
		t.Error("nonce harus acak: ciphertext sama untuk plaintext sama")
	}
	if bytes.Contains(c1, []byte("EAAB-token")) {
		t.Error("ciphertext memuat plaintext")
	}
	got, err := b.OpenString(c1)
	if err != nil || got != "EAAB-token" {
		t.Errorf("Open = %q, %v", got, err)
	}
	c1[len(c1)-1] ^= 1
	if _, err := b.Open(c1); err == nil {
		t.Error("ciphertext rusak seharusnya ditolak")
	}
	if _, err := b.Open([]byte("pendek")); err == nil {
		t.Error("data pendek seharusnya ditolak")
	}
}

func TestParseKey(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, KeySize))
	if _, err := ParseKey(good); err != nil {
		t.Error(err)
	}
	for _, bad := range []string{"", "bukan-base64!!", base64.StdEncoding.EncodeToString([]byte("terlalu pendek"))} {
		if _, err := ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) seharusnya error", bad)
		}
	}
}

func TestHintAndToken(t *testing.T) {
	if Hint("EAABsbCS1iHgBOx9Qz") != "EAAB…x9Qz" {
		t.Errorf("Hint = %q", Hint("EAABsbCS1iHgBOx9Qz"))
	}
	if Hint("abc") != "••••" {
		t.Error("rahasia pendek harus disamarkan penuh")
	}
	a, _ := RandomToken(16)
	b, _ := RandomToken(16)
	if len(a) != 32 || a == b {
		t.Error("RandomToken")
	}
	if len(HashToken("x")) != 32 {
		t.Error("HashToken")
	}
}
