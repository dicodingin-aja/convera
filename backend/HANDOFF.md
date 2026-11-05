# Catatan Serah Terima Backend

Status per 5 November 2026. Baca ini dulu sebelum melanjutkan backend atau integrasi Meta/WhatsApp.

## 1. Yang sudah selesai

- **Skema database bab 4** lengkap: 24 tabel (23 dari dokumen + `email_verifications`) dan data awal.
  Ringkasan tabel, kode nilai, dan perbedaan dari dokumen ada di [SCHEMA.md](SCHEMA.md).
- **Program migrasi** `cmd/migrate`: `createdb`, `up`, `down`, `status`, `reset`, `create`.
  Sudah diuji naik, turun, dan naik lagi di PostgreSQL 18, dengan 26 uji batasan data yang lulus.
- **Penyesuaian frontend** (webapp customer):
  - Login/logout memanggil `POST /api/v1/auth/login` dan `POST /api/v1/auth/logout`, sesuai dokumen.
  - URL webhook yang ditampilkan memakai `https://api.convera.id/webhook/...`, bukan `app.`.

**Belum ada:** server API Gin, worker, dan semua endpoint bab 5. Belum ada juga kode enkripsi,
hash password, atau pemanggilan Meta.

## 2. Menjalankan di komputer sendiri

```
cd backend
cp .env.example .env               # isi password PostgreSQL di .env (JANGAN di .env.example)
go run ./cmd/migrate createdb      # membuat db_convera
go run ./cmd/migrate up
```

Kebutuhan: Go 1.25+ dan PostgreSQL 15+.

## 3. Keputusan yang sudah diambil

| Keputusan | Isi |
|---|---|
| Stack | Go + Gin, `pgx` untuk database, `goose` untuk migrasi |
| Repo | Monorepo: `src/` (customer), `admin/` (admin), `backend/` |
| Domain webhook | `api.convera.id/webhook/{webhook_slug}` dan `/webhook/cloud/{webhook_slug}` |
| URL webhook | Memakai `workspaces.webhook_slug` (acak, 32 hex), **bukan** `slug` bisnis, agar tidak bisa ditebak |
| Kode nilai | Kode Inggris di database (`customer`, `contains`, `queued`, ...). Frontend masih memakai kode Indonesia (`pelanggan`, `mengandung`, `angka`); terjemahkan di API. Tabelnya ada di SCHEMA.md. |
| Koneksi WhatsApp | Satu koneksi per penyedia per workspace, mengikuti URL webhook per workspace di dokumen |

## 4. Penting: alur pesan WhatsApp

Dokumen meminta dua hal yang bertentangan:
- Pesan masuk ke antrean lalu diproses worker (bab 1 dan 5.2).
- Isi pesan tidak boleh disimpan di database (bab 4.3 dan 7).

Isi pesan yang masuk antrean di PostgreSQL berarti tersimpan. **Skema dibuat untuk alur ini:**

1. Webhook menerima pesan, memverifikasi signature, lalu mencatat `(provider, message_id)` di
   `webhook_inbox`. Kalau sudah ada, pesan dilewati.
2. **Di request yang sama**, langsung di memori: normalisasi nomor, ambil `ctwa_clid`, simpan atau
   perbarui `leads`, cocokkan `event_rules`, baca nilai, lalu buat baris `events` berstatus `queued`.
3. Jawab 200. Isi pesan dibuang.
4. Worker mengambil `events` berstatus `queued` (`FOR UPDATE SKIP LOCKED`, index `events_queue_idx`),
   mengirim ke Meta, lalu memperbarui status. Retry memakai `attempts` dan `next_retry_at`.

Langkah 2 hanya butuh hitungan milidetik, jadi webhook tetap cepat. Yang lambat (memanggil Meta)
tetap dikerjakan worker. Kalau alurnya mau diubah, isi pesan harus disimpan sementara, dan itu
melanggar aturan dokumen.

## 5. Bagian Meta dan WhatsApp: yang perlu diputuskan atau dicek

**Diperlukan sebelum menulis webhook (bab 5.2):**
- [ ] **Penyedia awal:** Kirim.chat, Cloud API, atau keduanya. Isi `wa_connections.provider` dengan
      kode tetap, misalnya `kirimchat` dan `cloud_api`.
- [ ] **Contoh payload asli** untuk pesan masuk, pesan keluar, dan pesan dari iklan (dengan
      referral). Samarkan nomor dan isi pesannya. Simpan di `backend/testdata/` untuk unit test adapter.
- [ ] **Cara verifikasi signature Kirim.chat:** nama header, algoritma, dan apa yang di-hash.
- [ ] **Letak `ctwa_clid`, ad ID, dan campaign ID** di payload. Kolom yang sudah disiapkan:
      `leads.ctwa_clid`, `ad_id`, `campaign_id`, `clicked_at`. Kalau ada field lain yang perlu
      disimpan, buat migrasi baru.
