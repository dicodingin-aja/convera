-- Bab 4.4 Operasional.

-- +goose Up
-- Lonceng notifikasi customer dan admin.
CREATE TABLE notifications (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid REFERENCES users (id) ON DELETE CASCADE,
  admin_id     uuid REFERENCES admins (id) ON DELETE CASCADE,
  workspace_id uuid REFERENCES workspaces (id) ON DELETE CASCADE,
  type         text NOT NULL, -- mis. payment_proof_new, payment_rejected, token_expiring
  title        text NOT NULL,
  body         text,          -- tambahan di luar dokumen
  link         text,          -- tambahan di luar dokumen: halaman tujuan saat diklik
  read_at      timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT notifications_one_recipient CHECK (num_nonnulls(user_id, admin_id) = 1)
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX notifications_admin_idx ON notifications (admin_id, created_at DESC) WHERE admin_id IS NOT NULL;

-- Catatan aktivitas (aktivasi, tolak, ubah paket, maintenance, ...). Hanya ditambah, tidak diubah.
CREATE TABLE audit_logs (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_type   text NOT NULL CHECK (actor_type IN ('admin', 'user', 'system', 'api_key')),
  actor_id     uuid,
  actor_label  text,          -- nama saat kejadian, agar tetap terbaca bila akun dihapus
  workspace_id uuid REFERENCES workspaces (id) ON DELETE SET NULL,
  action       text NOT NULL, -- mis. payment_proof.approve
  target_type  text,
  target_id    text,
  detail       jsonb NOT NULL DEFAULT '{}',
  ip           inet,
  at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_at_idx ON audit_logs (at DESC);
CREATE INDEX audit_logs_target_idx ON audit_logs (target_type, target_id);
CREATE INDEX audit_logs_workspace_idx ON audit_logs (workspace_id, at DESC) WHERE workspace_id IS NOT NULL;

-- API key admin/developer. Hanya hash yang disimpan; prefix untuk tampilan (cvr_admin_7Hq2…).
CREATE TABLE api_keys (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name         text NOT NULL,
  prefix       text NOT NULL,
  key_hash     bytea NOT NULL UNIQUE,
  created_by   uuid REFERENCES admins (id) ON DELETE SET NULL,
  last_used_at timestamptz,
  revoked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS notifications;
