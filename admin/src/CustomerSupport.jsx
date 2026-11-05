import React, { useState } from 'react';
import { ErrorNote, Loading, Modal, Pill } from './ui.jsx';
import { api, qs } from './api.js';
import { fmtDateTime, rp } from './calc.js';
import { useAction, useData } from './hooks.js';
import { statusOf } from './EventDetail.jsx';

const FILTERS = [['', 'Semua'], ['failed', 'Gagal'], ['queued', 'Antre'], ['pending_value', 'Menunggu nilai'], ['sent', 'Terkirim'], ['skipped', 'Dilewati']];

// Semua event satu customer (di dalam detail customer): filter status + halaman. Klik baris membuka detail event.
export function EventsPanel({ workspaceId, onOpen, onBack }) {
  const [st, setSt] = useState('');
  const [page, setPage] = useState(1);
  const q = useData(`/customers/${workspaceId}/events` + qs({ status: st, page, per_page: 10 }));
  const rows = q.data?.data || [];
  const counts = q.data?.meta.counts;
  const total = q.data?.meta.total || 0;
  const pages = Math.max(1, Math.ceil(total / 10));
  return (
    <div className="card" style={{ gap: 12 }}>
      <div className="row between"><b style={{ fontWeight: 600 }}>Semua event (30 hari terakhir)</b><button className="btn s" onClick={onBack}>Kembali</button></div>
      <div className="chips">{FILTERS.map(([k, l]) => <button key={k || 'all'} className={'chip' + (st === k ? ' on' : '')} onClick={() => { setSt(k); setPage(1); }}>{l}<b>{counts ? (k === '' ? counts.all : counts[k]) : '…'}</b></button>)}</div>
      {q.error ? <Loading error={q.error} /> : (
        <div className="tablewrap">
          <table>
            <thead><tr><th>Waktu</th><th>Event</th><th>Lead</th><th className="right">Nilai</th><th>Status</th><th>Keterangan</th></tr></thead>
            <tbody>
              {rows.map((e) => { const [label, tone] = statusOf(e.status); return (
                <tr key={e.id} className="click" onClick={() => onOpen(e.id)}>
                  <td className="sm">{fmtDateTime(e.created_at)}</td><td>{e.event_name}</td><td>{e.lead_name || '—'}<div className="muted sm">{e.phone_masked || ''}</div></td>
                  <td className="right">{e.value != null ? rp(e.value) : '—'}</td><td><Pill tone={tone}>{label}</Pill></td>
                  <td className="sm" style={{ maxWidth: 240 }}>{e.error_message ? (e.error_message.length > 70 ? e.error_message.slice(0, 70) + '…' : e.error_message) : <span className="muted">—</span>}</td>
                </tr>
              ); })}
              {q.loading && rows.length === 0 ? <tr><td colSpan="6" className="muted">Memuat…</td></tr> : null}
              {!q.loading && rows.length === 0 ? <tr><td colSpan="6" className="muted">Tidak ada event.</td></tr> : null}
            </tbody>
          </table>
        </div>
      )}
      {pages > 1 ? (
        <div className="row between"><span className="muted sm">{total} event · halaman {page} dari {pages}</span>
          <span className="row"><button className="btn s" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button><button className="btn s" disabled={page >= pages} onClick={() => setPage(page + 1)}>Berikutnya</button></span></div>
      ) : null}
    </div>
  );
}

const REASONS = ['Bantu atur koneksi Meta/WhatsApp', 'Cek event yang gagal terkirim', 'Bantu atur aturan event', 'Permintaan customer via WhatsApp'];

