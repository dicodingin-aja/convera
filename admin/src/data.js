import { NAMES, addMonths } from './calc.js';

const DAY = 864e5;
const ago = (days) => new Date(Date.now() - days * DAY).toISOString();

// Bentuk paket sama dengan PLANS di webapp customer (src/plans.js), ditambah price12 dan shown.
export const PLANS_SEED = [
  { id: 'workspace', name: 'Workspace', price: 150000, price12: 1500000, shown: true, features: ['1 workspace', 'Event & Log Event', 'Laporan', 'Koneksi Meta & WhatsApp'] },
];

// [nama, pemilik, kota, status, bulan sejak berlangganan, tanggal bayar, online (menit lalu, null = offline)]
const SHAPE = [
  ['Kopi Senja', 'Rizky Saputra', 'Jakarta Selatan', 'active', 6, 3, 2], ['Klinik Gigi Senyum', 'drg. Maya Anggraini', 'Bandung', 'active', 6, 9, null],
  ['Rumah Pijat Sehati', 'Agus Salim', 'Surabaya', 'active', 5, 12, 0], ['Laundry Kilat Bersih', 'Hendra Wijaya', 'Depok', 'active', 5, 18, null],
  ['Bimbel Cerdas Mandiri', 'Teguh Prasetyo', 'Medan', 'active', 4, 21, 14], ['Toko Hijab Amira', 'Amira Putri', 'Yogyakarta', 'active', 3, 6, null],
  ['Cuci Sepatu Kinclong', 'Fajar Nugroho', 'Yogyakarta', 'active', 3, 25, 1], ['Bakso Pak Harun', 'Harun Ar-Rasyid', 'Solo', 'active', 2, 15, 3],
  ['Salon Cantik Ayu', 'Ayu Lestari', 'Bekasi', 'active', 1, 2, null], ['Bengkel Motor Jaya', 'Slamet Riyadi', 'Semarang', 'active', 1, 28, 0],
  ['Katering Bu Wati', 'Wati Handayani', 'Malang', 'active', 0, 1, 7], ['Studio Foto Lumina', 'Dewi Anggraini', 'Bandung', 'trial', 0, 0, 4],
  ['Toko Bunga Mawar', 'Sri Wulandari', 'Tangerang', 'trial', 0, 0, null], ['Les Musik Nada', 'Budi Hartono', 'Jakarta Barat', 'expired', 4, 0, null],
  ['Apotek Sehat Sentosa', 'Nur Aini', 'Bogor', 'expired', 2, 0, null], ['Barbershop Gentleman', 'Rudi Hartono', 'Depok', 'pending', 0, 0, null],
];

export function seedCustomers() {
  const customers = []; const payments = [];
  SHAPE.forEach(([name, owner, city, status, since, payDay, online], i) => {
    const id = 'c' + (i + 1);
    const slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '');
    const c = { id, name, owner, city, phone: '0812-' + String(7000 + i * 137).padStart(4, '0') + '-' + String(1000 + i * 733).slice(0, 4), email: 'owner@' + slug + '.id', status, plan: status === 'active' || status === 'expired' ? 'workspace' : null, sales: NAMES[i % NAMES.length], online, lastActive: online === null ? ago(0.05 + (i % 6) * 0.4) : ago(0), until: null, trialUntil: null };
    if (status === 'active' || status === 'expired') {
      const now = new Date(); let last = null;
      const from = status === 'expired' ? since + 2 : since;
      for (let m = from; m >= (status === 'expired' ? since : 0); m--) {
        const dt = new Date(now.getFullYear(), now.getMonth() - m, Math.min(payDay || 10, 28), 10, 0, 0);
        if (dt.getTime() > Date.now()) continue;
        payments.push({ id: `p${id}-${m}`, customerId: id, date: dt.toISOString(), amount: 150000, plan: 'workspace', months: 1, sales: c.sales });
        last = dt;
      }
      c.until = last ? addMonths(last, 1) : null;
      if (status === 'active' && (!c.until || new Date(c.until).getTime() < Date.now())) c.until = new Date(Date.now() + 3 * DAY).toISOString();
    }
    if (status === 'trial') c.trialUntil = new Date(Date.now() + (i === 11 ? 5 : 9) * DAY).toISOString();
    customers.push(c);
  });
  return { customers, payments };
}

export function seedProofs() {
  return [
    { id: 'b1', customerId: 'c16', kind: 'new', bank: 'BCA', toBank: 'BCA 523 088 1290', sender: 'Rudi Hartono', ref: 'TRF041188234410', billed: 150000, received: 150000, months: 1, message: 'Halo kak, sudah transfer untuk daftar paket ya. Dapat info dari Mas Jordi.', uploadedAt: ago(0.02), status: 'pending' },
    { id: 'b2', customerId: 'c14', kind: 'renew', bank: 'Mandiri', toBank: 'Mandiri 137 000 4421 889', sender: 'Budi Hartono', ref: 'MDR2026110400172', billed: 150000, received: 151000, months: 1, message: 'Perpanjang sebulan lagi ya kak.', uploadedAt: ago(0.2), status: 'pending' },
    { id: 'b3', customerId: 'c12', kind: 'trial', bank: 'BRI', toBank: 'BCA 523 088 1290', sender: 'Dewi Anggraini', ref: 'BRI031121409981', billed: 450000, received: 450000, months: 3, message: 'Trial-nya cocok, lanjut 3 bulan ya kak.', uploadedAt: ago(0.9), status: 'pending' },
  ];
}

export const SERVER_KEYS = [
  { id: 'k1', name: 'Server produksi', key: 'cvr_admin_7Hq2••••••••xP4m', used: '1 menit lalu' },
  { id: 'k2', name: 'n8n otomasi', key: 'cvr_admin_b31K••••••••9wTe', used: 'Kemarin' },
];

export function buildSeed() {
  const { customers, payments } = seedCustomers();
  return {
    customers, payments, proofs: seedProofs(), plans: PLANS_SEED, paid: {},
    rules: { trialDays: 14, payMonths12: 10, graceDays: 3 },
    banks: [{ bank: 'BCA', number: '523 088 1290', holder: 'Convera (a.n. Jordi)', on: true }, { bank: 'Mandiri', number: '137 000 4421 889', holder: 'Convera (a.n. Axl)', on: true }],
    integrations: { meta: true, whatsapp: true, notifyProof: true, email: false, storage: true, sheets: false },
    flags: { qr: true, simulasi: true, laporan: true, tiktok: false, instagram: false },
    maintenance: false, maintenanceMsg: 'Convera sedang pemeliharaan singkat. Event tetap tercatat.',
    keys: SERVER_KEYS, webhook: 'https://api.convera.id/hooks/admin',
    log: [{ at: ago(0.1), who: 'Admin', text: 'Masuk ke Convera Admin' }],
    komisiPct: 10, kasPct: 10, dark: false,
  };
}
