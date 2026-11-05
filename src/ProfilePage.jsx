import React, { useState } from 'react';
import { isPhone, checkPassword } from './services/validators.js';
import { getSub, subInfo } from './plans.js';

const field = { width: '100%', height: 42, padding: '0 12px', border: '1px solid var(--border)', borderRadius: 8, background: 'var(--input)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none', boxSizing: 'border-box' };
const card = { background: 'var(--surface2)', borderRadius: 14, padding: 24, display: 'flex', flexDirection: 'column', gap: 16 };
const label = { display: 'flex', flexDirection: 'column', gap: 7, fontSize: 13, fontWeight: 500 };
const ghost = { fontFamily: 'inherit', height: 36, padding: '0 14px', border: 0, borderRadius: 8, background: 'var(--surface)', color: 'var(--text)', fontSize: 13, cursor: 'pointer' };

function ProfileForm({ profile, onSave }) {
  const [form, setForm] = useState({ name: '', username: '', email: '', phone: '', ...profile });
  const [pw, setPw] = useState({ current: '', next: '', again: '' });
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const [saving, setSaving] = useState(false);
  const upd = (k) => (e) => setForm((v) => ({ ...v, [k]: e.target.value }));
  const updPw = (k) => (e) => setPw((v) => ({ ...v, [k]: e.target.value }));

  const submit = async (e) => {
    e.preventDefault();
    if (!form.name.trim()) return setError('Nama wajib diisi');
    if (form.phone && !isPhone(form.phone)) return setError('Nomor telepon tidak valid');
    if (pw.current || pw.next || pw.again) {
      const pe = checkPassword(pw.current, pw.next, pw.again);
      if (pe) return setError(pe);
    }
    setSaving(true);
    try {
      await onSave({ name: form.name.trim(), phone: form.phone.trim(), ...(pw.next ? { passwordUpdatedAt: new Date().toISOString() } : {}) });
    } catch (err) { setSaving(false); return setError(err.message || 'Profil tidak dapat disimpan'); }
    setSaving(false); setPw({ current: '', next: '', again: '' }); setError(''); setSaved(true);
    window.setTimeout(() => setSaved(false), 2500);
  };

  return (
    <form onSubmit={submit} style={card}>
      <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>Profil</h2>
      <label style={label}>Username<input value={form.username || ''} readOnly disabled style={{ ...field, opacity: 0.7 }} /></label>
      <label style={label}>Email<input value={form.email || ''} readOnly disabled style={{ ...field, opacity: 0.7 }} /></label>
      {[['name', 'Nama lengkap', 'text'], ['phone', 'Nomor telepon', 'tel']].map(([k, l, type]) => (
        <label key={k} style={label}>{l}<input value={form[k]} onChange={upd(k)} type={type} style={field} /></label>
      ))}
      <div style={{ borderTop: '1px solid var(--border)', paddingTop: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
        <span style={{ fontSize: 14, fontWeight: 500 }}>Ubah password</span>
        {[['current', 'Password saat ini'], ['next', 'Password baru'], ['again', 'Ulangi password baru']].map(([k, l]) => (
          <label key={k} style={label}>{l}<input value={pw[k]} onChange={updPw(k)} type="password" autoComplete="new-password" style={field} /></label>
        ))}
      </div>
      {error ? <p role="alert" style={{ margin: 0, color: 'var(--danger)', fontSize: 13 }}>{error}</p> : null}
      {saved ? <p role="status" style={{ margin: 0, color: 'var(--text)', fontSize: 13 }}>Tersimpan.</p> : null}
      <button type="submit" disabled={saving} style={{ height: 42, border: 0, borderRadius: 8, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 500, cursor: saving ? 'wait' : 'pointer', opacity: saving ? 0.7 : 1 }}>{saving ? 'Menyimpan…' : 'Simpan'}</button>
    </form>
  );
}

export default function ProfilePage({ themeStyle, profile, onSave, workspaces, activeIndex, subs, onOpen, onPlans, onBack, onLogout }) {
  const pillBg = { ok: 'var(--ink)', warn: 'var(--accentSoft)', muted: 'var(--neutral)' };
  const pillFg = { ok: 'var(--inkText)', warn: 'var(--text)', muted: 'var(--muted)' };
  return (
    <div style={{ ...themeStyle, minHeight: '100vh', background: 'var(--bg)', color: 'var(--text)' }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '14px 24px', borderBottom: '1px solid var(--border)' }}>
        <button type="button" onClick={onBack} style={{ ...ghost, background: 'transparent', paddingLeft: 0 }}>← Kembali</button>
        <button type="button" onClick={onLogout} style={{ ...ghost, color: 'var(--danger)' }}>Keluar</button>
      </header>
      <main style={{ maxWidth: 720, margin: '0 auto', padding: 24, display: 'flex', flexDirection: 'column', gap: 20 }}>
        <h1 style={{ margin: 0, fontSize: 22, fontWeight: 600 }}>Profil saya</h1>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          <ProfileForm profile={profile} onSave={onSave} />
          <section style={card}>
            <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>Workspace</h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              {workspaces.map((w, i) => {
                const info = subInfo(getSub(subs, i));
                return (
                  <div key={w.name} style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12, padding: 14, borderRadius: 12, background: 'var(--surface)' }}>
                    <span style={{ width: 36, height: 36, flex: 'none', borderRadius: 9, background: i === activeIndex ? 'var(--ink)' : 'var(--neutral)', color: i === activeIndex ? 'var(--inkText)' : 'var(--text)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 12, fontWeight: 500 }}>{w.name.split(' ').map((x) => x[0]).join('').slice(0, 2).toUpperCase()}</span>
                    <div style={{ flex: '1 1 140px', minWidth: 0 }}>
                      <div style={{ fontSize: 14, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{w.name}</div>
                      <span style={{ display: 'inline-block', marginTop: 4, padding: '2px 10px', borderRadius: 999, fontSize: 12, background: pillBg[info.tone], color: pillFg[info.tone] }}>{info.label}</span>
                    </div>
                    <div style={{ display: 'flex', gap: 6 }}>
                      <button type="button" onClick={() => onOpen(i)} style={ghost}>{i === activeIndex ? 'Dibuka' : 'Buka'}</button>
                      <button type="button" onClick={() => onPlans(i)} style={{ ...ghost, background: info.active ? 'var(--surface2)' : 'var(--accent)', color: info.active ? 'var(--text)' : '#fff' }}>{info.btn}</button>
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        </div>
      </main>
    </div>
  );
}
