import React, { useEffect, useState } from 'react';
import { ErrorNote, Loading, Modal, Page, Toggle, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { api } from '../api.js';
import { fmtDateTime } from '../calc.js';
import { useAction, useData } from '../hooks.js';

const FLAG_INFO = {
  qr: ['Scan QR WhatsApp', 'Customer bisa menghubungkan WhatsApp lewat QR.'],
  simulasi: ['Simulasi event', 'Coba aturan event dengan pesan contoh.'],
  laporan: ['Laporan terjadwal', 'Kirim laporan lewat email/WhatsApp.'],
  tiktok: ['Integrasi TikTok', 'Kirim konversi dari TikTok Ads.'],
  instagram: ['Integrasi Instagram', 'Tangkap chat dari Instagram DM.'],
};

const uptime = (s) => { const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60); return d ? `${d} hr ${h} jam` : h ? `${h} jam ${m} mnt` : `${m} mnt`; };
const num = (n) => Number(n || 0).toLocaleString('id-ID');
const pct = (v) => (v == null ? '—' : (v * 100).toFixed(1).replace('.', ',') + '%');

function Status() {
  const q = useData('/system-status');
  if (q.error || !q.data) return <Loading error={q.error} />;
  const d = q.data.data;
  const sent = d.events_24h.sent, failed = d.events_24h.failed;
  const stats = [
    ['API admin aktif', uptime(d.uptime_seconds), `Database merespons ${d.db_ping_ms} ms`, false],
    ['Event 24 jam', num(sent + failed + d.events_24h.skipped), `${num(sent)} terkirim · ${num(failed)} gagal · ${num(d.events_24h.skipped)} dilewati`, false],
    ['Antrean retry', num(d.queue.retrying), `${num(d.queue.queued)} antre · ${num(d.queue.pending_value)} menunggu nilai`, d.queue.retrying > 0],
    ['Error rate 24 jam', pct(d.error_rate_24h), `${num(d.other.webhook_errors_24h)} dari ${num(d.other.webhooks_24h)} webhook bermasalah`, (d.error_rate_24h || 0) > 0.05],
  ];
  return (
    <>
      <div className="grid g4">
        {stats.map(([l, v, sub, warn]) => (
          <div className="card kpi" key={l}><span className="row nowrap" style={{ gap: 8 }}><span className={'dot' + (warn ? ' on' : '')} style={warn ? {} : { background: 'var(--ink)' }} />{l}</span><span className="big">{v}</span><span className="muted sm">{sub}</span></div>
        ))}
      </div>
      {d.other.connections_need_attention > 0 ? <div className="warnbox"><Icon name="alert" size={18} />{d.other.connections_need_attention} koneksi Meta customer perlu perhatian (token tidak valid/kedaluwarsa).</div> : null}
      {d.queue.oldest_wait_seconds > 300 ? <div className="warnbox"><Icon name="alert" size={18} />Antrean terlama menunggu {Math.round(d.queue.oldest_wait_seconds / 60)} menit — pastikan worker berjalan.</div> : null}
    </>
  );
}

function Flags() {
  const q = useData('/feature-flags');
  const { error, run } = useAction();
  const set = (k, v) => run(async () => { await api.put('/feature-flags/' + k, { enabled: v }); q.reload(); });
  return (
    <div className="card" style={{ gap: 4 }}>
      <h2>Fitur untuk customer</h2><span className="muted" style={{ marginBottom: 8 }}>Nyalakan/matikan fitur di semua workspace.</span>
      {q.error ? <Loading error={q.error} /> : (q.data?.data || []).map((f) => {
        const [label, desc] = FLAG_INFO[f.key] || [f.key, f.description || ''];
        return (
          <div className="listrow nowrap" key={f.key}>
            <div style={{ flex: 1 }}><b style={{ fontWeight: 600 }}>{label}</b> {f.badge ? <span className="badge">{f.badge}</span> : null}<div className="muted sm">{desc}</div></div>
            <Toggle on={f.enabled} label={label} onChange={(v) => set(f.key, v)} />
          </div>
        );
      })}
      <ErrorNote>{error}</ErrorNote>
    </div>
  );
}

function Maintenance({ isSuper }) {
  const q = useData('/maintenance');
  const { busy, error, run } = useAction();
  const [msg, setMsg] = useState('');
  useEffect(() => { if (q.data) setMsg(q.data.data.message); }, [q.data]);
  if (q.error || !q.data) return <div className="card"><h2>Mode maintenance</h2><Loading error={q.error} /></div>;
  const on = q.data.data.enabled;
  const save = (body) => run(async () => { await api.put('/maintenance', body); q.reload(); });
  return (
    <div className="card">
      <div className="row between nowrap"><div><h2>Mode maintenance</h2><span className="muted sm">{on ? 'Aktif. Customer hanya bisa memantau (tidak bisa buat workspace/event baru) dan melihat pesan di bawah.' : 'Nonaktif. Menyalakan akan mengirim notifikasi ke semua customer.'}</span></div>
        <Toggle on={on} label="Mode maintenance" onChange={(v) => { if (isSuper) save({ enabled: v }); }} /></div>
      <input className="input" value={msg} maxLength={300} onChange={(e) => setMsg(e.target.value)} />
      <div className="row"><button className="btn s" disabled={busy || !isSuper || !msg.trim() || msg === q.data.data.message} onClick={() => save({ message: msg })}>Simpan pesan</button>{!isSuper ? <span className="muted sm">Hanya superadmin yang bisa mengubah.</span> : null}</div>
      <ErrorNote>{error}</ErrorNote>
    </div>
  );
}

