import React from 'react';
import { Av, Kpi, Loading, Page, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { daysFromNow, fmtShort, lastSeenText, monthLabel, monthShort, rp } from '../calc.js';
import { useData } from '../hooks.js';

export default function Ringkasan() {
  const { go, pending } = useApp();
  const ov = useData('/overview');
  const share = useData('/revenue-share');
  const online = useData('/customers?filter=online&per_page=8');

  if (ov.loading || ov.error || !ov.data) return <Page title="Ringkasan" sub="Kondisi bisnis Convera hari ini"><Loading error={ov.error} /></Page>;
  const { kpi, revenue_series: series, ending_soon: ending, pending_proofs: pend } = ov.data.data;
  const now = series[series.length - 1].month;
  const max = Math.max(...series.map((t) => t.revenue), kpi.projected_month, 1);
  const people = share.data?.data.people || [];

  return (
    <Page title="Ringkasan" sub={'Kondisi bisnis Convera hari ini, ' + new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })}>
      {pend.length ? (
        <div className="banner">
          <Icon name="receipt" size={22} />
          <div className="t"><b style={{ fontWeight: 600, fontSize: 16 }}>{pending} bukti transfer menunggu verifikasi</b><span>{pend.map((p) => p.customer).join(', ')}</span></div>
          <button className="btn ink" onClick={() => go('verifikasi')}>Verifikasi sekarang</button>
        </div>
      ) : null}
      <div className="grid g3">
        <Kpi label="Pendapatan bulan ini" icon="wallet" value={rp(kpi.revenue_month)} sub={'Proyeksi akhir ' + monthShort(now) + ' ' + rp(kpi.projected_month)} />
        <Kpi label="MRR" icon="trend" value={rp(kpi.mrr)} sub="Pendapatan bulanan berulang" />
        <Kpi label="Customer berlangganan" icon="users" value={`${kpi.active} / ${kpi.total}`} sub={`${kpi.trial} trial · ${kpi.expired} tidak berlangganan`} />
        <Kpi label="Online sekarang" icon="wifi" value={kpi.online} sub="Aktif dalam 5 menit terakhir" />
      </div>
      <div className="grid" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(340px, 1fr))' }}>
        <div className="card" style={{ gridColumn: 'span 1' }}>
          <div className="row between"><h2>Pendapatan per bulan</h2><div className="legend"><span><i style={{ background: 'var(--accent)' }} />Masuk</span><span><i style={{ background: 'var(--accentSoft)' }} />Proyeksi {monthShort(now)}</span></div></div>
          <div className="chartwrap">
            {series.map((t) => (
              <div className="col" key={t.month}>
                <span className="muted sm">{t.revenue ? (t.revenue / 1e6).toFixed(1).replace('.', ',') + ' jt' : '—'}</span>
                <div style={{ width: '100%', maxWidth: 64, display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', height: 140 }}>
                  {t.month === now ? <div className="bar proj" style={{ height: Math.max(0, ((kpi.projected_month - t.revenue) / max) * 140), borderRadius: '10px 10px 0 0' }} /> : null}
                  <div className={'bar' + (t.month === now ? ' now' : '')} style={{ height: Math.max(4, (t.revenue / max) * 140), borderRadius: t.month === now ? 0 : '10px 10px 0 0' }} />
                </div>
                <span className="muted sm">{monthShort(t.month)}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="card">
          <div className="row between"><h2>Bagian bulan ini</h2><span className="muted sm">{monthLabel(now)} · berjalan</span></div>
          {share.loading ? <span className="muted">Memuat…</span> : share.error ? <span style={{ color: 'var(--danger)' }}>{share.error}</span> : (
            <>
              {people.map((p, i) => (
                <div className="row between nowrap" key={p.id}><span className="row nowrap"><Av name={p.name} i={i + 1} round /><span><b style={{ fontWeight: 500 }}>{p.name}</b><div className="muted sm">Bagi rata {rp(p.share)} + komisi {rp(p.komisi)}</div></span></span><b style={{ fontWeight: 600 }}>{rp(p.total)}</b></div>
              ))}
              <div className="listrow between"><span>Kas bersama</span><b style={{ fontWeight: 600 }}>{rp(share.data.data.kas)}</b></div>
            </>
          )}
        </div>
      </div>
      <div className="grid g2">
        <div className="card">
          <h2>Berakhir dalam 14 hari</h2>
          {ending.length === 0 ? <span className="muted">Tidak ada.</span> : ending.map((c, i) => (
            <div className="listrow nowrap" key={c.id}><Av name={c.name} i={i} /><div style={{ flex: 1 }}><b style={{ fontWeight: 500 }}>{c.name}</b><div className="muted sm">{c.owner} · {c.city || '—'}</div></div><span style={{ textAlign: 'right' }}>{fmtShort(c.until)}<div className="sm" style={{ color: 'var(--accentText)' }}>{daysFromNow(c.until)} hari lagi</div></span></div>
          ))}
        </div>
        <div className="card">
          <h2>Online sekarang</h2>
          {online.loading ? <span className="muted">Memuat…</span> : (online.data?.data || []).length === 0 ? <span className="muted">Tidak ada.</span> : online.data.data.map((c, i) => (
            <div className="listrow nowrap" key={c.id}><Av name={c.name} i={i} /><div style={{ flex: 1 }}><b style={{ fontWeight: 500 }}>{c.name}</b><div className="muted sm">{c.owner}</div></div><span className="row nowrap"><span className="dot on" /><span style={{ color: 'var(--accentText)' }}>{lastSeenText(c)}</span></span></div>
          ))}
        </div>
      </div>
    </Page>
  );
}
