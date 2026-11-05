import React, { useState } from 'react';
import { ErrorNote, Loading, Modal, Pill } from './ui.jsx';
import { api } from './api.js';
import { fmtDateTime, rp } from './calc.js';
import { useAction, useData } from './hooks.js';

export const EVENT_STATUS = {
  sent: ['Terkirim', 'ink'], failed: ['Gagal', 'accent'], queued: ['Antre', 'soft'], pending_value: ['Menunggu nilai', 'soft'], skipped: ['Dilewati', ''],
};
export const SOURCE = { rule: 'Aturan', manual: 'Manual', api: 'API', test: 'Uji koneksi' };
export const statusOf = (s) => EVENT_STATUS[s] || [s, ''];

const pretty = (v) => { try { return JSON.stringify(v, null, 2); } catch { return String(v); } };
const box = { margin: 0, padding: 12, borderRadius: 10, background: 'var(--surface)', fontSize: 12, overflow: 'auto', maxHeight: 220, whiteSpace: 'pre-wrap', wordBreak: 'break-word' };

// Detail satu event untuk dukungan: status, error + saran perbaikan, payload yang dikirim, dan respons Meta.
// Payload dan respons tidak memuat token; nomor HP tersamar. Pembukaan detail dicatat di audit log.
export default function EventDetail({ id, onClose, onChanged }) {
  const q = useData('/events/' + id);
  const { busy, error, run } = useAction();
  const [showRaw, setShowRaw] = useState(false);
  if (q.loading || q.error || !q.data) return <Modal title="Detail event" wide onClose={onClose}><Loading error={q.error} /></Modal>;
  const e = q.data.data;
  const [label, tone] = statusOf(e.status);
  const retry = async () => { if (await run(() => api.post(`/events/${id}/retry`))) { q.reload(); onChanged?.(); } };
  return (
    <Modal title={`${e.event_name} · ${e.customer}`} wide onClose={onClose}>
      <div className="row"><Pill tone={tone}>{label}</Pill><span className="muted">{fmtDateTime(e.created_at)} · {SOURCE[e.source] || e.source} · mode {String(e.mode || '—').toUpperCase()}</span></div>
      <div className="grid g3" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
        <div className="card"><span className="caps">Customer</span><b style={{ fontWeight: 500 }}>{e.customer}</b><span className="muted sm">{e.owner}</span></div>
        <div className="card"><span className="caps">Lead</span><b style={{ fontWeight: 500 }}>{e.lead_name || '—'}</b><span className="muted sm">{e.phone_masked || 'tanpa lead'}{e.lead_from_ad ? ' · dari iklan' : e.phone_masked ? ' · organik' : ''}</span></div>
        <div className="card"><span className="caps">Nilai</span><b style={{ fontWeight: 500 }}>{e.value != null ? rp(e.value) : '—'}</b><span className="muted sm">{e.rule_name ? 'Aturan: ' + e.rule_name : ''}</span></div>
        <div className="card"><span className="caps">Percobaan</span><b style={{ fontWeight: 500 }}>{e.attempts}</b><span className="muted sm">{e.next_retry_at && e.status === 'queued' ? 'berikutnya ' + fmtDateTime(e.next_retry_at) : e.sent_at ? 'terkirim ' + fmtDateTime(e.sent_at) : ''}</span></div>
      </div>

      {e.error_code || e.error_message ? (
        <div className="warnbox" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: 8 }}>
          <b style={{ fontWeight: 600 }}>Error {e.error_code ? `(${e.error_code})` : ''}</b>
          <span>{e.error_message}</span>
          {e.fbtrace_id ? <span className="mono">fbtrace_id: {e.fbtrace_id}</span> : null}
        </div>
      ) : null}
      {e.hint ? <div className="card" style={{ gap: 6 }}><span className="caps">Saran untuk tim dukungan</span><span>{e.hint}</span></div> : null}
      {e.lead_status === 'purchase' && e.status === 'failed' ? <span className="muted sm">Lead sudah berstatus purchase tetapi event ini gagal terkirim ke Meta.</span> : null}

      <div className="row between">
        <button className="btn s" onClick={() => setShowRaw((v) => !v)}>{showRaw ? 'Sembunyikan' : 'Lihat'} payload & respons Meta</button>
        {e.status === 'failed' && e.source !== 'test' ? <button className="btn primary" disabled={busy} onClick={retry}>{busy ? 'Memproses…' : 'Kirim ulang'}</button> : null}
      </div>
      {showRaw ? (
        <>
          <span className="caps">Payload dikirim ke Meta (tanpa token; data pribadi sudah di-hash)</span>
          <pre className="mono" style={box}>{e.request_payload ? pretty(e.request_payload) : 'Belum pernah dikirim.'}</pre>
          <span className="caps">Respons Meta</span>
          <pre className="mono" style={box}>{e.meta_response ? pretty(e.meta_response) : '—'}</pre>
          <span className="muted sm mono">event_id: {e.event_id}</span>
        </>
      ) : null}
      <ErrorNote>{error}</ErrorNote>
      <div className="row" style={{ justifyContent: 'flex-end' }}><button className="btn soft" onClick={onClose}>Tutup</button></div>
    </Modal>
  );
}
