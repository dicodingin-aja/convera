// Data nyata untuk dashboard customer (mode backend). Memuat overview, leads, event, dan aturan dari API lalu
// mengubahnya ke bentuk yang dipakai Logic.js. Hanya data yang memang ada di backend yang ditampilkan:
// biaya iklan, ROAS, dan CPP hanya tampil bila Meta Ads tersambung (Marketing API); skor EMQ tidak ditampilkan, bukan dikarang.
import { requestJson } from './api.js';

// Integrasi Meta Ads (biaya, ROAS, CPP) di halaman Performa Iklan. Opsional untuk pelanggan: tanpa koneksi, setiap iklan
// dihitung satu slot kampanye. Form koneksinya ada di KoneksiBackend.jsx (bagian "Akun Meta Ads").
export const ADS_ENABLED = true;

// ---------- Rentang tanggal (hari = Asia/Jakarta) ----------

const ymdJkt = (d = new Date()) => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(d);
const addDays = (ymd, n) => { const d = new Date(ymd + 'T00:00:00Z'); d.setUTCDate(d.getUTCDate() + n); return d.toISOString().slice(0, 10); };
const short = (ymd) => new Date(ymd + 'T00:00:00Z').toLocaleDateString('id-ID', { day: 'numeric', month: 'short', timeZone: 'UTC' });
const diffDays = (a, b) => Math.round((new Date(b + 'T00:00:00Z') - new Date(a + 'T00:00:00Z')) / 864e5);

// Sama urutannya dengan pilihan rentang di UI: Hari ini, Kemarin, 7 hari, 30 hari, Bulan ini, Bulan lalu.
export function rangeFor(idx) {
  const today = ymdJkt();
  const monthStart = today.slice(0, 8) + '01';
  const prevMonthEnd = addDays(monthStart, -1);
  const R = [
    ['Hari ini', today, today],
    ['Kemarin', addDays(today, -1), addDays(today, -1)],
    ['7 hari terakhir', addDays(today, -6), today],
    ['30 hari terakhir', addDays(today, -29), today],
    ['Bulan ini', monthStart, today],
    ['Bulan lalu', prevMonthEnd.slice(0, 8) + '01', prevMonthEnd],
  ][idx] || ['7 hari terakhir', addDays(today, -6), today];
  const [label, from, to] = R;
  return { label, from, to, days: diffDays(from, to) + 1, hint: from === to ? short(from) : short(from) + '–' + short(to) };
}

export function customRange(from, to) {
  const f = from || ymdJkt(), t = to || ymdJkt();
  return { label: 'Kustom', from: f, to: t, days: diffDays(f, t) + 1, hint: short(f) + (f === t ? '' : '–' + short(t)) };
}

// ---------- Pemetaan label ----------

const LEAD_ST = { new: 'Baru', lead: 'Lead', checkout: 'Checkout', purchase: 'Purchase', lost: 'Hilang' };
const SRC = { rule: 'Otomatis', manual: 'Manual', api: 'API', test: 'Tes koneksi' };
export const evStatus = (e) => (e.status === 'sent' ? 'Terkirim' : e.status === 'failed' ? 'Gagal' : e.status === 'skipped' ? 'Kedaluwarsa'
  : e.status === 'pending_value' ? 'Menunggu' : e.attempts > 0 ? 'Retry' : 'Mengirim');

const maskUi = (m) => String(m || '').replace('****', '•••');
const fullPhone = (p) => (p ? '+' + p : '');
const tfmt = (d) => d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' }) + ', ' + String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
const relTime = (iso) => {
  if (!iso) return '—';
  const min = Math.max(0, Math.round((Date.now() - new Date(iso)) / 6e4));
  if (min < 1) return 'baru saja';
  if (min < 60) return min + ' menit lalu';
  if (min < 1440) return Math.round(min / 60) + ' jam lalu';
  return Math.round(min / 1440) + ' hari lalu';
};

