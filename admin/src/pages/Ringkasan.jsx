import React from 'react';
import { Av, Kpi, Page, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { NAMES, daysFromNow, fmtShort, lastMonths, monthKey, monthLabel, monthShort, monthTotals, rp } from '../calc.js';

export default function Ringkasan() {
  const { s, go } = useApp();
  const now = monthKey(new Date());
  const months = lastMonths(7);
  const totals = months.map((m) => monthTotals(s.payments, m, s.komisiPct, s.kasPct));
  const cur = totals[totals.length - 1];
  const active = s.customers.filter((c) => c.status === 'active');
  const mrr = active.reduce((a, c) => a + (s.plans.find((p) => p.id === c.plan)?.price || 0), 0);
  // Proyeksi akhir bulan = yang sudah masuk + perpanjangan aktif yang jatuh tempo sisa bulan ini.
  const end = new Date(new Date().getFullYear(), new Date().getMonth() + 1, 0, 23, 59);
  const upcoming = active.filter((c) => new Date(c.until) <= end).reduce((a, c) => a + (s.plans.find((p) => p.id === c.plan)?.price || 0), 0);
  const projected = cur.revenue + upcoming;
  const max = Math.max(...totals.map((t) => t.revenue), projected, 1);
  const trial = s.customers.filter((c) => c.status === 'trial').length;
  const off = s.customers.filter((c) => c.status === 'expired').length;
  const online = s.customers.filter((c) => c.online !== null);
  const ending = active.filter((c) => daysFromNow(c.until) <= 14 && daysFromNow(c.until) >= 0).sort((a, b) => new Date(a.until) - new Date(b.until));
  const pend = s.proofs.filter((p) => p.status === 'pending');
  const cust = (id) => s.customers.find((c) => c.id === id);

  return (
    <Page title="Ringkasan" sub={'Kondisi bisnis Convera hari ini, ' + new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })}>
      {pend.length ? (
        <div className="banner">
          <Icon name="receipt" size={22} />
          <div className="t"><b style={{ fontWeight: 600, fontSize: 16 }}>{pend.length} bukti transfer menunggu verifikasi</b><span>{pend.map((p) => cust(p.customerId).name).join(', ')}</span></div>
          <button className="btn ink" onClick={() => go('verifikasi')}>Verifikasi sekarang</button>
        </div>
      ) : null}
      <div className="grid g3">
        <Kpi label="Pendapatan bulan ini" icon="wallet" value={rp(cur.revenue)} sub={'Proyeksi akhir ' + monthShort(now) + ' ' + rp(projected)} />
        <Kpi label="MRR" icon="trend" value={rp(mrr)} sub="Pendapatan bulanan berulang" />
        <Kpi label="Customer berlangganan" icon="users" value={`${active.length} / ${s.customers.length}`} sub={`${trial} trial · ${off} tidak berlangganan`} />
        <Kpi label="Online sekarang" icon="wifi" value={online.length} sub="Aktif dalam 5 menit terakhir" />
      </div>
      <div className="grid" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(340px, 1fr))' }}>
        <div className="card" style={{ gridColumn: 'span 1' }}>
          <div className="row between"><h2>Pendapatan per bulan</h2><div className="legend"><span><i style={{ background: 'var(--accent)' }} />Masuk</span><span><i style={{ background: 'var(--accentSoft)' }} />Proyeksi {monthShort(now)}</span></div></div>
          <div className="chartwrap">
            {months.map((m, i) => (
              <div className="col" key={m}>
                <span className="muted sm">{totals[i].revenue ? (totals[i].revenue / 1e6).toFixed(1).replace('.', ',') + ' jt' : '—'}</span>
                <div style={{ width: '100%', maxWidth: 64, display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', height: 140 }}>
                  {m === now ? <div className="bar proj" style={{ height: Math.max(0, ((projected - cur.revenue) / max) * 140), borderRadius: '10px 10px 0 0' }} /> : null}
                  <div className={'bar' + (m === now ? ' now' : '')} style={{ height: Math.max(4, (totals[i].revenue / max) * 140), borderRadius: m === now ? 0 : '10px 10px 0 0' }} />
                </div>
                <span className="muted sm">{monthShort(m)}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="card">
          <div className="row between"><h2>Bagian bulan ini</h2><span className="muted sm">{monthLabel(now)} · berjalan</span></div>
          {NAMES.map((n, i) => (
            <div className="row between nowrap" key={n}><span className="row nowrap"><Av name={n} i={i + 1} round /><span><b style={{ fontWeight: 500 }}>{n}</b><div className="muted sm">Bagi rata {rp(cur.per[n] - cur.komisi[n])} + komisi {rp(cur.komisi[n])}</div></span></span><b style={{ fontWeight: 600 }}>{rp(cur.per[n])}</b></div>
          ))}
          <div className="listrow between"><span>Kas bersama</span><b style={{ fontWeight: 600 }}>{rp(cur.kas)}</b></div>
        </div>
      </div>
      <div className="grid g2">
        <div className="card">
          <h2>Berakhir dalam 14 hari</h2>
          {ending.length === 0 ? <span className="muted">Tidak ada.</span> : ending.map((c, i) => (
            <div className="listrow nowrap" key={c.id}><Av name={c.name} i={i} /><div style={{ flex: 1 }}><b style={{ fontWeight: 500 }}>{c.name}</b><div className="muted sm">{c.owner} · {c.city}</div></div><span style={{ textAlign: 'right' }}>{fmtShort(c.until)}<div className="sm" style={{ color: daysFromNow(c.until) <= 7 ? 'var(--accentText)' : 'var(--muted)' }}>{daysFromNow(c.until)} hari lagi</div></span></div>
          ))}
        </div>
        <div className="card">
          <h2>Online sekarang</h2>
          {online.length === 0 ? <span className="muted">Tidak ada.</span> : online.map((c, i) => (
            <div className="listrow nowrap" key={c.id}><Av name={c.name} i={i} /><div style={{ flex: 1 }}><b style={{ fontWeight: 500 }}>{c.name}</b><div className="muted sm">{c.owner}</div></div><span className="row nowrap"><span className="dot on" /><span style={{ color: 'var(--accentText)' }}>{c.online === 0 ? 'Online' : c.online + ' mnt lalu'}</span></span></div>
          ))}
        </div>
      </div>
    </Page>
  );
}
