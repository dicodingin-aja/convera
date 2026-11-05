import React, { useState } from 'react';
import { createWorkspace } from './services/backend.js';

const CATEGORIES = ['Umum', 'F&B / Kuliner', 'Klinik / Kesehatan', 'Jasa', 'Pendidikan', 'Fashion', 'Properti', 'Otomotif'];
const field = { width: '100%', height: 44, padding: '0 12px', border: '1px solid var(--border)', borderRadius: 10, background: 'var(--input)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };
const label = { display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 700 };

// Layar pertama untuk akun yang belum punya workspace (mode backend). Satu workspace = satu bisnis.
export default function CreateWorkspace({ themeStyle, userName, onCreated, onLogout }) {
  const [f, setF] = useState({ name: '', category: CATEGORIES[0], city: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (k) => (e) => { setF((x) => ({ ...x, [k]: e.target.value })); setError(''); };
  const submit = async (e) => {
    e.preventDefault();
    if (!f.name.trim()) { setError('Nama bisnis wajib diisi'); return; }
    setBusy(true);
    try { await createWorkspace({ name: f.name.trim(), category: f.category, city: f.city.trim() }); await onCreated(); } catch (err) { setError(err.message || 'Gagal membuat workspace'); setBusy(false); }
  };
  return (
    <div style={{ ...themeStyle, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20, background: 'var(--bg)', color: 'var(--text)' }}>
      <form onSubmit={submit} style={{ width: 'min(420px, 100%)', background: 'var(--surface)', borderRadius: 16, padding: 32, display: 'flex', flexDirection: 'column', gap: 16, boxShadow: '0 20px 50px -25px rgba(0,0,0,.35)' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 20 }}>Buat workspace pertama</h1>
          <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 13 }}>Halo{userName ? ', ' + userName : ''}! Satu workspace mewakili satu bisnis. Trial gratis dimulai otomatis.</p>
        </div>
        <label style={label}>Nama bisnis<input autoFocus value={f.name} onChange={set('name')} maxLength={120} placeholder="mis. Kopi Senja" style={field} /></label>
        <label style={label}>Kategori<select value={f.category} onChange={set('category')} style={field}>{CATEGORIES.map((c) => <option key={c}>{c}</option>)}</select></label>
        <label style={label}>Kota<input value={f.city} onChange={set('city')} maxLength={80} placeholder="mis. Jakarta Selatan" style={field} /></label>
        {error ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{error}</div> : null}
        <button type="submit" disabled={busy} style={{ height: 44, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 700, cursor: busy ? 'default' : 'pointer', opacity: busy ? 0.7 : 1 }}>{busy ? 'Membuat…' : 'Buat workspace'}</button>
        <button type="button" onClick={onLogout} style={{ border: 0, background: 'none', color: 'var(--muted)', fontFamily: 'inherit', fontSize: 13, cursor: 'pointer' }}>Keluar</button>
      </form>
    </div>
  );
}
