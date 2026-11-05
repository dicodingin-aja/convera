# API Convera (implementasi `meta-integration.md`)

Format respons: sukses `{ "data": ..., "meta": {page, per_page, total} }`; error `{ "message", "code" }`.
Sesi memakai cookie httpOnly `cv_session`; request POST/PUT/DELETE wajib header `X-CSRF-Token` = cookie `csrf_token`.
Workspace dipilih dengan header `X-Workspace: <slug>` (atau `?workspace=`); bila user hanya punya satu, boleh dikosongkan.
Workspace milik orang lain dijawab 404.

## Alur

```
Webhook Cloud API ─┐
Forward /v1/leads ─┼─> lead (upsert per nomor) ─> event_rules cocok / /v1/events / tandai manual
Sesi QR (internal)─┘          └─> events (status queued) ─> cmd/worker ─> Meta CAPI ─> sent / failed
```

Isi pesan hanya dipakai di memori untuk mencocokkan aturan; tidak disimpan dan tidak di-log.
Pesan diproses di request webhook yang sama (milidetik) dan dijawab 200; yang lambat (panggil Meta) dikerjakan worker.
Idempoten lewat `webhook_inbox (provider, message_id)`.

## Route

| Route | Auth | Fungsi |
|---|---|---|
| `GET/POST /webhook/cloud/:webhook_slug` (alias `/webhooks/whatsapp/:slug`) | signature | Verifikasi (`hub.*`) dan pesan masuk WABA. `X-Hub-Signature-256` dihitung dari **raw body** dengan App Secret workspace |
| `POST /v1/leads` | Bearer API key | Forward lead dari sistem klien |
| `POST /v1/events` | Bearer API key | Pemicu konversi (202). Cari lead lewat `ctwa_clid` lalu `phone` |
| `POST /api/v1/auth/register`, `login`, `logout`; `GET auth/session` | – / sesi | Akun |
| `GET/POST /api/v1/workspaces` | sesi | Buat workspace -> otomatis langganan `trial` (`settings.trial_days`) |
| `GET /overview?from&to` | sesi | KPI, seri harian, top iklan |
| `GET /leads`, `GET /leads/:id`, `POST /leads/:id/conversions` | sesi | Lead dan tandai konversi manual |
| `GET /conversions`, `GET /events`, `GET /events/:id` | sesi | Event Log (`/conversions` tanpa event uji) |
| `POST /events/:id/retry` | sesi | Hanya status `failed` |
| `PUT /events/:id/value` | sesi | Isi nilai event `pending_value` |
| `GET/PUT /connections/meta`, `POST /connections/meta/test` | sesi | Koneksi Meta per workspace |
| `POST /connections/whatsapp/qr`, `GET .../status`, `DELETE /connections/whatsapp` | sesi | Mode QR (lewat layanan sesi) |
| `GET /subscription`, `GET /bank-accounts`, `GET/POST /payment-proofs` | sesi | Halaman bayar: status langganan, rekening tujuan, riwayat, dan **unggah bukti transfer** (multipart) |
| `GET/POST/PUT/DELETE /event-mappings`, `POST /event-mappings/test` | sesi | Aturan event (tabel `event_rules`) |
| `GET/POST/DELETE /api-keys` | sesi | API key; key penuh hanya tampil sekali |
| `POST /internal/qr/:workspace_id/messages`, `/status` | `X-Service-Token` | Dipanggil layanan sesi QR |

## Aturan yang ditegakkan

- Token Meta dan App Secret dienkripsi AES-256-GCM (`ENCRYPTION_KEY`), tidak pernah dikembalikan utuh (hanya `EAAB…x9Qz`).
- Mode WABA: hanya `LeadSubmitted, Purchase, AddToCart, InitiateCheckout, ViewContent`; wajib `ctwa_clid`
  (tanpa itu event `failed`/`not_from_ad`); di luar 7 hari sejak klik -> `skipped`/`attribution_expired`.
