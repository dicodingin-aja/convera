import React from 'react';
import { Page, Pill, Toggle, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { addLog } from '../store.js';

const ITEMS = [
  ['meta', 'Meta Conversions API', 'Pengiriman event konversi ke Meta.', 'trend'],
  ['whatsapp', 'WhatsApp', 'Penerimaan chat customer lewat webhook.', 'chat'],
  ['notifyProof', 'Notifikasi bukti transfer', 'Kabar ke admin saat customer mengunggah bukti.', 'bell'],
  ['email', 'Email', 'Email transaksional ke customer.', 'receipt'],
  ['storage', 'Penyimpanan file', 'Bukti transfer yang diunggah customer.', 'image'],
  ['sheets', 'Google Sheets', 'Ekspor rekap pembayaran otomatis.', 'dash'],
];

export default function Integrasi() {
  const { s, update } = useApp();
  return (
    <Page title="Integrasi" sub="Layanan yang tersambung ke Convera">
      <div className="grid g3">
        {ITEMS.map(([k, title, desc, icon]) => (
          <div className="card" key={k} style={{ gap: 14 }}>
            <div className="row between nowrap"><span className="av" style={{ width: 44, height: 44 }}><Icon name={icon} size={22} /></span><Toggle on={s.integrations[k]} label={title} onChange={(v) => update((x) => addLog({ ...x, integrations: { ...x.integrations, [k]: v } }, `${v ? 'Mengaktifkan' : 'Mematikan'} integrasi ${title}`))} /></div>
            <div><b style={{ fontWeight: 600, fontSize: 16 }}>{title}</b><div className="muted">{desc}</div></div>
            <div><Pill tone={s.integrations[k] ? 'ink' : ''}>{s.integrations[k] ? 'Terhubung' : 'Nonaktif'}</Pill></div>
          </div>
        ))}
      </div>
    </Page>
  );
}
