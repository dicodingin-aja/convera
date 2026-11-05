-- Layanan sesi WhatsApp Web (waqr): JID perangkat tertaut per workspace, agar sesi dipulihkan setelah restart.

-- +goose Up
ALTER TABLE wa_sessions ADD COLUMN device_jid text;
CREATE UNIQUE INDEX wa_sessions_device_jid_key ON wa_sessions (device_jid) WHERE device_jid IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS wa_sessions_device_jid_key;
ALTER TABLE wa_sessions DROP COLUMN IF EXISTS device_jid;
