import React from 'react';
import { Page, Toggle, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { addLog } from '../store.js';

// Angka status sistem masih contoh sampai backend tersedia.
const STATS = [['API uptime 30 hari', '99,98%', 'Gangguan terakhir 12 Okt, 4 menit', false], ['Event diproses 24 jam', '48.210', 'Rata-rata 33 event / menit', false], ['Antrean retry', '12', 'Semua di bawah 5 percobaan', true], ['Error rate', '0,4%', 'Mayoritas nomor tidak valid', false]];
const FLAGS = [['qr', 'Scan QR WhatsApp', 'Customer bisa menghubungkan WhatsApp lewat QR.', 'Beta'], ['simulasi', 'Simulasi event', 'Coba aturan event dengan pesan contoh.', ''], ['laporan', 'Laporan terjadwal', 'Kirim laporan lewat email/WhatsApp.', ''], ['tiktok', 'Integrasi TikTok', 'Kirim konversi ke TikTok Events API.', 'Segera'], ['instagram', 'Instagram DM', 'Lacak chat dari iklan Click-to-Instagram.', 'Segera']];

export default function Developer() {
  const { s, update, reset } = useApp();
  const addKey = () => update((x) => addLog({ ...x, keys: [...x.keys, { id: 'k' + Date.now(), name: 'Key baru', key: 'cvr_admin_' + Math.random().toString(36).slice(2, 6) + '••••••••' + Math.random().toString(36).slice(2, 6), used: 'Belum dipakai' }] }, 'Membuat API key admin'));

  return (
    <Page title="Developer" sub="Status sistem, fitur, dan akses teknis">
      <div className="grid g4">
        {STATS.map(([l, v, sub, warn]) => (
          <div className="card kpi" key={l}><span className="row nowrap" style={{ gap: 8 }}><span className={'dot' + (warn ? ' on' : '')} style={warn ? {} : { background: 'var(--ink)' }} />{l}</span><span className="big">{v}</span><span className="muted sm">{sub}</span></div>
        ))}
      </div>
      <div className="grid g2">
        <div className="card" style={{ gap: 4 }}>
          <h2>Fitur untuk customer</h2><span className="muted" style={{ marginBottom: 8 }}>Nyalakan/matikan fitur di semua workspace.</span>
          {FLAGS.map(([k, l, d, badge]) => (
            <div className="listrow nowrap" key={k}>
              <div style={{ flex: 1 }}><b style={{ fontWeight: 600 }}>{l}</b> {badge ? <span className="badge">{badge}</span> : null}<div className="muted sm">{d}</div></div>
              <Toggle on={s.flags[k]} label={l} onChange={(v) => update((x) => addLog({ ...x, flags: { ...x.flags, [k]: v } }, `${v ? 'Menyalakan' : 'Mematikan'} fitur ${l}`))} />
            </div>
          ))}
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <div className="card">
            <div className="row between nowrap"><div><h2>Mode maintenance</h2><span className="muted sm">{s.maintenance ? 'Aktif. Customer melihat pesan di bawah.' : 'Nonaktif. Customer bisa memakai dashboard normal.'}</span></div>
              <Toggle on={s.maintenance} label="Mode maintenance" onChange={(v) => update((x) => addLog({ ...x, maintenance: v }, `${v ? 'Menyalakan' : 'Mematikan'} mode maintenance`))} /></div>
            <input className="input" value={s.maintenanceMsg} onChange={(e) => update((x) => ({ ...x, maintenanceMsg: e.target.value }))} />
          </div>
          <div className="card" style={{ gap: 4 }}>
            <div className="row between"><h2>Admin API key</h2><button className="btn s" onClick={addKey}><Icon name="plus" size={15} />Buat key</button></div>
            {s.keys.map((k) => (
              <div className="listrow nowrap" key={k.id}><div style={{ flex: 1 }}><b style={{ fontWeight: 600 }}>{k.name}</b><div className="mono muted">{k.key}</div></div><span className="muted sm">{k.used}</span><button className="btn ghost s" onClick={() => update((x) => addLog({ ...x, keys: x.keys.filter((y) => y.id !== k.id) }, `Mencabut API key ${k.name}`))}>Cabut</button></div>
            ))}
          </div>
          <div className="card">
            <h2>Webhook</h2>
            <input className="input mono" value={s.webhook} onChange={(e) => update((x) => ({ ...x, webhook: e.target.value }))} />
          </div>
        </div>
      </div>
      <div className="card" style={{ gap: 4 }}>
        <div className="row between"><h2>Aktivitas admin</h2><button className="btn s" onClick={reset}>Reset data contoh</button></div>
        {s.log.map((l, i) => (
          <div className="listrow" key={i}><span className="muted sm" style={{ width: 130, flex: 'none' }}>{new Date(l.at).toLocaleString('id-ID', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}</span><b style={{ fontWeight: 500 }}>{l.who}</b><span>{l.text}</span></div>
        ))}
      </div>
    </Page>
  );
}
