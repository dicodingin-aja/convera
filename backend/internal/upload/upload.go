// Package upload menyimpan berkas bukti transfer di penyimpanan privat (folder di disk server).
// Berkas tidak pernah disajikan langsung: hanya lewat endpoint admin yang memeriksa sesi.
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// MaxSize adalah ukuran maksimal berkas bukti transfer (dokumen bab 8: maks. 5 MB).
const MaxSize = 5 << 20

// ErrTooLarge dikembalikan bila berkas melebihi batas.
var ErrTooLarge = errors.New("berkas terlalu besar")

// Kind adalah jenis berkas yang diizinkan.
type Kind struct{ Mime, Ext string }

var allowed = map[string]Kind{
	"image/jpeg":      {"image/jpeg", "jpg"},
	"image/png":       {"image/png", "png"},
	"application/pdf": {"application/pdf", "pdf"},
}

// Detect menentukan jenis berkas dari ISINYA (magic bytes), bukan dari nama file atau Content-Type kiriman klien.
func Detect(head []byte) (Kind, bool) {
	k, ok := allowed[http.DetectContentType(head)]
	return k, ok
}

var keyRe = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png|pdf)$`)

// NewKey membuat nama berkas acak (tidak berhubungan dengan nama asli dari pengguna).
func NewKey(k Kind) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + "." + k.Ext, nil
}

// ValidKey memastikan key hanya berbentuk nama acak buatan NewKey (mencegah path traversal).
func ValidKey(key string) bool { return keyRe.MatchString(key) }

// Store adalah penyimpanan berbasis folder.
type Store struct{ Dir string }

func (s *Store) path(key string) (string, error) {
	if !ValidKey(key) {
		return "", errors.New("key berkas tidak valid")
	}
	return filepath.Join(s.Dir, key), nil
}

// Save menulis berkas dari r dengan batas max byte. Gagal/terlalu besar -> berkas dihapus lagi.
// Mengembalikan jumlah byte yang tersimpan.
func (s *Store) Save(key string, r io.Reader, max int64) (int64, error) {
	p, err := s.path(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, max+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > max {
		err = ErrTooLarge
	}
	if err != nil {
		_ = os.Remove(p)
		return 0, err
	}
	return n, nil
}

// Open membuka berkas untuk dibaca.
func (s *Store) Open(key string) (*os.File, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// Remove menghapus berkas (diam bila tidak ada).
func (s *Store) Remove(key string) {
	if p, err := s.path(key); err == nil {
		_ = os.Remove(p)
	}
}
