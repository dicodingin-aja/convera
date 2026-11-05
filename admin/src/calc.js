// Format tampilan. Angka uang (bagi hasil, pendapatan) selalu dihitung server; di sini hanya format
// dan satu perkiraan pembagian untuk pratinjau di form aktivasi.
export const rp = (n) => 'Rp' + Math.round(Number(n) || 0).toLocaleString('id-ID');
export const monthKey = (d) => { const x = new Date(d); return x.getFullYear() + '-' + String(x.getMonth() + 1).padStart(2, '0'); };
export const monthLabel = (key) => new Date(key + '-01T00:00:00').toLocaleDateString('id-ID', { month: 'long', year: 'numeric' });
export const monthShort = (key) => new Date(key + '-01T00:00:00').toLocaleDateString('id-ID', { month: 'short' });
export const fmtDate = (d) => (d ? new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }) : '—');
export const fmtShort = (d) => (d ? new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' }) : '—');
export const fmtDateTime = (d) => (d ? new Date(d).toLocaleString('id-ID', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—');
export const daysFromNow = (d) => Math.ceil((new Date(d).getTime() - Date.now()) / 864e5);

export function lastMonths(count) {
  const out = []; const now = new Date();
  for (let i = count - 1; i >= 0; i--) out.push(monthKey(new Date(now.getFullYear(), now.getMonth() - i, 1)));
  return out;
}

// Cermin billing.Split di backend (pembulatan ke bawah, sisa masuk kas). Hanya untuk pratinjau.
export function previewSplit(amount, komisiPct, kasPct, founders) {
  const bp = (p) => Math.round(Number(p) * 100);
  const komisi = Math.floor((amount * bp(komisiPct)) / 10000);
  const kas = Math.floor((amount * bp(kasPct)) / 10000);
  const rest = amount - komisi - kas;
  if (founders <= 0) return { komisi, kas: kas + rest, each: 0 };
  const each = Math.floor(rest / founders);
  return { komisi, kas: kas + (rest - each * founders), each };
}

// Menit sejak terakhir aktif -> teks.
export function lastSeenText(c) {
  if (!c.last_seen_at) return 'Belum pernah';
  const min = Math.max(0, Math.round((Date.now() - new Date(c.last_seen_at)) / 6e4));
  if (c.online) return min <= 1 ? 'Online' : min + ' mnt lalu';
  if (min < 60) return min + ' mnt lalu';
  if (min < 60 * 24) return Math.round(min / 60) + ' jam lalu';
  return Math.round(min / 1440) + ' hari lalu';
}
