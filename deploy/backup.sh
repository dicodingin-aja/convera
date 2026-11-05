#!/usr/bin/env bash
# Cadangan database harian. Dijalankan dari cron di VPS (lihat DEPLOY.md).
# Hasil: /var/backups/convera/db-YYYY-MM-DD.sql.gz, disimpan 14 hari.
set -euo pipefail
cd "$(dirname "$0")"

DEST=/var/backups/convera
KEEP_DAYS=14
mkdir -p "$DEST"
umask 077

OUT="$DEST/db-$(date +%F).sql.gz"
docker compose exec -T db pg_dump -U convera -d db_convera --no-owner | gzip > "$OUT.tmp"
# pastikan hasilnya tidak kosong sebelum menggantikan cadangan hari ini
[ "$(stat -c %s "$OUT.tmp")" -gt 1000 ] || { echo "cadangan terlalu kecil, dibatalkan" >&2; rm -f "$OUT.tmp"; exit 1; }
mv "$OUT.tmp" "$OUT"
find "$DEST" -name 'db-*.sql.gz' -mtime +"$KEEP_DAYS" -delete
echo "OK $OUT ($(du -h "$OUT" | cut -f1))"

# Bukti transfer (berkas) ada di volume Docker terpisah dari database; cadangkan juga.
UP="$DEST/uploads-$(date +%F).tar.gz"
docker run --rm -v convera_uploads:/data:ro -v "$DEST":/out alpine tar czf "/out/$(basename "$UP").tmp" -C /data .
mv "$DEST/$(basename "$UP").tmp" "$UP"
find "$DEST" -name 'uploads-*.tar.gz' -mtime +"$KEEP_DAYS" -delete
echo "OK $UP ($(du -h "$UP" | cut -f1))"
