import { useCallback, useEffect, useState } from 'react';
import { buildSeed } from './data.js';
import { addMonths } from './calc.js';

const KEY = 'convera-admin:v2';

export function useStore() {
  const [s, setS] = useState(() => {
    const seed = buildSeed();
    try { const saved = JSON.parse(localStorage.getItem(KEY)); return saved ? { ...seed, ...saved } : seed; } catch { return seed; }
  });
  useEffect(() => { try { localStorage.setItem(KEY, JSON.stringify(s)); } catch { /* abaikan */ } }, [s]);
  const update = useCallback((fn) => setS((prev) => fn(prev)), []);
  const reset = useCallback(() => { try { localStorage.removeItem(KEY); } catch { /* abaikan */ } setS(buildSeed()); }, []);
  return [s, update, reset];
}

const addLog = (s, text) => ({ ...s, log: [{ at: new Date().toISOString(), who: 'Admin', text }, ...s.log].slice(0, 100) });

// Aktivasi/perpanjang: dari bukti transfer (proofId) atau manual dari halaman Customer.
export function activate(s, { customerId, proofId, planId, months, amount, sales }) {
  const now = new Date();
  const c = s.customers.find((x) => x.id === customerId);
  const base = c.status === 'active' && c.until && new Date(c.until) > now ? c.until : now.toISOString();
  const payment = { id: 'p' + Date.now(), customerId, date: now.toISOString(), amount: Number(amount), plan: planId, months: Number(months), sales };
  const next = {
    ...s,
    payments: [...s.payments, payment],
    customers: s.customers.map((x) => (x.id === customerId ? { ...x, status: 'active', plan: planId, until: addMonths(base, Number(months)), sales: x.sales || sales } : x)),
    proofs: proofId ? s.proofs.map((p) => (p.id === proofId ? { ...p, status: 'approved', sales, amount: Number(amount), decidedAt: now.toISOString() } : p)) : s.proofs,
  };
  return addLog(next, `Mengaktifkan ${c.name} (${months} bulan, sales ${sales})`);
}

export function rejectProof(s, proofId, reason) {
  const p = s.proofs.find((x) => x.id === proofId);
  const c = s.customers.find((x) => x.id === p.customerId);
  return addLog({ ...s, proofs: s.proofs.map((x) => (x.id === proofId ? { ...x, status: 'rejected', reason, decidedAt: new Date().toISOString() } : x)) }, `Menolak bukti transfer ${c.name}: ${reason}`);
}

export function stopSubscription(s, customerId) {
  const c = s.customers.find((x) => x.id === customerId);
  return addLog({ ...s, customers: s.customers.map((x) => (x.id === customerId ? { ...x, status: 'expired', until: new Date().toISOString() } : x)) }, `Menghentikan langganan ${c.name}`);
}

export function markPaid(s, month, person, value) {
  const cur = s.paid[month] || {};
  return addLog({ ...s, paid: { ...s.paid, [month]: { ...cur, [person]: value } } }, `${value ? 'Menandai' : 'Membatalkan tanda'} bagi hasil ${person} ${month} sudah ditransfer`);
}

export { addLog };
