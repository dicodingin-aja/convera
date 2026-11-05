# Deploy Convera ke VPS

Satu VPS menjalankan semuanya lewat Docker Compose:

```
Internet ──443──> Caddy (HTTPS otomatis)
                   ├─ app.contoh.id ──> frontend customer (file statis)
                   └─ api.contoh.id ──> API Go ──> PostgreSQL (hanya di jaringan Docker)
                                         Worker Go ──> Meta Conversions API
```

Hanya port 80/443 yang terbuka. PostgreSQL dan API **tidak** bisa diakses dari luar.
Ganti `contoh.id` dengan domain Anda di seluruh panduan.

## 1. Siapkan VPS
- Ubuntu 24.04 LTS, minimal **2 vCPU / 2 GB RAM / 25 GB disk** (build Docker butuh RAM; kalau hanya 1 GB, tambah swap 2 GB).
- Pilih region terdekat pengguna (Singapura atau Indonesia).
- Catat **IP publik** VPS.

## 2. Arahkan domain (di panel DNS domain Anda)
Buat dua A record ke IP VPS:

| Nama | Tipe | Isi |
|---|---|---|
| `app` | A | IP VPS |
| `api` | A | IP VPS |
| `admin` | A | IP VPS |

Tunggu beberapa menit, lalu cek dari komputer Anda: `nslookup app.contoh.id` dan `nslookup api.contoh.id` harus menampilkan IP VPS.
Jika memakai Cloudflare, set kedua record ke **DNS only** (awan abu-abu) dulu supaya sertifikat HTTPS bisa terbit.

## 3. Amankan VPS (login sebagai root sekali ini)
```bash
adduser deploy && usermod -aG sudo deploy
mkdir -p /home/deploy/.ssh && cp ~/.ssh/authorized_keys /home/deploy/.ssh/ && chown -R deploy:deploy /home/deploy/.ssh
# Uji dulu login SSH sebagai `deploy` dari komputer Anda SEBELUM lanjut.
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/; s/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
systemctl restart ssh
ufw allow 22/tcp && ufw allow 80/tcp && ufw allow 443/tcp && ufw allow 443/udp && ufw --force enable
apt update && apt install -y unattended-upgrades
```
Seterusnya login sebagai `deploy` (`ssh deploy@IP_VPS`).

## 4. Pasang Docker
```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker deploy      # lalu logout dan login lagi
docker compose version
```

## 5. Ambil kode dari GitHub
Repo privat, jadi pakai *deploy key* (hanya-baca):
```bash
ssh-keygen -t ed25519 -f ~/.ssh/convera_deploy -N ""
cat ~/.ssh/convera_deploy.pub        # salin
```
Di GitHub: repo → **Settings → Deploy keys → Add deploy key**, tempel, biarkan *write access* **tidak** dicentang.
```bash
printf 'Host github.com\n  IdentityFile ~/.ssh/convera_deploy\n' >> ~/.ssh/config
git clone git@github.com:dicodingin-aja/convera.git
cd convera/deploy
```

## 6. Isi konfigurasi rahasia
```bash
cp .env.example .env
chmod 600 .env
openssl rand -hex 24      # -> POSTGRES_PASSWORD
openssl rand -base64 32   # -> ENCRYPTION_KEY
nano .env
```
Isi `APP_DOMAIN`, `API_DOMAIN`, `ADMIN_DOMAIN`, `COOKIE_DOMAIN` (`.contoh.id`), dua rahasia di atas, dan `GRAPH_API_VERSION`
(cek versi yang masih didukung di dokumentasi Meta).

> **Simpan `ENCRYPTION_KEY` dan `POSTGRES_PASSWORD` di password manager.** Jika `ENCRYPTION_KEY` hilang,
> semua token Meta yang tersimpan tidak bisa dibaca lagi (pengguna harus mengisi ulang). Jangan pernah commit `.env`.

## 7. Jalankan
```bash
docker compose up -d --build
docker compose ps                  # db, api, worker, web = running; migrate = exited (0)
docker compose logs -f api         # Ctrl+C untuk keluar
```
Build pertama beberapa menit. Skema database dibuat otomatis oleh service `migrate`.
Cek: buka `https://api.contoh.id/healthz` (harus `{"data":{"status":"ok"}}`) lalu `https://app.contoh.id`.
Sertifikat HTTPS terbit otomatis dalam sekitar satu menit. Daftarkan akun pertama lewat tombol **Daftar** di halaman login.

### Buat akun Convera Admin pertama
Tidak ada pendaftaran admin lewat web; akun dibuat dari server:
```bash
docker compose run --rm --entrypoint /app/adminctl admin create boss "Nama Anda" superadmin
# ketik password (min. 12 karakter) saat diminta
docker compose run --rm --entrypoint /app/adminctl admin list
```
Peran `superadmin` boleh mengubah aturan langganan, maintenance, dan API key; peran `admin` tidak.
Ganti password: `... adminctl passwd boss` (semua sesi lama dicabut). Nonaktifkan: `... adminctl disable boss`.
Akses admin bisa dibatasi ke IP kantor/VPN dengan `ADMIN_ALLOW_IPS` di `deploy/.env`.
Setelah itu buka `https://admin.contoh.id` dan login dengan akun tadi.
Detail endpoint ada di `backend/ADMIN_API.md`.

