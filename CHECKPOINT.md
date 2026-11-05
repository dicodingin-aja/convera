# Checkpoint Convera — serah terima untuk yang melanjutkan

Status per **5 November 2026**. Baca ini dulu. `backend/HANDOFF.md` adalah catatan lama (sebelum server API ada) dan sudah usang.
Belum ada yang di-commit. Semua perubahan masih di working tree.

Legenda: ✅ sudah dan **terbukti dites** · 🟡 sudah dibuat tapi **belum terbukti** di kondisi nyata · ⬜ belum dikerjakan

---

## 0. Cara menjalankan (dev)

Lihat [DEV.md](DEV.md) dan [backend/README.md](backend/README.md). Ringkasnya ada 6 proses:

| Proses | Perintah | Port |
|---|---|---|
| API customer | `go run ./cmd/api` | 8080 |
| API admin | `go run ./cmd/admin` | 8081 |
| Worker | `go run ./cmd/worker` | - |
| Sesi WhatsApp (QR) | `go run ./cmd/waqr` | 8090 (internal saja) |
| Web customer | `VITE_BACKEND_ENABLED=true VITE_API_BASE_URL=http://localhost:8080 npx vite --port 5180` | 5180 |
| Web admin | `cd admin && ADMIN_API_URL=http://127.0.0.1:8081 npx vite --port 5181` | 5181 |

Env penting: `DATABASE_URL`, `ENCRYPTION_KEY` (base64 32 byte, `go run ./cmd/genkey`), `GRAPH_API_VERSION`,
`QR_SERVICE_URL` + `QR_SERVICE_TOKEN` (API) dan `QR_SERVICE_TOKEN` + `CONVERA_API_URL` (waqr).
Contoh lengkap: `backend/.env.example`, `deploy/.env.example`.

Setelah `migrate up` (sekarang sampai **00012**), aktifkan fitur QR: `INSERT INTO feature_flags(key,enabled) VALUES ('qr',true) ON CONFLICT (key) DO UPDATE SET enabled=true;`
(atau lewat Admin).

> ⚠️ `GRAPH_API_VERSION=v23.0` di dev **tebakan**, belum dicek ke dokumentasi Meta. Cek versi yang masih didukung sebelum produksi.

Akun dev (database lokal saja, jangan dipakai di produksi): customer `test@test.com` / `123123`, admin `admin` / `admin`.

---

## 1. Yang sudah ada

| Area | Status | Catatan |
|---|---|---|
| Backend Go (auth, workspace, leads, events, aturan, API publik, bukti transfer, langganan, notifikasi, jobs) | ✅ | Diuji dengan skrip e2e ke Postgres lokal. |
| API admin + UI admin (customer, paket, verifikasi bukti, bagi hasil, bantuan/impersonasi) | ✅ | Binary terpisah, DB sama. Lihat `backend/ADMIN_API.md`. |
| Dashboard customer memakai data nyata (`src/services/live.js`) | ✅ | Diuji lewat SSR smoke test, **belum diklik di browser sungguhan** (ekstensi Chrome tidak tersambung). |
| Koneksi Meta: Pixel ID + token CAPI (disimpan terenkripsi AES-GCM) | ✅ | Hanya dua isian ini yang wajib. Test Event Code opsional. |
| Mode QR: sesi WhatsApp Web (`cmd/waqr`, library whatsmeow) | 🟡 | QR berhasil terbit dari server WhatsApp, **sudah di-scan dan tersambung** oleh Jordi, pesan masuk tercatat. Lihat bagian 3. |
| Isi pesan, nama, nomor lengkap terlihat di dashboard customer | ✅ | Tabel `lead_messages`, retensi 90 hari. Admin tetap melihat nomor tersamar dan tanpa isi pesan. |
| Event per mode: QR memakai 17 event standar Meta, WABA 5 event | ✅ | Nama event Meta dipakai apa adanya di UI (sebelumnya "Lead" salah diterjemahkan jadi `LeadSubmitted`). |
| Webhook WABA tanpa App Secret (untuk CRM/BSP seperti kirim.chat) | 🟡 | Diuji dengan payload sintetis saja. Lihat bagian 4. |
| Deploy: `deploy/docker-compose.yml`, Caddy, `DEPLOY.md`, backup | 🟡 | **Docker build belum pernah dijalankan.** |
| Meta Ads (`ads_read`: biaya, ROAS, CPP) | 🟡 | Sengaja **dimatikan** (lihat bagian 5). |

---

## 2. Yang BELUM dikerjakan (urut prioritas)

