// genkey mencetak kunci enkripsi AES-256 acak (base64) untuk ENCRYPTION_KEY di .env.
// Simpan hasilnya hanya di .env / secret manager. Kehilangan kunci = token tersimpan tidak bisa dibaca.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
)

func main() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	fmt.Println(base64.StdEncoding.EncodeToString(b))
}
