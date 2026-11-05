import React, { useEffect, useRef, useState } from 'react';
import { ensureWorkspace, startQr, getQrStatus, disconnectQr } from './services/backend.js';

// Halaman khusus scan QR WhatsApp (mode QR). Dibuka setelah Pixel ID & Access Token CAPI tersimpan;
// setelah berhasil tertaut, otomatis kembali ke halaman Koneksi.
const btn = { height: 44, padding: '0 20px', border: 'none', borderRadius: 10, fontFamily: 'inherit', fontSize: 14, fontWeight: 600, cursor: 'pointer' };

export default function QrScanPage({ themeStyle, workspace, onDone, onBack }) {
  const [slug, setSlug] = useState(null);
  const [qr, setQr] = useState('');
  const [phase, setPhase] = useState('loading'); // loading | scan | expired | connected | error
  const [phone, setPhone] = useState('');
  const [error, setError] = useState('');
  const alive = useRef(true);

  const begin = async (s) => {
    setPhase('loading'); setError('');
    try {
      const r = await startQr(s);
      if (!alive.current) return;
      if (r.status === 'connected') { setPhase('connected'); return; }
      setQr(r.qr_image || ''); setPhase('scan');
    } catch (e) { if (alive.current) { setError(e.message || 'Gagal membuat QR'); setPhase('error'); } }
  };

  useEffect(() => {
    alive.current = true;
    (async () => {
      try {
        const city = String(workspace.meta || '').split('·')[1]?.trim();
        const s = workspace.slug || (await ensureWorkspace(workspace.name, { city }));
        if (!alive.current) return;
        setSlug(s);
        await begin(s);
      } catch (e) { if (alive.current) { setError(e.message || 'Gagal memuat'); setPhase('error'); } }
    })();
    return () => { alive.current = false; };
  }, []);

  // Selama menunggu scan: QR diperbarui & status dicek tiap 2,5 dtk.
  useEffect(() => {
    if (phase !== 'scan' || !slug) return undefined;
    const t = window.setInterval(async () => {
      try {
        const st = await getQrStatus(slug);
        if (!alive.current) return;
        if (st.status === 'connected') { setPhone(st.phone_masked || ''); setPhase('connected'); }
        else if (st.status === 'disconnected') setPhase('expired');
        else if (st.qr_image) setQr(st.qr_image);
      } catch { /* coba lagi pada putaran berikutnya */ }
    }, 2500);
    return () => window.clearInterval(t);
  }, [phase, slug]);

  // Berhasil: tampilkan sebentar lalu kembali ke Koneksi.
  useEffect(() => {
    if (phase !== 'connected') return undefined;
    const t = window.setTimeout(onDone, 1800);
    return () => window.clearTimeout(t);
  }, [phase]);

  const cancel = async () => { try { if (slug && phase === 'scan') await disconnectQr(slug); } catch { /* abaikan */ } onBack(); };

  return (
    <div style={{ ...themeStyle, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: 'var(--bg)', color: 'var(--text)' }}>
      <div style={{ width: 'min(460px, 100%)', background: 'var(--surface)', borderRadius: 16, padding: 32, display: 'flex', flexDirection: 'column', gap: 18, alignItems: 'center', textAlign: 'center', boxShadow: '0 20px 50px -25px rgba(0,0,0,.35)' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 20 }}>Hubungkan WhatsApp</h1>
          <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 13 }}>{workspace.name}</p>
        </div>

        {phase === 'loading' ? <div style={{ color: 'var(--muted)', fontSize: 14, padding: '60px 0' }}>Menyiapkan kode QR…</div> : null}

        {phase === 'scan' ? (
          <>
            <img src={qr} alt="Kode QR untuk menautkan WhatsApp" width={260} height={260} style={{ borderRadius: 12, background: '#fff', padding: 8 }} />
            <ol style={{ margin: 0, paddingLeft: 18, textAlign: 'left', fontSize: 13, color: 'var(--muted)', display: 'flex', flexDirection: 'column', gap: 4 }}>
              <li>Buka <b>WhatsApp Business</b> di ponsel.</li>
              <li>Menu ⋮ / Pengaturan → <b>Perangkat tertaut</b> → <b>Tautkan perangkat</b>.</li>
              <li>Arahkan kamera ke kode QR ini. Halaman akan kembali otomatis.</li>
            </ol>
          </>
        ) : null}

        {phase === 'connected' ? (
          <div style={{ padding: '40px 0', display: 'flex', flexDirection: 'column', gap: 8, alignItems: 'center' }}>
            <div style={{ width: 56, height: 56, borderRadius: '50%', background: 'var(--accent)', color: '#fff', fontSize: 28, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>✓</div>
            <div style={{ fontSize: 16, fontWeight: 700 }}>WhatsApp terhubung</div>
            {phone ? <div style={{ color: 'var(--muted)', fontSize: 13 }}>{String(phone).replace('****', '•••')}</div> : null}
            <div style={{ color: 'var(--muted)', fontSize: 13 }}>Mengembalikan ke halaman Koneksi…</div>
          </div>
        ) : null}

        {phase === 'expired' ? <div style={{ padding: '40px 0', color: 'var(--muted)', fontSize: 14 }}>Kode QR kedaluwarsa sebelum di-scan.</div> : null}
        {phase === 'error' ? <div role="alert" style={{ padding: '30px 0', color: 'var(--danger)', fontSize: 14, fontWeight: 600 }}>{error}</div> : null}

        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', justifyContent: 'center' }}>
          {phase === 'expired' || phase === 'error' ? <button type="button" onClick={() => slug && begin(slug)} disabled={!slug} style={{ ...btn, background: 'var(--accent)', color: '#fff' }}>Buat QR baru</button> : null}
          {phase !== 'connected' ? <button type="button" onClick={cancel} style={{ ...btn, background: 'var(--surface2)', color: 'var(--text)' }}>Kembali ke Koneksi</button> : null}
        </div>
        <span style={{ fontSize: 11, color: 'var(--muted)' }}>Convera hanya membaca pesan masuk dan tidak pernah mengirim pesan dari nomor Anda.</span>
      </div>
    </div>
  );
}
