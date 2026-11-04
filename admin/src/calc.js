// Tiga founder. Sales yang dipilih saat aktivasi menerima komisi.
export const NAMES = ['Jordi', 'Axl', 'Albert'];

export const rp = (n) => 'Rp' + Math.round(Number(n) || 0).toLocaleString('id-ID');
export const monthKey = (d) => { const x = new Date(d); return x.getFullYear() + '-' + String(x.getMonth() + 1).padStart(2, '0'); };
export const monthLabel = (key) => new Date(key + '-01T00:00:00').toLocaleDateString('id-ID', { month: 'long', year: 'numeric' });
export const monthShort = (key) => new Date(key + '-01T00:00:00').toLocaleDateString('id-ID', { month: 'short' });
export const fmtDate = (d) => new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' });
export const fmtShort = (d) => new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
export const daysFromNow = (d) => Math.ceil((new Date(d).getTime() - Date.now()) / 864e5);
export function addMonths(date, n) { const d = new Date(date); d.setMonth(d.getMonth() + n); return d.toISOString(); }
export function lastMonths(count) {
  const out = []; const now = new Date();
  for (let i = count - 1; i >= 0; i--) out.push(monthKey(new Date(now.getFullYear(), now.getMonth() - i, 1)));
  return out;
}

// Pembagian satu pembayaran: komisi → sales, kas bersama, sisanya dibagi rata (pembulatan masuk kas).
export function split(amount, sales, komisiPct, kasPct) {
  const komisi = NAMES.includes(sales) ? Math.floor((amount * komisiPct) / 100) : 0;
  const kas0 = Math.floor((amount * kasPct) / 100);
  const rest = amount - komisi - kas0;
  const each = Math.floor(rest / NAMES.length);
  const sisa = rest - each * NAMES.length;
  const per = {};
  NAMES.forEach((n) => { per[n] = each + (n === sales ? komisi : 0); });
  return { komisi, kas: kas0 + sisa, each, per };
}

export function monthTotals(payments, key, komisiPct, kasPct) {
  const t = { revenue: 0, kas: 0, per: Object.fromEntries(NAMES.map((n) => [n, 0])), komisi: Object.fromEntries(NAMES.map((n) => [n, 0])), count: 0 };
  payments.filter((p) => monthKey(p.date) === key).forEach((p) => {
    const s = split(p.amount, p.sales, komisiPct, kasPct);
    t.revenue += p.amount; t.kas += s.kas; t.count += 1;
    NAMES.forEach((n) => { t.per[n] += s.per[n]; if (n === p.sales) t.komisi[n] += s.komisi; });
  });
  return t;
}
