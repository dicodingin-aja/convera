import React, { useState } from 'react';
import { Av, ErrorNote, Kpi, Loading, Modal, Page, Pill, statusInfo, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { api, download, qs } from '../api.js';
import { daysFromNow, fmtDate, fmtShort, lastSeenText, rp } from '../calc.js';
import { useAction, useData, useDebounced } from '../hooks.js';
import ActivateModal from '../ActivateModal.jsx';
import EventDetail, { statusOf as eventStatus } from '../EventDetail.jsx';
import { EventsPanel, ResetPasswordModal, SupportAccessModal } from '../CustomerSupport.jsx';

const FILTERS = [['all', 'Semua'], ['online', 'Online'], ['offline', 'Offline'], ['active', 'Berlangganan'], ['expired', 'Tidak berlangganan'], ['trial', 'Trial'], ['pending', 'Menunggu']];

// Berlaku s/d: [tanggal, keterangan sisa hari, peringatan]
const untilInfo = (c) => {
  if ((c.status === 'active' || c.status === 'expired' || c.status === 'stopped') && c.until) {
    const d = daysFromNow(c.until);
    return [fmtDate(c.until), d >= 0 ? d + ' hari lagi' : 'lewat ' + Math.abs(d) + ' hari', d <= 7];
  }
  if (c.status === 'trial' && c.trial_until) return ['Trial s/d ' + fmtDate(c.trial_until), daysFromNow(c.trial_until) + ' hari lagi', daysFromNow(c.trial_until) <= 3];
  return ['—', '', false];
};

function Detail({ id, onClose, onChanged }) {
  const { me } = useApp();
  const q = useData('/customers/' + id);
  const evq = useData(`/customers/${id}/events?per_page=6`);
  const [ext, setExt] = useState(false);
  const [evSel, setEvSel] = useState(null);
  const [showAll, setShowAll] = useState(false);
  const [support, setSupport] = useState(false);
  const [reset, setReset] = useState(false);
  const { busy, error, run } = useAction();
  if (q.loading || q.error || !q.data) return <Modal title="Customer" wide onClose={onClose}><Loading error={q.error} /></Modal>;
  const d = q.data.data;
  const [label, tone] = statusInfo(d);
  const stop = async () => {
    if (!window.confirm(`Hentikan langganan ${d.name}? Aksi ini dicatat di audit log.`)) return;
    if (await run(() => api.post(`/customers/${id}/stop`))) { q.reload(); onChanged(); }
  };
  const canStop = d.status === 'active' || d.status === 'trial';
  return (
    <>
      <Modal title={d.name} wide onClose={onClose}>
        <div className="row"><Pill tone={tone}>{label}</Pill><span className="muted">{d.owner} · {d.phone || 'tanpa telepon'} · {d.city || '—'}</span></div>
        <div className="grid g3" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
          <div className="card"><span className="caps">Paket</span><b style={{ fontWeight: 500 }}>{d.plan_name || '—'}</b></div>
          <div className="card"><span className="caps">Sales</span><b style={{ fontWeight: 500 }}>{d.sales || '—'}</b></div>
          <div className="card"><span className="caps">Berlaku s/d</span><b style={{ fontWeight: 500 }}>{untilInfo(d)[0]}</b></div>
          <div className="card"><span className="caps">Terakhir aktif</span><b style={{ fontWeight: 500 }}>{lastSeenText(d)}</b></div>
          <div className="card"><span className="caps">Koneksi Meta</span><b style={{ fontWeight: 500 }}>{d.connection ? `${d.connection.mode.toUpperCase()} · ${d.connection.has_token ? d.connection.status : 'belum diisi'}` : '—'}</b></div>
          <div className="card"><span className="caps">Lead · event terkirim · gagal</span><b style={{ fontWeight: 500 }}>{d.stats.leads} · {d.stats.events_sent} · {d.stats.events_failed}</b></div>
        </div>
        <h2>Bantuan: kondisi event</h2>
        <div className="grid g3" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))' }}>
          <div className="card"><span className="caps">Gagal 24 jam</span><b style={{ fontWeight: 500, color: d.diagnostics.failed_24h > 0 ? 'var(--accentText)' : undefined }}>{d.diagnostics.failed_24h}</b></div>
          <div className="card"><span className="caps">Antre / menunggu nilai</span><b style={{ fontWeight: 500 }}>{d.diagnostics.queued} / {d.diagnostics.pending_value}</b></div>
          <div className="card"><span className="caps">Event terakhir</span><b style={{ fontWeight: 500 }}>{d.diagnostics.last_event_at ? fmtDate(d.diagnostics.last_event_at) : '—'}</b></div>
          <div className="card"><span className="caps">Chat terakhir masuk</span><b style={{ fontWeight: 500 }}>{d.diagnostics.last_message_at ? fmtDate(d.diagnostics.last_message_at) : '—'}</b></div>
        </div>
        {d.diagnostics.top_errors.length ? (
          <div className="card" style={{ gap: 8 }}>
            <span className="caps">Error paling sering (7 hari)</span>
            {d.diagnostics.top_errors.map((t, i) => (
              <div key={i}><b style={{ fontWeight: 500 }}>{t.total}x</b> {t.message || t.code}{t.hint ? <div className="muted sm">{t.hint}</div> : null}</div>
            ))}
          </div>
        ) : null}
        {showAll ? <EventsPanel workspaceId={id} onOpen={setEvSel} onBack={() => setShowAll(false)} /> : (
          <>
        <div className="row between"><h2>Event terbaru</h2><button className="btn s" onClick={() => setShowAll(true)}>Lihat semua event</button></div>
        <div className="tablewrap">
          <table>
            <thead><tr><th>Waktu</th><th>Event</th><th>Lead</th><th>Status</th></tr></thead>
            <tbody>
              {(evq.data?.data || []).map((e) => { const [label, tone] = eventStatus(e.status); return (
                <tr key={e.id} className="click" onClick={() => setEvSel(e.id)}><td className="sm">{fmtShort(e.created_at)}</td><td>{e.event_name}</td><td>{e.lead_name || '—'}</td><td><Pill tone={tone}>{label}</Pill></td></tr>
              ); })}
              {evq.loading ? <tr><td colSpan="4" className="muted">Memuat…</td></tr> : null}
              {!evq.loading && (evq.data?.data || []).length === 0 ? <tr><td colSpan="4" className="muted">Belum ada event.</td></tr> : null}
            </tbody>
          </table>
        </div>
          </>
        )}
        <h2>Riwayat pembayaran</h2>
        <div className="tablewrap">
          <table>
            <thead><tr><th>Tanggal</th><th>Periode</th><th>Paket</th><th className="right">Nominal</th><th>Sales</th></tr></thead>
            <tbody>
              {d.payments.map((p) => (
                <tr key={p.id}><td>{fmtShort(p.paid_at)}</td><td>{p.months} bulan · s/d {fmtShort(p.period_end)}</td><td>{p.plan_name}</td><td className="right">{rp(p.amount)}</td><td>{p.sales}</td></tr>
              ))}
              {d.payments.length === 0 ? <tr><td colSpan="5" className="muted">Belum ada pembayaran.</td></tr> : null}
            </tbody>
          </table>
        </div>
        <ErrorNote>{error}</ErrorNote>
        <div className="row" style={{ justifyContent: 'flex-end' }}>
          <button className="btn" disabled={me.role !== 'superadmin'} title={me.role !== 'superadmin' ? 'Hanya superadmin yang bisa masuk ke dashboard customer' : 'Buka dashboard customer untuk membantu'} onClick={() => setSupport(true)}><Icon name="eye" size={18} />Masuk ke dashboard</button>
          <button className="btn" disabled={me.role !== 'superadmin'} title={me.role !== 'superadmin' ? 'Hanya superadmin yang bisa reset password customer' : 'Buat password sementara untuk customer'} onClick={() => setReset(true)}><Icon name="lock" size={18} />Reset password</button>
          {canStop ? <button className="btn soft" disabled={busy} onClick={stop}>Hentikan langganan</button> : null}
          <button className="btn primary" onClick={() => setExt(true)}>{d.status === 'active' ? 'Perpanjang manual' : 'Aktifkan manual'}</button>
        </div>
      </Modal>
      {ext ? <ActivateModal customer={d} onClose={() => setExt(false)} onDone={() => { q.reload(); onChanged(); }} /> : null}
      {reset ? <ResetPasswordModal customer={{ id, name: d.name, owner: d.owner }} onClose={() => setReset(false)} /> : null}
      {support ? <SupportAccessModal customer={{ id, name: d.name }} onClose={() => setSupport(false)} /> : null}
      {evSel ? <EventDetail id={evSel} onClose={() => setEvSel(null)} onChanged={() => { evq.reload(); q.reload(); }} /> : null}
    </>
  );
}