// Event yang boleh dipakai per mode koneksi (sama dengan backend internal/meta). Nama event Meta dipakai apa adanya.
const QR_EVENTS = ['Purchase', 'Lead', 'Contact', 'Schedule', 'CompleteRegistration', 'AddToCart', 'InitiateCheckout', 'ViewContent', 'SubmitApplication', 'StartTrial', 'Subscribe', 'AddPaymentInfo', 'Search', 'FindLocation', 'CustomizeProduct', 'Donate', 'AddToWishlist'];
const WABA_EVENTS = ['LeadSubmitted', 'Purchase', 'AddToCart', 'InitiateCheckout', 'ViewContent'];
export const eventsForMode = (mode) => (mode === 'qr' ? QR_EVENTS : WABA_EVENTS);
export const LIVE_EVENTS = WABA_EVENTS;

export function ruleFromApi(r) {
  return {
    id: r.id, name: r.name, active: r.active,
    who: { customer: 'pelanggan', admin: 'admin', template: 'template' }[r.who] || 'pelanggan',
    op: r.op === 'equals' ? 'sama dengan' : 'mengandung',
    kw: (r.keywords || []).join(', '),
    ev: r.event_name || 'Purchase',
    val: r.value_mode === 'fixed' ? 'tetap' : r.value_mode === 'parse' ? 'angka' : 'manual',
    fixed: r.fixed_value ? String(r.fixed_value) : '',
    count: r.hit_count || 0, last: relTime(r.last_hit_at),
  };
}

export function ruleToApi(u) {
  const purchase = u.ev === 'Purchase';
  const fixed = Number(String(u.fixed || '').replace(/\D/g, '') || 0);
  return {
    name: u.name, who: { pelanggan: 'customer', admin: 'admin', template: 'template' }[u.who] || 'customer',
    op: u.op === 'sama dengan' ? 'equals' : 'contains',
    keywords: String(u.kw || '').split(',').map((x) => x.trim()).filter(Boolean),
    event_name: u.ev || 'Purchase',
    value_mode: !purchase ? 'none' : u.val === 'tetap' ? 'fixed' : 'angka' === u.val ? 'parse' : 'none',
    fixed_value: purchase && u.val === 'tetap' ? fixed : null,
    active: u.active !== false,
  };
}

// ---------- Memuat dan membentuk data ----------

export async function fetchLive(slug, rangeIdx, customFrom, customTo) {
  const r = (rangeIdx === 6 && customFrom && customTo) ? customRange(customFrom, customTo) : rangeFor(rangeIdx);
  const q = `?from=${r.from}&to=${r.to}`;
  const [ov, leads, events, rules, perf, conn] = await Promise.all([
    requestJson('/api/v1/overview' + q, { workspace: slug }),
    requestJson('/api/v1/leads?per_page=100', { workspace: slug }),
    requestJson('/api/v1/events?per_page=100', { workspace: slug }),
    requestJson('/api/v1/event-rules', { workspace: slug }),
    ADS_ENABLED ? requestJson('/api/v1/performance' + q, { workspace: slug }).then((x) => x.data).catch(() => null) : Promise.resolve(null), // gagal memuat iklan tidak boleh merusak dashboard
    requestJson('/api/v1/connections/meta', { workspace: slug }).then((x) => x.data).catch(() => null),
  ]);
  return buildLive({ overview: ov.data, leads: leads.data, leadsTotal: leads.meta.total, events: events.data, eventsTotal: events.meta.total, rules: rules.data.rules, range: r, perf, mode: conn && conn.mode });
}

export function emptyLive(rangeIdx = 2) {
  return buildLive({ overview: null, leads: [], leadsTotal: 0, events: [], eventsTotal: 0, rules: [], range: rangeFor(rangeIdx), loading: true });
}

// Pohon performa dari API -> bentuk PF di Logic.js: [kampanye, [[ad set, [[iklan, spend, chat, lead, purchase, nilai], ...]], ...]].
// Chat = percakapan yang tercatat Meta; Lead/Purchase/Nilai dari Convera (digabung lewat ID iklan).
export function perfToPF(perf) {
  if (!perf || !perf.connected) return [];
  return (perf.campaigns || []).map((c) => [c.name, (c.children || []).map((s) => [s.name,
    (s.children || []).map((a) => [a.name, a.spend || 0, a.conversations || a.leads || 0, a.leads || 0, a.purchases || 0, a.value || 0, a.id])])]);
}
const perfAdMap = (perf) => {
  const m = {};
  ((perf && perf.campaigns) || []).forEach((c) => (c.children || []).forEach((s) => (s.children || []).forEach((a) => { m[a.id] = a; })));
  return m;
};

