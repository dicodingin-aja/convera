-- Bab 4.2 Bisnis, langganan, dan pembayaran.

-- +goose Up
CREATE TABLE plans (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        citext NOT NULL UNIQUE, -- id yang dipakai frontend, mis. 'workspace'
  name        text NOT NULL,
  price_month numeric(14,2) NOT NULL CHECK (price_month >= 0),
  price_12m   numeric(14,2) NOT NULL CHECK (price_12m >= 0),
  features    jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(features) = 'array'),
  shown       boolean NOT NULL DEFAULT true, -- tampil di modal Upgrade customer
  sort        integer NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz
);
CREATE TRIGGER plans_updated_at BEFORE UPDATE ON plans FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Penerima komisi. is_founder = ikut dalam pembagian 80% rata; sales luar hanya menerima komisi.
CREATE TABLE sales_people (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       citext NOT NULL UNIQUE,
  is_founder boolean NOT NULL DEFAULT false,
  active     boolean NOT NULL DEFAULT true,
  sort       integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER sales_people_updated_at BEFORE UPDATE ON sales_people FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Rekening tujuan transfer yang tampil di halaman bayar.
CREATE TABLE bank_accounts (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bank       text NOT NULL,
  number     text NOT NULL,
  holder     text NOT NULL,
  active     boolean NOT NULL DEFAULT true,
  sort       integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE TRIGGER bank_accounts_updated_at BEFORE UPDATE ON bank_accounts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Satu workspace = satu bisnis = satu langganan.
CREATE TABLE workspaces (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
  slug          text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 60),
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  category      text,
  city          text,
  -- Bagian acak di URL webhook (api.convera.id/webhook/{webhook_slug}); sengaja berbeda dari slug
  -- agar URL webhook tidak bisa ditebak dari nama bisnis.
  webhook_slug  text NOT NULL UNIQUE DEFAULT replace(gen_random_uuid()::text, '-', ''),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  deleted_at    timestamptz
);
CREATE INDEX workspaces_owner_idx ON workspaces (owner_user_id) WHERE deleted_at IS NULL;
CREATE TRIGGER workspaces_updated_at BEFORE UPDATE ON workspaces FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Satu baris per workspace (status terkini). Riwayat pembayaran ada di payments.
CREATE TABLE subscriptions (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id       uuid NOT NULL UNIQUE REFERENCES workspaces (id) ON DELETE CASCADE,
  plan_id            uuid REFERENCES plans (id) ON DELETE RESTRICT,
  -- 'stopped' tambahan di luar dokumen: dihentikan admin (POST customers/{id}/stop).
  status             text NOT NULL CHECK (status IN ('trial', 'active', 'expired', 'pending', 'stopped')),
  trial_until        timestamptz,
  current_period_end timestamptz,
  sales_id           uuid REFERENCES sales_people (id) ON DELETE SET NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
-- Untuk tugas kedaluwarsa dan "berakhir 14 hari" di Ringkasan admin.
CREATE INDEX subscriptions_period_end_idx ON subscriptions (current_period_end) WHERE status = 'active';
CREATE INDEX subscriptions_trial_until_idx ON subscriptions (trial_until) WHERE status = 'trial';
CREATE TRIGGER subscriptions_updated_at BEFORE UPDATE ON subscriptions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Antrean verifikasi admin. File disimpan di penyimpanan privat; di sini hanya kuncinya.
CREATE TABLE payment_proofs (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id       uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
  uploaded_by        uuid REFERENCES users (id) ON DELETE SET NULL,
  kind               text NOT NULL DEFAULT 'new' CHECK (kind IN ('new', 'renew', 'trial')),
  plan_id            uuid REFERENCES plans (id) ON DELETE RESTRICT, -- pilihan customer di modal Upgrade
  months             integer NOT NULL DEFAULT 1 CHECK (months > 0),
  bank               text NOT NULL,          -- bank pengirim
  sender             text NOT NULL,          -- nama pemilik rekening pengirim
  ref                text,                   -- nomor referensi transfer
  billed             numeric(14,2) NOT NULL CHECK (billed >= 0),
  received           numeric(14,2) CHECK (received >= 0), -- diisi/dicek admin dari mutasi
  to_bank_account_id uuid REFERENCES bank_accounts (id) ON DELETE RESTRICT,
  file_key           text NOT NULL,          -- nama acak di penyimpanan privat
  file_mime          text NOT NULL CHECK (file_mime IN ('image/jpeg', 'image/png', 'application/pdf')),
  file_size          integer NOT NULL CHECK (file_size > 0),
  message            text CHECK (length(message) <= 1000),
  status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  reject_reason      text,
  decided_by         uuid REFERENCES admins (id) ON DELETE SET NULL,
  decided_at         timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz,
  CONSTRAINT payment_proofs_decision CHECK ((status = 'pending') = (decided_at IS NULL)),
  CONSTRAINT payment_proofs_reject_reason CHECK (status <> 'rejected' OR reject_reason IS NOT NULL)
);
CREATE INDEX payment_proofs_status_idx ON payment_proofs (status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX payment_proofs_workspace_idx ON payment_proofs (workspace_id, created_at DESC);
CREATE TRIGGER payment_proofs_updated_at BEFORE UPDATE ON payment_proofs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Pembayaran yang sudah disetujui. Persentase disimpan sebagai snapshot agar riwayat bagi hasil
-- tidak berubah ketika aturan komisi/kas diubah.
CREATE TABLE payments (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
  subscription_id  uuid REFERENCES subscriptions (id) ON DELETE SET NULL,
  plan_id          uuid NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
  payment_proof_id uuid UNIQUE REFERENCES payment_proofs (id) ON DELETE RESTRICT,
  amount           numeric(14,2) NOT NULL CHECK (amount >= 0),
  months           integer NOT NULL CHECK (months > 0),
  period_start     timestamptz NOT NULL,
  period_end       timestamptz NOT NULL CHECK (period_end > period_start),
  paid_at          timestamptz NOT NULL,
  sales_id         uuid NOT NULL REFERENCES sales_people (id) ON DELETE RESTRICT, -- wajib saat aktivasi
  komisi_pct       numeric(5,2) NOT NULL CHECK (komisi_pct BETWEEN 0 AND 100),
  kas_pct          numeric(5,2) NOT NULL CHECK (kas_pct BETWEEN 0 AND 100),
  approved_by      uuid REFERENCES admins (id) ON DELETE SET NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT payments_pct_total CHECK (komisi_pct + kas_pct <= 100)
);
CREATE INDEX payments_paid_at_idx ON payments (paid_at);
CREATE INDEX payments_workspace_idx ON payments (workspace_id, paid_at DESC);

-- Penanda bagi hasil sudah ditransfer. month = tanggal 1 bulan tersebut.
CREATE TABLE payouts (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  month          date NOT NULL CHECK (extract(day FROM month) = 1),
  person_id      uuid NOT NULL REFERENCES sales_people (id) ON DELETE RESTRICT,
  amount         numeric(14,2) NOT NULL CHECK (amount >= 0),
  transferred_at timestamptz NOT NULL DEFAULT now(),
  marked_by      uuid REFERENCES admins (id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (month, person_id)
);

-- Pengaturan global (trial_days, pay_months_12, grace_days, komisi_pct, kas_pct, maintenance, ...).
CREATE TABLE settings (
  key        text PRIMARY KEY,
  value      jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES admins (id) ON DELETE SET NULL
);
CREATE TRIGGER settings_updated_at BEFORE UPDATE ON settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Nyala/mati fitur untuk semua customer.
CREATE TABLE feature_flags (
  key         text PRIMARY KEY,
  enabled     boolean NOT NULL DEFAULT false,
  badge       text, -- mis. 'Beta', 'Baru'
  description text,
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER feature_flags_updated_at BEFORE UPDATE ON feature_flags FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS feature_flags;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS payouts;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS payment_proofs;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS bank_accounts;
DROP TABLE IF EXISTS sales_people;
DROP TABLE IF EXISTS plans;