- Mode QR: `action_source: chat`, `user_data` ter-hash (ph, fn, ct, country, external_id); mode ini juga butuh feature flag `qr` menyala.
- `Purchase` wajib `value > 0` dan `currency`. Aturan `parse` yang gagal membaca nominal -> event `pending_value`.
- Dedup `event_id`: `workspace + external_id + event`, atau `workspace + lead + event + value + tanggal`.
- Retry: jaringan, 5xx, dan rate limit (1m, 5m, 30m, 2j, 6j) lalu `failed`; error data/token -> langsung `failed`.
  Kode 190 -> koneksi `invalid` + notifikasi ke pemilik workspace.
- Tes Koneksi wajib `test_event_code` agar event uji tidak masuk data asli.
- Rate limit `/v1/*`: 10 req/detik per API key (burst 30). Login 10/menit dan register 5/menit per IP.

## Perbedaan dari dokumen (dan alasannya)

| Dokumen | Implementasi | Alasan |
|---|---|---|
| `phone_hash` + `phone_masked` | Tetap `leads.phone` (62xxx); API hanya mengembalikan nomor tersamar | Keputusan tim; hash dihitung saat kirim payload mode QR |
| `/webhooks/whatsapp/:workspaceId` | `/webhook/cloud/:webhook_slug` (alias lama tetap ada) | Slug acak agar URL tidak bisa ditebak (HANDOFF) |
| `/api/leads` dan seterusnya | `/api/v1/...` | Sesuai `src/services/backend.js` |
| Tabel `event_mappings`, `conversion_events`, `processed_messages` | Memakai `event_rules`, `events`, `webhook_inbox` yang sudah ada | "Jangan buat ulang yang sudah ada" |
| `source = mapping`; status `duplicate`, `expired` | `rule`; `duplicate` hanya di respons; `expired` -> `skipped` + `error_code` | Menjaga CHECK constraint yang ada |
| Role Owner/Admin/CS | Hanya pemilik workspace | Tabel anggota tim belum ada (HANDOFF bagian 8) |
| `note` pada konversi manual | Diabaikan | Belum ada kolomnya |

## Belum dikerjakan / perlu diputuskan

- **Layanan sesi WhatsApp Web (Baileys)** untuk mode QR belum dibuat; sisi Convera (endpoint proxy, ingest, status, tabel `wa_sessions`) sudah siap.
- `GRAPH_API_VERSION` harus diisi manual; heuristik terjemahan error Meta belum diverifikasi dengan respons asli.
- Payload WABA dan format Test Events belum diuji ke Meta sungguhan (hanya ke server tiruan).
- Verifikasi signature Kirim.chat belum ada; baru WhatsApp Cloud API.
- Refresh token sesi, verifikasi email, reset password, dan endpoint admin belum dibuat.
- Frontend belum disambungkan ke endpoint ini (masih mock, halaman Koneksi masih menyimpan token di localStorage).

## Unggah bukti transfer

`POST /api/v1/payment-proofs` (multipart/form-data, header `X-Workspace` bila punya >1 workspace, `X-CSRF-Token` wajib):

| Field | Wajib | Keterangan |
|---|---|---|
| `plan` | ya | kode paket yang tampil (`workspace`) |
| `months` | ya | 1, 3, atau 12 |
| `bank`, `sender` | ya | bank dan nama pemilik rekening pengirim |
| `ref`, `message`, `bank_account_id` | tidak | no. referensi, catatan, rekening tujuan yang dipakai (harus aktif) |
| `amount` | tidak | nominal yang ditransfer menurut customer (klaim, untuk dicocokkan admin; tagihan dihitung server) |
| `file` | ya | JPG/PNG/PDF, maks. 5 MB. **Jenis ditentukan dari isi berkas**, bukan nama file |

- Berkas disimpan di `UPLOAD_DIR` dengan nama acak (izin 0600), tidak pernah disajikan langsung. Admin membukanya lewat
  `GET /api/admin/v1/payment-proofs/{id}/file` (sesi admin, tiap pembukaan masuk audit log; `nosniff`, CSP sandbox, tanpa cache).
- Batas: 3 bukti menunggu per workspace (409), 10 unggahan/jam per user (429), body maks. 6 MB (413).
- Langganan menjadi `pending` kecuali sudah `stopped` atau masih `active` dengan sisa waktu (perpanjang lebih awal tidak
  membuat langganan yang berjalan tampak tertunda). `kind` otomatis: `renew` (pernah bayar), `trial`, atau `new`.
