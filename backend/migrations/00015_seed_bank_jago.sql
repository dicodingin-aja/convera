-- Rekening tujuan transfer Convera (Bank Jago). Bisa diubah/dinonaktifkan dari Convera Admin > Paket & Harga.

-- +goose Up
INSERT INTO bank_accounts (bank, number, holder, active, sort)
SELECT 'Bank Jago', '105362636213', 'Convera', true, 0
WHERE NOT EXISTS (SELECT 1 FROM bank_accounts WHERE number = '105362636213' AND deleted_at IS NULL);

-- +goose Down
DELETE FROM bank_accounts WHERE number = '105362636213' AND bank = 'Bank Jago';