export default function Customer() {
  const [f, setF] = useState('all');
  const [q, setQ] = useState('');
  const [page, setPage] = useState(1);
  const [sel, setSel] = useState(null);
  const dq = useDebounced(q);
  const query = qs({ filter: f === 'all' ? '' : f, q: dq.trim(), page, per_page: 20 });
  const list = useData('/customers' + query);

  const counts = list.data?.meta.counts;
  const rows = list.data?.data || [];
  const total = list.data?.meta.total || 0;
  const pages = Math.max(1, Math.ceil(total / 20));
  const change = (fn) => (v) => { fn(v); setPage(1); };
  const [exportErr, setExportErr] = useState('');

  return (
    <Page title="Customer" sub="Semua bisnis yang memakai Convera">
      <div className="grid g4">
        <Kpi label="Berlangganan" value={counts?.active ?? '—'} />
        <Kpi label="Trial" value={counts?.trial ?? '—'} />
        <Kpi label="Tidak berlangganan" value={counts?.expired ?? '—'} />
        <Kpi label="Online sekarang" value={counts?.online ?? '—'} />
      </div>
      <div className="row between">
        <div className="field" style={{ flex: '1 1 320px', maxWidth: 420 }}><Icon name="search" size={18} /><input className="input bare" placeholder="Cari bisnis, pemilik, kota, username, atau email…" value={q} onChange={(e) => change(setQ)(e.target.value)} /></div>
        <button className="btn soft" onClick={() => download('/customers.csv' + qs({ filter: f === 'all' ? '' : f, q: dq.trim() }), 'customer.csv').then(() => setExportErr(''), (e) => setExportErr(e.message))}><Icon name="download" size={18} />Ekspor CSV</button>
      </div>
      <ErrorNote>{exportErr}</ErrorNote>
      <div className="chips">{FILTERS.map(([k, l]) => <button key={k} className={'chip' + (f === k ? ' on' : '')} onClick={() => change(setF)(k)}>{l}<b>{counts?.[k] ?? '…'}</b></button>)}</div>
      {list.error ? <Loading error={list.error} /> : (
        <div className="tablewrap">
          <table>
            <thead><tr><th>Bisnis</th><th>Paket</th><th>Langganan</th><th>Aktivitas</th><th>Sales</th><th>Berlaku s/d</th></tr></thead>
            <tbody>
              {rows.map((c, i) => { const [label, tone] = statusInfo(c); const [d1, d2, warn] = untilInfo(c); return (
                <tr key={c.id} className="click" onClick={() => setSel(c.id)}>
                  <td><div className="row nowrap"><Av name={c.name} i={i} /><div><b style={{ fontWeight: 600 }}>{c.name}</b><div className="muted sm">{c.owner} · {c.city || '—'}</div></div></div></td>
                  <td>{c.plan_name || '—'}</td>
                  <td><Pill tone={tone}>{label}</Pill></td>
                  <td><span className="row nowrap"><span className={'dot' + (c.online ? ' on' : '')} /><span style={{ color: c.online ? 'var(--accentText)' : 'var(--muted)', fontWeight: c.online ? 500 : 400 }}>{lastSeenText(c)}</span></span></td>
                  <td>{c.sales || '—'}</td>
                  <td>{d1}{d2 ? <div className="sm" style={{ color: warn ? 'var(--accentText)' : 'var(--muted)' }}>{d2}</div> : null}</td>
                </tr>
              ); })}
              {list.loading && rows.length === 0 ? <tr><td colSpan="6" className="muted">Memuat…</td></tr> : null}
              {!list.loading && rows.length === 0 ? <tr><td colSpan="6" className="muted">Tidak ada customer.</td></tr> : null}
            </tbody>
          </table>
        </div>
      )}
      {pages > 1 ? (
        <div className="row between">
          <span className="muted sm">{total} customer · halaman {page} dari {pages}</span>
          <span className="row"><button className="btn s" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button><button className="btn s" disabled={page >= pages} onClick={() => setPage(page + 1)}>Berikutnya</button></span>
        </div>
      ) : null}
      {sel ? <Detail id={sel} onClose={() => setSel(null)} onChanged={list.reload} /> : null}
    </Page>
  );
}
