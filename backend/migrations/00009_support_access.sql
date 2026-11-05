-- Akses dukungan: admin Convera masuk ke dashboard customer untuk membantu (bab 5.4, fitur dukungan).
-- Selalu terbatas, singkat, beralasan, dan tercatat di audit_logs.

-- +goose Up

-- Kode sekali pakai dari Convera Admin; ditukar menjadi sesi customer lewat POST /api/v1/auth/support-login.
CREATE TABLE support_access_codes (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_id     uuid        NOT NULL REFERENCES admins (id) ON DELETE CASCADE,
  workspace_id uuid        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  token_hash   bytea       NOT NULL UNIQUE, -- SHA-256 dari kode; kode asli tidak disimpan
  reason       text        NOT NULL CHECK (length(reason) BETWEEN 3 AND 300),
  expires_at   timestamptz NOT NULL,
  used_at      timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX support_access_codes_expires_idx ON support_access_codes (expires_at);

-- Sesi customer yang dibuat dari akses dukungan: tahu admin siapa dan dibatasi ke satu workspace.
ALTER TABLE sessions
  ADD COLUMN impersonator_admin_id uuid REFERENCES admins (id) ON DELETE SET NULL,
  ADD COLUMN support_workspace_id  uuid REFERENCES workspaces (id) ON DELETE CASCADE;
ALTER TABLE sessions ADD CONSTRAINT sessions_support_scope
  CHECK (impersonator_admin_id IS NULL OR support_workspace_id IS NOT NULL);

-- +goose Down
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_support_scope;
ALTER TABLE sessions DROP COLUMN IF EXISTS support_workspace_id, DROP COLUMN IF EXISTS impersonator_admin_id;
DROP TABLE IF EXISTS support_access_codes;
