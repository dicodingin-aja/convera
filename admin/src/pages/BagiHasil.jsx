import React, { useState } from 'react';
import { Av, ErrorNote, Kpi, Loading, Page } from '../ui.jsx';
import { Icon } from '../Icon.jsx';
import { api, download } from '../api.js';
import { fmtShort, lastMonths, monthLabel, rp } from '../calc.js';
import { useAction, useData } from '../hooks.js';

// Semua angka dihitung server dari tabel payments (memakai snapshot persentase tiap pembayaran).
export default function BagiHasil() {
  const months = lastMonths(3).reverse();
  const [m, setM] = useState(months[0]);
  const q = useData('/revenue-share?month=' + m);
  const rules = useData('/subscription-rules');
  const { busy, error, run } = useAction();
  const [csvErr, setCsvErr] = useState('');

  const rep = q.data?.data;
  const komisiPct = rules.data?.data.komisi_pct;
  const kasPct = rules.data?.data.kas_pct;
  const rest = komisiPct != null ? 100 - komisiPct - kasPct : null;
  const founders = rep ? rep.people.filter((p) => p.is_founder) : [];
  const komisiTotal = rep ? rep.people.reduce((a, p) => a + p.komisi, 0) : 0;
  const equal = rep ? rep.people.reduce((a, p) => a + p.share, 0) : 0;
  const toggle = (p) => run(async () => { await api.post(`/revenue-share/${m}/${p.id}/mark-paid`, { paid: !p.paid }); q.reload(); });

  return (
    <Page title="Bagi Hasil" sub="Pendapatan & pembagian per bulan">
      <div className="row between">
        <div className="chips">{months.map((k, i) => <button key={k} className={'chip' + (m === k ? ' on' : '')} onClick={() => setM(k)}>{monthLabel(k)}{i === 0 ? ' · berjalan' : ''}</button>)}</div>
        <button className="btn soft" onClick={() => download('/revenue-share.csv?month=' + m, 'bagi-hasil-' + m + '.csv').then(() => setCsvErr(''), (e) => setCsvErr(e.message))}><Icon name="download" size={18} />Ekspor CSV</button>
      </div>
      <ErrorNote>{csvErr || error}</ErrorNote>
      {komisiPct != null ? (
        <div className="rule">
          <span className="caps">Setiap pembayaran (aturan saat ini)</span>
          <span className="p"><b style={{ fontWeight: 600, color: 'var(--accentText)' }}>{komisiPct}%</b>Komisi ke Sales yang mendaftarkan</span>
          <span className="p"><b style={{ fontWeight: 600, color: 'var(--accentText)' }}>{kasPct}%</b>Kas Bersama</span>
          <span className="p"><b style={{ fontWeight: 600, color: 'var(--accentText)' }}>{rest}%</b>Dibagi rata {founders.map((f) => f.name).join(' · ') || 'founder'}</span>
        </div>
      ) : null}
      {q.error || !rep ? <Loading error={q.error} /> : (
        <>
          <div className="grid g4">
            <Kpi label="Total pendapatan" value={rp(rep.revenue)} sub={rep.payments_count + ' pembayaran'} />
            <Kpi label="Komisi sales" value={rp(komisiTotal)} sub="Mengikuti persentase tiap pembayaran" />
            <Kpi label="Kas Bersama" value={rp(rep.kas)} sub="Termasuk sisa pembulatan" />
            <Kpi label="Dibagi rata" value={rp(equal)} sub={founders.length ? rp(equal / founders.length) + ' per orang' : '—'} />
          </div>
          <div className="grid g3">
            {rep.people.map((p, i) => (
              <div className="card" key={p.id} style={{ gap: 14 }}>
                <div className="row between nowrap">
                  <span className="row nowrap"><Av name={p.name} i={i + 1} round /><span><b style={{ fontWeight: 600 }}>{p.name}</b><div className="muted sm">{p.is_founder ? 'Co-founder' : 'Sales'}</div></span></span>
                  {p.paid ? <span className="pill ink">Sudah ditransfer</span> : null}
                </div>
                <span className="kpi"><span className="big" style={{ fontSize: 28, fontWeight: 600, letterSpacing: '-.02em' }}>{rp(p.total)}</span></span>
                <div className="row between sm"><span className="muted">Bagi rata{rep.revenue ? ' ' + ((p.share / rep.revenue) * 100).toFixed(1).replace('.', ',') + '%' : ''}</span><b style={{ fontWeight: 500 }}>{rp(p.share)}</b></div>
                <div className="row between sm"><span className="muted">Komisi dari {rep.payments.filter((x) => x.sales === p.name).length} pembayaran</span><b style={{ fontWeight: 500 }}>{rp(p.komisi)}</b></div>
                <button className={'btn' + (p.paid ? ' soft' : '')} disabled={busy || (p.total <= 0 && !p.paid)} onClick={() => toggle(p)}>{p.paid ? 'Batalkan tanda' : 'Tandai sudah ditransfer'}</button>
              </div>
            ))}
            {rep.people.length === 0 ? <div className="card muted">Belum ada penerima. Tandai minimal satu founder aktif di data sales.</div> : null}
          </div>
          <h2>Rincian per pembayaran</h2>
          <div className="tablewrap">
            <table>
              <thead><tr><th>Tanggal</th><th>Customer</th><th>Sales</th><th className="right">Nominal</th><th className="right">Komisi</th><th className="right">Per founder</th><th className="right">Kas</th></tr></thead>
              <tbody>
                {rep.payments.slice().reverse().map((p, i) => (
                  <tr key={i}><td>{fmtShort(p.paid_at)}</td><td>{p.customer}</td><td>{p.sales}</td><td className="right">{rp(p.amount)}</td><td className="right">{rp(p.komisi)}</td><td className="right">{rp(p.each)}</td><td className="right">{rp(p.kas)}</td></tr>
                ))}
                {rep.payments.length === 0 ? <tr><td colSpan="7" className="muted">Belum ada pembayaran di bulan ini.</td></tr> : null}
              </tbody>
            </table>
          </div>
          <span className="muted sm">Founder yang ikut bagian rata adalah founder aktif saat laporan dilihat. Persentase tiap pembayaran memakai nilai saat pembayaran itu dicatat.</span>
        </>
      )}
    </Page>
  );
}
