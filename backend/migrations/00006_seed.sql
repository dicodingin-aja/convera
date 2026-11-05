-- Data awal sesuai dokumen dan nilai yang sudah dipakai frontend (admin/src/data.js).
-- Rekening bank dan akun admin sengaja TIDAK di-seed: isi lewat Convera Admin / CLI.

-- +goose Up
INSERT INTO settings (key, value) VALUES
  ('trial_days',          '14'),
  ('pay_months_12',       '10'),   -- bayar 12 bulan setara 10 bulan
  ('grace_days',          '3'),
  ('komisi_pct',          '10'),
  ('kas_pct',             '10'),
  ('maintenance',         'false'),
  ('maintenance_message', '"Convera sedang pemeliharaan singkat. Event tetap tercatat."'),
  ('integrations',        '{"meta": true, "whatsapp": true, "notifyProof": true, "email": false, "storage": true, "sheets": false}')
ON CONFLICT (key) DO NOTHING;

INSERT INTO plans (code, name, price_month, price_12m, features, shown, sort) VALUES
  ('workspace', 'Workspace', 150000, 1500000,
   '["1 workspace", "Event & Log Event", "Laporan", "Koneksi Meta & WhatsApp"]', true, 0)
ON CONFLICT (code) DO NOTHING;

INSERT INTO sales_people (name, is_founder, sort) VALUES
  ('Jordi', true, 0),
  ('Axl', true, 1),
  ('Albert', true, 2)
ON CONFLICT (name) DO NOTHING;

-- qr dimatikan: WA Bisnis (scan QR) belum diputuskan (bab 7 dan 13).
INSERT INTO feature_flags (key, enabled, badge) VALUES
  ('qr',        false, 'Beta'),
  ('simulasi',  true,  NULL),
  ('laporan',   true,  NULL),
  ('tiktok',    false, 'Segera'),
  ('instagram', false, 'Segera')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM feature_flags WHERE key IN ('qr', 'simulasi', 'laporan', 'tiktok', 'instagram');
DELETE FROM sales_people WHERE name IN ('Jordi', 'Axl', 'Albert');
DELETE FROM plans WHERE code = 'workspace';
DELETE FROM settings WHERE key IN ('trial_days', 'pay_months_12', 'grace_days', 'komisi_pct', 'kas_pct',
  'maintenance', 'maintenance_message', 'integrations');