// Membuat tautan sekali pakai untuk masuk ke dashboard customer. Alasan wajib dan dicatat; customer mendapat notifikasi.
export function SupportAccessModal({ customer, onClose }) {
  const [reason, setReason] = useState('');
  const [link, setLink] = useState(null);
  const { busy, error, run } = useAction();
  const create = async () => { const r = await run(() => api.post(`/customers/${customer.id}/support-access`, { reason })); if (r) setLink(r.data); };
  return (
    <Modal title={`Masuk ke dashboard ${customer.name}`} onClose={onClose}>
      {link ? (
        <>
          <div className="card" style={{ gap: 10 }}>
            <b style={{ fontWeight: 600 }}>Tautan siap</b>
            <span className="muted">Berlaku {link.expires_in} detik dan hanya bisa dipakai sekali. Setelah dibuka, sesi dukungan berlaku {link.session_minutes / 60} jam.</span>
            <a className="btn primary" href={link.url} target="_blank" rel="noopener noreferrer" onClick={() => setTimeout(onClose, 300)}>Buka dashboard customer</a>
          </div>
        </>
      ) : (
        <>
          <div className="warnbox" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <b style={{ fontWeight: 600 }}>Anda akan bertindak atas nama customer ini</b>
            <span>Semua tindakan Anda dicatat di audit log, dan customer menerima notifikasi bahwa tim dukungan membuka akunnya.</span>
            <span>Tidak bisa: mengubah password/profil, menghapus akun atau workspace, mengunggah bukti bayar, membuat API key. Hanya workspace ini yang terbuka.</span>
          </div>
          <label className="f">Alasan akses (wajib)
            <input className="input" maxLength={300} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="mis. Bantu atur koneksi Meta" />
          </label>
          <div className="chips">{REASONS.map((r) => <button key={r} type="button" className="chip" onClick={() => setReason(r)}>{r}</button>)}</div>
          <ErrorNote>{error}</ErrorNote>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn soft" onClick={onClose}>Batal</button>
            <button className="btn primary" disabled={busy || reason.trim().length < 3} onClick={create}>{busy ? 'Memproses…' : 'Buat tautan akses'}</button>
          </div>
        </>
      )}
    </Modal>
  );
}

// Reset password customer (tidak ada reset lewat email). Password sementara hanya tampil sekali; semua sesi customer dicabut.
export function ResetPasswordModal({ customer, onClose }) {
  const [res, setRes] = useState(null);
  const [copied, setCopied] = useState(false);
  const { busy, error, run } = useAction();
  const create = async () => { const r = await run(() => api.post(`/customers/${customer.id}/reset-password`, {})); if (r) setRes(r.data); };
  const copy = async () => { try { await navigator.clipboard.writeText(res.temporary_password); setCopied(true); } catch { /* salin manual */ } };
  return (
    <Modal title={`Reset password ${customer.name}`} onClose={onClose}>
      {res ? (
        <div className="card" style={{ gap: 10 }}>
          <b style={{ fontWeight: 600 }}>Password sementara</b>
          <span className="muted">Username: <b>{res.username}</b>. Password ini hanya tampil sekali, kirim ke customer lewat kanal yang aman dan minta mereka menggantinya di Profil.</span>
          <code style={{ fontSize: 18, padding: '10px 12px', borderRadius: 8, background: 'var(--surface2, #f3f3f3)', userSelect: 'all' }}>{res.temporary_password}</code>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn" onClick={copy}>{copied ? 'Tersalin' : 'Salin'}</button>
            <button className="btn primary" onClick={onClose}>Selesai</button>
          </div>
        </div>
      ) : (
        <>
          <div className="warnbox" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 6 }}>
            <b style={{ fontWeight: 600 }}>Password lama {customer.owner || 'customer'} akan diganti</b>
            <span>Semua sesi login customer langsung keluar. Tindakan ini dicatat di audit log.</span>
          </div>
          <ErrorNote>{error}</ErrorNote>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn soft" onClick={onClose}>Batal</button>
            <button className="btn primary" disabled={busy} onClick={create}>{busy ? 'Memproses…' : 'Buat password sementara'}</button>
          </div>
        </>
      )}
    </Modal>
  );
}
