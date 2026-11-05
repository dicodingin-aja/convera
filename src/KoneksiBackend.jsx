import React, { useEffect, useState } from 'react';
import { ensureWorkspace, getMetaConnection, saveMetaConnection, testMetaConnection, getQrStatus, disconnectQr } from './services/backend.js';
import { isToken } from './services/validators.js';

// Halaman Koneksi untuk mode backend (VITE_BACKEND_ENABLED=true).
// Token dan app secret hanya dikirim ke server; yang tampil di sini hanya petunjuk tersamar (EAAB…x9Qz),
// dan tidak pernah disimpan di localStorage.

const card = { background: 'var(--surface2)', borderRadius: 14, padding: 24, display: 'flex', flexDirection: 'column', gap: 18 };
const field = { width: '100%', height: 46, padding: '0 14px', border: 'none', borderRadius: 12, background: 'var(--surface)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };
const label = { display: 'flex', flexDirection: 'column', gap: 8, fontSize: 13, fontWeight: 500, minWidth: 0 };
const err = { fontSize: 12, color: 'var(--danger)', fontWeight: 400 };
const hintStyle = { fontSize: 12, color: 'var(--muted)', fontWeight: 400 };
const h2 = { margin: 0, fontSize: 16, fontWeight: 600 };
const mono = { fontFamily: 'ui-monospace, Menlo, Consolas, monospace', fontSize: 13 };
const btn = { height: 46, padding: '0 18px', border: 'none', borderRadius: 12, fontFamily: 'inherit', fontSize: 14, fontWeight: 500, cursor: 'pointer' };

const STATUS = {
  connected: ['Terhubung', 'var(--accent)'],
  pending: ['Belum dites', 'var(--muted)'],
  invalid: ['Token tidak valid', 'var(--danger)'],
  expired: ['Token kedaluwarsa', 'var(--danger)'],
  error: ['Gagal', 'var(--danger)'],
  disconnected: ['Terputus', 'var(--danger)'],
};

function Secret({ value, onChange, placeholder }) {
  const [show, setShow] = useState(false);
  return (
    <div style={{ position: 'relative' }}>
      <input value={value} onChange={onChange} type={show ? 'text' : 'password'} autoComplete="off" spellCheck="false" placeholder={placeholder} style={{ ...field, paddingRight: 110 }} />
      <button type="button" onClick={() => setShow((v) => !v)} style={{ position: 'absolute', right: 8, top: 8, height: 30, padding: '0 10px', border: 'none', borderRadius: 8, background: 'var(--surface2)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 12, cursor: 'pointer' }}>{show ? 'Sembunyikan' : 'Tampilkan'}</button>
    </div>
  );
}

function CopyRow({ title, text }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    try { navigator.clipboard.writeText(text); } catch { /* diabaikan */ }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div style={label}>{title}
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        <input readOnly value={text} onFocus={(e) => e.target.select()} style={{ ...field, ...mono, flex: '1 1 260px' }} />
        <button type="button" onClick={copy} style={{ ...btn, flex: 'none', background: 'var(--ink)', color: 'var(--inkText)' }}>{copied ? 'Tersalin' : 'Salin'}</button>
      </div>
    </div>
  );
}

// Mode QR: kartu ini HANYA muncul setelah WhatsApp tertaut. Scan QR ada di halaman tersendiri (QrScanPage).
function QrSection({ slug, wa, onChanged }) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');
  const stop = async () => {
    if (!window.confirm('Batalkan koneksi WhatsApp? Chat baru tidak akan tercatat sampai Anda scan QR lagi.')) return;
    setBusy(true); setMsg('');
    try { await disconnectQr(slug); await onChanged(); } catch (e) { setMsg(e.message); } finally { setBusy(false); }
  };
  return (
    <section style={card}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <h2 style={h2}>WhatsApp</h2>
        <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--accent)' }}>Terhubung{wa.phone_masked ? ` · ${String(wa.phone_masked).replace('****', '•••')}` : ''}</span>
      </div>
      <span style={hintStyle}>Chat dari iklan di nomor ini tercatat otomatis. Biarkan WhatsApp Business di ponsel tetap aktif dan terhubung internet (buka minimal sekali tiap 14 hari agar sesi tidak dicabut).</span>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10, alignItems: 'center' }}>
        <button type="button" onClick={stop} disabled={busy} style={{ ...btn, background: 'transparent', color: 'var(--danger)', boxShadow: 'inset 0 0 0 1.5px var(--danger)' }}>{busy ? 'Membatalkan…' : 'Batalkan koneksi'}</button>
        {msg ? <span role="alert" style={{ fontSize: 13, color: 'var(--danger)', flex: '1 1 220px' }}>{msg}</span> : null}
      </div>
    </section>
  );
}

