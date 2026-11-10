import React from 'react';

// Isi tab "Detail" di panel Log Event: data yang dikirim ke Meta dalam bahasa biasa, jawaban Meta satu kalimat,
// dan JSON asli disembunyikan di "Lihat data teknis" (untuk cek masalah bersama tim support).

const EVENT_LABEL = {
  LeadSubmitted: 'Lead', Lead: 'Lead', QualifiedLead: 'Lead berkualitas', Purchase: 'Purchase', InitiateCheckout: 'Checkout',
  AddToCart: 'Tambah ke keranjang', ViewContent: 'Lihat produk', Contact: 'Kontak', Schedule: 'Jadwal', CompleteRegistration: 'Pendaftaran',
};
const CHANNEL = { whatsapp: 'WhatsApp', messenger: 'Messenger', instagram: 'Instagram' };
const SOURCE = { business_messaging: 'Pesan bisnis', website: 'Website', system_generated: 'Sistem', app: 'Aplikasi', other: 'Lainnya' };
// Penyebab umum dari kode error Meta, ditulis singkat.
const META_CODE = { 190: 'token tidak valid', 100: 'data event tidak valid', 10: 'izin tidak cukup', 200: 'izin tidak cukup', 803: 'dataset tidak ditemukan', 4: 'batas kiriman Meta', 17: 'batas kiriman Meta', 32: 'batas kiriman Meta', 613: 'batas kiriman Meta', 1: 'gangguan di Meta', 2: 'gangguan di Meta' };

const parse = (v) => {
  if (!v) return null;
  if (typeof v === 'object') return v;
  try { return JSON.parse(v); } catch { return null; }
};
const firstEvent = (p) => (p && Array.isArray(p.data) ? p.data[0] : p) || null;

function metaAnswer({ status, response, errorCode, errorMessage }) {
  const r = parse(response);
  const err = r && r.error;
  if (err) {
    const why = META_CODE[err.code] || err.error_user_msg || err.message || 'alasan tidak disebutkan';
    return { tone: 'err', text: `Ditolak · ${why}${err.code != null ? ` (kode ${err.code})` : ''}` };
  }
  if (status === 'sent' || status === 'Terkirim') {
    const n = r && r.events_received;
    return { tone: 'ok', text: n ? `Diterima Meta · ${n} event diterima` : 'Diterima Meta' };
  }
  if (r && r.skipped) return { tone: 'muted', text: 'Dilewati · event yang sama sudah pernah dikirim' };
  const m = /^meta_(\d+)$/.exec(errorCode || '');
  if (m) return { tone: 'err', text: `Ditolak · ${META_CODE[m[1]] || 'error dari Meta'} (kode ${m[1]})` };
  if (status === 'queued') return { tone: 'muted', text: 'Belum dikirim · masih dalam antrean' };
  if (status === 'pending_value') return { tone: 'muted', text: 'Belum dikirim · menunggu nilai pembelian diisi' };
  if (status === 'skipped' || status === 'Kedaluwarsa') return { tone: 'muted', text: 'Tidak dikirim' + (errorMessage ? ' · ' + errorMessage : '') };
  if (status === 'failed' || status === 'Gagal') return { tone: 'err', text: 'Gagal dikirim' + (errorMessage ? ' · ' + errorMessage : '') };
  return { tone: 'muted', text: 'Belum ada jawaban dari Meta' };
}

const box = { display: 'flex', flexDirection: 'column', borderRadius: 12, background: 'var(--surface2)', padding: '4px 16px' };
const row = { display: 'flex', justifyContent: 'space-between', gap: 16, padding: '11px 0', fontSize: 13, borderBottom: '1px solid var(--border)' };
const caps = { fontSize: 11, fontWeight: 500, letterSpacing: '0.08em', color: 'var(--muted)', textTransform: 'uppercase', margin: '0 0 8px' };
const mono = { margin: 0, padding: '14px 16px', borderRadius: 10, background: '#1d1c1a', color: '#e9e4dc', fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace", fontSize: 12, lineHeight: 1.6, whiteSpace: 'pre', overflowX: 'auto' };
const toneColor = { ok: 'var(--okFg)', err: 'var(--errFg)', muted: 'var(--text)' };

// props: eventName, status, source, value, eventTime (Date), eventId, payload, response, errorCode, errorMessage, fmtTime, rp
export default function EventDetail(props) {
  const { eventName, status, source, value, eventTime, eventId, payload, response, errorCode, errorMessage, fmtTime, rp } = props;
  const p = firstEvent(parse(payload));
  const ud = (p && p.user_data) || {};
  const cd = (p && p.custom_data) || {};
  const sentAt = p && p.event_time ? new Date(p.event_time * 1000) : eventTime;
  const val = cd.value != null ? cd.value : value;
  const name = (p && p.event_name) || eventName;
  const isTest = source === 'Tes koneksi' || source === 'test';
  const rows = [
    ['Event', EVENT_LABEL[name] || name],
    ['Waktu kejadian', sentAt ? fmtTime(sentAt) + ' WIB' : '—'],
    ['Nilai', val ? rp(val) : '—'],
    ['Dari iklan', ud.ctwa_clid ? (isTest ? 'Tes (ID klik contoh)' : 'Ya (ID klik iklan ada)') : p ? 'Tidak' : '—'],
    ['Nomor pelanggan', ud.ph ? 'Dikirim terenkripsi' : p ? 'Tidak dikirim' : '—'],
    ['Saluran', p ? CHANNEL[p.messaging_channel] || SOURCE[p.action_source] || p.action_source || '—' : '—'],
    ['Sumber', source || '—'],
  ];
  const ans = metaAnswer({ status, response, errorCode, errorMessage });
  const raw = parse(payload);
  const resp = parse(response);
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
      <section>
        <h3 style={caps}>{p ? 'Data yang dikirim ke Meta' : 'Data event'}</h3>
        <div style={box}>
          {rows.map(([k, v], i) => (
            <div key={k} style={{ ...row, borderBottom: i === rows.length - 1 ? 'none' : row.borderBottom }}>
              <span style={{ color: 'var(--muted)' }}>{k}</span><span style={{ color: 'var(--text)', textAlign: 'right' }}>{v}</span>
            </div>
          ))}
        </div>
        {!p ? <p style={{ margin: '8px 0 0', fontSize: 12, color: 'var(--muted)' }}>Event ini belum pernah dikirim ke Meta.</p> : null}
      </section>
      <section>
        <h3 style={caps}>Jawaban Meta</h3>
        <div style={{ ...box, padding: '12px 16px', fontSize: 13, color: toneColor[ans.tone] }}>{ans.text}</div>
      </section>
      {raw || resp || eventId ? (
        <details>
          <summary style={{ cursor: 'pointer', fontSize: 13, color: 'var(--muted)', userSelect: 'none' }}>Lihat data teknis</summary>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 12 }}>
            {eventId ? <span style={{ fontSize: 12, color: 'var(--muted)', wordBreak: 'break-all' }}>event_id: <span style={{ fontFamily: mono.fontFamily }}>{eventId}</span></span> : null}
            {raw ? <div><h3 style={caps}>Payload</h3><pre style={mono}>{JSON.stringify(raw, null, 2)}</pre></div> : null}
            {resp ? <div><h3 style={caps}>Respons Meta</h3><pre style={mono}>{JSON.stringify(resp, null, 2)}</pre></div> : null}
          </div>
        </details>
      ) : null}
    </div>
  );
}