export function buildLive(raw) {
  const ads = [];
  const adIdx = {};
  const clid = {};
  const lb = raw.leads.map((l) => {
    let ad = -1;
    if (l.from_ad) {
      const key = l.ad_id || 'iklan';
      if (adIdx[key] === undefined) {
        adIdx[key] = ads.length;
        ads.push({ name: l.ad_name || l.ad_headline || 'Iklan ' + String(l.ad_id || '').slice(-6), camp: l.campaign_name || l.campaign_id || 'Iklan Click-to-WhatsApp', set: l.adset_name || '—',
          head: l.ad_headline || l.ad_name || 'Iklan Click-to-WhatsApp', body: l.ad_name ? 'Nama iklan dari Meta Ads.' : 'Hubungkan Meta Ads di halaman Koneksi untuk melihat nama iklan & biayanya.' });
      }
      ad = adIdx[key];
      clid[l.id] = l.ctwa_clid || null;
    }
    const clickAt = new Date(l.clicked_at || l.first_message_at);
    const clickH = Math.max(0, (Date.now() - clickAt) / 36e5);
    return [l.id, l.name || 'Tanpa nama', fullPhone(l.phone) || maskUi(l.phone_masked), ad, l.last_message || '—', clickH, LEAD_ST[l.status] || 'Baru', l.value || 0];
  });
  const kv = raw.events.filter((e) => e.source !== 'test').map((e) => [
    tfmt(new Date(e.created_at)), e.lead_name || '—', fullPhone(e.phone) || maskUi(e.phone_masked) || '—', e.event_name, e.value || 0, SRC[e.source] || e.source, evStatus(e),
    e.rule_name ? 'Aturan “' + e.rule_name + '”' : e.source === 'api' ? 'Sistem kamu (API)' : 'Tim kamu',
  ]);
  const log = raw.events.map((e, i) => ({
    i, ev: e.event_name, st: evStatus(e), src: SRC[e.source] || e.source, d: new Date(e.created_at), value: e.value || 0,
    clid: e.ctwa_clid || fullPhone(e.phone) || maskUi(e.phone_masked) || '—', id: 'evt_' + String(e.id).slice(0, 8), order: e.event_id || e.id, eid: e.id, raw: e,
  }));
  const eventNames = [...new Set([...eventsForMode(raw.mode), ...raw.rules.map((x) => x.event_name)])];
  return { raw, mode: raw.mode || 'waba', eventNames, ads, lb, clid, kv, log, perf: raw.perf || { connected: false }, pf: perfToPF(raw.perf), rules: raw.rules.map(ruleFromApi), range: raw.range, loading: !!raw.loading, leadDetail: {}, eventDetail: {} };
}

// ---------- Timeline lead dan detail event (data nyata) ----------

export function liveTimeline(live, dl, tf, rp) {
  const d = live.leadDetail[dl.id];
  const TL = [];
  if (dl.A) TL.push(['Klik iklan “' + dl.A.name + '”', tf(dl.clickAt)]);
  if (!d) { TL.push(['Chat masuk', tf(dl.clickAt)]); TL.push(['Memuat riwayat…', '']); return TL; }
  const msgs = d.messages || [];
  if (!msgs.length) TL.push(['Chat masuk', tf(dl.clickAt)]);
  msgs.forEach((m) => TL.push([(m.direction === 'in' ? 'Pelanggan' : 'Kamu') + ' · “' + m.body + '”', tf(new Date(m.sent_at))]));
  d.events.slice().reverse().forEach((e) => {
    const st = evStatus(e);
    TL.push([e.event_name + ' ' + st.toLowerCase() + (e.value ? ' · ' + rp(e.value) : '') + (e.error_message && e.status === 'failed' ? ' — ' + e.error_message : ''), tf(new Date(e.sent_at || e.event_time))]);
  });
  if (dl.organic) TL.push(['Tidak dikirim ke Meta: chat ini bukan dari iklan', '']);
  return TL;
}