const EMPTY = { mode: 'waba', pixelId: '', token: '', testCode: '', wabaId: '', phoneNumberId: '', appSecret: '' };

export default function KoneksiBackend({ workspace, onSave, onScanQr }) {
  const [slug, setSlug] = useState(null);
  const [conn, setConn] = useState(null);
  const [f, setF] = useState(EMPTY);
  const [loadError, setLoadError] = useState('');
  const [errors, setErrors] = useState({});
  const [msg, setMsg] = useState(null); // { ok, text }
  const [busy, setBusy] = useState('');
  const [wa, setWa] = useState(null); // status sesi WhatsApp (mode QR)
  const loadWa = async (s = slug) => { try { setWa(await getQrStatus(s)); } catch { /* status tidak kritis */ } };

  const fill = (c) => setF({ ...EMPTY, mode: c.mode, pixelId: c.pixel_id || '', testCode: c.test_event_code || '', wabaId: c.waba_id || '', phoneNumberId: c.phone_number_id || '' });

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const city = String(workspace.meta || '').split('·')[1]?.trim();
        const s = workspace.slug || (await ensureWorkspace(workspace.name, { city }));
        const c = await getMetaConnection(s);
        if (!alive) return;
        setSlug(s); setConn(c); fill(c); loadWa(s);
      } catch (e) { if (alive) setLoadError(e.message || 'Gagal memuat koneksi'); }
    })();
    return () => { alive = false; };
  }, [workspace.name]);

  const set = (k) => (e) => { setF((v) => ({ ...v, [k]: e.target.value })); setMsg(null); };
  const isWaba = f.mode === 'waba';

  const save = async (opts = {}) => {
    const next = {};
    if (f.pixelId && !/^\d{5,25}$/.test(f.pixelId.trim())) next.pixelId = 'Pixel/Dataset ID hanya angka';
    if (f.token && !isToken(f.token)) next.token = 'Access token tidak valid';
    if (f.wabaId && !/^\d{5,25}$/.test(f.wabaId.trim())) next.wabaId = 'WABA ID hanya angka';
    if (f.phoneNumberId && !/^\d{5,25}$/.test(f.phoneNumberId.trim())) next.phoneNumberId = 'Phone Number ID hanya angka';
    setErrors(next);
    if (Object.keys(next).length) return false;
    setBusy('save'); setMsg(null);
    try {
      const body = { mode: f.mode, pixel_id: f.pixelId.trim(), test_event_code: f.testCode.trim(), waba_id: f.wabaId.trim(), phone_number_id: f.phoneNumberId.trim() };
      if (f.token.trim()) body.access_token = f.token.trim();
      if (f.appSecret.trim()) body.app_secret = f.appSecret.trim();
      const c = await saveMetaConnection(slug, body);
      setConn(c); fill(c);
      onSave({ waType: c.mode, pixelId: c.pixel_id || '' }); // tanpa token/secret
      setMsg({ ok: true, text: 'Tersimpan' });
      if (opts.scan && f.mode === 'qr' && c.pixel_id && c.has_token && !(wa && wa.status === 'connected')) onScanQr();
      return true;
    } catch (e) { setMsg({ ok: false, text: e.message }); return false; }
    finally { setBusy(''); }
  };

  const test = async () => {
    if (!(await save())) return;
    setBusy('test'); setMsg(null);
    try {
      const r = await testMetaConnection(slug);
      setMsg({ ok: r.ok, text: r.message });
      setConn(await getMetaConnection(slug));
    } catch (e) { setMsg({ ok: false, text: e.message }); }
    finally { setBusy(''); }
  };

  const radio = (on) => ({ fontFamily: 'inherit', textAlign: 'left', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, padding: 14, borderRadius: 12, border: 'none', cursor: 'pointer', background: 'var(--surface)', color: 'var(--text)', fontSize: 14, fontWeight: 500, boxShadow: on ? 'inset 0 0 0 2px var(--accent)' : 'none' });
  const dot = (on) => ({ width: 18, height: 18, flex: 'none', borderRadius: '50%', background: on ? 'var(--accent)' : 'var(--surface2)', boxShadow: on ? 'inset 0 0 0 4px var(--surface)' : 'inset 0 0 0 1.5px var(--borderStrong)' });
  const pick = (mode) => () => { setF((x) => ({ ...x, mode })); setMsg(null); };

  if (loadError) return <div role="alert" style={{ ...card, maxWidth: 760, color: 'var(--danger)', fontSize: 14 }}>{loadError}</div>;
  if (!conn) return <div style={{ ...card, maxWidth: 760, color: 'var(--muted)', fontSize: 14 }}>Memuat koneksi…</div>;
  const [stText, stColor] = STATUS[conn.status] || STATUS.pending;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 760 }}>
    <form onSubmit={(e) => { e.preventDefault(); save({ scan: true }); }} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <section style={card}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
          <h2 style={h2}>Meta CAPI</h2>
          <span style={{ fontSize: 12, fontWeight: 600, color: stColor }}>{stText}{conn.last_check_at ? ` · dicek ${new Date(conn.last_check_at).toLocaleString('id-ID')}` : ''}</span>
        </div>
        <div style={label}>Jenis koneksi WhatsApp
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 10 }}>
            <button type="button" onClick={pick('waba')} style={radio(isWaba)}>WABA (resmi)<span style={dot(isWaba)} /></button>
            <button type="button" onClick={pick('qr')} style={radio(!isWaba)}>WA Bisnis (scan QR) · Beta<span style={dot(!isWaba)} /></button>
          </div>
        </div>
        <label style={label}>Pixel / Dataset ID
          <input value={f.pixelId} onChange={set('pixelId')} inputMode="numeric" placeholder="123456789123456" style={field} />
          {errors.pixelId ? <span style={err}>{errors.pixelId}</span> : null}
        </label>
        <label style={label}>Access token CAPI
          <Secret value={f.token} onChange={set('token')} placeholder={conn.has_token ? `Tersimpan (${conn.access_token_hint}). Kosongkan untuk tidak mengubah.` : 'EAAB…'} />
          {errors.token ? <span style={err}>{errors.token}</span> : conn.has_token && !f.token ? <span style={{ ...hintStyle, color: 'var(--accent)' }}>✓ Token tersimpan ({conn.access_token_hint}). Isi kolom di atas hanya bila ingin menggantinya.</span> : null}
        </label>
        <label style={label}>Test event code (opsional)
          <input value={f.testCode} onChange={set('testCode')} placeholder="TEST12345" style={field} />
        </label>
      </section>

      {isWaba ? (
        <section style={card}>
          <h2 style={h2}>WABA API / CRM (webhook)</h2>
          <span style={hintStyle}>Salin URL di bawah, lalu tempel sebagai tujuan webhook di penyedia WABA / CRM Anda (mis. kirim.chat). Setiap chat dari iklan yang masuk ke sana akan diteruskan ke Convera. Anda tidak perlu mengisi apa pun lagi di sini.</span>
          <CopyRow title="Webhook URL" text={conn.webhook_url} />
          <details>
            <summary style={{ ...hintStyle, cursor: 'pointer' }}>Pengaturan lanjutan (opsional)</summary>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14, marginTop: 14 }}>
              <CopyRow title="Verify Token (bila penyedia meminta)" text={conn.verify_token || ''} />
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 14 }}>
                <label style={label}>WABA ID
                  <input value={f.wabaId} onChange={set('wabaId')} inputMode="numeric" style={field} />
                  {errors.wabaId ? <span style={err}>{errors.wabaId}</span> : <span style={hintStyle}>Disertakan di event ke Meta bila diisi.</span>}
                </label>
                <label style={label}>Phone Number ID
                  <input value={f.phoneNumberId} onChange={set('phoneNumberId')} inputMode="numeric" style={field} />
                  {errors.phoneNumberId ? <span style={err}>{errors.phoneNumberId}</span> : null}
                </label>
              </div>
              <label style={label}>App Secret
                <Secret value={f.appSecret} onChange={set('appSecret')} placeholder={conn.has_app_secret ? `Tersimpan (${conn.app_secret_hint}). Kosongkan untuk tidak mengubah.` : ''} />
                <span style={hintStyle}>Hanya bila webhook datang langsung dari Meta (bukan lewat CRM). Bila diisi, setiap request harus bertanda tangan valid.</span>
              </label>
            </div>
          </details>
        </section>
      ) : (
        wa && wa.status === 'connected' ? <QrSection slug={slug} wa={wa} onChanged={() => loadWa()} /> : null
      )}

      <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <button type="submit" disabled={!!busy} style={{ ...btn, padding: '0 24px', background: 'var(--accent)', color: '#fff', opacity: busy ? 0.7 : 1 }}>{busy === 'save' ? 'Menyimpan…' : f.mode === 'qr' && !(wa && wa.status === 'connected') ? 'Simpan & lanjut scan QR' : 'Simpan'}</button>
        {f.testCode.trim() ? (
  <button type="button" onClick={test} disabled={!!busy} style={{ ...btn, background: 'var(--surface2)', color: 'var(--text)', boxShadow: 'inset 0 0 0 1.5px var(--borderStrong)', opacity: busy ? 0.7 : 1 }}>{busy === 'test' ? 'Mengetes…' : 'Tes Koneksi'}</button>
        ) : null}
        {msg ? <span role="status" style={{ fontSize: 13, color: msg.ok ? 'var(--muted)' : 'var(--danger)', flex: '1 1 240px' }}>{msg.text}</span> : null}
      </div>
    </form>
    </div>
  );
}
