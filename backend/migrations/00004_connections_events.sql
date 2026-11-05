-- Bab 4.3 Koneksi, lead, dan event.
-- Isi pesan WhatsApp TIDAK disimpan di tabel mana pun.

-- +goose Up
CREATE TABLE meta_connections (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id      uuid NOT NULL UNIQUE REFERENCES workspaces (id) ON DELETE CASCADE,
  pixel_id          text CHECK (pixel_id ~ '^[0-9]{5,25}$'), -- Pixel / Dataset ID
  access_token_enc  bytea,  -- AES-256-GCM; tidak pernah dikembalikan utuh ke frontend
  access_token_hint text,   -- 4 karakter terakhir, untuk tampilan tersamar
  test_event_code   text,
  token_expires_at  timestamptz,
  status            text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'connected', 'invalid', 'expired', 'error')),
  last_error_code   text,
  last_check_at     timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX meta_connections_token_expires_idx ON meta_connections (token_expires_at) WHERE token_expires_at IS NOT NULL;
CREATE TRIGGER meta_connections_updated_at BEFORE UPDATE ON meta_connections FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE wa_connections (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id       uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  type               text NOT NULL CHECK (type IN ('waba', 'qr')),
  -- Teks bebas, mis. 'kirimchat', 'cloud_api'. Penyedia belum diputuskan, jadi tidak dibatasi CHECK.
  provider           text NOT NULL CHECK (provider ~ '^[a-z0-9_]+$'),
  phone              text CHECK (phone ~ '^62[0-9]{7,13}$'),
  label              text,
  webhook_secret_enc bytea, -- AES-256-GCM; hanya ditampilkan sekali saat dibuat
  waba_id            text,
  phone_number_id    text,
  -- Pengaturan khusus penyedia yang BUKAN rahasia. Rahasia wajib di kolom *_enc.
  provider_settings  jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(provider_settings) = 'object'),
  status             text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'connected', 'disconnected', 'error')),
  last_active_at     timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz
);
-- URL webhook di dokumen dibuat per workspace (/webhook/{slug} dan /webhook/cloud/{slug}),
-- jadi saat ini satu koneksi per penyedia per workspace. Hapus index ini bila nanti diputuskan
-- satu workspace boleh punya beberapa nomor pada penyedia yang sama.
CREATE UNIQUE INDEX wa_connections_workspace_provider_key ON wa_connections (workspace_id, provider) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX wa_connections_phone_number_id_key ON wa_connections (provider, phone_number_id)
  WHERE phone_number_id IS NOT NULL AND deleted_at IS NULL;