export function liveEventDetail(live, le, tab, tf, rp) {
  const e = live.eventDetail[le.eid];
  const pretty = (v) => JSON.stringify(v, null, 2);
  if (!e) return { id: le.id + ' · ' + le.order, pre: 'Memuat…', explain: false, tips: [], canResend: false, retry: [] };
  const hasErr = !!(e.error_message || e.hint);
  const retry = [[tf(new Date(e.created_at)), '—', 'Event dibuat · sumber ' + (SRC[e.source] || e.source)]];
  if (e.attempts > 0) retry.push([e.sent_at && e.status === 'sent' ? tf(new Date(e.sent_at)) : 'Terakhir', e.status === 'sent' ? '200' : (e.error_code || '—'), e.status === 'sent' ? 'Diterima Meta · ' + e.attempts + ' percobaan' : (e.error_message || 'Gagal') + ' · ' + e.attempts + ' percobaan']);
  if (e.status === 'queued' && e.next_retry_at) retry.push(['Terjadwal', '—', 'Percobaan berikutnya ' + tf(new Date(e.next_retry_at))]);
  return {
    id: le.id + ' · ' + le.order,
    explain: hasErr, exTitle: evStatus(e) === 'Gagal' ? 'Event gagal dikirim ke Meta' : evStatus(e) === 'Kedaluwarsa' ? 'Event tidak dikirim' : 'Perlu diperhatikan',
    exBody: e.error_message || '', tips: e.hint ? [e.hint] : [],
    facts: [['WAKTU', tf(new Date(e.created_at)) + ' WIB'], ['SUMBER', SRC[e.source] || e.source], ['NILAI', e.value ? rp(e.value) : '—'], ['CTWA_CLID', e.ctwa_clid || '—']].map(([k, v]) => ({ k, v })),
    pre: tab === 'payload' ? (e.request_payload ? pretty(e.request_payload) : 'Belum pernah dikirim ke Meta.') : (e.meta_response ? pretty(e.meta_response) : 'Belum ada respons dari Meta.'),
    retry: retry.map(([t, code, msg], k) => ({ t, code, msg, dot: { width: 10, height: 10, borderRadius: '50%', flex: 'none', marginTop: 4, background: k === retry.length - 1 ? 'var(--accent)' : 'var(--borderStrong)' } })),
    canResend: e.status === 'failed' && !['attribution_expired', 'not_from_ad'].includes(e.error_code),
  };
}

// ---------- Overview (data nyata) ----------

const fmtN = (n) => Math.round(n).toLocaleString('id-ID');
const nice = (m) => { const mx = Math.max(m, 4); const p = Math.pow(10, Math.floor(Math.log10(mx))); return Math.ceil(mx / p) * p; };

