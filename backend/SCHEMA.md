# Skema Database Convera

Ringkasan skema PostgreSQL berdasarkan bab 4 dokumen *Convera – Kebutuhan Backend*.
Definisi lengkap ada di folder [migrations/](migrations/).

## Konvensi

- Kunci utama UUID, waktu `timestamptz` (UTC), uang `NUMERIC(14,2)` dalam Rupiah.
- Nomor HP format `62xxxxxxxxxx` (dicek oleh database).
- Kolom `*_enc` berisi ciphertext AES-256-GCM (`bytea`). Kunci enkripsi ada di luar database.
- Hapus lunak dengan `deleted_at` untuk data bisnis.
- Nilai kode (status, tipe) memakai kode Inggris huruf kecil. Label Indonesia ditampilkan frontend.
- `updated_at` diperbarui otomatis oleh trigger.
- **Isi pesan WhatsApp tidak disimpan di tabel mana pun.**

## Tabel

### 4.1 Akun dan akses — `00002_accounts.sql`

| Tabel | Isi |
|---|---|
| `users` | Akun customer. `username` unik (tidak peka huruf besar/kecil) di antara akun yang belum dihapus; login memakai username; `email` tetap disimpan sebagai kontak (unik, tanpa verifikasi; migrasi 00013). |
| `admins` | Akun Convera Admin. Role `superadmin` / `admin`. |
| `sessions` | Sesi login (hash refresh token). Milik tepat satu: user **atau** admin. |
| `password_resets` | Token reset password sekali pakai (hash). |

### 4.2 Bisnis, langganan, dan pembayaran — `00003_business.sql`

| Tabel | Isi |
|---|---|
| `workspaces` | Satu bisnis. `slug` untuk URL app, `webhook_slug` acak untuk URL webhook. |
| `plans` | Paket. `code` = id yang dipakai frontend (`workspace`). |
| `subscriptions` | Status langganan terkini, **satu baris per workspace**. |
| `payment_proofs` | Bukti transfer dan antrean verifikasi admin. |
| `payments` | Pembayaran yang disetujui, dengan snapshot `komisi_pct` dan `kas_pct`. Sales wajib. |
| `bank_accounts` | Rekening tujuan transfer. |
| `sales_people` | Penerima komisi. `is_founder` = ikut pembagian 80% rata. |
| `payouts` | Penanda bagi hasil sudah ditransfer, per bulan per orang. |
| `settings` | Pengaturan global (key → jsonb). |
| `feature_flags` | Nyala/mati fitur. |

### 4.3 Koneksi, lead, dan event — `00004_connections_events.sql`

| Tabel | Isi |
|---|---|
| `meta_connections` | Pixel ID dan access token CAPI (terenkripsi), satu per workspace. |
| `wa_connections` | Koneksi WhatsApp. Satu per penyedia per workspace. |
| `leads` | Kontak dari chat: nomor, `ctwa_clid`, iklan, jendela atribusi 7 hari. Satu per nomor per workspace. |
| `event_rules` | Aturan pencocokan pesan → event. |
| `events` | Event Log **sekaligus antrean** pengiriman ke Meta. `event_id` unik per workspace (dedup). |
| `webhook_inbox` | Penjamin idempoten: `(provider, message_id)` hanya diproses sekali. |

### 4.4 Operasional — `00005_operations.sql`

| Tabel | Isi |
|---|---|
| `notifications` | Lonceng customer/admin. |
| `audit_logs` | Catatan aktivitas admin/user/sistem. |
| `api_keys` | API key (hanya hash + prefix untuk tampilan). |

### Data awal — `00006_seed.sql`

- `settings`: trial 14 hari, bayar 12 bulan = 10 bulan, masa tenggang 3 hari, komisi 10%, kas 10%,
  maintenance mati, pesan maintenance, toggle integrasi.
- `plans`: Workspace, Rp150.000/bulan, Rp1.500.000/12 bulan.
- `sales_people`: Jordi, Axl, Albert (founder).
- `feature_flags`: qr (mati), simulasi, laporan, tiktok (mati), instagram (mati).
- Tidak di-seed: rekening bank dan akun admin.

