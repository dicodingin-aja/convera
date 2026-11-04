import React, { createContext, useContext } from 'react';
import { Icon } from './Icon.jsx';

export const Ctx = createContext(null);
export const useApp = () => useContext(Ctx);

export const Toggle = ({ on, onChange, label }) => (
  <button type="button" role="switch" aria-checked={on} aria-label={label} className={'toggle' + (on ? ' on' : '')} onClick={() => onChange(!on)}><i /></button>
);
export const Field = ({ label, children }) => <label className="f">{label}{children}</label>;
export const Unit = ({ value, onChange, unit }) => <div className="unit"><input inputMode="numeric" value={value} onChange={onChange} /><span>{unit}</span></div>;

export function Page({ title, sub, children }) {
  const { s, go } = useApp();
  const pending = s.proofs.filter((p) => p.status === 'pending').length;
  return (
    <>
      <div className="top">
        <div><h1>{title}</h1>{sub ? <p>{sub}</p> : null}</div>
        <div className="row nowrap">
          <button className="bellbtn" aria-label="Verifikasi pembayaran" onClick={() => go('verifikasi')}><Icon name="bell" size={19} />{pending ? <i>{pending}</i> : null}</button>
          <span className="av ink round" style={{ width: 40, height: 40 }}>A</span>
        </div>
      </div>
      <div className="body">{children}</div>
    </>
  );
}

export const Tabs = ({ value, onChange, items }) => (
  <div className="tabs">{items.map(([k, l]) => <button key={k} className={value === k ? 'on' : ''} onClick={() => onChange(k)}>{l}</button>)}</div>
);

export function Modal({ title, onClose, wide, children }) {
  return (
    <div className="overlay" role="presentation" onMouseDown={onClose}>
      <section className={'modal' + (wide ? ' wide' : '')} role="dialog" aria-modal="true" onMouseDown={(e) => e.stopPropagation()}>
        <div className="row between nowrap"><h2>{title}</h2><button className="iconbtn" onClick={onClose} aria-label="Tutup"><Icon name="x" size={18} /></button></div>
        {children}
      </section>
    </div>
  );
}

export const Kpi = ({ label, value, sub, icon }) => (
  <div className="card kpi"><div className="lbl"><span>{label}</span>{icon ? <Icon name={icon} size={18} /> : null}</div><span className="big">{value}</span>{sub ? <span className="muted sm">{sub}</span> : null}</div>
);

const AV = ['solid', 'soft', '', 'ink'];
export const initials = (n) => n.split(' ').map((x) => x[0]).join('').slice(0, 2).toUpperCase();
export const Av = ({ name, i = 0, round }) => <span className={'av ' + AV[i % AV.length] + (round ? ' round' : '')}>{initials(name)}</span>;

export const statusInfo = (c) => ({ active: ['Berlangganan', 'ink'], trial: ['Trial', 'soft'], expired: ['Berakhir', 'accent'], pending: ['Menunggu', 'soft'] }[c.status]);
export const Pill = ({ tone = '', children }) => <span className={'pill ' + tone}>{children}</span>;

export function downloadCsv(name, rows) {
  const esc = (v) => '"' + String(v ?? '').replace(/"/g, '""') + '"';
  const blob = new Blob([rows.map((r) => r.map(esc).join(',')).join('\n')], { type: 'text/csv;charset=utf-8' });
  const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = name; a.click(); URL.revokeObjectURL(a.href);
}
