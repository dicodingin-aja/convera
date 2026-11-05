# Menjalankan Convera di komputer sendiri (Windows / PowerShell)

Butuh: Node.js 20+, Go 1.25+, PostgreSQL 15+ (yang sudah terpasang cukup). Semua perintah dari folder `convera`.

## 1. Sekali saja

```powershell
# Backend
cd backend
copy .env.example .env            # lalu edit .env (lihat di bawah)
go run ./cmd/genkey               # tempel hasilnya ke ENCRYPTION_KEY di .env
go run ./cmd/migrate createdb
go run ./cmd/migrate up
$env:ADMIN_PASSWORD = "ganti-dengan-password-12-karakter"
go run ./cmd/adminctl create boss "Nama Anda" superadmin
Remove-Item Env:ADMIN_PASSWORD
cd ..

# Frontend
npm install
cd admin; npm install; cd ..
copy .env.local.example .env.local
```

Isi minimal `backend/.env`:

```
DATABASE_URL=postgres://postgres:PASSWORD_ANDA@localhost:5432/db_convera?sslmode=disable
ENCRYPTION_KEY=<hasil go run ./cmd/genkey>
GRAPH_API_VERSION=<versi Graph API yang masih didukung, mis. v23.0>
```

Tanpa `SMTP_HOST`, email (verifikasi, reset password, pengingat) **tidak dikirim**; isinya dicetak di terminal
tempat API/worker berjalan, termasuk tautannya. Cukup salin tautan itu ke browser.

## 2. Setiap kali development (5 terminal)

| Terminal | Folder | Perintah | Alamat |
|---|---|---|---|
| API customer | `backend` | `go run ./cmd/api` | http://localhost:8080/healthz |
| API admin | `backend` | `go run ./cmd/admin` | http://localhost:8081/healthz |
| Worker | `backend` | `go run ./cmd/worker` | (pengirim ke Meta, pengingat, rekap, retensi) |
| Webapp customer | `.` | `npm run dev` | http://localhost:5173 |
| Convera Admin | `admin` | `npm run dev` | http://localhost:5174 |

Worker boleh tidak dijalankan untuk melihat tampilan saja, tetapi tanpa worker event tidak terkirim ke Meta,
status langganan tidak otomatis berakhir, dan rekap harian lama tidak terhitung.
Bila port 5173/5174 dipakai proses lain, Vite memakai port berikutnya dan menampilkannya di terminal.
`.env.local` (frontend customer) berisi `VITE_BACKEND_ENABLED=true` dan `VITE_API_BASE_URL=http://localhost:8080`;
hapus/ubah jadi `false` untuk kembali ke mode demo tanpa backend.

## 3. Yang bisa dicoba

1. **Customer** (http://localhost:5173): Daftar → (email verifikasi tercetak di terminal API) → buka menu Koneksi
   (workspace dibuat otomatis dari daftar di UI) → Simpan / Tes Koneksi.
   Menu Upgrade → pilih paket → unggah bukti transfer (rekening tujuan dibuat admin dulu, lihat langkah 2).
   "Lupa password?" di halaman masuk → tautan reset tercetak di terminal API.
2. **Admin** (http://localhost:5174): masuk dengan akun `adminctl`.
   Paket & Harga → tambah **rekening bank** (agar customer punya tujuan transfer) →
   Verifikasi → bukti dari customer muncul → Lihat bukti → Aktifkan akun. Cek Customer, Bagi Hasil, Ringkasan.
3. Mendorong chat WhatsApp sungguhan butuh URL publik (tunnel) dan akun Meta; lihat `deploy/DEPLOY.md` bagian "Sambungkan ke Meta".

## Masuk ke dashboard customer (untuk dukungan)

Di Convera Admin: Customer -> klik customer -> **Masuk ke dashboard** (butuh akun superadmin dan alasan). Tab baru terbuka
sebagai customer itu dengan banner "Mode dukungan". Tindakan dicatat di Developer -> Aktivitas admin.

## 4. Data nyata vs data contoh di webapp customer

Mode backend (`VITE_BACKEND_ENABLED=true`) memakai **data nyata** dari API untuk workspace yang sedang dibuka:

| Halaman | Isi |
|---|---|
| Overview | Chat dari iklan, lead, purchase, nilai pembelian (dengan perbandingan periode sebelumnya), funnel, grafik harian, iklan teratas, kesehatan koneksi, checklist awal |
| Leads | Daftar lead, detail + timeline dari event nyata, tandai Purchase (masuk antrean pengiriman), tandai Hilang |
| Konversi, Event Log | Event nyata beserta status, payload yang dikirim, respons Meta, penjelasan + saran perbaikan, dan Kirim ulang |
| Aturan | Aturan event nyata (tambah, ubah, aktif/nonaktif, hapus). Nama event yang tersedia: Contact, Lead, Hot Lead, Purchase |
| Laporan | KPI dan iklan teratas dari rentang tanggal yang dipilih (PDF/CSV) |
| Pengaturan | Hapus data kontak (menghapus permanen di server) |
| Koneksi, Upgrade/bayar, sidebar | Seperti sebelumnya |

Hal yang **tidak** ditampilkan karena memang belum ada sumbernya: biaya iklan, ROAS, cost per purchase, skor EMQ, dan kreatif
iklan (butuh integrasi Meta Ads / login Facebook). Halaman **Performa** menampilkan keadaan "belum terhubung".
Isi pesan WhatsApp tidak disimpan, jadi detail lead tidak menampilkan teks chat. Daftar leads/event memuat 100 data terbaru.

Masih data contoh / lokal di browser: **Integrasi**, **Developer** (API key, webhook), sebagian **Pengaturan** (preferensi), dan **Agency**.
Mode demo (`VITE_BACKEND_ENABLED=false`) tetap memakai seluruh data contoh.

## 5. Menjalankan tes

```powershell
cd backend; go test ./...        # tanpa database
cd ..; npm run build; cd admin; npm run build
```