## Kode nilai

| Kolom | Nilai |
|---|---|
| `subscriptions.status` | `trial`, `active`, `expired`, `pending`, `stopped` |
| `payment_proofs.status` | `pending`, `approved`, `rejected` |
| `payment_proofs.kind` | `new`, `renew`, `trial` |
| `leads.status` | `new` (Baru), `lead`, `checkout`, `purchase`, `lost` (Hilang) |
| `event_rules.who` | `customer` (pelanggan), `admin`, `template` |
| `event_rules.op` | `contains` (mengandung), `equals` (sama dengan) |
| `event_rules.value_mode` | `parse` (angka dari pesan), `fixed` (nilai tetap), `none` (tanpa nilai) |
| `events.status` | `queued` (antre), `sent` (terkirim), `failed` (gagal), `skipped` (tidak dikirim) |
| `events.source` | `rule`, `manual`, `test` |
| `wa_connections.type` | `waba`, `qr` |
| `wa_connections.provider` | teks bebas, mis. `kirimchat`, `cloud_api` |

## Perbedaan dari dokumen

| Perubahan | Alasan |
|---|---|
| `payment_proofs`: `kind`, `plan_id`, `months`, `uploaded_by`, `file_mime`, `file_size` | Paket pilihan customer dan validasi file (jpg/png/pdf) dicatat sebelum admin memutuskan. |
| `payments`: `plan_id`, `payment_proof_id`, `period_start`, `period_end` | Riwayat periode yang dibayar dan tautan ke bukti transfernya. |
| `subscriptions.status` tambah `stopped` | Untuk aksi admin "Stop" (POST customers/{id}/stop). |
| `events.status` tambah `skipped` | Event di luar jendela atribusi 7 hari tetap terlihat di Event Log. |
| `sales_people.is_founder` | Membedakan founder (ikut 80% rata) dari sales luar (hanya komisi). |
| `notifications`: `body`, `link`, `workspace_id` | Isi dan tujuan klik notifikasi. |
| `audit_logs`: `actor_type`, `actor_label`, `workspace_id` | Nama pelaku tetap terbaca walau akun dihapus. |
| `meta_connections`: `access_token_hint`, `last_error_code` | Tampilan token tersamar dan saran perbaikan. |
| `wa_connections`: `phone`, `label`, `provider_settings` | Data tampilan dan pengaturan penyedia yang bukan rahasia. |
| `leads`: `clicked_at`, `last_message_at`, `value`, `wa_connection_id` | Dasar jendela atribusi dan tampilan halaman Leads. |
| `webhook_inbox.provider + message_id` unik bersama | Dua penyedia bisa memakai ID pesan yang sama. |

## Catatan untuk integrasi Meta dan WhatsApp

Bagian ini belum final dan perlu dicek oleh yang menangani Meta/WhatsApp:

- **Penyedia WhatsApp** belum dipilih. `wa_connections.provider` sengaja teks bebas.
- **Field dari payload webhook** yang disimpan baru `ctwa_clid`, `ad_id`, `campaign_id`, `clicked_at`.
  Bila payload asli punya field lain yang perlu disimpan, tambahkan lewat migrasi baru.
- **Nama custom event** (Hot Lead, Complaint, Report) disimpan sebagai teks, jadi bisa diganti tanpa
  mengubah skema.
- **Isi pesan tidak disimpan.** Pencocokan aturan sebaiknya dilakukan langsung saat webhook diterima
  (di memori, hitungan milidetik). Yang masuk antrean adalah baris `events`, bukan pesannya.
  Kalau pencocokan dipindah ke worker, isi pesan harus disimpan sementara, dan itu bertentangan
  dengan aturan bab 7.
- **Satu koneksi per penyedia per workspace** mengikuti URL webhook per workspace di dokumen.
  Kalau satu workspace perlu beberapa nomor pada penyedia yang sama, hapus index
  `wa_connections_workspace_provider_key` dan buat URL webhook per koneksi.
