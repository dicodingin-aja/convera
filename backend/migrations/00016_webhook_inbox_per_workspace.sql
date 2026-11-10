-- +goose Up
-- Penjamin idempoten pesan WhatsApp dibuat per workspace. Sebelumnya unik (provider, message_id) untuk seluruh
-- sistem: bila pesan yang sama masuk ke dua workspace (mis. satu nomor WA dipakai dua workspace lewat penyedia
-- yang meneruskan webhook ke keduanya), workspace kedua diam-diam membuangnya.
ALTER TABLE webhook_inbox DROP CONSTRAINT webhook_inbox_provider_message_id_key;
ALTER TABLE webhook_inbox ADD CONSTRAINT webhook_inbox_workspace_provider_message_key UNIQUE (workspace_id, provider, message_id);

-- +goose Down
-- Baris dengan (provider, message_id) sama di beberapa workspace disisakan satu agar batasan lama bisa dipasang lagi.
DELETE FROM webhook_inbox a USING webhook_inbox b
WHERE a.provider = b.provider AND a.message_id = b.message_id AND a.received_at > b.received_at;
DELETE FROM webhook_inbox a USING webhook_inbox b
WHERE a.provider = b.provider AND a.message_id = b.message_id AND a.received_at = b.received_at AND a.id > b.id;
ALTER TABLE webhook_inbox DROP CONSTRAINT webhook_inbox_workspace_provider_message_key;
ALTER TABLE webhook_inbox ADD CONSTRAINT webhook_inbox_provider_message_id_key UNIQUE (provider, message_id);
