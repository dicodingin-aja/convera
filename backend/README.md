# Convera Backend

Backend Go + Gin untuk webapp customer dan Convera Admin. Saat ini baru berisi **skema database**
(bab 4 dokumen *Kebutuhan Backend*). Endpoint (bab 5) menyusul.

## Kebutuhan

- Go 1.25+
- PostgreSQL 15+ (dikembangkan dengan PostgreSQL 18)

## Menyiapkan database lokal

1. Salin `.env.example` menjadi `.env`, lalu isi password PostgreSQL Anda:

   ```
   DATABASE_URL=postgres://postgres:PASSWORD_ANDA@localhost:5432/db_convera?sslmode=disable
   ```

   Karakter khusus di password harus di-encode (mis. `@` menjadi `%40`).

2. Buat database dan jalankan migrasi (dari folder `backend/`):

   ```
   go run ./cmd/migrate createdb   # membuat db_convera bila belum ada
   go run ./cmd/migrate up         # membuat semua tabel + data awal
   go run ./cmd/migrate status     # cek migrasi yang sudah jalan
   ```

   Database juga bisa dibuat manual di pgAdmin (nama `db_convera`, encoding UTF8), lalu cukup
   jalankan `up`.

## Perintah migrasi

| Perintah | Fungsi |
|---|---|
| `go run ./cmd/migrate createdb` | Buat database dari `DATABASE_URL` bila belum ada |
| `go run ./cmd/migrate up` | Jalankan semua migrasi yang belum jalan |
| `go run ./cmd/migrate status` | Daftar migrasi dan waktu dijalankan |
| `go run ./cmd/migrate down` | Mundurkan satu migrasi terakhir |
| `go run ./cmd/migrate reset` | Mundurkan semua migrasi (**menghapus semua tabel dan data**) |
| `go run ./cmd/migrate create nama_perubahan` | Buat file migrasi baru di `migrations/` |

Aturan migrasi: **jangan mengubah file migrasi yang sudah pernah dijalankan** di server bersama.
Untuk perubahan skema, buat file migrasi baru.

## Struktur

```
backend/
  cmd/migrate/     program migrasi (createdb, up, down, status, ...)
  migrations/      file SQL berversi (format goose), ikut ter-embed ke binary
  SCHEMA.md        ringkasan tabel dan keputusan skema
```

Ringkasan tabel dan keputusan desain ada di [SCHEMA.md](SCHEMA.md).
Status pekerjaan, keputusan, dan hal yang masih terbuka ada di [HANDOFF.md](HANDOFF.md).
