-- Slot kampanye iklan: customer membayar Rp150.000 (harga paket per bulan) per kampanye yang dilacak, per 30 hari.
-- Satu slot = satu kampanye Meta. Bila kampanye sebuah iklan belum diketahui (akun Meta Ads belum tersambung atau
-- belum tersinkron), satu iklan dihitung satu slot.

-- +goose Up

-- Peta iklan -> kampanye dari sinkron Meta Ads. Sengaja tidak dihapus saat koneksi diputus, agar kampanye yang
-- sudah dilacak tetap dikenali untuk iklan-iklan yang pernah tersinkron.
CREATE TABLE ad_campaign_map (
  workspace_id  uuid        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  ad_id         text        NOT NULL,
  campaign_id   text        NOT NULL,
  campaign_name text,
  ad_name       text,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, ad_id)
);
CREATE INDEX ad_campaign_map_campaign_idx ON ad_campaign_map (workspace_id, campaign_id);

INSERT INTO ad_campaign_map (workspace_id, ad_id, campaign_id, campaign_name, ad_name)
SELECT DISTINCT ON (workspace_id, ad_id) workspace_id, ad_id, campaign_id, campaign_name, ad_name
FROM ad_insights_daily WHERE campaign_id IS NOT NULL
ORDER BY workspace_id, ad_id, day DESC;

-- Kampanye (atau iklan) yang dilacak. Satu baris memakai satu slot sejak started_at. Bila dilepas (stopped_at),
-- slotnya tetap terpakai sampai 30 hari sejak started_at, agar satu slot tidak dipindah-pindah ke banyak kampanye.
CREATE TABLE tracked_targets (
  id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid       NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  kind        text        NOT NULL CHECK (kind IN ('campaign', 'ad')),
  target_id   text        NOT NULL CHECK (target_id ~ '^[A-Za-z0-9_.:-]{1,64}$'),
  name        text,
  auto        boolean     NOT NULL DEFAULT false, -- dipilih otomatis oleh Convera karena slot masih kosong
  started_at  timestamptz NOT NULL DEFAULT now(),
  stopped_at  timestamptz,
  CONSTRAINT tracked_targets_stop_after_start CHECK (stopped_at IS NULL OR stopped_at >= started_at)
);
CREATE UNIQUE INDEX tracked_targets_active_key ON tracked_targets (workspace_id, kind, target_id) WHERE stopped_at IS NULL;
CREATE INDEX tracked_targets_ws_idx ON tracked_targets (workspace_id, started_at);

-- Jumlah slot yang dibeli. purchase: 'subscription' = langganan (perpanjangan berurutan), 'slots' = tambah slot
-- di tengah periode (30 hari sejak disetujui, berjalan sendiri).
ALTER TABLE payment_proofs
  ADD COLUMN ad_slots integer NOT NULL DEFAULT 1 CHECK (ad_slots BETWEEN 1 AND 100),
  ADD COLUMN purchase text    NOT NULL DEFAULT 'subscription' CHECK (purchase IN ('subscription', 'slots'));
ALTER TABLE payments
  ADD COLUMN ad_slots integer NOT NULL DEFAULT 1 CHECK (ad_slots BETWEEN 1 AND 100),
  ADD COLUMN purchase text    NOT NULL DEFAULT 'subscription' CHECK (purchase IN ('subscription', 'slots'));
CREATE INDEX payments_slots_idx ON payments (workspace_id, period_start, period_end);

INSERT INTO settings (key, value) VALUES ('trial_ad_slots', '1') ON CONFLICT (key) DO NOTHING;

UPDATE plans SET features = '["Per kampanye iklan", "Event & Log Event", "Laporan & Performa Iklan", "Koneksi Meta & WhatsApp"]'::jsonb
WHERE code = 'workspace' AND features = '["1 workspace", "Event & Log Event", "Laporan", "Koneksi Meta & WhatsApp"]'::jsonb;

-- +goose Down
UPDATE plans SET features = '["1 workspace", "Event & Log Event", "Laporan", "Koneksi Meta & WhatsApp"]'::jsonb
WHERE code = 'workspace' AND features = '["Per kampanye iklan", "Event & Log Event", "Laporan & Performa Iklan", "Koneksi Meta & WhatsApp"]'::jsonb;
DELETE FROM settings WHERE key = 'trial_ad_slots';
DROP INDEX IF EXISTS payments_slots_idx;
ALTER TABLE payments DROP COLUMN purchase, DROP COLUMN ad_slots;
ALTER TABLE payment_proofs DROP COLUMN purchase, DROP COLUMN ad_slots;
DROP TABLE IF EXISTS tracked_targets;
DROP TABLE IF EXISTS ad_campaign_map;