- [ ] **Message Sent:** apakah penyedia mengirim webhook untuk pesan dari admin dan template.
      Fitur aturan `who = admin` dan `who = template` bergantung pada ini.

**Diperlukan sebelum menulis dispatcher (bab 5.3):**
- [ ] **Versi Graph API.** Frontend masih memakai `v19.0` di `src/services/capi.js`. Versi itu
      kemungkinan sudah tidak didukung, jadi cek versi aktif terbaru.
- [ ] **Field payload CAPI untuk WhatsApp.** Contoh di frontend (`src/services/capi.js`):
      `action_source: business_messaging`, `messaging_channel: whatsapp`, `user_data.ctwa_clid`,
      `user_data.ph` (SHA-256). Cek apakah `page_id` atau `whatsapp_business_account_id` juga wajib.
- [ ] **Nama event.** Frontend punya pemetaan `InitiateCheckout`/`AddToCart` → `Hot Lead` (Logic.js).
      Putuskan apakah Hot Lead dikirim sebagai event standar (misalnya `InitiateCheckout`) atau
      custom event. Complaint dan Report juga perlu diputuskan.
- [ ] **Kode error Meta** yang boleh di-retry (sementara) dan yang tidak (token salah, parameter
      salah). Simpan di `events.error_code`; saran perbaikan ditampilkan di UI.
- [ ] **Masa berlaku token** CAPI atau System User. Disimpan di `meta_connections.token_expires_at`
      untuk dipakai tugas pengingat token.

**Catatan keamanan dari frontend saat ini:**
- `src/services/capi.js` mengirim access token lewat query string langsung dari browser (mode
  `VITE_LIVE_API=true`). Saat backend siap, panggilan ini pindah ke server dan token tidak
  boleh ada di browser lagi.
- `KoneksiSettings` masih menyimpan token di localStorage dan harus diganti ke
  `PUT /connections`.

## 6. Aturan saat melanjutkan

- **Jangan mengubah file migrasi yang sudah pernah dijalankan.** Buat migrasi baru:
  `go run ./cmd/migrate create nama_perubahan`.
- **Rahasia** (token Meta, webhook secret) hanya disimpan di kolom `*_enc` (AES-256-GCM, kunci dari
  environment). Jangan simpan rahasia di `provider_settings` (jsonb) atau di log.
- **Isi pesan WhatsApp** tidak boleh masuk database, log, atau Sentry.
- **Nomor HP** selalu dinormalisasi ke `62xxxxxxxxxx` sebelum disimpan. Database menolak format lain.
- **Endpoint workspace** wajib cek kepemilikan. Workspace milik orang lain dijawab 404.
- **Setiap aksi admin** (approve, reject, ubah paket, maintenance) ditulis ke `audit_logs`.

## 7. Langkah berikutnya yang disarankan (bab 5)

1. **Fondasi server:** `cmd/api` dengan Gin, config dari env, pool `pgx`, format respons
   `{ data }` / `{ message, code }` (frontend membaca `data` dan `message` di
   `src/services/api.js`), CORS, rate limit, dan helper AES-256-GCM.
2. **Auth dan workspace:** register, login, sesi cookie httpOnly, CSRF, profil, workspace. Saat
   workspace dibuat, langsung buat `subscriptions` berstatus `trial` dengan
   `trial_until = now + settings.trial_days`. Buat juga CLI untuk akun admin pertama.
3. **Koneksi, webhook, aturan event, dan dispatcher** (butuh poin 5 di atas). Selama data dari
   penyedia belum ada, pakai implementasi tiruan di balik interface `WhatsAppAdapter` dan
   `MetaClient`.
4. **Langganan dan admin:** unggah bukti, verifikasi, aktivasi, bagi hasil.
5. **Dashboard dan tugas terjadwal.**

## 8. Masih terbuka (belum ada di dokumen atau skema)

- **Anggota tim per workspace.** Frontend sudah menampilkan peran Owner, Admin, CS, dan Viewer
  serta undangan anggota (audit di Logic.js), tapi dokumen dan skema belum punya tabelnya. Saat ini
  satu workspace = satu pemilik.
- **PPN 11%.** Halaman checkout dan invoice di frontend menghitung PPN, tapi dokumen tidak.
  Tabel `payments` belum punya kolom pajak.
- **Batas trial.** Satu akun bisa membuat workspace baru berulang kali untuk mendapat trial lagi.
- **Lainnya dari bab 13:** retensi data, sales di luar founder, payment gateway, jumlah paket,
  dan WA QR. Flag `qr` sengaja dimatikan di data awal.
