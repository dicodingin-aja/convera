import React, { useState } from 'react';
import { Av, Kpi, Page, useApp } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { NAMES, fmtShort, lastMonths, monthKey, monthLabel, monthTotals, rp, split } from '../calc.js';
import { markPaid } from '../store.js';
import { downloadCsv } from '../ui.jsx';

export default function BagiHasil() {
  const { s, update } = useApp();
  const months = lastMonths(3).reverse();
  const [m, setM] = useState(months[0]);
  const t = monthTotals(s.payments, m, s.komisiPct, s.kasPct);
  const paid = s.paid[m] || {};
  const rows = s.payments.filter((p) => monthKey(p.date) === m).sort((a, b) => new Date(b.date) - new Date(a.date));
  const name = (id) => s.customers.find((c) => c.id === id)?.name || id;
  const rest = 100 - s.komisiPct - s.kasPct;
  const equal = t.revenue - Object.values(t.komisi).reduce((a, b) => a + b, 0) - t.kas;
  const komisiTotal = Object.values(t.komisi).reduce((a, b) => a + b, 0);
  const share = (n) => (t.revenue ? ((t.per[n] - t.komisi[n]) / t.revenue) * 100 : 0);
  const csv = () => downloadCsv('bagi-hasil-' + m + '.csv', [['Tanggal', 'Customer', 'Sales', 'Nominal', ...NAMES, 'Kas'], ...rows.map((p) => { const sp = split(p.amount, p.sales, s.komisiPct, s.kasPct); return [fmtShort(p.date), name(p.customerId), p.sales, p.amount, ...NAMES.map((n) => sp.per[n]), sp.kas]; })]);

  return (
    <Page title="Bagi Hasil" sub="Pendapatan & pembagian Jordi, Axl, Albert per bulan">
      <div className="row between">
        <div className="chips">{months.map((k, i) => <button key={k} className={'chip' + (m === k ? ' on' : '')} onClick={() => setM(k)}>{monthLabel(k)}{i === 0 ? ' · berjalan' : ''}</button>)}</div>
        <button className="btn soft" onClick={csv}><Icon name="download" size={18} />Ekspor CSV</button>
      </div>
      <div className="rule">
        <span className="caps">Setiap pembayaran</span>
        <span className="p"><b style={{ fontWeight: 600, color: 'var(--accentText)' }}>{s.komisiPct}%</b>Komisi ke Sales yang mendaftarkan</span>
        <span className="p"><b style={{ fontWeight: 600, color: 'var(--accentText)' }}>{s.kasPct}%</b>Kas Bersama</span>
        <span className="p"><b style={{ fontWeight: 600, color: 'var(--accentText)' }}>{rest}%</b>Dibagi rata Jordi · Axl · Albert</span>
      </div>
      <div className="grid g4">
        <Kpi label="Total pendapatan" value={rp(t.revenue)} sub={t.count + ' pembayaran'} />
        <Kpi label="Komisi sales" value={rp(komisiTotal)} sub={s.komisiPct + '% dari total'} />
        <Kpi label="Kas Bersama" value={rp(t.kas)} sub={s.kasPct + '% + sisa pembulatan'} />
        <Kpi label="Dibagi rata" value={rp(equal - 0)} sub={rp(equal / 3) + ' per orang'} />
      </div>
      <div className="grid g3">
        {NAMES.map((n, i) => (
          <div className="card" key={n} style={{ gap: 14 }}>
            <div className="row between nowrap">
              <span className="row nowrap"><Av name={n} i={i + 1} round /><span><b style={{ fontWeight: 600 }}>{n}</b><div className="muted sm">Co-founder</div></span></span>
              {paid[n] ? <span className="pill ink">Sudah ditransfer</span> : null}
            </div>
            <span className="kpi"><span className="big" style={{ fontSize: 28, fontWeight: 600, letterSpacing: '-.02em' }}>{rp(t.per[n])}</span></span>
            <div className="row between sm"><span className="muted">Bagi rata {share(n).toFixed(1).replace('.', ',')}%</span><b style={{ fontWeight: 500 }}>{rp(t.per[n] - t.komisi[n])}</b></div>
            <div className="row between sm"><span className="muted">Komisi dari {rows.filter((p) => p.sales === n).length} customer</span><b style={{ fontWeight: 500 }}>{rp(t.komisi[n])}</b></div>
            <button className={'btn' + (paid[n] ? ' soft' : '')} onClick={() => update((x) => markPaid(x, m, n, !paid[n]))}>{paid[n] ? 'Batalkan tanda' : 'Tandai sudah ditransfer'}</button>
          </div>
        ))}
      </div>
      <h2>Rincian per pembayaran</h2>
      <div className="tablewrap">
        <table>
          <thead><tr><th>Tanggal</th><th>Customer</th><th>Sales</th><th className="right">Nominal</th>{NAMES.map((n) => <th key={n} className="right">{n}</th>)}<th className="right">Kas</th></tr></thead>
          <tbody>
            {rows.map((p) => { const sp = split(p.amount, p.sales, s.komisiPct, s.kasPct); return (
              <tr key={p.id}><td>{fmtShort(p.date)}</td><td>{name(p.customerId)}</td><td>{p.sales}</td><td className="right">{rp(p.amount)}</td>
                {NAMES.map((n) => <td key={n} className="right">{rp(sp.per[n])}</td>)}<td className="right">{rp(sp.kas)}</td></tr>
            ); })}
            {rows.length === 0 ? <tr><td colSpan="8" className="muted">Belum ada pembayaran di bulan ini.</td></tr> : null}
          </tbody>
        </table>
      </div>
    </Page>
  );
}
