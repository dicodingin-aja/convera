import React from 'react';
import { PLANS, rupiah } from './plans.js';

export default function PlansModal({ themeStyle, workspace, renew, onPick, onClose }) {
  return (
    <div role="presentation" onMouseDown={onClose} style={{ ...themeStyle, position: 'fixed', inset: 0, zIndex: 90, background: 'rgba(0,0,0,.35)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
      <section role="dialog" aria-modal="true" aria-labelledby="plans-title" onMouseDown={(e) => e.stopPropagation()} style={{ ...themeStyle, width: 'min(720px, 100%)', minHeight: 0, maxHeight: '100%', overflowY: 'auto', background: 'var(--surface)', color: 'var(--text)', borderRadius: 16, padding: 28, display: 'flex', flexDirection: 'column', gap: 20 }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div>
            <h2 id="plans-title" style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>Pilih paket</h2>
            <p style={{ margin: '6px 0 0', color: 'var(--muted)', fontSize: 14 }}>{workspace}</p>
          </div>
          <button type="button" onClick={onClose} aria-label="Tutup" style={{ border: 0, background: 'var(--surface2)', color: 'var(--text)', borderRadius: 8, width: 34, height: 34, cursor: 'pointer', fontSize: 20, lineHeight: 1 }}>×</button>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 12 }}>
          {PLANS.map((p) => (
            <div key={p.id} style={{ display: 'flex', flexDirection: 'column', gap: 14, padding: 20, borderRadius: 14, background: 'var(--surface2)' }}>
              <span style={{ fontSize: 16, fontWeight: 600 }}>{p.name}</span>
              <div><span style={{ fontSize: 26, fontWeight: 600 }}>{rupiah(p.price)}</span><span style={{ color: 'var(--muted)', fontSize: 13 }}> / {p.period}</span></div>
              <ul style={{ margin: 0, padding: 0, listStyle: 'none', display: 'flex', flexDirection: 'column', gap: 8, fontSize: 14 }}>
                {p.features.map((f) => <li key={f}>✓ {f}</li>)}
              </ul>
              <button type="button" onClick={() => onPick(p)} style={{ marginTop: 'auto', height: 44, border: 0, borderRadius: 10, background: 'var(--accent)', color: '#fff', fontFamily: 'inherit', fontSize: 14, fontWeight: 500, cursor: 'pointer' }}>{renew ? 'Perpanjang' : 'Pilih paket'}</button>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
