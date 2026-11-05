import React, { useEffect, useState } from 'react';
import { ErrorNote, Loading, Modal, Page, Pill, Tabs, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { api, fetchBlob } from '../api.js';
import { fmtDateTime, rp } from '../calc.js';
import { useAction, useData } from '../hooks.js';
import ActivateModal from '../ActivateModal.jsx';

const REASONS = ['Nominal tidak sesuai', 'Bukti transfer tidak terbaca', 'Transfer belum masuk ke rekening', 'Bukti transfer ganda'];
const KIND = { new: ['Daftar baru', 'accent'], renew: ['Perpanjang', 'soft'], trial: ['Lanjut dari trial', 'soft'] };
const kb = (n) => (n >= 1024 * 1024 ? (n / 1024 / 1024).toFixed(1) + ' MB' : Math.max(1, Math.round(n / 1024)) + ' KB');
const waLink = (phone) => (phone ? 'https://wa.me/' + String(phone).replace(/\D/g, '') : null);

// Menampilkan berkas bukti transfer dari penyimpanan privat. Gambar tampil langsung; PDF dibuka di tab baru.
function ProofViewer({ proof, onClose }) {
  const [state, setState] = useState({ url: '', error: '', loading: true });
  useEffect(() => {
    let alive = true; let url = '';
    fetchBlob(`/payment-proofs/${proof.id}/file`)
      .then((b) => { url = URL.createObjectURL(b); if (alive) setState({ url, error: '', loading: false }); })
      .catch((e) => { if (alive) setState({ url: '', error: e.message, loading: false }); });
    return () => { alive = false; if (url) URL.revokeObjectURL(url); };
  }, [proof.id]);
  const isPdf = proof.file_mime === 'application/pdf';
  return (
    <Modal title={`Bukti transfer · ${proof.customer}`} wide onClose={onClose}>
      <div className="muted">{proof.bank} · {String(proof.sender).toUpperCase()} · {rp(proof.received ?? proof.billed)}</div>
      {state.loading ? <Loading /> : state.error ? <Loading error={state.error} /> : isPdf ? (
        <div className="card"><span>Berkas PDF tidak ditampilkan di sini.</span><a className="btn primary" href={state.url} target="_blank" rel="noopener noreferrer">Buka PDF di tab baru</a></div>
      ) : (
        <img src={state.url} alt={'Bukti transfer ' + proof.customer} style={{ maxWidth: '100%', maxHeight: '70vh', objectFit: 'contain', borderRadius: 12, alignSelf: 'center' }} />
      )}
      <div className="row" style={{ justifyContent: 'flex-end' }}><button className="btn soft" onClick={onClose}>Tutup</button></div>
    </Modal>
  );
}

export default function Verifikasi() {
  const { refreshPending } = useApp();
  const [tab, setTab] = useState('pending');
  const [act, setAct] = useState(null);
  const [rej, setRej] = useState(null);
  const [view, setView] = useState(null);
  const [reason, setReason] = useState(REASONS[0]);
  const q = useData(tab === 'pending' ? '/payment-proofs?status=pending&per_page=100' : '/payment-proofs?status=all&per_page=100');
  const { busy, error, setError, run } = useAction();

  const all = q.data?.data || [];
  const list = tab === 'pending' ? all : all.filter((p) => p.status !== 'pending');
  const done = () => { q.reload(); refreshPending(); };
  const reject = async () => {
    if (await run(() => api.post(`/payment-proofs/${rej.id}/reject`, { reason }))) { setRej(null); done(); }
  };

  return (
    <Page title="Verifikasi Pembayaran" sub="Bukti transfer dari customer yang perlu dicek & diaktifkan">
      <div className="row" style={{ gap: 16 }}>
        <Tabs value={tab} onChange={setTab} items={[['pending', 'Menunggu'], ['history', 'Riwayat']]} />
        <span className="muted">Cocokkan nominal & nama pengirim dengan mutasi rekening sebelum mengaktifkan.</span>
      </div>
      {q.error ? <Loading error={q.error} /> : q.loading && !q.data ? <Loading /> : null}
      {!q.loading && !q.error && list.length === 0 ? <div className="card muted">{tab === 'pending' ? 'Tidak ada bukti transfer yang menunggu.' : 'Belum ada riwayat.'}</div> : null}
      {list.map((p) => {
        const diff = p.received - p.billed; const [kl, kt] = KIND[p.kind] || KIND.new;
        const wa = waLink(p.phone);
        return (
          <div className="card proof" key={p.id}>
            <div className="receipt">
              <div className="paper">
                <div className="row between nowrap"><b style={{ fontWeight: 600, fontSize: 16 }}>{p.bank}</b><Pill tone="ink"><span style={{ fontSize: 11, letterSpacing: '.06em' }}>BUKTI</span></Pill></div>
                <span className="amt">{rp(p.received ?? p.billed)}</span>
                <span className="sm muted">Dari: <b style={{ color: 'var(--text)', fontWeight: 500 }}>{String(p.sender).toUpperCase()}</b></span>
                <span className="sm muted">Ke: {p.to_bank || '—'}</span>
                <span className="mono muted">{p.ref || '—'}</span>
                <span className="sm muted">{fmtDateTime(p.uploaded_at)}</span>
              </div>
              <button className="btn s" style={{ justifyContent: 'center' }} onClick={() => setView(p)}><Icon name="image" size={16} />Lihat bukti · {p.file_mime.replace('image/', '').replace('application/', '').toUpperCase()} · {kb(p.file_size)}</button>
            </div>
            <div style={{ flex: 1, minWidth: 280, display: 'flex', flexDirection: 'column', gap: 14 }}>
              <div className="row between nowrap" style={{ alignItems: 'flex-start' }}>
                <div><h2 style={{ fontSize: 18 }}>{p.customer}</h2><div className="muted">{p.owner} · {p.phone || 'tanpa telepon'} · {p.city || '—'}</div></div>
                {p.status === 'pending' ? <Pill tone={kt}>{kl}</Pill> : p.status === 'approved' ? <Pill tone="ink">Diaktifkan</Pill> : <Pill>Ditolak</Pill>}
              </div>
              <div className="facts">
                <div><span className="caps">Paket</span><b>{p.plan_name || '—'} · {p.months} bulan</b></div>
                <div><span className="caps">Tagihan</span><b>{rp(p.billed)}</b></div>
                <div><span className="caps">Diterima</span><b>{rp(p.received ?? p.billed)}</b></div>
              </div>
              {p.amount_mismatch && p.status === 'pending' ? <div className="warnbox"><Icon name="alert" size={18} />Transfer {diff > 0 ? 'lebih' : 'kurang'} {rp(Math.abs(diff))} dari tagihan — kemungkinan kode unik / pembulatan.</div> : null}
              {p.message ? <div>“{p.message}”</div> : null}
              {p.status === 'rejected' ? <div className="muted">Alasan: {p.reject_reason}</div> : null}
              {p.status === 'pending' ? (
                <div className="row between" style={{ marginTop: 'auto' }}>
                  <span className="row">
                    <button className="btn ghost" onClick={() => { setReason(REASONS[0]); setError(''); setRej(p); }}>Tolak</button>
                    {wa ? <a className="btn" href={wa} target="_blank" rel="noopener noreferrer"><Icon name="chat" size={18} />Chat customer</a> : null}
                  </span>
                  <button className="btn primary" onClick={() => setAct(p)}><Icon name="check" size={18} />Aktifkan akun</button>
                </div>
              ) : null}
            </div>
          </div>
        );
      })}
      {view ? <ProofViewer proof={view} onClose={() => setView(null)} /> : null}
      {act ? <ActivateModal customer={{ id: act.workspace_id, name: act.customer, owner: act.owner, city: act.city, plan: act.plan }} proof={act} onClose={() => setAct(null)} onDone={done} /> : null}
      {rej ? (
        <Modal title="Tolak bukti transfer" onClose={() => setRej(null)}>
          <div className="muted">{rej.customer}</div>
          <select className="select" value={reason} onChange={(e) => setReason(e.target.value)}>{REASONS.map((r) => <option key={r}>{r}</option>)}</select>
          <ErrorNote>{error}</ErrorNote>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn soft" onClick={() => setRej(null)}>Batal</button>
            <button className="btn ink" disabled={busy} onClick={reject}>{busy ? 'Memproses…' : 'Tolak'}</button>
          </div>
        </Modal>
      ) : null}
    </Page>
  );
}
