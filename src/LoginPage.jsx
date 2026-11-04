import React, { useState } from 'react';
import { login } from './services/auth.js';
import { appConfig } from './services/config.js';

const field = { width: '100%', height: 44, padding: '0 12px', border: '1px solid var(--border)', borderRadius: 10, background: 'var(--input)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };

export default function LoginPage({ themeStyle, onLogin }) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try { onLogin(await login(email, password)); } catch (e) { setError(e.message || 'Login gagal'); setBusy(false); }
  };

  return (
    <div style={{ ...themeStyle, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: 'var(--bg)', color: 'var(--text)' }}>
      <form onSubmit={submit} style={{ width: 'min(400px, 100%)', background: 'var(--surface)', borderRadius: 16, padding: 32, display: 'flex', flexDirection: 'column', gap: 18, boxShadow: '0 20px 50px -25px rgba(0,0,0,.35)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <svg width="30" height="30" viewBox="0 0 32 32"><path d="M8 3h16a5 5 0 0 1 5 5v11a5 5 0 0 1-5 5h-9l-6.5 5.2a.6.6 0 0 1-1-.5V24H8a5 5 0 0 1-5-5V8a5 5 0 0 1 5-5z" fill="var(--accent)" /><path d="M9.5 18.5l4.5-4.5 3 3 5.5-5.5M18.5 11.5h4v4" fill="none" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
          <span style={{ fontSize: 22, fontWeight: 800, letterSpacing: '-0.035em' }}>Convera</span>
        </div>
        <div>
          <h1 style={{ margin: 0, fontSize: 20 }}>Masuk ke akun Anda</h1>
          <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 13 }}>Lihat performa iklan WhatsApp dan kelola event konversi.</p>
        </div>
        <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
          Email
          <input type="email" autoComplete="email" autoFocus value={email} onChange={(e) => setEmail(e.target.value)} placeholder="nama@bisnis.id" style={field} />
        </label>
        <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
          Password
          <input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} style={field} />
        </label>
        {error ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{error}</div> : null}
        <button type="submit" disabled={busy} style={{ height: 44, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 700, cursor: busy ? 'default' : 'pointer', opacity: busy ? 0.7 : 1 }}>{busy ? 'Memproses…' : 'Masuk'}</button>
        {!appConfig.backendEnabled ? <p style={{ margin: 0, color: 'var(--muted)', fontSize: 12, textAlign: 'center' }}>Mode demo: email valid + password minimal 6 karakter. Belum terhubung ke server.</p> : null}
      </form>
    </div>
  );
}
