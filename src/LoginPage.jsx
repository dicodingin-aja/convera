import React, { useState } from 'react';
import { login, register } from './services/auth.js';
import { appConfig } from './services/config.js';

const field = { width: '100%', height: 44, padding: '0 12px', border: '1px solid var(--border)', borderRadius: 10, background: 'var(--input)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };

export default function LoginPage({ themeStyle, onLogin }) {
  const read = (k) => { try { return sessionStorage.getItem(k) || ''; } catch { return ''; } };
  const [info] = useState(() => read('convera:notice'));
  // login | register (register hanya di mode backend)
  const [mode, setMode] = useState('login');
  const [name, setName] = useState('');
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      onLogin(mode === 'register' ? await register({ name, username, email, phone, password }) : await login(username, password));
    } catch (e) { setError(e.message || 'Login gagal'); setBusy(false); }
  };

  return (
    <div style={{ ...themeStyle, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: 'var(--bg)', color: 'var(--text)' }}>
      <form onSubmit={submit} style={{ width: 'min(400px, 100%)', background: 'var(--surface)', borderRadius: 16, padding: 32, display: 'flex', flexDirection: 'column', gap: 18, boxShadow: '0 20px 50px -25px rgba(0,0,0,.35)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <svg width="30" height="30" viewBox="0 0 32 32"><path d="M8 3h16a5 5 0 0 1 5 5v11a5 5 0 0 1-5 5h-9l-6.5 5.2a.6.6 0 0 1-1-.5V24H8a5 5 0 0 1-5-5V8a5 5 0 0 1 5-5z" fill="var(--accent)" /><path d="M9.5 18.5l4.5-4.5 3 3 5.5-5.5M18.5 11.5h4v4" fill="none" stroke="#fff" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
          <span style={{ fontSize: 22, fontWeight: 800, letterSpacing: '-0.035em' }}>Convera</span>
        </div>
        <div>
          <h1 style={{ margin: 0, fontSize: 20 }}>{mode === 'register' ? 'Buat akun baru' : 'Masuk ke akun Anda'}</h1>
          <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 13 }}>Lihat performa iklan WhatsApp dan kelola event konversi.</p>
        </div>
        {mode === 'register' ? (
          <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
            Nama lengkap
            <input autoComplete="name" autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Nama Anda" style={field} />
          </label>
        ) : null}
        <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
          Username
          <input autoComplete="username" autoCapitalize="none" spellCheck={false} autoFocus={mode === 'login'} value={username} onChange={(e) => setUsername(e.target.value)} placeholder="namabisnis" style={field} />
        </label>
        {mode === 'register' ? (
          <>
            <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
              Email
              <input type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="nama@bisnis.id" style={field} />
            </label>
            <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
              Nomor telepon / WhatsApp
              <input type="tel" autoComplete="tel" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="0812-3456-7890" style={field} />
            </label>
          </>
        ) : null}
        <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 }}>
          Password
          <input type="password" autoComplete={mode === 'register' ? 'new-password' : 'current-password'} value={password} onChange={(e) => setPassword(e.target.value)} style={field} />
        </label>
        {info ? <div role="status" style={{ color: 'var(--accent)', fontSize: 13, fontWeight: 600 }}>{info}</div> : null}
        {error ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{error}</div> : null}
        <button type="submit" disabled={busy} style={{ height: 44, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 700, cursor: busy ? 'default' : 'pointer', opacity: busy ? 0.7 : 1 }}>{busy ? 'Memproses…' : mode === 'register' ? 'Daftar' : 'Masuk'}</button>
        {appConfig.backendEnabled ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, alignItems: 'center' }}>
            <button type="button" onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError(''); setInfo(''); }} style={{ border: 0, background: 'none', color: 'var(--accent)', fontFamily: 'inherit', fontSize: 13, fontWeight: 600, cursor: 'pointer' }}>{mode === 'login' ? 'Belum punya akun? Daftar' : 'Kembali ke halaman masuk'}</button>
            {mode === 'login' ? <span style={{ color: 'var(--muted)', fontSize: 12 }}>Lupa password? Hubungi tim Convera.</span> : null}
          </div>
        ) : null}
        {!appConfig.backendEnabled ? <p style={{ margin: 0, color: 'var(--muted)', fontSize: 12, textAlign: 'center' }}>Mode demo: username apa saja + password minimal 6 karakter. Belum terhubung ke server.</p> : null}
      </form>
    </div>
  );
}
