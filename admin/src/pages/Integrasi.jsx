import React from 'react';
import { ErrorNote, Loading, Page, Pill, Toggle } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { api } from '../api.js';
import { useAction, useData } from '../hooks.js';

const ITEMS = [
  ['meta', 'Meta Conversions API', 'Pengiriman event konversi ke Meta.', 'trend'],
  ['whatsapp', 'WhatsApp', 'Penerimaan chat customer lewat webhook.', 'chat'],
  ['notifyProof', 'Notifikasi bukti transfer', 'Kabar ke admin saat customer mengunggah bukti.', 'bell'],
  ['email', 'Email', 'Email transaksional ke customer.', 'receipt'],
  ['storage', 'Penyimpanan file', 'Bukti transfer yang diunggah customer.', 'image'],
  ['sheets', 'Google Sheets', 'Ekspor rekap pembayaran otomatis.', 'dash'],
];

export default function Integrasi() {
  const q = useData('/integrations');
  const { error, run } = useAction();
  const on = q.data?.data || {};
  const set = (k, v) => run(async () => { await api.put('/integrations', { [k]: v }); q.reload(); });
  return (
    <Page title="Integrasi" sub="Layanan yang tersambung ke Convera">
      <ErrorNote>{error}</ErrorNote>
      {q.error || (q.loading && !q.data) ? <Loading error={q.error} /> : (
        <div className="grid g3">
          {ITEMS.map(([k, title, desc, icon]) => (
            <div className="card" key={k} style={{ gap: 14 }}>
              <div className="row between nowrap"><span className="av" style={{ width: 44, height: 44 }}><Icon name={icon} size={22} /></span><Toggle on={!!on[k]} label={title} onChange={(v) => set(k, v)} /></div>
              <div><b style={{ fontWeight: 600, fontSize: 16 }}>{title}</b><div className="muted">{desc}</div></div>
              <div><Pill tone={on[k] ? 'ink' : ''}>{on[k] ? 'Aktif' : 'Nonaktif'}</Pill></div>
            </div>
          ))}
        </div>
      )}
      <span className="muted sm">Sakelar ini mencatat pengaturan; penyambungan tiap layanan dikerjakan terpisah.</span>
    </Page>
  );
}
