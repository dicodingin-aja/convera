-- Akun customer disederhanakan: username + password saja. Email dibuang (tanpa verifikasi, reset lewat email, atau SMTP).
-- Reset password sekarang dilakukan superadmin lewat Convera Admin.

-- +goose Up
ALTER TABLE users ADD COLUMN username citext;

-- Backfill dari bagian lokal email lama: huruf kecil, hanya a-z 0-9 . _ -, mulai dengan huruf/angka, 3-25 karakter.
-- Bila ada yang kembar, yang ke-2 dan seterusnya diberi akhiran dari id.
WITH base AS (
  SELECT id,
         left(
           CASE WHEN length(b) >= 3 THEN b ELSE b || 'usr' END,
           25) AS b
  FROM (
    SELECT id, regexp_replace(regexp_replace(lower(split_part(email::text, '@', 1)), '[^a-z0-9._-]', '', 'g'), '^[^a-z0-9]+', '') AS b
    FROM users WHERE deleted_at IS NULL
  ) x
), ranked AS (
  SELECT id, b, row_number() OVER (PARTITION BY b ORDER BY id) AS rn FROM base
)
UPDATE users u
SET username = CASE WHEN r.rn = 1 THEN r.b ELSE r.b || '-' || substr(replace(u.id::text, '-', ''), 1, 4) END
FROM ranked r WHERE r.id = u.id;

UPDATE users SET username = 'deleted-' || substr(replace(id::text, '-', ''), 1, 12) WHERE deleted_at IS NOT NULL;

ALTER TABLE users ALTER COLUMN username SET NOT NULL;
-- Unik di antara akun yang belum dihapus, agar username bisa dipakai lagi setelah akun dihapus.
CREATE UNIQUE INDEX users_username_key ON users (username) WHERE deleted_at IS NULL;
ALTER TABLE users ADD CONSTRAINT users_username_format
  CHECK (deleted_at IS NOT NULL OR username ~ '^[a-z0-9][a-z0-9._-]{2,29}$');

ALTER TABLE users DROP COLUMN email_verified_at;
DROP TABLE IF EXISTS email_verifications;
DROP TABLE IF EXISTS password_resets;

-- +goose Down
CREATE TABLE password_resets (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX password_resets_user_idx ON password_resets (user_id);
CREATE TABLE email_verifications (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  email      citext NOT NULL,
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_verifications_user_idx ON email_verifications (user_id);

ALTER TABLE users ADD COLUMN email_verified_at timestamptz;

ALTER TABLE users DROP CONSTRAINT users_username_format;
DROP INDEX users_username_key;
ALTER TABLE users DROP COLUMN username;
