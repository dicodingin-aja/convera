// Package secure berisi enkripsi AES-256-GCM untuk kolom *_enc dan helper token/hash.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// KeySize adalah panjang kunci AES-256 dalam byte.
const KeySize = 32

// ParseKey membaca kunci enkripsi base64 (standar atau URL-safe) dan memastikan panjangnya 32 byte.
func ParseKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, errors.New("ENCRYPTION_KEY kosong")
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if k, err := enc.DecodeString(b64); err == nil {
			if len(k) != KeySize {
				return nil, fmt.Errorf("ENCRYPTION_KEY harus %d byte setelah base64-decode (didapat %d)", KeySize, len(k))
			}
			return k, nil
		}
	}
	return nil, errors.New("ENCRYPTION_KEY bukan base64 yang valid")
}

// Box mengenkripsi dan mendekripsi rahasia. Format ciphertext: nonce(12) || sealed.
type Box struct{ aead cipher.AEAD }

// NewBox membuat Box dari kunci 32 byte.
func NewBox(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("kunci harus %d byte", KeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal mengenkripsi plaintext. Nonce acak dibuat per panggilan.
func (b *Box) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

// Open mendekripsi hasil Seal. Mengembalikan error bila data rusak atau kunci salah.
func (b *Box) Open(data []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(data) < n+b.aead.Overhead() {
		return nil, errors.New("ciphertext terlalu pendek")
	}
	return b.aead.Open(nil, data[:n], data[n:], nil)
}

// SealString dan OpenString adalah versi string dari Seal dan Open.
func (b *Box) SealString(s string) ([]byte, error) { return b.Seal([]byte(s)) }

func (b *Box) OpenString(data []byte) (string, error) {
	p, err := b.Open(data)
	return string(p), err
}

// RandomToken menghasilkan token acak n byte dalam hex (panjang 2n karakter).
func RandomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// HashToken mengembalikan SHA-256 dari token, untuk disimpan sebagai pengganti token asli.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Hint mengembalikan tampilan tersamar rahasia: "EAAB…x9Qz". Rahasia pendek disamarkan seluruhnya.
func Hint(s string) string {
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
