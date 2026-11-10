import React, { useEffect, useState } from 'react';
import { ErrorNote, Field, Loading, Modal, Page, Pill, Toggle, Unit, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { api } from '../api.js';
import { rp } from '../calc.js';
import { useAction, useData } from '../hooks.js';

const slug = (n) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 30) || 'paket';

function PlanModal({ plan, onClose, onDone }) {
  const [f, setF] = useState(plan ? { name: plan.name, price: String(plan.price_month), price12: String(plan.price_12m || ''), shown: plan.shown, features: (plan.features || []).join('\n') } : { name: '', price: '', price12: '', shown: true, features: '' });
  const { busy, error, run } = useAction();
  const set = (k) => (e) => setF((v) => ({ ...v, [k]: e.target.value }));
  const num = (k) => (e) => setF((v) => ({ ...v, [k]: e.target.value.replace(/\D/g, '') }));
  const ok = f.name.trim() && Number(f.price) > 0;
  const save = async () => {
    const body = { name: f.name.trim(), price_month: Number(f.price), price_12m: Number(f.price12) || 0, shown: f.shown, sort: plan?.sort ?? 0, features: f.features.split('\n').map((x) => x.trim()).filter(Boolean) };
    const r = await run(() => (plan ? api.put('/plans/' + plan.id, body) : api.post('/plans', { ...body, code: slug(f.name) })));
    if (r) { onDone(); onClose(); }
  };
  return (
    <Modal title={plan ? 'Ubah paket' : 'Tambah paket'} onClose={onClose}>
      <Field label="Nama paket"><input className="input" value={f.name} onChange={set('name')} /></Field>
      <div className="grid g2" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <Field label="Harga per bulan"><Unit value={f.price} onChange={num('price')} unit="Rp" /></Field>
        <Field label="Harga 12 bulan"><Unit value={f.price12} onChange={num('price12')} unit="Rp" /></Field>
      </div>
      <Field label="Fitur (satu per baris)"><textarea className="textarea" value={f.features} onChange={set('features')} /></Field>
      <div className="row between"><span>Tampilkan ke customer</span><Toggle on={f.shown} onChange={(v) => setF((x) => ({ ...x, shown: v }))} label="Tampilkan" /></div>
      {plan ? <span className="muted sm">Kode paket ({plan.code}) tidak bisa diubah.</span> : null}
      <ErrorNote>{error}</ErrorNote>
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn soft" onClick={onClose}>Batal</button>
        <button className="btn primary" disabled={!ok || busy} onClick={save}>{busy ? 'Menyimpan…' : 'Simpan'}</button>
      </div>
    </Modal>
  );
}

const RULE_FIELDS = [['trial_days', 'Trial gratis', 'hari'], ['pay_months_12', 'Bayar 12 bulan', 'bulan'], ['grace_days', 'Masa tenggang', 'hari'], ['komisi_pct', 'Komisi sales', '%'], ['kas_pct', 'Kas bersama', '%'], ['retention_event_days', 'Simpan log event', 'hari'], ['retention_inbox_days', 'Simpan jejak webhook', 'hari']];

function Rules() {
  const q = useData('/subscription-rules');
  const { busy, error, run } = useAction();
  const [f, setF] = useState(null);
  const [saved, setSaved] = useState(false);
  useEffect(() => { if (q.data) setF(Object.fromEntries(Object.entries(q.data.data).map(([k, v]) => [k, String(v)]))); }, [q.data]);
  if (q.error || !f) return <div className="card"><h2>Aturan langganan</h2><Loading error={q.error} /></div>;
  const save = async () => {
    const body = Object.fromEntries(RULE_FIELDS.map(([k]) => [k, Number(f[k])]));
    if (await run(() => api.put('/subscription-rules', body))) { setSaved(true); q.reload(); setTimeout(() => setSaved(false), 2500); }
  };
  return (
    <div className="card" style={{ gap: 16 }}>
      <h2>Aturan langganan & bagi hasil</h2>
      <div className="grid" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
        {RULE_FIELDS.map(([k, label, unit]) => (
          <Field key={k} label={label}><Unit value={f[k]} onChange={(e) => { setF({ ...f, [k]: e.target.value.replace(/[^\d.]/g, '') }); setSaved(false); }} unit={unit} /></Field>
        ))}
      </div>
      <span className="muted sm">Perubahan komisi/kas hanya berlaku untuk pembayaran berikutnya; riwayat tidak berubah. Hanya superadmin yang bisa menyimpan.</span>
      <ErrorNote>{error}</ErrorNote>
      <div className="row"><button className="btn primary" disabled={busy} onClick={save}>{busy ? 'Menyimpan…' : 'Simpan aturan'}</button>{saved ? <span className="muted" role="status">Tersimpan</span> : null}</div>
    </div>
  );
}

