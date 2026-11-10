import React, { useState } from 'react';
import { createWorkspace } from './services/backend.js';

// Kartu "Tambah workspace" di Dashboard (mode backend), seperti "Tambah Proyek Baru" di PixelAD: cukup nama bisnis.
// Satu workspace = satu bisnis. Trial gratis hanya untuk workspace pertama; workspace berikutnya wajib dibayar.
// first = akun belum punya workspace (tanpa tombol Batal).
export default function AddWorkspaceCard({ userName, first, onCreated, onCancel }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e) => {
    e.preventDefault();
    const n = name.trim();
    if (!n) { setError('Nama bisnis wajib diisi'); return; }
    setBusy(true);
    try { const created = await createWorkspace({ name: n }); await onCreated(created); } catch (err) { setError(err.message || 'Gagal membuat workspace'); setBusy(false); }
  };
  return (
    <form onSubmit={submit} style={{ border: '1px dashed var(--border)', borderRadius: 14, padding: 24, display: 'flex', flexDirection: 'column', gap: 14, background: 'var(--surface2)' }}>
      <div>
        <div style={{ fontSize: 15, fontWeight: 700, color: 'var(--accent)' }}>+ {first ? 'Tambah workspace pertama' : 'Tambah workspace baru'}</div>
        {first ? <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 13 }}>Halo{userName ? ', ' + userName : ''}! Satu workspace untuk satu bisnis. Trial gratis langsung aktif.</p>
          : <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 13 }}>Trial gratis hanya untuk workspace pertama. Workspace ini perlu berlangganan dulu sebelum dipakai.</p>}
      </div>
      <label style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 12, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--muted)' }}>
        NAMA WORKSPACE
        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
          <input autoFocus value={name} onChange={(e) => { setName(e.target.value); setError(''); }} maxLength={120} placeholder="Nama bisnis kamu"
            style={{ flex: '1 1 220px', minWidth: 0, height: 44, padding: '0 14px', border: '1px solid var(--border)', borderRadius: 10, background: 'var(--input)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 14, outline: 'none' }} />
          <button type="submit" disabled={busy} style={{ height: 44, padding: '0 20px', border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 700, cursor: busy ? 'default' : 'pointer', opacity: busy ? 0.7 : 1 }}>{busy ? 'Menambah…' : 'Tambah →'}</button>
          {!first ? <button type="button" onClick={onCancel} style={{ height: 44, padding: '0 14px', border: 0, background: 'none', color: 'var(--muted)', fontFamily: 'inherit', fontSize: 13, cursor: 'pointer' }}>Batal</button> : null}
        </div>
      </label>
      {error ? <div role="alert" style={{ color: 'var(--danger)', fontSize: 13, fontWeight: 600 }}>{error}</div> : null}
    </form>
  );
}
