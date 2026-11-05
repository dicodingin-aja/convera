-- Bab 4.1 Akun dan akses.

-- +goose Up
CREATE TABLE users (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email             citext NOT NULL,
  password_hash     text NOT NULL,
  name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  phone             text CHECK (phone ~ '^62[0-9]{7,13}$'),
  email_verified_at timestamptz,
  status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
  last_seen_at      timestamptz, -- status Online = aktif dalam 5 menit terakhir
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz
);
-- Email unik di antara akun yang belum dihapus, agar email bisa dipakai lagi setelah akun dihapus.
CREATE UNIQUE INDEX users_email_key ON users (email) WHERE deleted_at IS NULL;
CREATE INDEX users_last_seen_idx ON users (last_seen_at DESC) WHERE deleted_at IS NULL;
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE admins (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username      citext NOT NULL UNIQUE,
  password_hash text NOT NULL,
  name          text NOT NULL DEFAULT '',
  role          text NOT NULL DEFAULT 'admin' CHECK (role IN ('superadmin', 'admin')),
  status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  last_login_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admins_updated_at BEFORE UPDATE ON admins FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Satu baris = satu sesi login (refresh token). Customer dan admin memakai sesi terpisah.
CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid REFERENCES users (id) ON DELETE CASCADE,
  admin_id     uuid REFERENCES admins (id) ON DELETE CASCADE,
  token_hash   bytea NOT NULL UNIQUE, -- SHA-256 dari refresh token; token asli tidak disimpan
  expires_at   timestamptz NOT NULL,
  ip           inet,
  user_agent   text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  revoked_at   timestamptz,
  CONSTRAINT sessions_one_owner CHECK (num_nonnulls(user_id, admin_id) = 1)
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX sessions_admin_idx ON sessions (admin_id) WHERE admin_id IS NOT NULL;
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE password_resets (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX password_resets_user_idx ON password_resets (user_id);

-- Tambahan di luar dokumen: token verifikasi email (kolom users.email_verified_at sudah ada di dokumen).
CREATE TABLE email_verifications (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  email      citext NOT NULL, -- alamat yang diverifikasi (bisa berbeda bila user mengganti email)
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_verifications_user_idx ON email_verifications (user_id);

-- +goose Down
DROP TABLE IF EXISTS email_verifications;
DROP TABLE IF EXISTS password_resets;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS admins;
DROP TABLE IF EXISTS users;
