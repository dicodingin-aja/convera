package upload

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	png = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	jpg = append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF"), bytes.Repeat([]byte{0}, 64)...)
	pdf = []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n")
)

func TestDetectUsesContentNotName(t *testing.T) {
	for name, tc := range map[string]struct {
		data []byte
		mime string
	}{"png": {png, "image/png"}, "jpg": {jpg, "image/jpeg"}, "pdf": {pdf, "application/pdf"}} {
		k, ok := Detect(tc.data)
		if !ok || k.Mime != tc.mime {
			t.Errorf("%s: %v %v", name, k, ok)
		}
	}
	for name, data := range map[string][]byte{
		"html":   []byte("<html><script>alert(1)</script></html>"),
		"teks":   []byte("halo ini bukan gambar"),
		"exe":    []byte("MZ\x90\x00\x03\x00\x00\x00"),
		"svg":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`),
		"gif":    []byte("GIF89a\x01\x00\x01\x00"),
		"kosong": {},
	} {
		if _, ok := Detect(data); ok {
			t.Errorf("%s seharusnya ditolak", name)
		}
	}
}

func TestKeys(t *testing.T) {
	k, err := NewKey(Kind{"image/png", "png"})
	if err != nil || !ValidKey(k) || !strings.HasSuffix(k, ".png") {
		t.Fatalf("NewKey = %q, %v", k, err)
	}
	k2, _ := NewKey(Kind{"image/png", "png"})
	if k == k2 {
		t.Error("key harus acak")
	}
	for _, bad := range []string{"", "../etc/passwd", "a.png", strings.Repeat("a", 32) + ".exe", strings.Repeat("A", 32) + ".png",
		strings.Repeat("a", 32) + ".png/../x", "/" + strings.Repeat("a", 32) + ".png", strings.Repeat("a", 32) + ".png\x00.jpg"} {
		if ValidKey(bad) {
			t.Errorf("key %q seharusnya ditolak", bad)
		}
	}
}

func TestStoreSaveOpenLimit(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "up")}
	key, _ := NewKey(Kind{"image/png", "png"})
	n, err := s.Save(key, bytes.NewReader(png), MaxSize)
	if err != nil || n != int64(len(png)) {
		t.Fatalf("Save: %d %v", n, err)
	}
	f, err := s.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if fi, _ := os.Stat(filepath.Join(s.Dir, key)); fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("izin berkas terlalu longgar: %v", fi.Mode())
	}
	if _, err := s.Save(key, bytes.NewReader(png), MaxSize); err == nil {
		t.Error("menimpa berkas yang ada seharusnya gagal (O_EXCL)")
	}
	s.Remove(key)
	if _, err := s.Open(key); err == nil {
		t.Error("berkas seharusnya sudah terhapus")
	}

	big, _ := NewKey(Kind{"image/png", "png"})
	_, err = s.Save(big, bytes.NewReader(bytes.Repeat([]byte{1}, 101)), 100)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("batas ukuran: %v", err)
	}
	if _, err := s.Open(big); err == nil {
		t.Error("berkas yang terlalu besar harus dihapus lagi")
	}
	if _, err := s.Save("../../evil.png", bytes.NewReader(png), 100); err == nil {
		t.Error("path traversal harus ditolak")
	}
	if _, err := s.Open("../x.png"); err == nil {
		t.Error("Open dengan path traversal harus ditolak")
	}
}
