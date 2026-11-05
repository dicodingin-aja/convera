import React, { useEffect, useMemo, useState } from 'react';
import { requestJson } from './services/api.js';
import { downloadCsv } from './services/exporter.js';

// Laporan harian + kartu/filter dashboard (mode backend). Data dari GET /api/v1/reports/daily.

const card = { background: 'var(--surface2)', borderRadius: 14, padding: 20, display: 'flex', flexDirection: 'column', gap: 14 };
const h2 = { margin: 0, fontSize: 16, fontWeight: 600, color: 'var(--text)' };
const muted = { fontSize: 13, color: 'var(--muted)' };
const input = { height: 38, padding: '0 12px', border: 'none', borderRadius: 10, background: 'var(--surface)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 13, outline: 'none' };
const btn = { fontFamily: 'inherit', height: 38, padding: '0 14px', border: 'none', borderRadius: 10, fontSize: 13, fontWeight: 500, cursor: 'pointer', background: 'var(--surface)', color: 'var(--text)' };
const th = { textAlign: 'left', padding: '10px 12px', fontSize: 12, fontWeight: 500, color: 'var(--muted)', whiteSpace: 'nowrap' };
const td = { padding: '10px 12px', fontSize: 13, color: 'var(--text)', borderTop: '1px solid var(--surface)', fontVariantNumeric: 'tabular-nums' };
const num = (n) => Math.round(n || 0).toLocaleString('id-ID');
const rupiah = (n) => 'Rp ' + num(n);

const STATUS = [['', 'Semua status'], ['new', 'Baru'], ['lead', 'Lead'], ['checkout', 'Checkout'], ['purchase', 'Purchase'], ['lost', 'Hilang']];
const SOURCE_LABEL = { qr: 'WhatsApp QR', webhook: 'WABA / CRM', api: 'API', organic: 'Organik', manual: 'Manual' };
const sourceLabel = (s) => SOURCE_LABEL[s] || s;

const ymKey = (d = new Date()) => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta', year: 'numeric', month: '2-digit' }).format(d).slice(0, 7);
const shiftMonth = (ym, n) => { const [y, m] = ym.split('-').map(Number); const d = new Date(Date.UTC(y, m - 1 + n, 1)); return d.toISOString().slice(0, 7); };
const monthLabel = (ym) => new Date(ym + '-01T00:00:00Z').toLocaleDateString('id-ID', { month: 'long', year: 'numeric', timeZone: 'UTC' });
const dayLabel = (ymd) => new Date(ymd + 'T00:00:00Z').toLocaleDateString('id-ID', { weekday: 'short', day: 'numeric', month: 'short', timeZone: 'UTC' });

function useReport(slug, params) {
  const [state, setState] = useState({ loading: true, error: '', data: null });
  const key = slug + '|' + JSON.stringify(params);
  useEffect(() => {
    let live = true;
    setState((s) => ({ ...s, loading: true, error: '' }));
    const qs = Object.entries(params).filter(([, v]) => v).map(([k, v]) => k + '=' + encodeURIComponent(v)).join('&');
    requestJson('/api/v1/reports/daily?' + qs, { workspace: slug })
      .then((r) => live && setState({ loading: false, error: '', data: r.data }))
      .catch((e) => live && setState((s) => ({ ...s, loading: false, error: e.message || 'Gagal memuat laporan' })));
    return () => { live = false; };
  }, [key]); // eslint-disable-line react-hooks/exhaustive-deps
  return state;
}

function Filters({ source, setSource, status, setStatus, sources }) {
  return (
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      <select aria-label="Sumber" value={source} onChange={(e) => setSource(e.target.value)} style={input}>
        <option value="">Semua sumber</option>
        {(sources || []).map((s) => <option key={s} value={s}>{sourceLabel(s)}</option>)}
      </select>
      <select aria-label="Status lead" value={status} onChange={(e) => setStatus(e.target.value)} style={input}>
        {STATUS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
    </div>
  );
}

export function EventBreakdown({ rows }) {
  const max = Math.max(1, ...rows.map((r) => r.total));
  if (!rows.length) return <span style={muted}>Belum ada event pada periode ini.</span>;
  return (
    <div style={{ overflowX: 'auto' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 460 }}>
        <thead><tr><th style={th}>Event</th><th style={{ ...th, width: '34%' }}>Sebaran</th><th style={th}>Total</th><th style={th}>Terkirim</th><th style={th}>Gagal</th><th style={th}>Lainnya</th><th style={{ ...th, textAlign: 'right' }}>Nilai</th></tr></thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.event_name}>
              <td style={{ ...td, fontWeight: 500 }}>{r.event_name}</td>
              <td style={td}><div style={{ height: 8, borderRadius: 999, background: 'var(--surface)' }}><div style={{ height: '100%', width: (r.total / max * 100) + '%', minWidth: 3, borderRadius: 999, background: 'var(--accent)' }} /></div></td>
              <td style={td}>{num(r.total)}</td><td style={td}>{num(r.sent)}</td>
              <td style={{ ...td, color: r.failed ? 'var(--danger)' : 'var(--text)' }}>{num(r.failed)}</td><td style={td}>{num(r.other)}</td>
              <td style={{ ...td, textAlign: 'right' }}>{r.value ? rupiah(r.value) : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ---------- Halaman Laporan ----------

export function LaporanPage({ slug, workspaceName }) {
  const [month, setMonth] = useState(() => ymKey());
  const [source, setSource] = useState('');
  const [status, setStatus] = useState('');
  const { loading, error, data } = useReport(slug, { month, source, status });
  const days = data?.days || [];
  const total = data?.total;
  const current = ymKey();

  const exportCsv = () => {
    if (!data) return;
    const rows = days.map((d) => [d.date, d.conversations, d.from_ads, d.purchases, d.revenue, d.events_sent, d.events_failed]);
    rows.push(['TOTAL', total.conversations, total.from_ads, total.purchases, total.revenue, total.events_sent, total.events_failed]);
    rows.push([]);
    rows.push(['event', 'total', 'terkirim', 'gagal', 'lainnya', 'nilai']);
    data.by_event.forEach((e) => rows.push([e.event_name, e.total, e.sent, e.failed, e.other, e.value]));
    downloadCsv(`convera-laporan-harian-${month}.csv`, ['tanggal', 'percakapan', 'dari iklan', 'purchase', 'revenue', 'event terkirim', 'event gagal'], rows);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between' }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <button type="button" aria-label="Bulan sebelumnya" style={btn} onClick={() => setMonth(shiftMonth(month, -1))}>‹</button>
          <input type="month" aria-label="Pilih bulan" value={month} max={current} onChange={(e) => e.target.value && setMonth(e.target.value)} style={input} />
          <button type="button" aria-label="Bulan berikutnya" style={btn} disabled={month >= current} onClick={() => setMonth(shiftMonth(month, 1))}>›</button>
          <Filters source={source} setSource={setSource} status={status} setStatus={setStatus} sources={data?.sources} />
        </div>
        <button type="button" style={{ ...btn, background: 'var(--accent)', color: '#fff', opacity: data ? 1 : 0.6 }} disabled={!data} onClick={exportCsv}>Unduh CSV</button>
      </div>

      <div style={card}>
        <div><h2 style={h2}>Laporan harian</h2><span style={muted}>{workspaceName} · {monthLabel(month)}</span></div>
        {error ? <span role="alert" style={{ ...muted, color: 'var(--danger)' }}>{error}</span> : null}
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', minWidth: 560 }}>
            <thead><tr><th style={th}>Tanggal</th><th style={th}>Percakapan</th><th style={th}>Dari iklan</th><th style={th}>Purchase</th><th style={{ ...th, textAlign: 'right' }}>Revenue</th></tr></thead>
            <tbody>
              {days.map((d) => (
                <tr key={d.date}>
                  <td style={td}>{dayLabel(d.date)}</td><td style={td}>{num(d.conversations)}</td><td style={td}>{num(d.from_ads)}</td>
                  <td style={td}>{num(d.purchases)}</td><td style={{ ...td, textAlign: 'right' }}>{d.revenue ? rupiah(d.revenue) : '—'}</td>
                </tr>
              ))}
              {total ? (
                <tr>
                  <td style={{ ...td, fontWeight: 600 }}>TOTAL</td><td style={{ ...td, fontWeight: 600 }}>{num(total.conversations)}</td><td style={{ ...td, fontWeight: 600 }}>{num(total.from_ads)}</td>
                  <td style={{ ...td, fontWeight: 600 }}>{num(total.purchases)}</td><td style={{ ...td, fontWeight: 600, textAlign: 'right' }}>{rupiah(total.revenue)}</td>
                </tr>
              ) : null}
            </tbody>
          </table>
          {loading && !data ? <span style={muted}>Memuat…</span> : null}
        </div>
      </div>

      <div style={card}>
        <div><h2 style={h2}>Jumlah per event</h2><span style={muted}>Event yang dicatat Convera pada bulan ini (tidak termasuk tes koneksi).</span></div>
        <EventBreakdown rows={data?.by_event || []} />
      </div>
    </div>
  );
}

// ---------- Kartu dan filter di Dashboard ----------

const Stat = ({ label, value, sub }) => (
  <div style={{ flex: '1 1 200px', ...card, gap: 6 }}>
    <span style={muted}>{label}</span>
    <span style={{ fontSize: 26, fontWeight: 600, color: 'var(--text)', fontVariantNumeric: 'tabular-nums' }}>{value}</span>
    {sub ? <span style={{ fontSize: 12, color: 'var(--muted)' }}>{sub}</span> : null}
  </div>
);

export function DailyBars({ days }) {
  const W = 700, H = 120, n = Math.max(days.length, 1);
  const max = Math.max(1, ...days.map((d) => d.conversations));
  const bw = W / n;
  return (
    <svg viewBox={`0 0 ${W} ${H + 18}`} width="100%" role="img" aria-label="Percakapan per hari" style={{ display: 'block' }}>
      {days.map((d, i) => {
        const h = d.conversations / max * H;
        return <rect key={d.date} x={i * bw + bw * 0.15} y={H - h} width={bw * 0.7} height={Math.max(h, d.conversations ? 2 : 0)} rx="2" fill="var(--accent)"><title>{`${dayLabel(d.date)}: ${d.conversations} percakapan, ${d.purchases} purchase, ${rupiah(d.revenue)}`}</title></rect>;
      })}
      <line x1="0" x2={W} y1={H + 0.5} y2={H + 0.5} stroke="var(--surface)" />
      {[0, Math.floor((n - 1) / 2), n - 1].filter((v, i, a) => a.indexOf(v) === i && days[v]).map((i) => (
        <text key={i} x={i * bw + bw / 2} y={H + 14} fontSize="10" textAnchor={i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle'} fill="var(--muted)">{dayLabel(days[i].date)}</text>
      ))}
    </svg>
  );
}

// range = { from, to, label } (YYYY-MM-DD, sama dengan filter tanggal dashboard).
export function DashboardExtras({ slug, range }) {
  const [source, setSource] = useState('');
  const [status, setStatus] = useState('');
  const params = useMemo(() => ({ from: range.from, to: range.to, source, status }), [range.from, range.to, source, status]);
  const { loading, error, data } = useReport(slug, params);
  const t = data?.total;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between' }}>
        <span style={{ ...muted, fontWeight: 500 }}>Ringkasan percakapan & revenue · {range.label}</span>
        <Filters source={source} setSource={setSource} status={status} setStatus={setStatus} sources={data?.sources} />
      </div>
      {error ? <span role="alert" style={{ ...muted, color: 'var(--danger)' }}>{error}</span> : null}
      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', opacity: loading && !data ? 0.6 : 1 }}>
        <Stat label="Percakapan" value={t ? num(t.conversations) : '…'} sub={t ? num(t.from_ads) + ' dari iklan' : ''} />
        <Stat label="Purchase" value={t ? num(t.purchases) : '…'} sub={t ? num(t.events_sent) + ' event terkirim · ' + num(t.events_failed) + ' gagal' : ''} />
        <Stat label="Revenue" value={t ? rupiah(t.revenue) : '…'} sub="Dari event Purchase yang terkirim ke Meta" />
      </div>
      {data && data.days.length > 1 ? (
        <div style={card}><h2 style={h2}>Percakapan per hari</h2><DailyBars days={data.days} /></div>
      ) : null}
      {data && data.by_event.length ? (
        <div style={card}><h2 style={h2}>Distribusi event</h2><EventBreakdown rows={data.by_event} /></div>
      ) : null}
    </div>
  );
}