- Efek samping: notifikasi ke semua admin aktif dan audit log `payment_proof.upload`. Bila request gagal, berkas yang sudah ditulis dihapus lagi.

## Akun, sesi, dan data pribadi (bab 5.1)

| Route | Fungsi |
|---|---|
| `POST /auth/forgot-password` | Selalu menjawab sama (tidak membocorkan email terdaftar); tautan berlaku 1 jam, sekali pakai |
| `POST /auth/reset-password` | `{token, password}`; mencabut semua sesi lama |
| `POST /auth/verify-email`, `POST /auth/resend-verification` | Verifikasi email (tautan 48 jam). Belum membatasi fitur apa pun |
| `POST /auth/refresh`, `GET /auth/csrf` | Memutar token sesi (token lama dicabut); membaca token CSRF |
| `GET/PATCH /me/profile`, `POST /me/password` | Profil (nama, telepon dinormalisasi 62...), ganti password (sesi lain dicabut) |
| `GET/PATCH/DELETE /workspaces/{slug}` | Detail, ubah, dan hapus workspace (`{confirm: slug}`) |
| `DELETE /me` | `{password}`; menghapus semua workspace pemilik + mengosongkan data pribadi akun |
| `GET /notifications`, `POST /notifications/read-all`, `POST /notifications/{id}/read` | Lonceng customer |
| `POST /presence/heartbeat`, `GET /status` | Kehadiran (untuk "online sekarang" di admin), status maintenance publik |
| `GET /events.csv` | Ekspor Event Log (filter sama dengan `/events`; nomor tersamar; mencegah injeksi rumus) |
| `POST /leads/{id}/mark-purchase` | Alias resmi tandai purchase |
| `/event-rules`, `/event-rules/simulate`, `/connections/test-capi` | Nama resmi di dokumen untuk aturan event, simulasi (pesan tidak disimpan), dan tes CAPI (maks. 5/menit) |

**Penghapusan data (UU PDP).** Menghapus workspace/akun menghapus lead, event, aturan, koneksi Meta (token terenkripsi),
sesi WhatsApp, API key, dan rekap. Catatan keuangan (`payments`, `payment_proofs` beserta berkasnya) **dipertahankan** untuk
pembukuan; lama simpannya belum diputuskan (bab 13).

## Tugas terjadwal (worker)

| Tugas | Jadwal | Isi |
|---|---|---|
| Kedaluwarsa langganan | tiap 5 menit | active melewati masa tenggang / trial lewat -> `expired` + notifikasi |
| Rekap harian | tiap 5 menit (2 hari), tiap jam (35 hari) | `daily_stats` per workspace (hari Asia/Jakarta); Overview membacanya |
| Pengingat | tiap jam | Langganan & trial H-7/H-3/H-1, token Meta H-7/H-1, token kedaluwarsa -> status `expired`. Lonceng + email; sekali per tanggal berakhir (`reminder_log`) |
| Retensi | tiap 24 jam | Hapus event lama (`retention_event_days`, default 365, min. 60), `webhook_inbox` (default 30), sesi & token kedaluwarsa, notifikasi lama yang sudah dibaca. Data keuangan tidak disentuh |

Isi `token_expires_at` di `PUT /connections/meta` (tanggal berlaku token bila Meta menyebutnya) agar pengingat token jalan.
Token System User yang tidak kedaluwarsa dibiarkan kosong.

## Dashboard customer (data nyata)

| Route | Fungsi |
|---|---|
| `GET /overview?from&to` | Ditambah `previous` (periode pembanding), `funnel`, `top_ads`, `health`, dan seri harian dari `daily_stats` |
| `POST /leads/bulk-status` | `{ids, status}` hanya `lost` (bukan dari purchase) atau `new` (memulihkan yang hilang) |
| `POST /leads/erase` | `{phone}` menghapus permanen lead dengan nomor itu + event-nya di workspace ini (UU PDP); audit hanya menyimpan nomor tersamar. Diblokir dalam mode dukungan |
| `GET /events`, `GET /events/{id}` | Ditambah `event_id`, `ctwa_clid`, dan `hint` (penjelasan + saran untuk customer per kode error) |
