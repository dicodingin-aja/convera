// Paket langganan. Satu workspace = satu langganan per bulan.
// Tambah paket baru cukup dengan menambah item di PLANS; modal Upgrade otomatis menampilkannya.
export const PLANS = [
  { id: 'workspace', name: 'Workspace', price: 150000, period: 'bulan', features: ['1 workspace', 'Event & Log Event', 'Laporan', 'Koneksi Meta & WhatsApp'] },
];

const DAY = 864e5;
export const rupiah = (n) => 'Rp' + Number(n).toLocaleString('id-ID');
const fmtDate = (d) => new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });

// Data contoh sampai ada database: workspace 0 trial, 1–2 aktif, sisanya belum berlangganan.
export function getSub(subs, i) {
  const s = (subs || {})[i];
  if (s) return s;
  if (i === 0) return { status: 'trial', days: 9 };
  if (i === 1) return { status: 'active', until: new Date(Date.now() + 20 * DAY).toISOString() };
  if (i === 2) return { status: 'active', until: new Date(Date.now() + 6 * DAY).toISOString() };
  return { status: 'none' };
}

export function subInfo(sub) {
  if (sub.status === 'active') {
    if (new Date(sub.until).getTime() > Date.now()) return { label: 'Aktif · s/d ' + fmtDate(sub.until), tone: 'ok', active: true, btn: 'Perpanjang' };
    return { label: 'Berakhir', tone: 'warn', active: false, btn: 'Upgrade' };
  }
  if (sub.status === 'trial') return { label: 'Trial · ' + sub.days + ' hari', tone: 'warn', active: false, btn: 'Upgrade' };
  return { label: 'Belum berlangganan', tone: 'muted', active: false, btn: 'Upgrade' };
}

// Perpanjang dihitung dari tanggal berakhir bila langganan masih aktif.
export function activate(plan, sub) {
  const base = sub.status === 'active' && new Date(sub.until).getTime() > Date.now() ? new Date(sub.until).getTime() : Date.now();
  return { status: 'active', plan: plan.id, until: new Date(base + 30 * DAY).toISOString() };
}
