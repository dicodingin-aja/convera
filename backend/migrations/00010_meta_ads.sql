-- Koneksi Meta Ads (Marketing API): biaya, klik, dan percakapan per iklan untuk Performa, ROAS, dan cost per purchase.

-- +goose Up

-- Satu akun iklan per workspace. Token (izin ads_read saja) dienkripsi AES-256-GCM; tidak pernah dikembalikan ke frontend.
CREATE TABLE ad_connections (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id      uuid NOT NULL UNIQUE REFERENCES workspaces (id) ON DELETE CASCADE,
  ad_account_id     text NOT NULL CHECK (ad_account_id ~ '^[0-9]{5,25}$'), -- tanpa awalan "act_"
  account_name      text,
  currency          char(3),
  access_token_enc  bytea NOT NULL,
  access_token_hint text,
  status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'connected', 'invalid', 'error')),
  last_error        text,
  last_sync_at      timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER ad_connections_updated_at BEFORE UPDATE ON ad_connections FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Data harian per iklan dari Insights API (level ad). Hari mengikuti zona waktu akun iklan di Meta.
-- ad_id = referral.source_id pada pesan CTWA, sehingga bisa digabung dengan leads.ad_id dan events.
CREATE TABLE ad_insights_daily (
  workspace_id  uuid          NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  day           date          NOT NULL,
  ad_id         text          NOT NULL,
  campaign_id   text,
  campaign_name text,
  adset_id      text,
  adset_name    text,
  ad_name       text,
  spend         numeric(14,2) NOT NULL DEFAULT 0, -- dalam mata uang akun iklan
  impressions   bigint        NOT NULL DEFAULT 0,
  clicks        bigint        NOT NULL DEFAULT 0,
  conversations integer       NOT NULL DEFAULT 0, -- percakapan WhatsApp dimulai menurut Meta
  currency      char(3),
  synced_at     timestamptz   NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, day, ad_id)
);
CREATE INDEX ad_insights_ad_idx ON ad_insights_daily (workspace_id, ad_id, day DESC);

-- +goose Down
DROP TABLE IF EXISTS ad_insights_daily;
DROP TABLE IF EXISTS ad_connections;
