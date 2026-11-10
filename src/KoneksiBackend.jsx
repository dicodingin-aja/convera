import React, { useEffect, useState } from 'react';
import { ensureWorkspace, getMetaConnection, saveMetaConnection, testMetaConnection, getQrStatus, disconnectQr, getAdsConnection, saveAdsConnection, syncAdsConnection, deleteAdsConnection } from './services/backend.js';
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
  const connected = wa.status === 'connected';
  const stop = async () => {
    if (!window.confirm('Batalkan koneksi WhatsApp? Chat baru tidak akan tercatat sampai kamu scan QR lagi.')) return;
    setBusy(true); setMsg('');
    try { await disconnectQr(slug); await onChanged(); } catch (e) { setMsg(e.message); } finally { setBusy(false); }
  };
  return (
    <section style={card}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <h2 style={h2}>WhatsApp</h2>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 600, color: connected ? 'var(--connectedText)' : 'var(--disconnectedText)' }}>
          <span aria-hidden="true" style={{ width: 7, height: 7, borderRadius: '50%', background: connected ? 'var(--connectedText)' : 'var(--disconnectedText)' }} />
          {connected ? 'terhubung' : 'belum terhubung'}{wa.phone_masked ? ` · ${String(wa.phone_masked).replace('****', '•••')}` : ''}
        </span>
      </div>
      <span style={hintStyle}>Chat dari iklan di nomor ini otomatis tercatat. Buka WhatsApp di HP minimal sekali tiap 14 hari supaya tidak terputus.</span>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10, alignItems: 'center' }}>
        <button type="button" onClick={stop} disabled={busy} style={{ ...btn, background: 'transparent', color: 'var(--danger)', boxShadow: 'inset 0 0 0 1.5px var(--danger)' }}>{busy ? 'Membatalkan…' : 'Batalkan koneksi'}</button>
        {msg ? <span role="alert" style={{ fontSize: 13, color: 'var(--danger)', flex: '1 1 220px' }}>{msg}</span> : null}
      </div>
    </section>
  );
}

