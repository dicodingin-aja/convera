import React, { useEffect, useState } from 'react';
import { getAdSlots, releaseAdSlot, trackAdSlot } from './services/backend.js';
import { rupiah } from './plans.js';

// Panel "Slot kampanye" di atas tabel Performa Iklan (mode backend).
// Customer membayar per kampanye yang dilacak, per 30 hari. Event dari chat iklan hanya dikirim ke Meta bila
// kampanyenya memakai slot; slot kosong terisi otomatis oleh kampanye pertama yang mendatangkan chat.
// Chat organik (tanpa iklan) tidak memakai slot dan tidak tampil di sini.

const card = { background: 'var(--surface2)', borderRadius: 14, padding: 20, display: 'flex', flexDirection: 'column', gap: 14, marginBottom: 16 };
const btn = { height: 34, padding: '0 14px', border: 'none', borderRadius: 9, fontFamily: 'inherit', fontSize: 13, fontWeight: 500, cursor: 'pointer', whiteSpace: 'nowrap' };
const row = { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '10px 12px', borderRadius: 10, background: 'var(--surface)', flexWrap: 'wrap' };
const muted = { fontSize: 12, color: 'var(--muted)' };
const fmtDate = (d) => new Date(d).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
const kindLabel = (k) => (k === 'campaign' ? 'Kampanye' : 'Iklan');
const titleOf = (x) => x.name || `${kindLabel(x.kind)} ${String(x.id).slice(-6)}`;

export default function AdSlotsPanel({ slug, onAddSlots }) {
  const [st, setSt] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [showAll, setShowAll] = useState(false);

  const load = async () => {
    try { setSt(await getAdSlots(slug)); setError(''); } catch (e) { setError(e.message || 'Gagal memuat slot kampanye'); }
  };
  useEffect(() => { setSt(null); load(); }, [slug]);

  const act = (fn, key) => async () => {
    setBusy(key); setError('');
    try { setSt(await fn()); } catch (e) { setError(e.message || 'Gagal menyimpan'); } finally { setBusy(''); }
  };

  if (!st) {
    return <section style={card}>{error ? <span role="alert" style={{ color: 'var(--danger)', fontSize: 13 }}>{error}</span> : <span style={muted}>Memuat slot kampanye…</span>}</section>;
  }
  const pct = st.quota ? Math.min(100, Math.round((st.used / st.quota) * 100)) : 100;
  const free = st.free > 0;
  const tracked = st.tracked || [];
  const options = (st.candidates || []).filter((c) => !c.tracked);
  const shown = showAll ? options : options.slice(0, 5);

  return (
    <section style={card} aria-labelledby="slots-title">
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 }}>
          <h2 id="slots-title" style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>Slot kampanye</h2>
          <span style={muted}>{rupiah(st.price_per_slot || 0)} per kampanye per 30 hari. Event dari kampanye yang tidak memakai slot tidak dikirim ke Meta.</span>
        </div>
        <button type="button" onClick={onAddSlots} style={{ ...btn, background: 'var(--accent)', color: '#fff' }}>Tambah slot</button>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13 }}>
          <span><b>{st.used}</b> dari <b>{st.quota}</b> slot terpakai</span>
          <span style={muted}>{free ? `${st.free} slot kosong` : 'Slot penuh'}</span>
        </div>
        <div style={{ height: 8, borderRadius: 999, background: 'var(--surface)', overflow: 'hidden' }}>
          <div style={{ width: pct + '%', height: '100%', borderRadius: 999, background: free ? 'var(--accent)' : 'var(--danger)' }} />
        </div>
      </div>

      {!st.connected ? (
        <span style={muted}>Akun Meta Ads belum terhubung, jadi <b>tiap iklan dihitung 1 slot</b>. Setelah terhubung, slot dihitung per kampanye.</span>
      ) : null}

      {tracked.length ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {tracked.map((x) => {
            const released = !!x.stopped_at;
            const key = x.kind + ':' + x.id;
            const status = released ? `Dilepas · slot kosong lagi ${fmtDate(x.locked_until)}` : x.effective ? 'Dilacak' : 'Slot penuh, event tidak dikirim';
            return (
              <div key={key} style={row}>
                <span style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0, flex: '1 1 220px' }}>
                  <b style={{ fontSize: 14, fontWeight: 600, overflowWrap: 'anywhere' }}>{titleOf(x)}</b>
                  <span style={muted}>{kindLabel(x.kind)}{x.auto ? ' · dipilih otomatis' : ''} · sejak {fmtDate(x.started_at)}</span>
                </span>
                <span style={{ fontSize: 12, fontWeight: 600, color: released || !x.effective ? 'var(--muted)' : 'var(--connectedText)' }}>{status}</span>
                {released ? (
                  <button type="button" disabled={!!busy} onClick={act(() => trackAdSlot(slug, x.kind, x.id), key)} style={{ ...btn, background: 'var(--surface2)', color: 'var(--text)' }}>{busy === key ? 'Menyimpan…' : 'Lacak lagi'}</button>
                ) : (
                  <button type="button" disabled={!!busy} onClick={act(() => releaseAdSlot(slug, x.kind, x.id), key)} style={{ ...btn, background: 'transparent', color: 'var(--text)', boxShadow: 'inset 0 0 0 1.5px var(--borderStrong)' }}>{busy === key ? 'Menyimpan…' : 'Lepas'}</button>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <span style={muted}>Belum ada kampanye. Kampanye pertama yang dapat chat otomatis masuk slot, atau pilih sendiri di bawah.</span>
      )}

      {options.length ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <b style={{ fontSize: 13 }}>{st.connected ? 'Kampanye lain' : 'Iklan lain yang dapat chat'}</b>
          {shown.map((x) => {
            const key = x.kind + ':' + x.id;
            return (
              <div key={key} style={row}>
                <span style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0, flex: '1 1 220px' }}>
                  <span style={{ fontSize: 14, overflowWrap: 'anywhere' }}>{titleOf(x)}</span>
                  <span style={muted}>{kindLabel(x.kind)}{x.kind === 'campaign' ? ` · ${x.ads} iklan` : ''} · {x.leads_30d} lead 30 hari{x.spend_30d ? ` · spend ${rupiah(x.spend_30d)}` : ''}</span>
                </span>
                <button type="button" disabled={!!busy || !free} title={free ? '' : 'Slot penuh. Tambah slot atau lepas kampanye lain.'} onClick={act(() => trackAdSlot(slug, x.kind, x.id), key)}
                  style={{ ...btn, background: free ? 'var(--ink)' : 'var(--surface2)', color: free ? 'var(--inkText)' : 'var(--muted)', cursor: free ? 'pointer' : 'not-allowed' }}>{busy === key ? 'Menyimpan…' : 'Lacak'}</button>
              </div>
            );
          })}
          {options.length > 5 ? <button type="button" onClick={() => setShowAll((v) => !v)} style={{ ...btn, alignSelf: 'flex-start', background: 'transparent', color: 'var(--muted)', padding: 0 }}>{showAll ? 'Tampilkan lebih sedikit' : `Tampilkan semua (${options.length})`}</button> : null}
        </div>
      ) : null}

      <span style={muted}>Kampanye yang dilepas tetap memakai slot sampai {st.lock_days} hari sejak mulai dilacak. Chat yang bukan dari iklan tidak dihitung.</span>
      {error ? <span role="alert" style={{ color: 'var(--danger)', fontSize: 13 }}>{error}</span> : null}
    </section>
  );
}
