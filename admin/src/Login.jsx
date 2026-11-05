import React, { useState } from 'react';
import { login } from './auth.js';
import { Logo } from './Icon.jsx';

export default function Login({ onLogin, notice }) {
  const [user, setUser] = useState('');
  const [pw, setPw] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e) => {
    e.preventDefault(); setBusy(true); setError('');
    try { onLogin(await login(user, pw)); } catch (err) { setError(err.message); setBusy(false); }
  };
  return (
    <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
      <form onSubmit={submit} className="card" style={{ width: 'min(400px, 100%)', padding: 32, gap: 18, background: 'var(--surface2)' }}>
        <div className="brand" style={{ padding: 0 }}><Logo />Convera <span className="tag">ADMIN</span></div>
        <h2>Masuk</h2>
        <label className="f">Username<input className="input" autoFocus autoComplete="username" value={user} onChange={(e) => setUser(e.target.value)} /></label>
        <label className="f">Password<input className="input" type="password" autoComplete="current-password" value={pw} onChange={(e) => setPw(e.target.value)} /></label>
        {error || notice ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13 }}>{error || notice}</div> : null}
        <button className="btn primary" style={{ height: 44 }} disabled={busy}>{busy ? 'Memproses…' : 'Masuk'}</button>
      </form>
    </div>
  );
}
