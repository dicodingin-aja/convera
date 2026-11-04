import React, { useState } from 'react';
import { Modal, Page, Pill, Tabs, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { rp } from '../calc.js';
import { rejectProof } from '../store.js';
import ActivateModal from '../ActivateModal.jsx';

const REASONS = ['Nominal tidak sesuai', 'Bukti transfer tidak terbaca', 'Transfer belum masuk ke rekening', 'Bukti transfer ganda'];
const KIND = { new: ['Daftar baru', 'accent'], renew: ['Perpanjang', 'soft'], trial: ['Lanjut dari trial', 'soft'] };
const wib = (d) => new Date(d).toLocaleString('id-ID', { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }).replace(/\./g, ':').replace(',', ',') + ' WIB';

export default function Verifikasi() {
  const { s, update } = useApp();
  const [tab, setTab] = useState('pending');
  const [act, setAct] = useState(null);
  const [rej, setRej] = useState(null);
  const [reason, setReason] = useState(REASONS[0]);
  const pendingCount = s.proofs.filter((p) => p.status === 'pending').length;
  const list = s.proofs.filter((p) => (tab === 'pending' ? p.status === 'pending' : p.status !== 'pending'));
  const cust = (id) => s.customers.find((c) => c.id === id);

  return (
    <Page title="Verifikasi Pembayaran" sub="Bukti transfer dari customer yang perlu dicek & diaktifkan">
      <div className="row" style={{ gap: 16 }}>
        <Tabs value={tab} onChange={setTab} items={[['pending', `Menunggu (${pendingCount})`], ['history', 'Riwayat']]} />
        <span className="muted">Cocokkan nominal & nama pengirim dengan mutasi rekening sebelum mengaktifkan.</span>
      </div>
      {list.length === 0 ? <div className="card muted">Tidak ada data.</div> : null}
      {list.map((p) => {
        const c = cust(p.customerId); const diff = p.received - p.billed; const [kl, kt] = KIND[p.kind];
        return (
          <div className="card proof" key={p.id}>
            <div className="receipt">
              <div className="paper">
                <div className="row between nowrap"><b style={{ fontWeight: 600, fontSize: 16 }}>{p.bank}</b><Pill tone="ink"><span style={{ fontSize: 11, letterSpacing: '.06em' }}>BERHASIL</span></Pill></div>
                <span className="amt">{rp(p.received)}</span>
                <span className="sm muted">Dari: <b style={{ color: 'var(--text)', fontWeight: 500 }}>{p.sender.toUpperCase()}</b></span>
                <span className="sm muted">Ke: {p.toBank}</span>
                <span className="mono muted">{p.ref}</span>
                <span className="sm muted">{wib(p.uploadedAt)}</span>
              </div>
              <span className="row nowrap muted sm" style={{ justifyContent: 'center' }}><Icon name="image" size={16} />bukti_transfer.jpg · 412 KB</span>
            </div>
            <div style={{ flex: 1, minWidth: 280, display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div className="row between nowrap" style={{ alignItems: 'flex-start' }}>
                <div><h2 style={{ fontSize: 18 }}>{c.name}</h2><div className="muted">{c.owner} · {c.phone} · {c.city}</div></div>
                {p.status === 'pending' ? <Pill tone={kt}>{kl}</Pill> : p.status === 'approved' ? <Pill tone="ink">Diaktifkan</Pill> : <Pill>Ditolak</Pill>}
              </div>
              <div className="facts">
                <div><span className="caps">Paket</span><b>{p.plan || s.plans[0].name} · {p.months} bulan</b></div>
                <div><span className="caps">Tagihan</span><b>{rp(p.billed)}</b></div>
                <div><span className="caps">Diterima</span><b>{rp(p.received)}</b></div>
              </div>
              {diff !== 0 && p.status === 'pending' ? <div className="warnbox"><Icon name="alert" size={18} />Transfer {diff > 0 ? 'lebih' : 'kurang'} {rp(Math.abs(diff))} dari tagihan — kemungkinan kode unik / pembulatan.</div> : null}
              {p.message ? <div>“{p.message}”</div> : null}
              {p.status === 'rejected' ? <div className="muted">Alasan: {p.reason}</div> : null}
              {p.status === 'pending' ? (
                <div className="row between" style={{ marginTop: 'auto' }}>
                  <span className="row">
                    <button className="btn ghost" onClick={() => { setReason(REASONS[0]); setRej(p); }}>Tolak</button>
                    <button className="btn"><Icon name="chat" size={18} />Chat customer</button>
                  </span>
                  <button className="btn primary" onClick={() => setAct(p)}><Icon name="check" size={18} />Aktifkan akun</button>
                </div>
              ) : null}
            </div>
          </div>
        );
      })}
      {act ? <ActivateModal customer={cust(act.customerId)} proof={act} onClose={() => setAct(null)} /> : null}
      {rej ? (
        <Modal title="Tolak bukti transfer" onClose={() => setRej(null)}>
          <div className="muted">{cust(rej.customerId).name}</div>
          <select className="select" value={reason} onChange={(e) => setReason(e.target.value)}>{REASONS.map((r) => <option key={r}>{r}</option>)}</select>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn soft" onClick={() => setRej(null)}>Batal</button>
            <button className="btn ink" onClick={() => { update((x) => rejectProof(x, rej.id, reason)); setRej(null); }}>Tolak</button>
          </div>
        </Modal>
      ) : null}
    </Page>
  );
}
