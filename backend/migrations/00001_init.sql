-- Fondasi skema Convera.
-- Konvensi (Kebutuhan Backend bab 4):
--   * kunci utama UUID (gen_random_uuid, bawaan PostgreSQL 13+)
--   * waktu timestamptz (UTC); agregasi harian memakai zona Asia/Jakarta di query
--   * uang NUMERIC(14,2) dalam Rupiah
--   * nomor HP format 62xxxxxxxxxx
--   * kolom berakhiran _enc berisi ciphertext AES-256-GCM (bytea); kunci di luar database
--   * hapus lunak (deleted_at) untuk data bisnis
--   * nilai kode (status, tipe) memakai kode Inggris huruf kecil; label Indonesia diatur di frontend

-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS set_updated_at();
-- Ekstensi citext sengaja tidak dihapus: bisa dipakai objek lain di database.
