-- Simpan nomor perangkat WhatsApp terenkripsi agar dashboard dapat menampilkan nomor koneksi,
-- bukan nomor telepon pemilik akun.

-- +goose Up
ALTER TABLE wa_sessions ADD COLUMN phone_enc bytea;

-- +goose Down
ALTER TABLE wa_sessions DROP COLUMN IF EXISTS phone_enc;