### 2.1 Login/register customer disederhanakan: username + password saja ⬜
Permintaan Jordi: pembuatan akun **simple**, hanya username dan password, supaya tabel DB juga sederhana. **Tanpa email sama sekali** (tanpa verifikasi, tanpa lupa password via email, tanpa SMTP).

Yang perlu dikerjakan:
- Migrasi: `users.username citext` unik (di antara user yang belum dihapus), backfill dari email lama, `users.email` jadi nullable (atau dihapus bila aman).
- Aturan username: `^[a-z0-9][a-z0-9._-]{2,29}$`, password min. 8 karakter. Nama tampilan default = username.
- Register/login memakai username. Hapus endpoint `forgot-password`, `reset-password`, `verify-email`, `resend-verification`, fungsi `sendVerification`, paket `internal/mail`, konfigurasi SMTP (config, compose, `.env.example`, docs), dan pengiriman email di `jobs` (pengingat tetap lewat notifikasi dalam aplikasi).
- Frontend: `src/services/auth.js`, `LoginPage.jsx` (hapus mode forgot/reset), `ProfilePage`, `links.js` dan `main.jsx` (hapus penanganan tautan verify/reset), profil menampilkan username.
- Admin: daftar/cari/CSV customer memakai username, respons support-login memakai username. Tambah **"reset password customer"** di admin (khusus superadmin: buat password sementara yang tampil sekali, cabut semua sesi, catat di audit log) karena tidak ada lagi reset lewat email.
- `deleteAccount` (anonimisasi) harus ikut menganonimkan username.
- Perbarui semua tes dan dokumentasi.

### 2.2 Laporan harian + filter dashboard (meniru PixelAD) ⬜
Referensi: dua screenshot PixelAD dari Jordi (Dashboard dan "Laporan Harian").
- **Laporan:** tabel per tanggal (percakapan/klik WA, purchase, revenue) untuk satu bulan, baris TOTAL, ekspor CSV, pemilih bulan. Ditambah rincian **jumlah per event** (Contact/Lead/Purchase/dst: berapa terkirim, gagal, total nilai). Saat ini Laporan hanya 4 KPI + 3 iklan teratas + CSV.
- **Dashboard:** kartu **Percakapan** dan **Revenue**, filter tanggal (sudah ada), tambah filter **sumber** (QR/WABA/API/manual) dan **status lead**, tren harian, distribusi event.
- Datanya sudah ada di tabel `events`, `leads`, `daily_stats`. Perlu endpoint agregat per hari/per event.

### 2.3 Hal lain yang belum
- ⬜ Commit ke branch baru, dipecah per fitur (lihat bagian 7).
- ⬜ Skrip e2e saat ini **hanya di folder scratchpad Claude, bukan di repo**. Pindahkan menjadi tes di repo (Python atau Go integration test). Daftar skrip: `e2e.py`, `e2e_admin.py`, `e2e_upload.py`, `e2e_functions.py`, `e2e_support.py`, `e2e_ads.py`, `e2e_qr.py`, `e2e_qr_gate.py`, `e2e_msgs.py`, `ssr_*.jsx`. Tanya pemilik sesi bila perlu.
- ⬜ Perbarui `backend/API.md` dan `backend/SCHEMA.md`: endpoint baru (`/performance`, `/connections/ads*`, `/connections/whatsapp/*` dengan `qr_image`), field lead baru (`phone`, `last_message`, `messages`, `ad_name`), tabel baru (`ad_connections`, `ad_insights_daily`, `lead_messages`, kolom `wa_sessions.device_jid`).
- ⬜ Halaman mock yang belum nyata: Integrasi, Developer, Agency, sebagian Pengaturan.
- ⬜ Kebijakan Privasi dan Syarat (isi pesan disimpan 90 hari, nomor tampil penuh ke pelanggan, mode QR memakai protokol tidak resmi).
- ⬜ Verifikasi Docker build, restore backup, domain/DNS/TLS, SPF/DKIM (hilang bila email dibuang).
- ⬜ Membersihkan kode mati "anggota tim per workspace" di `src/Logic.js` (`members`, `invite`, `perms`). Fitur ini **diputuskan dihapus** (keep simple seperti PixelAD).

### 2.4 Diputuskan TIDAK dikerjakan
- **PPN 11%:** tidak diimplementasikan, tidak perlu.
- **Anggota tim per workspace:** dihapus dari lingkup.
- **Email/SMTP:** dibuang (lihat 2.1).

---

