# API Convera Admin (`/api/admin/v1`)

API internal untuk tim Convera: melihat customer, paket & harga, langganan, verifikasi pembayaran, bagi hasil, dan pengaturan.
Berjalan sebagai proses terpisah (`go run ./cmd/admin`, port `:8081`) tetapi memakai **database yang sama** dengan API customer.

- Sesi, cookie (`cv_admin`, `csrf_admin`), dan tabel sesi **terpisah** dari customer: login customer tidak berlaku di sini.
- Format respons sama: `{ "data", "meta" }` atau `{ "message", "code" }`. POST/PUT/DELETE wajib header `X-CSRF-Token` = cookie `csrf_admin`.
- Login dibatasi 10/menit per IP dan 5/menit per username. Sesi 12 jam.
- Akun dibuat dengan CLI: `go run ./cmd/adminctl create <username> [nama] [superadmin|admin]` (password dari `ADMIN_PASSWORD` atau stdin, min. 12 karakter).
- Setiap aksi yang mengubah data ditulis ke `audit_logs` dalam transaksi yang sama dengan perubahannya.

## Endpoint

| Endpoint | Fungsi |
|---|---|
| `POST auth/login`, `auth/logout`, `GET auth/session` | Sesi admin |
| `GET overview` | Pendapatan bulan ini, MRR, proyeksi akhir bulan, jumlah per status, online sekarang, berakhir ≤14 hari, seri 7 bulan, antrean bukti |
| `GET customers?filter&q&page` | Daftar bisnis + langganan. `filter`: `online`, `offline`, `active`, `expired`, `trial`, `pending`. `meta.counts` memuat jumlah tiap filter |
| `GET customers/{id}` | Detail: pemilik, paket, sales, riwayat pembayaran, ringkasan koneksi Meta (tanpa token), statistik lead/event |
| `GET customers.csv` | Ekspor CSV (mengikuti filter/pencarian) |
| `POST customers/{id}/extend` | Aktifkan/perpanjang manual: `{plan, months (1/3/12), amount, sales_id}` |
| `POST customers/{id}/stop` | Hentikan langganan |
| `GET events`, `GET customers/{id}/events`, `GET events/{id}`, `POST events/{id}/retry` | Dukungan: event semua customer / satu customer (nomor tersamar, saran perbaikan per kode error), detail dengan payload & respons Meta (pembukaan dicatat), dan kirim ulang event gagal |
| `POST customers/{id}/support-access` | **Masuk ke dashboard customer** (superadmin, alasan wajib): membuat tautan sekali pakai 90 detik; lihat bagian di bawah |
| `GET payment-proofs/{id}/file` | Berkas bukti (gambar/PDF) dari penyimpanan privat; tiap pembukaan dicatat di audit log |
| `GET payment-proofs?status` | Antrean verifikasi (`pending` default, `approved`, `rejected`, `all`); `amount_mismatch` menandai selisih nominal |
| `POST payment-proofs/{id}/approve` | `{plan, months, amount, sales_id}`; **sales wajib** |
| `POST payment-proofs/{id}/reject` | `{reason}` wajib; customer diberi notifikasi |
| `GET revenue-share?month=YYYY-MM`, `.csv` | Bagi hasil per orang + kas + rincian per pembayaran |
| `POST revenue-share/{month}/{person_id}/mark-paid` | `{paid: true|false}`; nominal dihitung server |
| `GET/POST/PUT/DELETE plans` | Paket & harga (`code` tidak bisa diubah; hapus = sembunyikan, ditolak bila masih dipakai langganan aktif) |
| `GET/POST/PUT/DELETE bank-accounts`, `sales-people` | Rekening tujuan dan penerima komisi (hapus sales = nonaktifkan) |
| `GET/PUT subscription-rules` | `trial_days`, `pay_months_12`, `grace_days`, `komisi_pct`, `kas_pct` (*superadmin* untuk PUT) |
| `GET/PUT feature-flags[/{key}]`, `GET/PUT maintenance`, `GET/PUT integrations` | Pengaturan global (maintenance: *superadmin*) |
| `GET/POST/DELETE api-keys` | Kelola API key admin (*superadmin*); belum dipakai sebagai autentikasi |
| `GET audit-logs?action&workspace_id&from&to` | Catatan aktivitas |
| `GET system-status` | Uptime, ping DB, event 24 jam, antrean/retry, error rate, bukti menunggu, error webhook |
| `GET notifications`, `POST notifications/read-all` | Lonceng admin |

