import React, { useState } from 'react';
import { Field, Modal, Page, Pill, Toggle, Unit, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { rp } from '../calc.js';
import { addLog } from '../store.js';

const slug = (n) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'paket';

function PlanModal({ plan, onSave, onClose }) {
  const [f, setF] = useState(plan ? { ...plan, price: String(plan.price), price12: plan.price12 ? String(plan.price12) : '', features: plan.features.join('\n') } : { name: '', price: '', price12: '', shown: true, features: '' });
  const set = (k) => (e) => setF((v) => ({ ...v, [k]: e.target.value }));
  const num = (k) => (e) => setF((v) => ({ ...v, [k]: e.target.value.replace(/\D/g, '') }));
  const ok = f.name.trim() && Number(f.price) > 0;
  return (
    <Modal title={plan ? 'Ubah paket' : 'Tambah paket'} onClose={onClose}>
      <Field label="Nama paket"><input className="input" value={f.name} onChange={set('name')} /></Field>
      <div className="grid g2" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <Field label="Harga per bulan"><Unit value={f.price} onChange={num('price')} unit="Rp" /></Field>
        <Field label="Harga 12 bulan"><Unit value={f.price12} onChange={num('price12')} unit="Rp" /></Field>
      </div>
      <Field label="Fitur (satu per baris)"><textarea className="textarea" value={f.features} onChange={set('features')} /></Field>
      <div className="row between"><span>Tampilkan ke customer</span><Toggle on={f.shown} onChange={(v) => setF((x) => ({ ...x, shown: v }))} label="Tampilkan" /></div>
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn soft" onClick={onClose}>Batal</button>
        <button className="btn primary" disabled={!ok} onClick={() => onSave({ id: plan?.id || slug(f.name), name: f.name.trim(), price: Number(f.price), price12: Number(f.price12) || 0, shown: f.shown, features: f.features.split('\n').map((x) => x.trim()).filter(Boolean) })}>Simpan</button>
      </div>
    </Modal>
  );
}

export default function Paket() {
  const { s, update } = useApp();
  const [edit, setEdit] = useState(null); // null | 'new' | plan
  const [bank, setBank] = useState({ bank: '', number: '', holder: '' });
  const count = (id) => s.customers.filter((c) => c.status === 'active' && c.plan === id).length;
  const savePlan = (p) => {
    update((x) => addLog({ ...x, plans: x.plans.some((y) => y.id === p.id) ? x.plans.map((y) => (y.id === p.id ? p : y)) : [...x.plans, p] }, `Menyimpan paket ${p.name}`));
    setEdit(null);
  };
  const rule = (k) => (e) => update((x) => ({ ...x, rules: { ...x.rules, [k]: Number(e.target.value.replace(/\D/g, '')) || 0 } }));
  // Format yang sama dengan PLANS di webapp customer (src/plans.js).
  const exportPlans = () => {
    const data = s.plans.filter((p) => p.shown).map((p) => ({ id: p.id, name: p.name, price: p.price, period: 'bulan', features: p.features }));
    const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })); a.download = 'plans.json'; a.click(); URL.revokeObjectURL(a.href);
  };

  return (
    <Page title="Paket & Harga" sub="Paket langganan yang ditawarkan ke customer">
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn soft" onClick={exportPlans}><Icon name="download" size={18} />Ekspor plans.json</button>
        <button className="btn primary" onClick={() => setEdit('new')}><Icon name="plus" size={18} />Tambah paket</button>
      </div>
      <div className="grid g3">
        {s.plans.map((p) => (
          <div className="card" key={p.id} style={{ gap: 14 }}>
            <div className="row between nowrap"><h2 style={{ fontSize: 18 }}>{p.name}</h2><Pill tone={p.shown ? 'ink' : ''}>{p.shown ? 'Ditampilkan' : 'Disembunyikan'}</Pill></div>
            <div><span style={{ fontSize: 30, fontWeight: 600, letterSpacing: '-.02em' }}>{rp(p.price)}</span><span className="muted"> / bulan</span></div>
            {p.price12 ? <span className="muted sm">12 bulan: {rp(p.price12)} (hemat {rp(p.price * 12 - p.price12)})</span> : null}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>{p.features.map((f) => <span key={f} className="row nowrap"><span style={{ color: 'var(--accent)' }}><Icon name="check" size={16} /></span>{f}</span>)}</div>
            <div className="row between" style={{ marginTop: 'auto', borderTop: '1px solid var(--bg)', paddingTop: 14 }}>
              <span className="muted sm">{count(p.id)} customer aktif</span>
              <span className="row">
                {count(p.id) === 0 && s.plans.length > 1 ? <button className="btn s" onClick={() => update((x) => addLog({ ...x, plans: x.plans.filter((y) => y.id !== p.id) }, `Menghapus paket ${p.name}`))}>Hapus</button> : null}
                <button className="btn s" onClick={() => setEdit(p)}><Icon name="pencil" size={15} />Ubah</button>
              </span>
            </div>
          </div>
        ))}
      </div>
      <div className="grid g2">
        <div className="card" style={{ gap: 16 }}>
          <h2>Aturan langganan</h2>
          <div className="grid" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
            <Field label="Trial gratis"><Unit value={s.rules.trialDays} onChange={rule('trialDays')} unit="hari" /></Field>
            <Field label="Bayar 12 bulan"><Unit value={s.rules.payMonths12} onChange={rule('payMonths12')} unit="bulan" /></Field>
            <Field label="Masa tenggang"><Unit value={s.rules.graceDays} onChange={rule('graceDays')} unit="hari" /></Field>
          </div>
        </div>
        <div className="card" style={{ gap: 14 }}>
          <div className="row between"><h2>Rekening tujuan transfer</h2><span className="muted sm">tampil di halaman bayar customer</span></div>
          {s.banks.map((b, i) => (
            <div className="row between nowrap" key={i} style={{ padding: 12, borderRadius: 12, background: 'var(--surface)' }}>
              <span className="row nowrap"><span className="av ink" style={{ width: 'auto', minWidth: 52, height: 36, padding: '0 10px', fontSize: 12 }}>{b.bank}</span><span><b className="mono" style={{ fontSize: 14, fontWeight: 600 }}>{b.number}</b><div className="muted sm">{b.holder}</div></span></span>
              <Toggle on={b.on} label={'Aktifkan ' + b.bank} onChange={(v) => update((x) => ({ ...x, banks: x.banks.map((y, j) => (j === i ? { ...y, on: v } : y)) }))} />
            </div>
          ))}
          <div className="row">
            <input className="input" style={{ flex: '1 1 90px' }} placeholder="Bank" value={bank.bank} onChange={(e) => setBank({ ...bank, bank: e.target.value })} />
            <input className="input" style={{ flex: '2 1 150px' }} placeholder="No. rekening" value={bank.number} onChange={(e) => setBank({ ...bank, number: e.target.value })} />
            <input className="input" style={{ flex: '2 1 150px' }} placeholder="Atas nama" value={bank.holder} onChange={(e) => setBank({ ...bank, holder: e.target.value })} />
            <button className="btn ink" disabled={!bank.bank || !bank.number} onClick={() => { update((x) => ({ ...x, banks: [...x.banks, { ...bank, on: true }] })); setBank({ bank: '', number: '', holder: '' }); }}>Tambah</button>
          </div>
        </div>
      </div>
      {edit ? <PlanModal plan={edit === 'new' ? null : edit} onSave={savePlan} onClose={() => setEdit(null)} /> : null}
    </Page>
  );
}