## 3. Mode QR — apa yang sudah dan belum terbukti

**Sudah:** QR terbit dari server WhatsApp, di-scan, status "Terhubung", nama perangkat tampil "Convera" (`QR_DEVICE_NAME`, hanya untuk penautan baru), pesan masuk membuat lead, event bisa dibuat dari aturan. Alur UI: isi Pixel ID + token → Simpan → halaman scan QR terpisah → kembali ke Koneksi dengan kartu "WhatsApp Terhubung" dan tombol "Batalkan koneksi". Backend menolak membuat QR bila Pixel ID/token belum tersimpan.

### ❓ Pertanyaan terbuka: apakah event dari QR benar-benar masuk ke Meta / Ads Manager?
**Belum terbukti.** Yang sudah dites hanyalah: pesan masuk → lead → event dibuat di database Convera. Pengiriman nyata ke Conversions API Meta dan tampil di Ads Manager **belum diverifikasi** oleh siapa pun. Cara mengetesnya:

1. Di Meta **Events Manager → Data Sources → pilih Pixel → tab Test Events**, salin **Test Event Code** (mis. `TEST12345`).
2. Di Convera → Koneksi, isi Test Event Code, Simpan, klik **Tes Koneksi**. Event sintetis `Lead` harus muncul di tab Test Events dalam beberapa detik. Kalau gagal, Log Event Convera menyimpan pesan error Meta dan `fbtrace_id`.
3. Dengan Test Event Code **masih terisi**, kirim chat nyata ke nomor tertaut dari kontak yang cocok aturan (mis. kata kunci "sudah transfer"). Lihat event muncul di Test Events dan statusnya **Terkirim** di Log Event Convera.
4. Cek **Match Quality** di Events Manager (Overview/Diagnostics). Di mode QR identitas pengguna hanya **hash nomor telepon (+ nama depan, kota, external_id)** dengan `action_source: chat`. Match rate bisa rendah.
5. Kosongkan Test Event Code, kirim event nyata, lalu cek di **Ads Manager** kolom Results/Purchases dan nilai konversi. **Atribusi ke iklan tertentu bisa tertunda hingga 24 jam dan tidak dijamin** di mode QR, karena tidak ada `ctwa_clid`.

Catatan risiko mode QR:
- Data iklan di pesan (`externalAdReply`: `ctwaClid`, `sourceID`) **tidak selalu ada** di protokol WhatsApp Web. Bisa kosong, sehingga lead tidak punya `ad_id`. Cek dengan chat dari iklan Click-to-WhatsApp sungguhan.
- WhatsApp kini memakai alamat `@lid` untuk sebagian kontak. Dipetakan ke nomor lewat data whatsmeow; bila gagal pesan dilewati.
- Protokol **tidak resmi**: nomor bisa dibatasi WhatsApp, library perlu di-update berkala (`go get go.mau.fi/whatsmeow@latest`).
- Kunci sesi WhatsApp tersimpan di tabel `whatsmeow_*` (database yang sama, tidak dienkripsi di level kolom).
- Satu proses `waqr` menampung semua sesi; pantau memori bila pelanggan banyak.
- Aturan lama yang memakai nama event WABA (`LeadSubmitted`) akan ditolak bila workspace pindah ke mode QR (dan sebaliknya). Belum ada migrasi otomatis.

---

## 4. Mode WABA / CRM (kirim.chat, dll) — ⚠️ BELUM DITES, JANGAN ANGGAP SUDAH BENAR

Alur yang diinginkan: pelanggan hanya mengisi **Pixel ID + token CAPI**, lalu menempel **URL webhook** Convera di penyedia WABA/CRM-nya.

Yang ada: URL `…/webhook/cloud/{slug}` (UUID acak sebagai rahasia), verifikasi GET (`hub.verify_token`), parser payload format Cloud API, **App Secret opsional** (bila diisi, tanda tangan `X-Hub-Signature-256` wajib valid; bila kosong, rahasia URL yang melindungi).

**Belum terbukti:**
- Tidak pernah dites dengan **kirim.chat atau CRM nyata**. Hanya payload sintetis buatan sendiri berformat Cloud API yang menghasilkan lead dengan `ad_id` dan `ctwa_clid`.
- **Asumsi besar:** CRM meneruskan payload yang **sama persis** dengan webhook Cloud API Meta (termasuk `referral.ctwa_clid` dan `source_id`). Bila formatnya berbeda, parser (`internal/wa`) harus disesuaikan. Mintalah contoh payload asli dari penyedia.
- Payload CAPI `business_messaging` (`messaging_channel: whatsapp`, `ctwa_clid`, `whatsapp_business_account_id` opsional) belum divalidasi ke Meta asli. WABA ID hanya dikirim bila diisi di "Pengaturan lanjutan"; Meta mungkin mewajibkannya.
- Lead tanpa `ctwa_clid` (organik) tidak dikirim di mode WABA (`ErrNotFromAd`).
- Jendela atribusi 7 hari sejak klik; lewat itu Meta menolak.

