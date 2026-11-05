-- Isi pesan per lead (ditampilkan ke pelanggan di dashboard). Dihapus otomatis setelah 90 hari dan saat kontak dihapus.

-- +goose Up
CREATE TABLE lead_messages (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
  lead_id      uuid NOT NULL REFERENCES leads (id) ON DELETE CASCADE,
  direction    text NOT NULL CHECK (direction IN ('in', 'out')),
  body         text NOT NULL,
  sent_at      timestamptz NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX lead_messages_lead_idx ON lead_messages (lead_id, sent_at DESC);
CREATE INDEX lead_messages_sent_idx ON lead_messages (sent_at);

-- +goose Down
DROP TABLE IF EXISTS lead_messages;
