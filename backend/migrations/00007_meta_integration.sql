-- Penyesuaian skema dengan meta-integration.md (mode WABA / QR, pengiriman Meta CAPI, API key per workspace).
-- Tidak mengubah file migrasi lama. Nomor HP tetap disimpan di leads.phone (keputusan tim).

-- +goose Up

-- Mode koneksi dan kredensial WABA per workspace (bagian 3 dan 4 dokumen).
ALTER TABLE meta_connections
  ADD COLUMN mode            text NOT NULL DEFAULT 'waba' CHECK (mode IN ('waba', 'qr')),
  ADD COLUMN waba_id         text,
  ADD COLUMN phone_number_id text,
  ADD COLUMN app_secret_enc  bytea,  -- AES-256-GCM; untuk verifikasi X-Hub-Signature-256
  ADD COLUMN app_secret_hint text,
  ADD COLUMN verify_token    text;   -- token verifikasi webhook GET (hub.verify_token)

ALTER TABLE meta_connections DROP CONSTRAINT IF EXISTS meta_connections_status_check;
ALTER TABLE meta_connections ADD CONSTRAINT meta_connections_status_check
  CHECK (status IN ('pending', 'connected', 'invalid', 'expired', 'error', 'disconnected'));

-- Sesi WhatsApp Web (mode QR). Dijalankan oleh service terpisah; kredensial sesi terenkripsi.
CREATE TABLE wa_sessions (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id     uuid NOT NULL UNIQUE REFERENCES workspaces (id) ON DELETE CASCADE,
  session_data_enc bytea,
  phone_masked     text,
  status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'connected', 'disconnected')),
  last_seen_at     timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER wa_sessions_updated_at BEFORE UPDATE ON wa_sessions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Info iklan, sumber lead, dan kota (bagian 5 dan 6 dokumen).
ALTER TABLE leads
  ADD COLUMN ad_headline text,
  ADD COLUMN source_url  text,
  ADD COLUMN city        text,
  ADD COLUMN push_name   text,
  ADD COLUMN source      text NOT NULL DEFAULT 'webhook' CHECK (source IN ('webhook', 'api', 'qr', 'organic'));
CREATE UNIQUE INDEX leads_workspace_ctwa_key ON leads (workspace_id, ctwa_clid)
  WHERE ctwa_clid IS NOT NULL AND deleted_at IS NULL;

-- Event: mode saat dibuat, payload (tanpa token), fbtrace_id, status menunggu nilai, sumber API.
ALTER TABLE events
  ADD COLUMN mode            text CHECK (mode IN ('waba', 'qr')),
  ADD COLUMN request_payload jsonb,
  ADD COLUMN fbtrace_id      text;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_status_check;
ALTER TABLE events ADD CONSTRAINT events_status_check
  CHECK (status IN ('queued', 'sent', 'failed', 'skipped', 'pending_value'));
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_source_check;
ALTER TABLE events ADD CONSTRAINT events_source_check
  CHECK (source IN ('rule', 'manual', 'test', 'api'));

-- API key per workspace (untuk POST /v1/leads dan /v1/events). workspace_id NULL = key admin lama.
ALTER TABLE api_keys
  ADD COLUMN workspace_id    uuid REFERENCES workspaces (id) ON DELETE CASCADE,
  ADD COLUMN created_by_user uuid REFERENCES users (id) ON DELETE SET NULL;
CREATE INDEX api_keys_workspace_idx ON api_keys (workspace_id) WHERE workspace_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS api_keys_workspace_idx;
ALTER TABLE api_keys DROP COLUMN IF EXISTS created_by_user, DROP COLUMN IF EXISTS workspace_id;

UPDATE events SET status = 'queued' WHERE status = 'pending_value';
UPDATE events SET source = 'manual' WHERE source = 'api';
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_source_check;
ALTER TABLE events ADD CONSTRAINT events_source_check CHECK (source IN ('rule', 'manual', 'test'));
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_status_check;
ALTER TABLE events ADD CONSTRAINT events_status_check CHECK (status IN ('queued', 'sent', 'failed', 'skipped'));
ALTER TABLE events DROP COLUMN IF EXISTS fbtrace_id, DROP COLUMN IF EXISTS request_payload, DROP COLUMN IF EXISTS mode;

DROP INDEX IF EXISTS leads_workspace_ctwa_key;
ALTER TABLE leads
  DROP COLUMN IF EXISTS source, DROP COLUMN IF EXISTS push_name, DROP COLUMN IF EXISTS city,
  DROP COLUMN IF EXISTS source_url, DROP COLUMN IF EXISTS ad_headline;

DROP TABLE IF EXISTS wa_sessions;

UPDATE meta_connections SET status = 'error' WHERE status = 'disconnected';
ALTER TABLE meta_connections DROP CONSTRAINT IF EXISTS meta_connections_status_check;
ALTER TABLE meta_connections ADD CONSTRAINT meta_connections_status_check
  CHECK (status IN ('pending', 'connected', 'invalid', 'expired', 'error'));
ALTER TABLE meta_connections
  DROP COLUMN IF EXISTS verify_token, DROP COLUMN IF EXISTS app_secret_hint, DROP COLUMN IF EXISTS app_secret_enc,
  DROP COLUMN IF EXISTS phone_number_id, DROP COLUMN IF EXISTS waba_id, DROP COLUMN IF EXISTS mode;
