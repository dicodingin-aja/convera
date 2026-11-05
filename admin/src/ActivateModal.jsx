import React, { useEffect, useState } from 'react';
import { ErrorNote, Field, Loading, Modal, Unit } from './ui.jsx';
import { Icon } from './Icon.jsx';
import { api } from './api.js';
import { previewSplit, rp } from './calc.js';
import { useAction, useData } from './hooks.js';

// Form aktivasi/perpanjang. Sales wajib dipilih (penerima komisi). Dari bukti transfer -> approve;
// tanpa bukti -> perpanjang manual. Pembagian yang tampil hanya perkiraan; angka final dihitung server.
export default function ActivateModal({ customer, proof, onClose, onDone }) {
  const plansQ = useData('/plans');
  const salesQ = useData('/sales-people');
  const rulesQ = useData('/subscription-rules');
  const { busy, error, run } = useAction();
  const [planCode, setPlanCode] = useState('');
  const [months, setMonths] = useState(proof ? proof.months : 1);
  const [amount, setAmount] = useState('');
  const [salesId, setSalesId] = useState('');

  const plans = plansQ.data?.data || [];
  const sales = (salesQ.data?.data || []).filter((x) => x.active);
  const rules = rulesQ.data?.data;
  const plan = plans.find((p) => p.code === planCode);
  const suggestedFor = (p, m) => (!p ? 0 : m === 12 && p.price_12m ? p.price_12m : p.price_month * m);
  const suggested = suggestedFor(plan, months);

  // Nilai awal setelah data siap.
  useEffect(() => {
    if (!plans.length || planCode) return;
    const initial = plans.find((p) => p.code === (proof?.plan || customer.plan)) || plans[0];
    setPlanCode(initial.code);
    setAmount(String(proof ? Math.round(proof.received ?? proof.billed) : suggestedFor(initial, months)));
  }, [plans.length]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (salesId || !sales.length) return;
    const known = sales.find((x) => x.name === customer.sales);
    if (known) setSalesId(known.id);
  }, [sales.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const pick = (m, p = plan) => { setMonths(m); setAmount(String(suggestedFor(p, m))); };
  const founders = sales.filter((x) => x.is_founder);
  const chosen = sales.find((x) => x.id === salesId);
  const calc = chosen && rules ? previewSplit(Number(amount) || 0, rules.komisi_pct, rules.kas_pct, founders.length) : null;
  const diff = proof ? Math.round((proof.received ?? proof.billed) - suggested) : 0;

  const submit = async () => {
    const body = { plan: planCode, months, amount: Number(amount), sales_id: salesId };
    const r = await run(() => (proof ? api.post(`/payment-proofs/${proof.id}/approve`, body) : api.post(`/customers/${customer.id}/extend`, body)));
    if (r) { onDone?.(); onClose(); }
  };

  const loading = plansQ.loading || salesQ.loading || rulesQ.loading;
  const loadErr = plansQ.error || salesQ.error || rulesQ.error;
  return (
    <Modal title={proof ? 'Aktifkan akun' : 'Perpanjang manual'} onClose={onClose}>
      <div><b style={{ fontWeight: 600, fontSize: 16 }}>{customer.name}</b><div className="muted">{customer.owner} · {customer.city}</div></div>
      {loading || loadErr || !plan ? <Loading error={loadErr || (!loading && !plans.length ? 'Belum ada paket. Buat paket dulu di halaman Paket & Harga.' : '')} /> : (
        <>
          <div className="grid g2" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            <Field label="Paket">
              <select className="select" value={planCode} onChange={(e) => { setPlanCode(e.target.value); pick(months, plans.find((p) => p.code === e.target.value)); }}>
                {plans.map((p) => <option key={p.code} value={p.code}>{p.name} · {rp(p.price_month)}/bulan</option>)}
              </select>
            </Field>
            <Field label="Periode">
              <select className="select" value={months} onChange={(e) => pick(Number(e.target.value))}>
                {[1, 3, 12].map((m) => <option key={m} value={m}>{m} bulan</option>)}
              </select>
            </Field>
            <Field label="Nominal diterima"><Unit value={amount} onChange={(e) => setAmount(e.target.value.replace(/\D/g, ''))} unit="Rp" /></Field>
            <Field label="Sales yang mendaftarkan">
              <select className="select" value={salesId} onChange={(e) => setSalesId(e.target.value)}>
                <option value="">Pilih sales…</option>{sales.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
              </select>
            </Field>
          </div>
          {proof && diff !== 0 ? <div className="warnbox"><Icon name="alert" size={18} />Nominal {diff > 0 ? 'lebih' : 'kurang'} {rp(Math.abs(diff))} dari tagihan {rp(suggested)}.</div> : null}
          <div className="card" style={{ gap: 8 }}>
            <span className="caps">Perkiraan pembagian pembayaran ini</span>
            {calc ? (
              <>
                {founders.map((n) => <div key={n.id} className="row between"><span>{n.name}{n.id === salesId ? <span className="badge" style={{ marginLeft: 8 }}>sales</span> : null}</span><b style={{ fontWeight: 500 }}>{rp(calc.each + (n.id === salesId ? calc.komisi : 0))}</b></div>)}
                {chosen && !chosen.is_founder ? <div className="row between"><span>{chosen.name}<span className="badge" style={{ marginLeft: 8 }}>sales</span></span><b style={{ fontWeight: 500 }}>{rp(calc.komisi)}</b></div> : null}
                <div className="row between"><span>Kas bersama</span><b style={{ fontWeight: 500 }}>{rp(calc.kas)}</b></div>
              </>
            ) : <span className="muted">Pilih sales untuk melihat pembagian.</span>}
          </div>
          <ErrorNote>{error}</ErrorNote>
        </>
      )}
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn soft" onClick={onClose}>Batal</button>
        <button className="btn primary" disabled={busy || !plan || !salesId || !Number(amount)} onClick={submit}><Icon name="check" size={18} />{busy ? 'Memproses…' : 'Aktifkan'}</button>
      </div>
    </Modal>
  );
}