function Banks() {
  const q = useData('/bank-accounts');
  const { busy, error, run } = useAction();
  const [bank, setBank] = useState({ bank: '', number: '', holder: '' });
  const rows = q.data?.data || [];
  const toggle = (b, v) => run(async () => { await api.put('/bank-accounts/' + b.id, { bank: b.bank, number: b.number, holder: b.holder, sort: b.sort, active: v }); q.reload(); });
  const add = async () => { if (await run(() => api.post('/bank-accounts', bank))) { setBank({ bank: '', number: '', holder: '' }); q.reload(); } };
  const del = async (b) => { if (window.confirm(`Hapus rekening ${b.bank} ${b.number}?`)) run(async () => { await api.del('/bank-accounts/' + b.id); q.reload(); }); };
  return (
    <div className="card" style={{ gap: 14 }}>
      <div className="row between"><h2>Rekening tujuan transfer</h2><span className="muted sm">tampil di halaman bayar customer</span></div>
      {q.error ? <Loading error={q.error} /> : null}
      {rows.map((b) => (
        <div className="row between nowrap" key={b.id} style={{ padding: 12, borderRadius: 12, background: 'var(--surface)' }}>
          <span className="row nowrap"><span className="av ink" style={{ width: 'auto', minWidth: 52, height: 36, padding: '0 10px', fontSize: 12 }}>{b.bank}</span><span><b className="mono" style={{ fontSize: 14, fontWeight: 600 }}>{b.number}</b><div className="muted sm">{b.holder}</div></span></span>
          <span className="row nowrap"><button className="btn ghost s" onClick={() => del(b)}>Hapus</button><Toggle on={b.active} label={'Aktifkan ' + b.bank} onChange={(v) => toggle(b, v)} /></span>
        </div>
      ))}
      {!q.loading && rows.length === 0 ? <span className="muted">Belum ada rekening. Tambahkan agar customer tahu tujuan transfer.</span> : null}
      <div className="row">
        <input className="input" style={{ flex: '1 1 90px' }} placeholder="Bank" value={bank.bank} onChange={(e) => setBank({ ...bank, bank: e.target.value })} />
        <input className="input" style={{ flex: '2 1 150px' }} placeholder="No. rekening" value={bank.number} onChange={(e) => setBank({ ...bank, number: e.target.value })} />
        <input className="input" style={{ flex: '2 1 150px' }} placeholder="Atas nama" value={bank.holder} onChange={(e) => setBank({ ...bank, holder: e.target.value })} />
        <button className="btn ink" disabled={busy || !bank.bank || !bank.number || !bank.holder} onClick={add}>Tambah</button>
      </div>
      <ErrorNote>{error}</ErrorNote>
    </div>
  );
}

export default function Paket() {
  const { me } = useApp();
  const q = useData('/plans');
  const [edit, setEdit] = useState(null); // null | 'new' | plan
  const { error, run } = useAction();
  const plans = q.data?.data || [];
  const del = async (p) => { if (window.confirm(`Hapus paket ${p.name}? Paket disembunyikan dari customer.`)) run(async () => { await api.del('/plans/' + p.id); q.reload(); }); };

  return (
    <Page title="Paket & Harga" sub="Paket langganan yang ditawarkan ke customer">
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn primary" onClick={() => setEdit('new')}><Icon name="plus" size={18} />Tambah paket</button>
      </div>
      <ErrorNote>{error}</ErrorNote>
      {q.error || (q.loading && !q.data) ? <Loading error={q.error} /> : (
        <div className="grid g3">
          {plans.map((p) => (
            <div className="card" key={p.id} style={{ gap: 14 }}>
              <div className="row between nowrap"><h2 style={{ fontSize: 18 }}>{p.name}</h2><Pill tone={p.shown ? 'ink' : ''}>{p.shown ? 'Ditampilkan' : 'Disembunyikan'}</Pill></div>
              <div><span style={{ fontSize: 30, fontWeight: 600, letterSpacing: '-.02em' }}>{rp(p.price_month)}</span><span className="muted"> / kampanye / 30 hari</span></div>
              {p.price_12m ? <span className="muted sm">12 bulan: {rp(p.price_12m)} (hemat {rp(p.price_month * 12 - p.price_12m)})</span> : null}
              <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>{(p.features || []).map((f) => <span key={f} className="row nowrap"><span style={{ color: 'var(--accent)' }}><Icon name="check" size={16} /></span>{f}</span>)}</div>
              <div className="row between" style={{ marginTop: 'auto', borderTop: '1px solid var(--bg)', paddingTop: 14 }}>
                <span className="muted sm">{p.active_customers} customer aktif</span>
                <span className="row">
                  {p.active_customers === 0 && plans.length > 1 ? <button className="btn s" onClick={() => del(p)}>Hapus</button> : null}
                  <button className="btn s" onClick={() => setEdit(p)}><Icon name="pencil" size={15} />Ubah</button>
                </span>
              </div>
            </div>
          ))}
          {plans.length === 0 ? <div className="card muted">Belum ada paket.</div> : null}
        </div>
      )}
      <div className="grid g2">
        <Rules />
        <Banks />
      </div>
      {me.role !== 'superadmin' ? <span className="muted sm">Anda login sebagai {me.role}: aturan langganan hanya bisa diubah superadmin.</span> : null}
      {edit ? <PlanModal plan={edit === 'new' ? null : edit} onClose={() => setEdit(null)} onDone={q.reload} /> : null}
    </Page>
  );
}
