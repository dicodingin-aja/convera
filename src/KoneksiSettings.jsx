import React, { useState } from 'react';
import { appConfig } from './services/config.js';
import { isDatasetId, isToken } from './services/validators.js';
import KoneksiBackend from './KoneksiBackend.jsx';

export const CONN_DEFAULT = { category: 'Umum', waMode: 'Kirim.chat', waType: 'waba', pixelId: '', token: '', testCode: '', secret: '' };
const CATEGORIES = ['Umum', 'F&B / Kuliner', 'Klinik / Kesehatan', 'Jasa', 'Pendidikan', 'Fashion', 'Properti', 'Otomotif'];
const WA_MODES = ['Kirim.chat', 'WhatsApp Cloud API (resmi)'];

const card = { background: 'var(--surface2)', borderRadius: 14, padding: 24, display: 'flex', flexDirection: 'column', gap: 18 };
const field = { width: '100%', height: 46, padding: '0 14px', border: 'none', borderRadius: 12, background: 'var(--surface)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };
const label = { display: 'flex', flexDirection: 'column', gap: 8, fontSize: 13, fontWeight: 500, minWidth: 0 };
const err = { fontSize: 12, color: 'var(--danger)', fontWeight: 400 };
const h2 = { margin: 0, fontSize: 16, fontWeight: 600 };

function Secret({ value, onChange, placeholder, invalid }) {
  const [show, setShow] = useState(false);
  return (
    <div style={{ position: 'relative' }}>
      <input value={value} onChange={onChange} type={show ? 'text' : 'password'} autoComplete="off" spellCheck="false" placeholder={placeholder} aria-invalid={invalid || undefined} style={{ ...field, paddingRight: 110 }} />
      <button type="button" onClick={() => setShow((v) => !v)} style={{ position: 'absolute', right: 8, top: 8, height: 30, padding: '0 10px', border: 'none', borderRadius: 8, background: 'var(--surface2)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 12, cursor: 'pointer' }}>{show ? 'Sembunyikan' : 'Tampilkan'}</button>
    </div>
  );
}

// Mode backend memakai KoneksiBackend (token disimpan terenkripsi di server, bukan di browser).
export default function KoneksiSettings(props) {
  return appConfig.backendEnabled ? <KoneksiBackend workspace={props.workspace} onSave={props.onSave} onScanQr={props.onScanQr} /> : <KoneksiDemo {...props} />;
}

function KoneksiDemo({ workspace, value, onSave }) {
  const [f, setF] = useState({ ...CONN_DEFAULT, ...value });
  const [errors, setErrors] = useState({});
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  const set = (k) => (e) => { setF((v) => ({ ...v, [k]: e.target.value })); setSaved(false); };
  const pick = (k, v) => () => { setF((x) => ({ ...x, [k]: v })); setSaved(false); };

  const slug = workspace.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  const webhookUrl = `${appConfig.webhookBase}/${slug}`;

  const submit = (e) => {
    e.preventDefault();
    const next = {};
    if (!isDatasetId(f.pixelId)) next.pixelId = 'Pixel ID berisi 15–16 digit angka';
    if (!isToken(f.token)) next.token = 'Access token tidak valid';
    setErrors(next);
    if (Object.keys(next).length) return;
    onSave({ ...f, pixelId: f.pixelId.trim(), token: f.token.trim(), testCode: f.testCode.trim() });
    setSaved(true);
    window.setTimeout(() => setSaved(false), 2500);
  };

  const copy = () => {
    try { navigator.clipboard.writeText(webhookUrl); } catch { /* diabaikan */ }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  const radio = (on) => ({ fontFamily: 'inherit', textAlign: 'left', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, padding: 14, borderRadius: 12, border: 'none', cursor: 'pointer', background: 'var(--surface)', color: 'var(--text)', fontSize: 14, fontWeight: 500, boxShadow: on ? 'inset 0 0 0 2px var(--accent)' : 'none' });
  const dot = (on) => ({ width: 18, height: 18, flex: 'none', borderRadius: '50%', background: on ? 'var(--accent)' : 'var(--surface2)', boxShadow: on ? 'inset 0 0 0 4px var(--surface)' : 'inset 0 0 0 1.5px var(--borderStrong)' });
  const isWaba = f.waType === 'waba';

  return (
    <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 16, maxWidth: 760 }}>
      <section style={card}>
        <h2 style={h2}>Meta CAPI</h2>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 14 }}>
          <label style={label}>Kategori bisnis
            <select value={f.category} onChange={set('category')} style={field}>{CATEGORIES.map((c) => <option key={c}>{c}</option>)}</select>
          </label>
          {isWaba ? (
            <label style={label}>Mode WhatsApp
              <select value={f.waMode} onChange={set('waMode')} style={field}>{WA_MODES.map((c) => <option key={c}>{c}</option>)}</select>
            </label>
          ) : null}
        </div>
        <div style={label}>Jenis koneksi WhatsApp
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 10 }}>
            <button type="button" onClick={pick('waType', 'waba')} style={radio(isWaba)}>WABA (resmi)<span style={dot(isWaba)} /></button>
            <button type="button" onClick={pick('waType', 'qr')} style={radio(!isWaba)}>WA Bisnis (scan QR)<span style={dot(!isWaba)} /></button>
          </div>
        </div>
        <label style={label}>Pixel ID
          <input value={f.pixelId} onChange={set('pixelId')} inputMode="numeric" placeholder="123456789123456789" aria-invalid={errors.pixelId ? true : undefined} style={field} />
          {errors.pixelId ? <span style={err}>{errors.pixelId}</span> : null}
        </label>
        <label style={label}>Access token CAPI
          <Secret value={f.token} onChange={set('token')} placeholder="EAAB…" invalid={!!errors.token} />
          {errors.token ? <span style={err}>{errors.token}</span> : null}
        </label>
        <label style={label}>Test event code
          <input value={f.testCode} onChange={set('testCode')} placeholder="TEST12345" style={field} />
        </label>
      </section>

      {isWaba ? (
        <section style={card}>
          <h2 style={h2}>{f.waMode === 'Kirim.chat' ? 'Kirim.chat' : 'WhatsApp Cloud API'}</h2>
          <div style={label}>Webhook URL
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              <input readOnly value={webhookUrl} onFocus={(e) => e.target.select()} style={{ ...field, flex: '1 1 260px', fontFamily: 'ui-monospace, Menlo, Consolas, monospace', fontSize: 13 }} />
              <button type="button" onClick={copy} style={{ flex: 'none', height: 46, padding: '0 18px', border: 'none', borderRadius: 12, background: 'var(--ink)', color: 'var(--inkText)', fontFamily: 'inherit', fontSize: 14, fontWeight: 500, cursor: 'pointer' }}>{copied ? 'Tersalin' : 'Salin'}</button>
            </div>
          </div>
          <label style={label}>Webhook secret
            <Secret value={f.secret} onChange={set('secret')} />
          </label>
        </section>
      ) : (
        <section style={card}>
          <h2 style={h2}>WhatsApp Bisnis</h2>
          <div style={{ width: 168, height: 168, borderRadius: 12, background: 'var(--surface)', boxShadow: 'inset 0 0 0 1.5px var(--borderStrong)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--muted)', fontSize: 13 }}>QR</div>
        </section>
      )}

      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <button type="submit" style={{ height: 46, padding: '0 24px', border: 'none', borderRadius: 12, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 500, cursor: 'pointer' }}>Simpan</button>
        {saved ? <span role="status" style={{ fontSize: 13, color: 'var(--muted)' }}>Tersimpan</span> : null}
      </div>
    </form>
  );
}
