import React, { useState } from 'react';
import { Av, Kpi, Modal, Page, Pill, downloadCsv, statusInfo, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { daysFromNow, fmtDate, fmtShort, rp } from '../calc.js';
import { stopSubscription } from '../store.js';
import ActivateModal from '../ActivateModal.jsx';

const lastSeen = (c) => (c.online === null ? Math.max(1, Math.round((Date.now() - new Date(c.lastActive)) / 36e5)) + ' jam lalu' : c.online === 0 ? 'Online' : c.online + ' mnt lalu');

export default function Customer() {
  const { s, update } = useApp();
  const [f, setF] = useState('all');
  const [q, setQ] = useState('');
  const [sel, setSel] = useState(null);
  const [ext, setExt] = useState(null);
  const C = s.customers;
  const has = {
    all: () => true, online: (c) => c.online !== null, offline: (c) => c.online === null, active: (c) => c.status === 'active',
    expired: (c) => c.status === 'expired', trial: (c) => c.status === 'trial', pending: (c) => c.status === 'pending',
  };
  const FILTERS = [['all', 'Semua'], ['online', 'Online'], ['offline', 'Offline'], ['active', 'Berlangganan'], ['expired', 'Tidak berlangganan'], ['trial', 'Trial'], ['pending', 'Menunggu']];
  const rows = C.filter((c) => has[f](c) && (c.name + c.owner + c.city).toLowerCase().includes(q.toLowerCase()));
  const detail = sel && C.find((c) => c.id === sel);
  const planName = (c) => (c.plan ? s.plans.find((p) => p.id === c.plan)?.name || c.plan : '—');
  const until = (c) => (c.status === 'active' || c.status === 'expired') && c.until ? [fmtDate(c.until), daysFromNow(c.until) >= 0 ? daysFromNow(c.until) + ' hari lagi' : 'lewat ' + Math.abs(daysFromNow(c.until)) + ' hari', daysFromNow(c.until) <= 7] : c.status === 'trial' ? [fmtDate(c.trialUntil), daysFromNow(c.trialUntil) + ' hari lagi', daysFromNow(c.trialUntil) <= 7] : ['—', '', false];
  const exportCsv = () => downloadCsv('customer.csv', [['Bisnis', 'Pemilik', 'Kota', 'Status', 'Sales', 'Berlaku s/d'], ...rows.map((c) => [c.name, c.owner, c.city, statusInfo(c)[0], c.sales, c.until ? fmtDate(c.until) : ''])]);

  return (
    <Page title="Customer" sub="Semua bisnis yang memakai Convera">
      <div className="grid g4">
        <Kpi label="Berlangganan" value={C.filter(has.active).length} />
        <Kpi label="Trial" value={C.filter(has.trial).length} />
        <Kpi label="Tidak berlangganan" value={C.filter(has.expired).length} />
        <Kpi label="Online sekarang" value={C.filter(has.online).length} />
      </div>
      <div className="row between">
        <div className="field" style={{ flex: '1 1 320px', maxWidth: 420 }}><Icon name="search" size={18} /><input className="input bare" placeholder="Cari bisnis, pemilik, atau kota…" value={q} onChange={(e) => setQ(e.target.value)} /></div>
        <button className="btn soft" onClick={exportCsv}><Icon name="download" size={18} />Ekspor CSV</button>
      </div>
      <div className="chips">{FILTERS.map(([k, l]) => <button key={k} className={'chip' + (f === k ? ' on' : '')} onClick={() => setF(k)}>{l}<b>{C.filter(has[k]).length}</b></button>)}</div>
      <div className="tablewrap">
        <table>
          <thead><tr><th>Bisnis</th><th>Paket</th><th>Langganan</th><th>Aktivitas</th><th>Sales</th><th>Berlaku s/d</th></tr></thead>
          <tbody>
            {rows.map((c, i) => { const [label, tone] = statusInfo(c); const [d1, d2, warn] = until(c); return (
              <tr key={c.id} className="click" onClick={() => setSel(c.id)}>
                <td><div className="row nowrap"><Av name={c.name} i={i} /><div><b style={{ fontWeight: 600 }}>{c.name}</b><div className="muted sm">{c.owner} · {c.city}</div></div></div></td>
                <td>{planName(c)}</td>
                <td><Pill tone={tone}>{label}</Pill></td>
                <td><span className="row nowrap"><span className={'dot' + (c.online !== null ? ' on' : '')} /><span style={{ color: c.online !== null ? 'var(--accentText)' : 'var(--muted)', fontWeight: c.online !== null ? 500 : 400 }}>{lastSeen(c)}</span></span></td>
                <td>{c.sales || '—'}</td>
                <td>{d1}{d2 ? <div className="sm" style={{ color: warn ? 'var(--accentText)' : 'var(--muted)' }}>{d2}</div> : null}</td>
              </tr>
            ); })}
            {rows.length === 0 ? <tr><td colSpan="6" className="muted">Tidak ada customer.</td></tr> : null}
          </tbody>
        </table>
      </div>
      {detail ? (
        <Modal title={detail.name} wide onClose={() => setSel(null)}>
          <div className="row"><Pill tone={statusInfo(detail)[1]}>{statusInfo(detail)[0]}</Pill><span className="muted">{detail.owner} · {detail.phone} · {detail.city}</span></div>
          <div className="grid g3" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
            <div className="card"><span className="caps">Sales</span><b style={{ fontWeight: 500 }}>{detail.sales || '—'}</b></div>
            <div className="card"><span className="caps">Berlaku s/d</span><b style={{ fontWeight: 500 }}>{until(detail)[0]}</b></div>
            <div className="card"><span className="caps">Terakhir aktif</span><b style={{ fontWeight: 500 }}>{lastSeen(detail)}</b></div>
          </div>
          <h2>Riwayat pembayaran</h2>
          <div className="tablewrap">
            <table>
              <thead><tr><th>Tanggal</th><th>Periode</th><th className="right">Nominal</th><th>Sales</th></tr></thead>
              <tbody>
                {s.payments.filter((p) => p.customerId === detail.id).sort((a, b) => new Date(b.date) - new Date(a.date)).map((p) => (
                  <tr key={p.id}><td>{fmtShort(p.date)}</td><td>{p.months} bulan</td><td className="right">{rp(p.amount)}</td><td>{p.sales}</td></tr>
                ))}
                {s.payments.every((p) => p.customerId !== detail.id) ? <tr><td colSpan="4" className="muted">Belum ada pembayaran.</td></tr> : null}
              </tbody>
            </table>
          </div>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            {detail.status === 'active' ? <button className="btn soft" onClick={() => update((x) => stopSubscription(x, detail.id))}>Hentikan langganan</button> : null}
            <button className="btn primary" onClick={() => setExt(detail)}>{detail.status === 'active' ? 'Perpanjang manual' : 'Aktifkan manual'}</button>
          </div>
        </Modal>
      ) : null}
      {ext ? <ActivateModal customer={ext} onClose={() => setExt(null)} /> : null}
    </Page>
  );
}