---

## 5. Meta Ads (`ads_read`) — dimatikan, tapi kodenya ada

Diputuskan: pelanggan **tidak** menyambungkan Meta Ads (cukup Pixel + token). Biaya iklan, ROAS, dan CPP bisa dilihat di Ads Manager.
- UI dimatikan lewat `ADS_ENABLED = false` di `src/services/live.js`; bagian Meta Ads dihapus dari halaman Koneksi.
- Backend tetap ada: `internal/ads`, `internal/server/ads.go`, migrasi 00010, endpoint `/connections/ads*` dan `/performance`, sinkron tiap 30 menit di worker.
- 🟡 Hanya diuji dengan server Graph **palsu**. Nama kolom Insights dan action type `onsite_conversion.messaging_conversation_started_7d` **belum dicek ke akun iklan nyata**. ROAS hanya bermakna untuk akun IDR.
- Bila ingin dihidupkan lagi: set `ADS_ENABLED = true` dan kembalikan komponen Meta Ads di `KoneksiBackend.jsx` (ada di riwayat; fungsi API sudah ada di `src/services/backend.js`).

---

## 6. Keputusan penting yang sudah diambil

- `leads.phone` disimpan mentah (tidak di-hash); hash hanya saat dikirim ke Meta.
- Graph API version selalu dari env `GRAPH_API_VERSION` (tanpa default).
- Token Meta dienkripsi AES-256-GCM dengan `ENCRYPTION_KEY`; **kunci hilang = semua token tidak terbaca**, simpan di password manager.
- Isi pesan **disimpan** di `lead_messages` (keputusan Jordi: semua data terlihat untuk pelanggan), retensi 90 hari (angka ini pilihan sendiri, bisa diubah di `internal/jobs`).
- Panel admin tetap menyamarkan nomor dan tidak menampilkan isi pesan. Tanyakan Jordi bila tim support butuh melihatnya.
- Admin API terpisah (port 8081), DB sama, cookie `cv_admin` SameSite Strict.
- Impersonasi (masuk ke dashboard customer) dicatat di audit log dan ada bannernya.

---

## 7. Status git dan saran commit

Tidak ada commit. `admin/src/data.js` dan `admin/src/store.js` sudah dihapus (staged). `backend/cmd/migrate/main.go` hanya berubah akhir baris, jangan di-commit. Saran pemecahan branch/commit:
1. Skema + backend inti + docs.
2. Admin API + UI admin.
3. Dashboard customer data nyata.
4. Integrasi Meta (CAPI, webhook, aturan).
5. Mode QR (`cmd/waqr`, UI scan, migrasi 00011).
6. Meta Ads (dimatikan) dan isi pesan (00012).
7. Deploy (compose, Caddy, docs).

`go.mod` bertambah banyak dependensi (whatsmeow, go-qrcode, zerolog, dll). Jalankan `go mod tidy` sebelum commit.

---

## 8. Checklist sebelum dipasang di VPS

- [ ] Selesaikan 2.1 (akun username + password) dan buang SMTP.
- [ ] `docker compose build` benar-benar berhasil, termasuk `waqr` (build `CGO_ENABLED=0`).
- [ ] Isi `deploy/.env`: `POSTGRES_PASSWORD`, `ENCRYPTION_KEY`, `GRAPH_API_VERSION` (versi yang valid), `QR_SERVICE_TOKEN`, domain.
- [ ] Aktifkan feature flag `qr` di admin.
- [ ] Buat akun admin: `go run ./cmd/adminctl` (jangan pakai `admin/admin`).
- [ ] Tes backup + restore (`deploy/backup.sh`); sesi QR ikut terbackup lewat tabel `whatsmeow_*`.
- [ ] Tes event nyata ke Meta (bagian 3) dan WABA/CRM nyata (bagian 4).
- [ ] Kebijakan Privasi dan Syarat.
- [ ] Rotasi: jangan ada kredensial dev di produksi; ganti token QR_SERVICE_TOKEN dan ENCRYPTION_KEY baru.