CREATE TRIGGER wa_connections_updated_at BEFORE UPDATE ON wa_connections FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE leads (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id           uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  wa_connection_id       uuid REFERENCES wa_connections (id) ON DELETE SET NULL,
  phone                  text NOT NULL CHECK (phone ~ '^62[0-9]{7,13}$'),
  name                   text,
  ctwa_clid              text,  -- NULL = chat organik (bukan dari iklan)
  ad_id                  text,
  campaign_id            text,
  clicked_at             timestamptz,
  attribution_expires_at timestamptz, -- clicked_at + 7 hari
  first_message_at       timestamptz NOT NULL,
  last_message_at        timestamptz,
  status                 text NOT NULL DEFAULT 'new'
                         CHECK (status IN ('new', 'lead', 'checkout', 'purchase', 'lost')),
  value                  numeric(14,2) CHECK (value >= 0),
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now(),
  deleted_at             timestamptz,
  CONSTRAINT leads_attribution CHECK (attribution_expires_at IS NULL OR clicked_at IS NOT NULL)
);
CREATE UNIQUE INDEX leads_workspace_phone_key ON leads (workspace_id, phone) WHERE deleted_at IS NULL;
CREATE INDEX leads_workspace_first_msg_idx ON leads (workspace_id, first_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX leads_workspace_status_idx ON leads (workspace_id, status) WHERE deleted_at IS NULL;
CREATE INDEX leads_ctwa_clid_idx ON leads (ctwa_clid) WHERE ctwa_clid IS NOT NULL;
CREATE TRIGGER leads_updated_at BEFORE UPDATE ON leads FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE event_rules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  -- customer = pelanggan mengirim pesan; admin = admin/CS membalas; template = template terkirim
  who         text NOT NULL CHECK (who IN ('customer', 'admin', 'template')),
  -- contains = mengandung; equals = sama persis
  op          text NOT NULL CHECK (op IN ('contains', 'equals')),
  keywords    text[] NOT NULL CHECK (cardinality(keywords) > 0),
  -- Teks bebas (Contact, Lead, Hot Lead, Purchase, Complaint, Report); nama custom event belum final.
  event_name  text NOT NULL CHECK (length(event_name) BETWEEN 1 AND 50),
  -- parse = baca angka dari pesan ('Rp 450.000', '215rb'); fixed = nilai tetap; none = tanpa nilai
  value_mode  text NOT NULL DEFAULT 'none' CHECK (value_mode IN ('parse', 'fixed', 'none')),
  fixed_value numeric(14,2) CHECK (fixed_value >= 0),
  active      boolean NOT NULL DEFAULT true,
  sort        integer NOT NULL DEFAULT 0,
  hit_count   bigint NOT NULL DEFAULT 0,
  last_hit_at timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  CONSTRAINT event_rules_fixed_value CHECK (value_mode <> 'fixed' OR fixed_value IS NOT NULL)
);
CREATE INDEX event_rules_workspace_idx ON event_rules (workspace_id, sort) WHERE deleted_at IS NULL AND active;
CREATE TRIGGER event_rules_updated_at BEFORE UPDATE ON event_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Sumber Event Log sekaligus antrean pengiriman ke Meta (dispatcher mengambil status 'queued'
-- dengan FOR UPDATE SKIP LOCKED).
CREATE TABLE events (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  lead_id       uuid REFERENCES leads (id) ON DELETE SET NULL,
  rule_id       uuid REFERENCES event_rules (id) ON DELETE SET NULL,
  event_id      text NOT NULL, -- dikirim ke Meta untuk dedup
  event_name    text NOT NULL,
  event_time    timestamptz NOT NULL DEFAULT now(),
  value         numeric(14,2) CHECK (value >= 0),
  currency      char(3) NOT NULL DEFAULT 'IDR',
  -- rule = dari aturan; manual = tandai purchase manual; test = tombol Tes koneksi
  source        text NOT NULL CHECK (source IN ('rule', 'manual', 'test')),
  -- queued = antre; sent = terkirim; failed = gagal;
  -- skipped = tidak dikirim (mis. di luar jendela atribusi 7 hari) — tambahan di luar dokumen
  status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'failed', 'skipped')),
  attempts      integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_retry_at timestamptz DEFAULT now(),
  meta_response jsonb,
  error_code    text,
  error_message text,
  sent_at       timestamptz,
  created_by    uuid REFERENCES users (id) ON DELETE SET NULL, -- untuk source = manual
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workspace_id, event_id),
  CONSTRAINT events_sent_at CHECK (status <> 'sent' OR sent_at IS NOT NULL)
);
CREATE INDEX events_queue_idx ON events (next_retry_at) WHERE status = 'queued';
CREATE INDEX events_workspace_created_idx ON events (workspace_id, created_at DESC);
CREATE INDEX events_workspace_status_idx ON events (workspace_id, status, created_at DESC);
CREATE INDEX events_lead_idx ON events (lead_id) WHERE lead_id IS NOT NULL;
CREATE TRIGGER events_updated_at BEFORE UPDATE ON events FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Penjamin idempoten webhook: satu ID pesan hanya diproses sekali. Tidak menyimpan isi pesan.
CREATE TABLE webhook_inbox (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  wa_connection_id uuid REFERENCES wa_connections (id) ON DELETE SET NULL,
  provider         text NOT NULL,
  message_id       text NOT NULL,
  direction        text NOT NULL CHECK (direction IN ('in', 'out')), -- Message Received / Message Sent
  received_at      timestamptz NOT NULL DEFAULT now(),
  processed_at     timestamptz,
  error            text, -- ringkasan error tanpa isi pesan
  UNIQUE (provider, message_id)
);
CREATE INDEX webhook_inbox_received_idx ON webhook_inbox (received_at);

-- +goose Down
DROP TABLE IF EXISTS webhook_inbox;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS event_rules;
DROP TABLE IF EXISTS leads;
DROP TABLE IF EXISTS wa_connections;
DROP TABLE IF EXISTS meta_connections;