function Keys({ isSuper }) {
  const q = useData(isSuper ? '/api-keys' : null);
  const { busy, error, run } = useAction();
  const [name, setName] = useState('');
  const [fresh, setFresh] = useState(null);
  const create = async () => { const r = await run(() => api.post('/api-keys', { name })); if (r) { setFresh(r.data); setName(''); q.reload(); } };
  const revoke = (k) => { if (window.confirm(`Cabut key ${k.name}? Tidak bisa dibatalkan.`)) run(async () => { await api.del('/api-keys/' + k.id); q.reload(); }); };
  return (
    <div className="card" style={{ gap: 4 }}>
      <div className="row between"><h2>Admin API key</h2></div>
      {!isSuper ? <span className="muted">Hanya superadmin yang bisa mengelola API key.</span> : (
        <>
          {q.error ? <Loading error={q.error} /> : (q.data?.data || []).map((k) => (
            <div className="listrow nowrap" key={k.id}>
              <div style={{ flex: 1 }}><b style={{ fontWeight: 600 }}>{k.name}</b><div className="mono muted">{k.prefix}••••••••</div></div>
              <span className="muted sm">{k.revoked_at ? 'Dicabut' : k.last_used_at ? fmtDateTime(k.last_used_at) : 'Belum dipakai'}</span>
              {!k.revoked_at ? <button className="btn ghost s" onClick={() => revoke(k)}>Cabut</button> : null}
            </div>
          ))}
          <div className="row" style={{ marginTop: 8 }}>
            <input className="input" style={{ flex: 1 }} placeholder="Nama key (mis. n8n otomasi)" value={name} onChange={(e) => setName(e.target.value)} />
            <button className="btn s" disabled={busy || !name.trim()} onClick={create}><Icon name="plus" size={15} />Buat key</button>
          </div>
          <span className="muted sm">Key baru belum dipakai sebagai autentikasi API admin; baru bisa dikelola di sini.</span>
          <ErrorNote>{error}</ErrorNote>
        </>
      )}
      {fresh ? (
        <Modal title="Simpan key ini sekarang" onClose={() => setFresh(null)}>
          <span className="muted">Key lengkap hanya ditampilkan sekali dan tidak bisa dilihat lagi.</span>
          <input className="input mono" readOnly value={fresh.key} onFocus={(e) => e.target.select()} />
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn soft" onClick={() => { try { navigator.clipboard.writeText(fresh.key); } catch { /* abaikan */ } }}>Salin</button>
            <button className="btn primary" onClick={() => setFresh(null)}>Sudah saya simpan</button>
          </div>
        </Modal>
      ) : null}
    </div>
  );
}

function Audit() {
  const [page, setPage] = useState(1);
  const q = useData('/audit-logs?per_page=15&page=' + page);
  const rows = q.data?.data || [];
  const pages = Math.max(1, Math.ceil((q.data?.meta.total || 0) / 15));
  return (
    <div className="card" style={{ gap: 4 }}>
      <div className="row between"><h2>Aktivitas admin</h2><span className="muted sm">{q.data?.meta.total ?? 0} catatan</span></div>
      {q.error ? <Loading error={q.error} /> : null}
      {rows.map((l) => (
        <div className="listrow" key={l.id}><span className="muted sm" style={{ width: 130, flex: 'none' }}>{fmtDateTime(l.at)}</span><b style={{ fontWeight: 500 }}>{l.actor_label || l.actor_type}</b><span>{l.action}{l.workspace ? ' · ' + l.workspace : ''}</span></div>
      ))}
      {!q.loading && !q.error && rows.length === 0 ? <span className="muted">Belum ada aktivitas.</span> : null}
      {pages > 1 ? (
        <div className="row between" style={{ marginTop: 8 }}>
          <span className="muted sm">Halaman {page} dari {pages}</span>
          <span className="row"><button className="btn s" disabled={page <= 1} onClick={() => setPage(page - 1)}>Sebelumnya</button><button className="btn s" disabled={page >= pages} onClick={() => setPage(page + 1)}>Berikutnya</button></span>
        </div>
      ) : null}
    </div>
  );
}

export default function Developer() {
  const { me } = useApp();
  const isSuper = me.role === 'superadmin';
  return (
    <Page title="Developer" sub="Status sistem, fitur, dan akses teknis">
      <Status />
      <div className="grid g2">
        <Flags />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Maintenance isSuper={isSuper} />
          <Keys isSuper={isSuper} />
        </div>
      </div>
      <Audit />
    </Page>
  );
}