API customer mendapat dua endpoint baca yang diisi dari sini: `GET /api/v1/plans` (paket yang tampil) dan `GET /api/v1/public/status` (maintenance + feature flag).

## Aturan bisnis yang ditegakkan

- **Status efektif.** Admin selalu menghitung ulang status: `active` tetap aktif selama masa tenggang (`grace_days`) setelah
  `current_period_end`; `trial` yang lewat dianggap `expired`. Worker juga menyimpannya ke database tiap 5 menit dan
  memberi notifikasi ke pemilik.
- **Aktivasi** (approve / extend): periode baru mulai dari `current_period_end` bila langganan masih aktif (sisa hari tidak
  hilang), selain itu dari sekarang. Penambahan bulan memakai bulan kalender dengan penjepitan akhir bulan (31 Jan + 1 bulan = 28 Feb).
  Satu transaksi: kunci baris langganan → buat `payments` → ubah langganan → notifikasi → audit log.
- **Snapshot persentase.** Komisi/kas disalin ke tiap pembayaran; mengubah aturan hanya berlaku untuk pembayaran berikutnya.
- **Bagi hasil.** Komisi% ke sales pada pembayaran itu, kas%, sisanya dibagi rata ke founder aktif; pembulatan ke bawah dan
  sisa masuk kas, jadi jumlah selalu utuh. Founder yang ikut bagian rata adalah founder aktif **saat laporan dilihat**.
  Pengaman: tidak boleh menonaktifkan founder aktif terakhir.
- **Tolak bukti.** Bila langganan berstatus `pending` hanya karena bukti itu, statusnya kembali ke aktif / trial / berakhir.
- CSV mencegah injeksi rumus (sel berawalan `= + - @` diberi apostrof) dan memakai BOM agar terbaca di Excel.

## Reset password customer

- `POST /customers/{id}/reset-password` (superadmin): membuat password sementara untuk pemilik workspace, tampil sekali di respons `{username, temporary_password}`, semua sesi customer dicabut, dicatat di audit log `customer.password_reset`.

## Belum ada

- Pengingat kedaluwarsa H-7/H-3/H-1, pengiriman WhatsApp ke admin, dan autentikasi API admin lewat API key belum ada.
- Peran hanya `superadmin` dan `admin`; belum ada daftar admin yang bisa dikelola dari web.

## Masuk ke dashboard customer (akses dukungan)

Dari Customer -> klik customer -> **Masuk ke dashboard**. Superadmin mengisi alasan, lalu mendapat tautan
`<APP_URL>/?support=<kode>` yang dibuka di tab baru (webapp customer menukar kode lewat `POST /api/v1/auth/support-login`).

Pengaman:
- Hanya **superadmin**; alasan 3-300 karakter wajib; kode **sekali pakai, 90 detik**, hanya hash yang disimpan.
- Sesi customer **2 jam**, tidak bisa diperpanjang, dibatasi ke **satu workspace** (workspace lain milik customer yang sama menjawab 404).
- **Diblokir** (403 `support_mode_blocked`): ganti password, ubah profil, hapus akun/workspace, buat workspace, unggah bukti bayar,
  buat/cabut API key, refresh sesi. Yang boleh: mengubah koneksi Meta, aturan event, tandai purchase, dan sebagainya.
- **Jejak:** audit log `support.access.create`, `support.access.login`, `support.access.end`, dan `support.request` untuk setiap
  permintaan yang mengubah data (metode, pola path, status; tanpa isi body). Sesi ini tidak menandai customer "online".
- **Transparansi:** customer menerima notifikasi "Tim dukungan Convera mengakses akun Anda" beserta alasannya.
- Dashboard menampilkan banner **Mode dukungan** dan tombol "Akhiri sesi dukungan".
