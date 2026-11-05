-- Tugas terjadwal (bab 5.3): pengingat, rekap harian, retensi data.

-- +goose Up

-- Mencegah pengingat dikirim dua kali. kind: sub_7/sub_3/sub_1 (langganan), trial_7/trial_3/trial_1,
-- token_7/token_1/token_exp (token Meta). ref_date = tanggal berakhir yang diingatkan, sehingga
-- perpanjangan (tanggal berakhir baru) otomatis memulai rangkaian pengingat baru.
CREATE TABLE reminder_log (
  workspace_id uuid        NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  kind         text        NOT NULL,
  ref_date     timestamptz NOT NULL,
  sent_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, kind, ref_date)
);
CREATE INDEX reminder_log_sent_idx ON reminder_log (sent_at);

-- Rekap harian per workspace (hari = tanggal Asia/Jakarta) untuk Overview dan laporan.
CREATE TABLE daily_stats (
  workspace_id   uuid          NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  day            date          NOT NULL,
  chats_from_ads integer       NOT NULL DEFAULT 0,  -- lead baru yang punya ctwa_clid
  leads          integer       NOT NULL DEFAULT 0,  -- semua lead baru
  checkouts      integer       NOT NULL DEFAULT 0,  -- event InitiateCheckout terkirim
  purchases      integer       NOT NULL DEFAULT 0,  -- event Purchase terkirim
  purchase_value numeric(14,2) NOT NULL DEFAULT 0,
  events_sent    integer       NOT NULL DEFAULT 0,
  events_failed  integer       NOT NULL DEFAULT 0,
  updated_at     timestamptz   NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, day)
);

-- Retensi awal (keputusan bab 13 masih terbuka; angka ini bisa diubah admin).
INSERT INTO settings (key, value) VALUES
  ('retention_event_days', '365'),  -- log event lama dihapus; minimal 60 hari agar rekap tetap benar
  ('retention_inbox_days', '30')    -- webhook_inbox (penjamin idempoten) dibersihkan setelah ini
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM settings WHERE key IN ('retention_event_days', 'retention_inbox_days');
DROP TABLE IF EXISTS daily_stats;
DROP TABLE IF EXISTS reminder_log;