// Mode QR: sesi pernah tertaut tetapi terputus (dicabut dari ponsel, kedaluwarsa, atau layanan sesi berhenti).
function QrLostSection({ onReconnect }) {
  return (
    <section style={{ ...card, boxShadow: 'inset 0 0 0 1.5px var(--danger)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <h2 style={h2}>WhatsApp</h2>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 600, color: 'var(--disconnectedText)' }}>
          <span aria-hidden="true" style={{ width: 7, height: 7, borderRadius: '50%', background: 'var(--disconnectedText)' }} />
          terputus
        </span>
      </div>
      <span style={hintStyle}>Koneksi WhatsApp ke Convera terputus (mis. perangkat tertaut dihapus dari ponsel). Chat baru tidak tercatat sampai kamu scan QR lagi.</span>
      <div><button type="button" onClick={onReconnect} style={{ ...btn, background: 'var(--accent)', color: '#fff' }}>Hubungkan ulang (scan QR)</button></div>
    </section>
  );
}

// Akun Meta Ads (opsional): biaya, klik, dan kampanye per iklan untuk halaman Performa Iklan dan slot kampanye.
// Hanya izin baca (ads_read). Tanpa koneksi ini setiap iklan dihitung satu slot kampanye.
function MetaAdsSection({ slug }) {
  const [ads, setAds] = useState(null);
  const [f, setF] = useState({ account: '', token: '' });
  const [busy, setBusy] = useState('');
  const [msg, setMsg] = useState(null); // { ok, text }
  const [confirmOff, setConfirmOff] = useState(false);
  const load = async () => { try { setAds(await getAdsConnection(slug)); } catch (e) { setMsg({ ok: false, text: e.message }); } };
  useEffect(() => { if (slug) load(); }, [slug]);

  const run = (key, fn, okText) => async (e) => {
    if (e && e.preventDefault) e.preventDefault();
    setBusy(key); setMsg(null);
    try {
      const out = await fn();
      if (out && typeof out === 'object' && 'connected' in out) setAds(out); else await load();
      if (okText) setMsg({ ok: true, text: okText });
    } catch (err) { setMsg({ ok: false, text: err.message }); } finally { setBusy(''); setConfirmOff(false); }
  };
  const save = run('save', async () => {
    const out = await saveAdsConnection(slug, { ad_account_id: f.account.trim(), access_token: f.token.trim() });
    setF({ account: '', token: '' });
    return out;
  }, 'Tersambung. Data iklan sedang disinkronkan, biasanya selesai dalam 1–2 menit.');

  if (!ads) return <section style={card}><h2 style={h2}>Akun Meta Ads</h2><span style={hintStyle}>Memuat…</span></section>;
  const on = ads.connected;
  const stText = !on ? 'belum terhubung' : ads.status === 'connected' ? 'connected' : ads.status === 'invalid' ? 'token tidak valid' : ads.status === 'pending' ? 'menyinkronkan' : 'error';
  const stColor = on && ads.status === 'connected' ? 'var(--connectedText)' : 'var(--disconnectedText)';
  const when = (d) => (d ? new Date(d).toLocaleString('id-ID', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : 'belum pernah');
  const canSave = !busy && f.account.trim() && f.token.trim();

  return (
    <section style={card}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <h2 style={h2}>Akun Meta Ads <span style={{ ...hintStyle, fontSize: 12 }}>· opsional</span></h2>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 600, color: stColor }}>
          <span aria-hidden="true" style={{ width: 7, height: 7, borderRadius: '50%', background: stColor }} />{stText}
        </span>
      </div>
      <span style={hintStyle}>Untuk melihat spend dan ROAS di Performa Iklan, dan supaya slot dihitung per kampanye. Convera hanya membaca data, tidak mengubah iklan kamu.</span>
      {on ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 10, fontSize: 14 }}>
          <span><span style={hintStyle}>Ad Account</span><br /><b style={mono}>act_{ads.ad_account_id}</b>{ads.account_name ? ' · ' + ads.account_name : ''}</span>
          <span><span style={hintStyle}>Sinkron terakhir</span><br />{when(ads.last_sync_at)} · tiap 30 menit</span>
          <span><span style={hintStyle}>Mata uang · token</span><br />{ads.currency || '—'} · <span style={mono}>{ads.access_token_hint || '—'}</span></span>
        </div>
      ) : null}
      {on && ads.last_error ? <span role="alert" style={{ fontSize: 13, color: 'var(--danger)' }}>{ads.last_error}</span> : null}
      {on && ads.currency && ads.currency !== 'IDR' ? <span style={hintStyle}>Mata uang akun iklan {ads.currency}: ROAS di Convera hanya akurat untuk akun dalam Rupiah.</span> : null}
      {!ads.graph_configured ? <span style={{ ...hintStyle, color: 'var(--danger)' }}>Server belum dikonfigurasi untuk Meta (GRAPH_API_VERSION kosong). Hubungi admin Convera.</span> : null}

      <form onSubmit={save} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
        <label style={label}>{on ? 'Ganti Ad Account ID' : 'Ad Account ID'}
          <input value={f.account} onChange={(e) => setF((x) => ({ ...x, account: e.target.value }))} inputMode="numeric" placeholder="act_1234567890" style={{ ...field, ...mono }} />
        </label>
        <label style={label}>Access token (izin ads_read)
          <Secret value={f.token} onChange={(e) => setF((x) => ({ ...x, token: e.target.value }))} placeholder={on ? 'Tersimpan (' + (ads.access_token_hint || '') + '). Isi untuk mengganti.' : 'EAAB…'} />
          <span style={hintStyle}>Pakai token System User supaya tidak kedaluwarsa. Akun iklan ini harus sudah diberikan ke System User itu.</span>
        </label>
        <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 10 }}>
          <button type="submit" disabled={!canSave} style={{ ...btn, background: 'var(--accent)', color: '#fff', opacity: canSave ? 1 : 0.5 }}>{busy === 'save' ? 'Menghubungkan…' : on ? 'Simpan akun baru' : 'Hubungkan'}</button>
          {on ? <button type="button" disabled={!!busy} onClick={run('sync', () => syncAdsConnection(slug), 'Sinkronisasi dimulai. Buka lagi halaman Performa Iklan dalam 1–2 menit.')} style={{ ...btn, background: 'var(--surface)', color: 'var(--text)' }}>{busy === 'sync' ? 'Memulai…' : 'Sinkronkan sekarang'}</button> : null}
          {on && !confirmOff ? <button type="button" disabled={!!busy} onClick={() => setConfirmOff(true)} style={{ ...btn, background: 'transparent', color: 'var(--danger)', boxShadow: 'inset 0 0 0 1.5px var(--danger)' }}>Putuskan</button> : null}
          {on && confirmOff ? (
            <span style={{ display: 'inline-flex', flexWrap: 'wrap', gap: 8, alignItems: 'center', fontSize: 13 }}>Data biaya iklan dihapus dari Convera. Lanjut?
              <button type="button" disabled={!!busy} onClick={run('off', () => deleteAdsConnection(slug), 'Akun Meta Ads diputus.')} style={{ ...btn, height: 36, background: 'var(--danger)', color: '#fff' }}>{busy === 'off' ? 'Memutus…' : 'Ya, putuskan'}</button>
              <button type="button" onClick={() => setConfirmOff(false)} style={{ ...btn, height: 36, background: 'var(--surface)', color: 'var(--text)' }}>Batal</button>
            </span>
          ) : null}
        </div>
        {msg ? <span role="status" style={{ fontSize: 13, color: msg.ok ? 'var(--muted)' : 'var(--danger)' }}>{msg.text}</span> : null}
      </form>
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

  // Status WhatsApp dicek berkala agar pemutusan dari ponsel langsung terlihat tanpa memuat ulang halaman.
  useEffect(() => {
    if (!slug) return undefined;
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') loadWa(slug); }, 15000);
    return () => window.clearInterval(t);
  }, [slug]);

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
  const realtimeStatus = f.mode === 'qr' ? (wa && wa.status === 'connected' ? 'connected' : 'disconnected') : (conn.status === 'connected' ? 'connected' : 'disconnected');
  const stColor = realtimeStatus === 'connected' ? 'var(--connectedText)' : 'var(--disconnectedText)';

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 760 }}>
    <form onSubmit={(e) => { e.preventDefault(); save({ scan: true }); }} style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <section style={card}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
          <h2 style={h2}>Meta CAPI</h2>
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, fontWeight: 600, color: stColor }}>
            <span aria-hidden="true" style={{ width: 7, height: 7, borderRadius: '50%', background: stColor }} />
            {realtimeStatus === 'connected' ? 'terhubung' : 'belum terhubung'}
          </span>
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
          <span style={hintStyle}>Salin URL di bawah, lalu tempel sebagai tujuan webhook di penyedia WABA atau CRM kamu (mis. kirim.chat). Chat dari iklan yang masuk ke sana akan diteruskan ke Convera. Selesai, tidak ada yang perlu diisi lagi.</span>
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
        wa && wa.status === 'connected' ? <QrSection slug={slug} wa={wa} onChanged={() => loadWa()} />
          : wa && wa.status === 'disconnected' && wa.phone_masked ? <QrLostSection onReconnect={onScanQr} /> : null
      )}

      <div style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <button type="submit" disabled={!!busy} style={{ ...btn, padding: '0 24px', background: 'var(--accent)', color: '#fff', opacity: busy ? 0.7 : 1 }}>{busy === 'save' ? 'Menyimpan…' : f.mode === 'qr' && !(wa && wa.status === 'connected') ? 'Simpan & lanjut scan QR' : 'Simpan'}</button>
        {f.testCode.trim() ? (
  <button type="button" onClick={test} disabled={!!busy} style={{ ...btn, background: 'var(--surface2)', color: 'var(--text)', boxShadow: 'inset 0 0 0 1.5px var(--borderStrong)', opacity: busy ? 0.7 : 1 }}>{busy === 'test' ? 'Mengetes…' : 'Tes Koneksi'}</button>
        ) : null}
        {msg ? <span role="status" style={{ fontSize: 13, color: msg.ok ? 'var(--muted)' : 'var(--danger)', flex: '1 1 240px' }}>{msg.text}</span> : null}
      </div>
    </form>
    {slug ? <MetaAdsSection slug={slug} /> : null}
    </div>
  );
}
