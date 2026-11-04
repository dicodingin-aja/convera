import React, { useState } from 'react';
import { Field, Modal, Unit, useApp } from './ui.jsx';
import { Icon } from './Icon.jsx';
import { NAMES, rp, split } from './calc.js';
import { activate } from './store.js';

// Form aktivasi/perpanjang. Sales wajib dipilih (penerima komisi).
export default function ActivateModal({ customer, proof, onClose }) {
  const { s, update } = useApp();
  const plans = s.plans;
  const [planId, setPlanId] = useState(customer.plan && plans.find((p) => p.id === customer.plan) ? customer.plan : plans[0].id);
  const [months, setMonths] = useState(proof ? proof.months : 1);
  const plan = plans.find((p) => p.id === planId);
  const suggested = months === 12 && plan.price12 ? plan.price12 : plan.price * months;
  const [amount, setAmount] = useState(proof ? proof.received : suggested);
  const [sales, setSales] = useState(customer.sales && NAMES.includes(customer.sales) ? customer.sales : '');
  const pick = (m, p = plan) => { setMonths(m); setAmount(m === 12 && p.price12 ? p.price12 : p.price * m); };
  const calc = sales ? split(Number(amount) || 0, sales, s.komisiPct, s.kasPct) : null;
  const diff = proof ? proof.received - suggested : 0;

  const submit = () => {
    update((x) => activate(x, { customerId: customer.id, proofId: proof?.id, planId, months, amount, sales }));
    onClose();
  };
  return (
    <Modal title={proof ? 'Aktifkan akun' : 'Perpanjang manual'} onClose={onClose}>
      <div><b style={{ fontWeight: 600, fontSize: 16 }}>{customer.name}</b><div className="muted">{customer.owner} · {customer.city}</div></div>
      <div className="grid g2" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
        <Field label="Paket">
          <select className="select" value={planId} onChange={(e) => { setPlanId(e.target.value); pick(months, plans.find((p) => p.id === e.target.value)); }}>
            {plans.map((p) => <option key={p.id} value={p.id}>{p.name} · {rp(p.price)}/bulan</option>)}
          </select>
        </Field>
        <Field label="Periode">
          <select className="select" value={months} onChange={(e) => pick(Number(e.target.value))}>
            {[...new Set([1, 3, 12, months])].sort((a, b) => a - b).map((m) => <option key={m} value={m}>{m} bulan</option>)}
          </select>
        </Field>
        <Field label="Nominal diterima"><Unit value={amount} onChange={(e) => setAmount(e.target.value.replace(/\D/g, ''))} unit="Rp" /></Field>
        <Field label="Sales yang mendaftarkan">
          <select className="select" value={sales} onChange={(e) => setSales(e.target.value)}>
            <option value="">Pilih sales…</option>{NAMES.map((n) => <option key={n}>{n}</option>)}
          </select>
        </Field>
      </div>
      {proof && diff !== 0 ? <div className="warnbox"><Icon name="alert" size={18} />Nominal {diff > 0 ? 'lebih' : 'kurang'} {rp(Math.abs(diff))} dari tagihan {rp(suggested)}.</div> : null}
      <div className="card" style={{ gap: 8 }}>
        <span className="caps">Pembagian pembayaran ini</span>
        {calc ? (
          <>
            {NAMES.map((n) => <div key={n} className="row between"><span>{n}{n === sales ? <span className="badge" style={{ marginLeft: 8 }}>sales</span> : null}</span><b style={{ fontWeight: 500 }}>{rp(calc.per[n])}</b></div>)}
            <div className="row between"><span>Kas bersama</span><b style={{ fontWeight: 500 }}>{rp(calc.kas)}</b></div>
          </>
        ) : <span className="muted">Pilih sales untuk melihat pembagian.</span>}
      </div>
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn soft" onClick={onClose}>Batal</button>
        <button className="btn primary" disabled={!sales || !Number(amount)} onClick={submit}><Icon name="check" size={18} />Aktifkan</button>
      </div>
    </Modal>
  );
}