## 8. Database di VPS
- Data ada di volume Docker `convera_dbdata` (bertahan saat container di-restart atau di-update).
- **Jangan** jalankan `docker compose down -v` (flag `-v` menghapus database).
- Masuk ke database bila perlu: `docker compose exec db psql -U convera -d db_convera`.

### Cadangan harian
```bash
chmod +x backup.sh
sudo mkdir -p /var/backups/convera && sudo chown deploy: /var/backups/convera
./backup.sh                         # uji sekali
crontab -e                          # tambahkan baris ini:
15 2 * * * /home/deploy/convera/deploy/backup.sh >> /home/deploy/backup.log 2>&1
```
Cadangan disimpan 14 hari **di VPS yang sama**, jadi belum aman bila VPS-nya rusak. Salin juga ke tempat lain
(mis. `rclone` ke Google Drive/S3, atau fitur snapshot dari penyedia VPS) dan uji pemulihan:
```bash
gunzip -c /var/backups/convera/db-TANGGAL.sql.gz | docker compose exec -T db psql -U convera -d db_convera
```
(Untuk pemulihan penuh, lakukan pada database kosong.)

## 9. Update versi baru
```bash
cd ~/convera && git pull
cd deploy && docker compose up -d --build
```
Migrasi baru dijalankan otomatis. Aturan dari tim: jangan ubah file migrasi lama, buat yang baru.
Sebelum update yang menyertakan migrasi, jalankan `./backup.sh` dulu.

## 10. Sambungkan ke Meta
1. Login ke `https://app.contoh.id` → menu **Koneksi**; isi Dataset ID, access token, Test Event Code, WABA ID, Phone Number ID, App Secret → **Simpan** → **Tes Koneksi**.
2. Salin **Webhook URL** (`https://api.contoh.id/webhook/cloud/...`) dan **Verify Token** dari halaman itu.
3. Di Meta app (WhatsApp → Configuration → Webhook): tempel URL dan Verify Token, klik Verify, lalu subscribe field **messages**.
4. Kosongkan **Test Event Code** setelah selesai uji, supaya event masuk ke data asli.

## 11. Pantau
- Log: `docker compose logs -f api` atau `worker`.
- Pasang uptime monitor gratis (mis. UptimeRobot) ke `https://api.contoh.id/healthz`.
- Disk: `df -h`; Docker: `docker system df`.

## Masalah umum
| Gejala | Penyebab / solusi |
|---|---|
| Sertifikat HTTPS tidak terbit | DNS belum mengarah ke VPS, atau port 80/443 tertutup (`ufw status`). Lihat `docker compose logs web` |
| Login berhasil tapi aksi lain `403 csrf` | `COOKIE_DOMAIN` salah (harus `.contoh.id`, dengan titik) atau `app` dan `api` beda domain induk |
| Browser error CORS | `APP_DOMAIN` di `.env` tidak sama persis dengan alamat yang dibuka |
| `api` restart terus | `docker compose logs api`: biasanya `ENCRYPTION_KEY` bukan base64 32 byte atau `DATABASE_URL` salah |
| Tes Koneksi: "GRAPH_API_VERSION belum diisi" | Isi di `deploy/.env` lalu `docker compose up -d` |

## Belum siap untuk produksi (jujur)
- **UI Convera Admin sudah tersambung ke API admin** (data sungguhan, bukan data contoh). Karena berisi data keuangan,
  batasi dengan `ADMIN_ALLOW_IPS` bila tim punya IP tetap/VPN.
- Bukti transfer disimpan di volume Docker `convera_uploads` (bukan di database). `backup.sh` ikut mengarsipkannya; jangan jalankan `docker compose down -v`.
- Tidak ada email: akun memakai username + password. Lupa password diselesaikan superadmin lewat Convera Admin (Customer > Reset password).
- Pengiriman ke Meta baru diuji ke server tiruan; uji end-to-end dengan Test Events Meta sebelum dipakai klien.
- Mode QR (Baileys) belum ada. Satu VPS tanpa redundansi: cadangan di luar VPS wajib.
- File Docker di folder ini sudah divalidasi sintaksnya tetapi **belum dibuild di mesin lain**; build pertama di VPS bisa menampakkan penyesuaian kecil.


## Mode QR (WhatsApp Web)

Layanan `waqr` ikut berjalan lewat `docker compose` (hanya di jaringan internal). Syarat:

1. Isi `QR_SERVICE_TOKEN` di `deploy/.env` (`openssl rand -hex 24`).
2. Aktifkan fitur `qr` (feature flag) lewat Admin, lalu pelanggan memilih "WA Bisnis (scan QR)" di halaman Koneksi.
3. Sesi tertaut disimpan di tabel `whatsmeow_*` (database yang sama, sudah masuk `backup.sh`). Setelah restore, sesi tetap berjalan.
4. Ini memakai protokol WhatsApp Web tidak resmi (whatsmeow): ada risiko nomor dibatasi WhatsApp, dan protokol bisa berubah
   sehingga library perlu di-update (`go get go.mau.fi/whatsmeow@latest`, build ulang). Layanan hanya membaca pesan.
5. Satu proses menampung semua sesi (±puluhan MB per nomor aktif); pantau memori bila pelanggan banyak.