// h = alat dari Logic.js: { pill, thumb, ic, rp, rpS, goP, dotC, ini }
export function liveOverview(live, h) {
  const { raw } = live;
  const ovd = raw.overview;
  const days = raw.range.days;
  if (!ovd) return { starter: false, full: true, kpis: [], funnel: [], funnelRate: '', chatLine: '', chatArea: '', purchLine: '', xL: [], yL: [], yR: [], topAds: [], health: [], steps: [] };
  const k = ovd.kpi, prev = ovd.previous;
  const cmp = days === 1 ? 'vs hari sebelumnya' : 'vs ' + days + ' hari sebelumnya';
  const delta = (cur, before) => {
    if (!before) return cur ? ['baru', true] : ['—', true];
    const p = ((cur - before) / before) * 100;
    return [(p >= 0 ? '+' : '−') + Math.abs(p).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '%', p >= 0];
  };
  const card = (label, value, cur, before, icon) => {
    const [d, good] = before === undefined ? ['', true] : delta(cur, before);
    return { label, value, delta: d, icon, growth: false, cmp: before === undefined ? '' : cmp, ds: h.pill(d ? (good ? 'ok' : 'err') : 'neutral', { height: 22, padding: '0 8px', fontSize: 11 }) };
  };
  const rate = k.match_rate == null ? '—' : (k.match_rate * 100).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '%';
  const kpis = [
    card('Chat dari iklan', fmtN(k.chats_from_ads), k.chats_from_ads, prev.chats_from_ads, h.ic.chat),
    card('Lead', fmtN(k.leads), k.leads, prev.leads, h.ic.users),
    card('Purchase', fmtN(k.purchases), k.purchases, prev.purchases, h.ic.bag),
    card('Nilai pembelian', h.rp(k.purchase_value), k.purchase_value, prev.purchase_value, h.ic.wallet),
    card('Event terkirim ke Meta', fmtN(k.events_sent), 0, undefined, h.ic.link),
    card('Event diterima Meta', rate, 0, undefined, h.ic.shield),
  ];
  // Biaya iklan, Cost per Purchase, dan ROAS hanya bila Meta Ads tersambung. ROAS hanya untuk akun IDR (nilai purchase dicatat dalam rupiah).
  const adsB = ovd.ads && ovd.ads.connected ? ovd.ads : null;
  if (adsB) {
    const idr = !adsB.currency || adsB.currency === 'IDR';
    const roas = adsB.spend ? k.purchase_value / adsB.spend : 0, proas = adsB.previous_spend ? prev.purchase_value / adsB.previous_spend : 0;
    const cpp = k.purchases ? adsB.spend / k.purchases : 0, pcpp = prev.purchases ? adsB.previous_spend / prev.purchases : 0;
    const ratio = (cur, before, lowerIsBetter) => { const d = before ? ((cur - before) / before) * 100 : null; return d == null ? ['—', true] : [(d >= 0 ? '+' : '−') + Math.abs(d).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '%', lowerIsBetter ? d <= 0 : d >= 0]; };
    const mk = (label, value, dg, icon) => ({ label, value, delta: dg[0], icon, growth: true, cmp, ds: h.pill(dg[0] === '—' ? 'neutral' : dg[1] ? 'ok' : 'err', { height: 22, padding: '0 8px', fontSize: 11 }) });
    kpis.splice(4, 0,
      mk('Biaya iklan', h.rp(adsB.spend), delta(adsB.spend, adsB.previous_spend), h.ic.wallet),
      mk('Cost per Purchase', k.purchases ? h.rp(Math.round(cpp)) : '—', ratio(cpp, pcpp, true), h.ic.target),
      mk('ROAS', idr ? (adsB.spend ? (roas).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '×' : '—') : 'Bukan IDR', idr ? ratio(roas, proas, false) : ['—', true], h.ic.trend));
  }

  const f = ovd.funnel;
  const FN = [['Chat masuk', f.leads], ['Lead', f.lead + f.checkout + f.purchase], ['Checkout', f.checkout + f.purchase], ['Purchase', f.purchase]];
  if (ovd.ads && ovd.ads.connected && ovd.ads.clicks > 0) FN.unshift(['Klik iklan', ovd.ads.clicks]);
  const base = Math.max(FN[0][1], 1);
  const funnel = FN.map(([label, v], i) => ({ label, value: fmtN(v), rate: i && FN[i - 1][1] ? (v / FN[i - 1][1] * 100).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '%' : '',
    bar: { height: '100%', width: (v / base * 100) + '%', minWidth: v ? 8 : 2, borderRadius: 999, background: i === FN.length - 1 ? 'var(--accent)' : 'var(--text)', opacity: i === FN.length - 1 ? 1 : 0.8 - i * 0.18 } }));

  let series = ovd.series.slice();
  if (series.length === 1) series = [series[0], series[0]];
  const n = series.length;
  const cs = series.map((x) => x.chats_from_ads), ps = series.map((x) => x.purchases);
  const cMax = nice(Math.max(...cs) * 1.15), pMax = nice(Math.max(...ps) * 1.15);
  const path = (arr, mx) => arr.map((v, i) => (i ? 'L' : 'M') + (i / (n - 1) * 700).toFixed(1) + ',' + (219 - v / mx * 214).toFixed(1)).join(' ');
  const chatLine = path(cs, cMax);
  const xL = Array.from({ length: 6 }, (_, j) => short(series[Math.round(j / 5 * (n - 1))].date));

  const pm = perfAdMap(raw.perf);
  const idrAcc = !raw.perf || !raw.perf.currency || raw.perf.currency === 'IDR';
  const topAds = ovd.top_ads.slice(0, 5).map((a, i) => {
    const pa = pm[a.ad_id];
    const name = (pa && pa.name) || a.headline || 'Iklan ' + String(a.ad_id || '').slice(-6);
    const hasSpend = pa && pa.spend > 0 && idrAcc;
    const r = hasSpend ? a.purchase_value / pa.spend : 0;
    return { name, camp: h.rpS(a.purchase_value) + ' nilai purchase', ini: h.ini(name), thumb: h.thumb(i), spend: pa && pa.spend ? h.rpS(pa.spend) : '—', chat: fmtN(a.leads), purch: fmtN(a.purchases),
      roas: hasSpend ? r.toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '×' : '—', rs: h.pill(!hasSpend ? 'neutral' : r >= 5 ? 'ok' : r >= 2 ? 'soft' : 'err', { justifySelf: 'end' }) };
  });

  const hl = ovd.health || {};
  const tokenDays = hl.token_expires_at ? Math.ceil((new Date(hl.token_expires_at) - Date.now()) / 864e5) : null;
  const metaK = !hl.configured ? 'err' : hl.meta_status === 'connected' ? 'ok' : hl.meta_status === 'pending' ? 'warn' : 'err';
  const metaTxt = !hl.configured ? 'Belum diisi' : { connected: 'Terhubung', pending: 'Belum dites', invalid: 'Token tidak valid', expired: 'Token kedaluwarsa', error: 'Gagal' }[hl.meta_status] || hl.meta_status;
  const healthRows = [
    ['Meta Dataset', metaTxt, metaK],
    ['Chat WhatsApp masuk', hl.last_message_at ? 'Terakhir ' + relTime(hl.last_message_at) : 'Belum ada chat masuk', hl.last_message_at ? 'ok' : 'warn'],
    ['Event gagal 24 jam', fmtN(hl.failed_24h || 0) + ' event', hl.failed_24h ? 'warn' : 'ok'],
    ['Masa berlaku token Meta', tokenDays == null ? 'Tidak diatur' : tokenDays <= 0 ? 'Kedaluwarsa' : tokenDays + ' hari lagi', tokenDays != null && tokenDays < 14 ? 'warn' : 'ok'],
  ];
  const health = healthRows.map(([label, value, kk]) => ({ label, value, dot: { width: 8, height: 8, borderRadius: '50%', flex: 'none', background: h.dotC(kk) }, vs: { fontSize: 13, fontWeight: 500, color: kk === 'ok' ? 'var(--text)' : 'var(--accentText)' } }));

  const testSent = raw.events.some((e) => e.source === 'test' && e.status === 'sent');
  const stepsRaw = [
    ['Buat workspace', 'Workspace sudah dibuat', true],
    ['Hubungkan Meta Dataset', 'Dataset & access token untuk Conversions API', !!hl.configured, 'Hubungkan', 'koneksi'],
    ['Hubungkan WhatsApp', 'Supaya chat dari iklan tercatat sebagai Lead', !!hl.last_message_at, 'Hubungkan', 'koneksi'],
    ['Pilih cara kirim konversi', 'API, tandai manual, atau aturan otomatis', live.rules.length > 0 || raw.events.some((e) => ['manual', 'api'].includes(e.source)), 'Atur', 'aturan'],
    ['Kirim event percobaan', 'Pastikan event muncul di Meta Test Events', testSent, 'Kirim tes', 'koneksi'],
  ];
  const steps = stepsRaw.map(([title, desc, done, cta, pg]) => ({ title, desc, todo: !done, cta, go: h.goP(pg), mark: done ? h.ic.check : '',
    ts: { fontSize: 14, fontWeight: 500, color: done ? 'var(--muted)' : 'var(--text)', textDecoration: done ? 'line-through' : 'none' },
    dot: { width: 26, height: 26, flex: 'none', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', background: done ? 'var(--accent)' : 'var(--neutral)', color: '#fff' } }));

  const hasData = raw.leadsTotal > 0 || raw.eventsTotal > 0;
  return { starter: !hasData, full: hasData, kpis, funnel, funnelRate: 'Chat → Purchase ' + (f.leads ? (f.purchase / f.leads * 100).toFixed(1).replace('.', ',') : '0') + '%',
    chatLine, chatArea: chatLine + ' L700,219 L0,219 Z', purchLine: path(ps, pMax), xL, yL: [cMax, cMax / 2, 0].map(fmtN), yR: [pMax, pMax / 2, 0].map(fmtN), topAds, health, steps, goPerf: ADS_ENABLED ? h.goP('performa') : null };
}

// ---------- Tindakan ke server ----------

// Lonceng notifikasi (level akun, bukan per workspace).
export const notifApi = {
  list: () => requestJson('/api/v1/notifications').then((r) => ({ items: r.data || [], unread: (r.meta && r.meta.unread) || 0 })),
  readAll: () => requestJson('/api/v1/notifications/read-all', { method: 'POST' }),
};

export const liveApi = {
  leadDetail: (slug, id) => requestJson('/api/v1/leads/' + id, { workspace: slug }).then((r) => r.data),
  eventDetail: (slug, id) => requestJson('/api/v1/events/' + id, { workspace: slug }).then((r) => r.data),
  convert: (slug, leadId, eventName, value) => requestJson(`/api/v1/leads/${leadId}/conversions`, { method: 'POST', workspace: slug,
    body: JSON.stringify({ event_name: eventName, ...(value ? { value } : {}) }) }).then((r) => r.data),
  retry: (slug, id) => requestJson(`/api/v1/events/${id}/retry`, { method: 'POST', workspace: slug }),
  bulkStatus: (slug, ids, status) => requestJson('/api/v1/leads/bulk-status', { method: 'POST', workspace: slug, body: JSON.stringify({ ids, status }) }),
  saveRule: (slug, rule) => {
    const body = JSON.stringify(ruleToApi(rule));
    return rule.id ? requestJson('/api/v1/event-rules/' + rule.id, { method: 'PUT', workspace: slug, body })
      : requestJson('/api/v1/event-rules', { method: 'POST', workspace: slug, body });
  },
  erase: (slug, phone) => requestJson('/api/v1/leads/erase', { method: 'POST', workspace: slug, body: JSON.stringify({ phone }) }).then((r) => r.data),
  deleteLead: (slug, id) => requestJson('/api/v1/leads/' + id, { method: 'DELETE', workspace: slug }),
  status: () => requestJson('/api/v1/status').then((r) => r.data),
  deleteRule: (slug, id) => requestJson('/api/v1/event-rules/' + id, { method: 'DELETE', workspace: slug }),
};

// ---------- Laporan (data nyata, sama dengan Overview untuk rentang terpilih) ----------

export function liveReport(live, h) {
  const ovd = live.raw.overview;
  if (!ovd) return {};
  const k = ovd.kpi, prev = ovd.previous;
  const dl = (cur, before) => (before ? ((cur - before) / before * 100 >= 0 ? '+' : '−') + Math.abs((cur - before) / before * 100).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '%' : 'baru');
  const kpis = [['Chat dari iklan', fmtN(k.chats_from_ads), dl(k.chats_from_ads, prev.chats_from_ads)], ['Lead', fmtN(k.leads), dl(k.leads, prev.leads)],
    ['Purchase', fmtN(k.purchases), dl(k.purchases, prev.purchases)], ['Nilai pembelian', h.rpS(k.purchase_value), dl(k.purchase_value, prev.purchase_value)]]
    .map(([label, value, delta]) => ({ label, value, delta: delta + ' vs periode lalu' }));
  const top = ovd.top_ads.slice(0, 3).map((a, i) => ({ n: i + 1, name: a.headline || 'Iklan ' + String(a.ad_id || '').slice(-6), purch: fmtN(a.purchases) + ' purchase', roas: h.rpS(a.purchase_value) }));
  return {
    kpis, top, range: live.range.label + ' · ' + live.range.hint + ' · dibuat ' + h.tfmt(new Date()) + ' WIB',
    csv: () => h.csv('convera-laporan.csv', ['iklan', 'lead', 'purchase', 'nilai purchase'], ovd.top_ads.map((a) => [a.headline || 'Iklan ' + String(a.ad_id || '').slice(-6), a.leads, a.purchases, a.purchase_value])),
  };
}
