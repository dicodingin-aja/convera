import React from 'react';
import { DCLogic } from './dc.jsx';
import { printReport, downloadCsv, downloadJson, printInvoice } from './services/exporter.js';
import { buildEvent, sendEvent, testConnection } from './services/capi.js';
import { testWebhook } from './services/webhook.js';
import { liveOverview, liveTimeline, liveEventDetail, liveReport, rangeFor, LIVE_EVENTS } from './services/live.js';
import { randomToken, uid } from './services/crypto.js';
import { parseRecipients, checkPassword, isHttps, normPhone } from './services/validators.js';

const displayPhone = (phone) => {
  const raw = String(phone || '');
  const digits = raw.replace(/\D/g, '');
  if (digits.startsWith('62') && !raw.includes('*')) {
    const local = digits.slice(2);
    return '+62 ' + local.slice(0, 3) + '-••••-' + local.slice(-4);
  }
  return raw.replace(/^62/, '+62 ').replace('****', '••••');
};

const liveWaConnected = (health, fallback) => {
  if (!health) return fallback;
  if (health.mode === 'qr' || health.qr_status != null) return health.qr_status === 'connected';
  if (health.mode === 'waba') return health.configured === true;
  return fallback;
};

class Component extends DCLogic {
  state = {
    dark: null, screen: null, step: 0, width: 1440,
    f: { biz: 'Kopi Senja', industry: 'F&B / Kuliner', tz: 'WIB — Asia/Jakarta (UTC+7)', dataset: '1029384756102938', token: 'EAAGm0PX4ZCpsBAJ8xZq2LmNvT7', phoneId: '1098765432101234', waba: '2345678901234567', waToken: 'EAAGsys0PX4ZCpsBAK2' },
    test: 'idle', wa: 'cloud', methods: { api: true, manual: true, auto: false }, send: 'idle', pop: false, guide: false,
    page: 'overview', collapsed: null, menu: null, wsQuery: '', ws: 0, range: 2, read: false, more: false,
    banners: { wa: true, token: true, quota: true, trial: true },
    profile: { name: '', username: '', phone: '' },
  };
  componentDidMount() { this.onR = () => this.setState({ width: window.innerWidth }); this.onR(); window.addEventListener('resize', this.onR); }
  componentWillUnmount() { window.removeEventListener('resize', this.onR); clearTimeout(this.t1); clearTimeout(this.t2); clearTimeout(this.tt); clearInterval(this.iv); }

  theme(dark, accent, status) {
    const A = accent === 'indigo'
      ? (dark ? { a: '#8b93ff', as: '#a5abff', at: '#a5abff', aS: '#24244a' } : { a: '#4f46e5', as: '#4338ca', at: '#4338ca', aS: '#eef0ff' })
      : (dark ? { a: '#ef8240', as: '#f59d66', at: '#d8d3c9', aS: '#2b2926' } : { a: '#e8732c', as: '#c95f1f', at: '#4a4640', aS: '#f3f1ee' });
    const B = dark
      ? { bg: '#151413', surface: '#151413', surface2: '#1f1e1c', input: '#1f1e1c', border: '#302e2a', borderStrong: '#47433d', text: '#f3f0ea', muted: '#a39e95', ink: '#f3f0ea', inkText: '#1b1a18', neutral: '#2b2926', danger: '#f59d66', shadow: 'none' }
      : { bg: '#ffffff', surface: '#ffffff', surface2: '#f6f5f3', input: '#f6f5f3', border: '#e7e2db', borderStrong: '#d3ccc2', text: '#1d1c1a', muted: '#6b675f', ink: '#2f2e2c', inkText: '#ffffff', neutral: '#ece8e2', danger: '#b8530f', shadow: 'none' };
    const sem = status === 'semantic';
    const S = sem
      ? (dark ? { okBg: '#16301f', okFg: '#5fd39a', warnBg: '#342912', warnFg: '#e8b84a', errBg: '#3a1b1b', errFg: '#f58a8a' } : { okBg: '#e3f2ea', okFg: '#1e7a4f', warnBg: '#fbf0d6', warnFg: '#8a5d04', errBg: '#fbe6e4', errFg: '#b42323' })
      : { okBg: B.ink, okFg: B.inkText, warnBg: A.aS, warnFg: A.at, errBg: dark ? '#2b2926' : '#f3f1ee', errFg: dark ? '#f3f0ea' : '#1d1c1a' };
    S.warnBorder = sem ? S.warnFg : A.a;
    S.errDotStrong = sem ? S.errFg : A.a;
    S.errRing = sem ? S.errBg : A.aS;
    const all = { ...B, accent: A.a, accentStrong: A.as, accentText: A.at, accentSoft: A.aS, ...S };
    const o = {}; Object.keys(all).forEach((k) => (o['--' + k] = all[k]));
    return o;
  }

  icons() {
    const mk = (ch, size = 18) => React.createElement('svg', { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 2, strokeLinecap: 'round', strokeLinejoin: 'round', style: { flex: 'none', display: 'block' } }, ch.map((c, i) => typeof c === 'string' ? React.createElement('path', { key: i, d: c }) : React.createElement(c[0], { key: i, ...c[1] })));
    const R = (x, y, w, h, rx = 1) => ['rect', { x, y, width: w, height: h, rx }];
    const Ci = (cx, cy, r) => ['circle', { cx, cy, r }];
    return {
      dash: mk([R(3, 3, 7, 9), R(14, 3, 7, 5), R(14, 12, 7, 9), R(3, 16, 7, 5)]),
      users: mk(['M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2', Ci(9, 7, 4), 'M22 21v-2a4 4 0 0 0-3-3.87', 'M16 3.13a4 4 0 0 1 0 7.75']),
      bag: mk(['M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4Z', 'M3 6h18', 'M16 10a4 4 0 0 1-8 0']),
      bars: mk(['M3 3v18h18', 'M18 17V9', 'M13 17V5', 'M8 17v-3']),
      zap: mk(['M13 2 3 14h9l-1 8 10-12h-9l1-8z']),
      list: mk(['M8 6h13', 'M8 12h13', 'M8 18h13', 'M3 6h.01', 'M3 12h.01', 'M3 18h.01']),
      file: mk(['M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z', 'M14 2v6h6', 'M16 13H8', 'M16 17H8']),
      link: mk(['M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71', 'M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71']),
      blocks: mk([R(14, 3, 7, 7), 'M10 21V8a1 1 0 0 0-1-1H4a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-5a1 1 0 0 0-1-1H3']),
      code: mk(['m16 18 6-6-6-6', 'm8 6-6 6 6 6']),
      sliders: mk(['M21 4h-7', 'M10 4H3', 'M21 12h-9', 'M8 12H3', 'M21 20h-5', 'M12 20H3', 'M14 2v4', 'M8 10v4', 'M16 18v4']),
      grid: mk([R(3, 3, 7, 7), R(14, 3, 7, 7), R(14, 14, 7, 7), R(3, 14, 7, 7)], 16),
      more: mk([Ci(12, 12, 1), Ci(19, 12, 1), Ci(5, 12, 1)]),
      bell: mk(['M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9', 'M10.3 21a1.94 1.94 0 0 0 3.4 0'], 17),
      help: mk([Ci(12, 12, 10), 'M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3', 'M12 17h.01'], 17),
      search: mk([Ci(11, 11, 8), 'm21 21-4.3-4.3'], 15),
      chevDown: mk(['m6 9 6 6 6-6'], 16), chevLeft: mk(['m15 18-6-6 6-6'], 16), chevUpDown: mk(['m7 15 5 5 5-5', 'm7 9 5-5 5 5'], 15),
      arrowRight: mk(['M5 12h14', 'm12 5 7 7-7 7'], 16),
      calendar: mk([R(3, 4, 18, 18, 2), 'M16 2v4', 'M8 2v4', 'M3 10h18'], 16),
      panel: mk([R(3, 3, 18, 18, 2), 'M9 3v18'], 17),
      x: mk(['M18 6 6 18', 'm6 6 12 12'], 16), check: mk(['M20 6 9 17l-5-5'], 16), checkLg: mk(['M20 6 9 17l-5-5'], 26),
      plus: mk(['M5 12h14', 'M12 5v14'], 16), lock: mk([R(3, 11, 18, 11, 2), 'M7 11V7a5 5 0 0 1 10 0v4'], 16),
      alert: mk(['m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z', 'M12 9v4', 'M12 17h.01'], 16),
      clock: mk([Ci(12, 12, 10), 'M12 6v6l4 2'], 14), shield: mk(['M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10'], 14),
      cloud: mk(['M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z']),
      qr: mk([R(3, 3, 5, 5), R(16, 3, 5, 5), R(3, 16, 5, 5), 'M21 16h-3a2 2 0 0 0-2 2v3', 'M21 21v.01', 'M12 7v3a2 2 0 0 1-2 2H7', 'M3 12h.01', 'M12 3h.01', 'M12 16v.01', 'M16 12h1', 'M21 12v.01', 'M12 21v-1']),
      send: mk(['m22 2-7 20-4-9-9-4Z', 'M22 2 11 13'], 16), hand: mk(['M18 11V6a2 2 0 0 0-4 0v1', 'M14 10V4a2 2 0 0 0-4 0v2', 'M10 10.5V6a2 2 0 0 0-4 0v8', 'M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.86-5.99-2.34l-3.6-3.6a2 2 0 0 1 2.83-2.82L7 15']),
      book: mk(['M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20'], 16), play: mk([Ci(12, 12, 10), 'm10 8 6 4-6 4Z'], 16),
      chat: mk(['M7.9 20A9 9 0 1 0 4 16.1L2 22Z'], 16), activity: mk(['M22 12h-4l-3 9L9 3l-3 9H2'], 16),
      user: mk([Ci(12, 8, 5), 'M20 21a8 8 0 0 0-16 0'], 16), logout: mk(['M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4', 'm16 17 5-5-5-5', 'M21 12H9'], 16),
      moon: mk(['M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z'], 16), sun: mk([Ci(12, 12, 4), 'M12 2v2', 'M12 20v2', 'm4.93 4.93 1.41 1.41', 'm17.66 17.66 1.41 1.41', 'M2 12h2', 'M20 12h2', 'm6.34 17.66-1.41 1.41', 'm19.07 4.93-1.41 1.41'], 16),
      chevRight: mk(['m9 18 6-6-6-6'], 16), download: mk(['M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4', 'm7 10 5 5 5-5', 'M12 15V3'], 15),
      copy: mk([R(8, 8, 14, 14, 2), 'M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2'], 14),
      wallet: mk(['M19 7V4a1 1 0 0 0-1-1H5a2 2 0 0 0 0 4h15a1 1 0 0 1 1 1v4h-3a2 2 0 0 0 0 4h3a1 1 0 0 0 1-1v-2a1 1 0 0 0-1-1', 'M3 5v14a2 2 0 0 0 2 2h15a1 1 0 0 0 1-1v-4'], 17),
      target: mk([Ci(12, 12, 10), Ci(12, 12, 6), Ci(12, 12, 2)], 17), trend: mk(['m22 7-8.5 8.5-5-5L2 17', 'M16 7h6v6'], 17),
      monitor: mk([R(2, 3, 20, 14, 2), 'M8 21h8', 'M12 17v4'], 17), phone: mk([R(5, 2, 14, 20, 2), 'M12 18h.01'], 17), meta: mk(['M2 12c0-4 2-7 4.5-7 3.5 0 5 7 7.5 11 1.5 2.4 3 3 4 3 1.7 0 4-1.5 4-6 0-4-2-7-4.5-7-3.5 0-5 7-7.5 11C8.5 19.4 7 20 6 20c-1.7 0-4-1.5-4-8Z'], 18),
      plug: mk(['M12 22v-5', 'M9 8V2', 'M15 8V2', 'M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z'], 16),
    };
  }

  spin(dark) {
    return React.createElement('span', { style: { width: 14, height: 14, flex: 'none', borderRadius: '50%', border: '2px solid var(--border)', borderTopColor: 'var(--accent)', display: 'inline-block', animation: 'cvSpin 0.8s linear infinite' } }, React.createElement('style', null, '@keyframes cvSpin{to{transform:rotate(360deg)}}'));
  }


  pagesVals(s, P, ic, W, mobile, WS) {
    const set = (o) => this.setState(o);
    const ownerPhone = P.profilePhone ? normPhone(P.profilePhone) : '';
    const ownerPhoneMasked = ownerPhone ? ownerPhone.slice(0, 4) + '-•••-' + ownerPhone.slice(-4) : 'nomor belum diisi';
    // Mode backend: P.live berisi data nyata dari API (leads, event, aturan, overview). Tanpa itu, data contoh dipakai.
    const LV = P.live || null;
    const liveHealth = LV?.raw?.overview?.health || null;
    const waPhoneMasked = displayPhone(liveHealth?.wa_phone || liveHealth?.wa_phone_masked) || ownerPhoneMasked;
    const sem = P.statusStyle === 'semantic';
    const fmt = (n) => Math.round(n).toLocaleString('id-ID');
    const rp = (n) => 'Rp ' + fmt(n);
    const rpS = (n) => n >= 1e6 ? 'Rp ' + (n / 1e6).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + ' jt' : n >= 1e3 ? 'Rp ' + Math.round(n / 1e3) + ' rb' : rp(n);
    const PL = { ok: ['var(--okBg)', 'var(--okFg)'], err: ['var(--errBg)', 'var(--errFg)'], warn: ['var(--warnBg)', 'var(--warnFg)'], neutral: ['var(--neutral)', 'var(--muted)'], soft: ['var(--accentSoft)', 'var(--text)'], accent: ['var(--ink)', 'var(--inkText)'], plain: ['var(--surface)', 'var(--text)'] };
    const pill = (k, ex = {}) => ({ display: 'inline-flex', alignItems: 'center', gap: 6, height: 24, padding: '0 10px', borderRadius: 999, fontSize: 12, fontWeight: 500, whiteSpace: 'nowrap', justifySelf: 'start', alignSelf: 'center', background: PL[k][0], color: PL[k][1], ...ex });
    const chip = (on) => ({ fontFamily: 'inherit', display: 'inline-flex', alignItems: 'center', gap: 6, height: 34, padding: '0 12px', borderRadius: 999, border: 'none', cursor: 'pointer', fontSize: 13, fontWeight: on ? 500 : 400, whiteSpace: 'nowrap', flex: 'none', background: on ? 'var(--text)' : 'var(--surface2)', color: on ? 'var(--surface)' : 'var(--text)' });
    const say = (title, sub = '') => { set({ toast: { title, sub } }); clearTimeout(this.tt); this.tt = setTimeout(() => this.setState({ toast: null }), 3200); };
    const copy = (txt, key) => { try { navigator.clipboard.writeText(txt); } catch (e) {} set({ copied: key }); clearTimeout(this.ct); this.ct = setTimeout(() => this.setState({ copied: null }), 1400); };
    const sw = (on, toggle) => ({ toggle, track: { fontFamily: 'inherit', width: 40, height: 24, borderRadius: 999, border: 'none', padding: 2, cursor: 'pointer', background: on ? 'var(--accent)' : 'var(--borderStrong)', display: 'flex', justifyContent: on ? 'flex-end' : 'flex-start', flex: 'none', justifySelf: 'end' }, knob: { width: 20, height: 20, borderRadius: '50%', background: '#fff', display: 'block' } });
    const TT = [['var(--ink)', 'var(--inkText)'], ['var(--accent)', '#fff'], ['var(--accentSoft)', 'var(--accentText)'], ['var(--neutral)', 'var(--text)']];
    const thumb = (i, z = 36) => ({ width: z, height: z, flex: 'none', borderRadius: z > 40 ? 14 : 8, background: TT[i % 4][0], color: TT[i % 4][1], display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: z > 40 ? 20 : z < 32 ? 10 : 12, fontWeight: 600 });
    const ini = (t) => t.split(/\s+/).filter((w) => /[A-Za-z0-9]/.test(w[0] || '')).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
    const BTN = { fontFamily: 'inherit', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', gap: 8, height: 36, padding: '0 14px', borderRadius: 10, border: 'none', fontSize: 13, fontWeight: 500, cursor: 'pointer', whiteSpace: 'nowrap', flex: 'none' };
    const bP = { ...BTN, background: 'var(--accent)', color: '#fff' }, bW = { ...BTN, background: 'var(--surface)', color: 'var(--text)' }, bN = { ...BTN, background: 'var(--neutral)', color: 'var(--muted)' }, bS = { ...BTN, background: 'var(--surface2)', color: 'var(--text)' }, bD = { ...BTN, background: 'transparent', color: 'var(--danger)' };
    const goP = (page) => (e) => { if (e && e.preventDefault) e.preventDefault(); set({ page }); };
    const NOW = LV ? new Date() : new Date(2026, 9, 17, 14, 40);
    const tfmt = (d) => d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' }) + ', ' + String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
    const CH = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789', HEX = '0123456789abcdef';
    const clidOf = (i) => 'AR' + Array.from({ length: 22 }, (_, k) => CH[(i * 31 + k * 17 + k * k * 7) % CH.length]).join('');
    const sha = (i) => Array.from({ length: 64 }, (_, k) => HEX[(i * 13 + k * 7 + k * k) % 16]).join('');
    const SRCB = { display: 'inline-flex', alignItems: 'center', justifySelf: 'start', height: 24, padding: '0 8px', borderRadius: 6, background: 'var(--surface)', color: 'var(--text)', fontFamily: "'Poppins',monospace", fontSize: 11, fontWeight: 400 };
    const EVP = { Mengirim: 'warn', Terkirim: 'ok', Gagal: 'err', Duplikat: 'neutral', Retry: 'warn', Kedaluwarsa: 'neutral', Menunggu: 'warn' };
    const dotC = (k) => k === 'ok' ? (sem ? 'var(--okFg)' : 'var(--text)') : k === 'err' ? 'var(--errDotStrong)' : 'var(--accentText)';
    const disc = s.disc || {}; const metaOn = !disc.meta, waOn = liveWaConnected(liveHealth, !!s.waOn && !disc.wa), adOn = LV ? !!(LV.perf && LV.perf.connected) : !disc.ad && s.ws !== 1;
    const tokenDays = s.tokenDays ?? 5;

    const ADS = LV ? LV.ads : [
      { name: 'Promo Kopi Susu 2.0', camp: 'Promo Oktober', set: 'Jaksel 18–34', head: 'Kopi Susu 2.0 — beli 2 gratis 1', body: 'Khusus pesan via WhatsApp sampai 31 Okt. Klik & chat sekarang!' },
      { name: 'Paket Bundling Hampers', camp: 'Hampers Korporat', set: 'Pekerja kantor', head: 'Hampers kopi untuk kantor', body: 'Mulai Rp 350.000/box, gratis kartu ucapan & ongkir Jabodetabek.' },
      { name: 'Carousel Menu Baru', camp: 'Menu Baru', set: 'Interest kopi', head: '5 menu baru musim hujan', body: 'Aren Latte, Hojicha, sampai Pandan Coffee. Tanya stok via chat.' },
      { name: 'Diskon Ojol 20%', camp: 'Promo Oktober', set: 'Retarget 30 hari', head: 'Hemat 20% pesan langsung', body: 'Lebih murah dari aplikasi ojol. Chat admin untuk kode diskon.' },
      { name: 'Testimoni Pelanggan', camp: 'Awareness', set: 'Broad Jabodetabek', head: '“Kopi susu terenak di Tebet”', body: 'Sudah 12.000+ cup terjual bulan ini.' },
      { name: 'Kopi Gratis Ulang Tahun', camp: 'Awareness', set: 'Ulang tahun bulan ini', head: 'Ulang tahun? Kopi gratis!', body: 'Tunjukkan KTP di outlet, berlaku 7 hari.' },
    ];
    const adIdx = (n) => Math.max(0, ADS.findIndex((a) => a.name === n));
    const PF = LV ? LV.pf : [
      ['Promo Oktober', [['Jaksel 18–34', [['Promo Kopi Susu 2.0', 1850000, 412, 210, 74, 31200000], ['Kopi Susu Foto Produk', 520000, 96, 40, 8, 3100000]]], ['Retarget 30 hari', [['Diskon Ojol 20%', 760000, 174, 88, 21, 4620000]]]]],
      ['Hampers Korporat', [['Pekerja kantor', [['Paket Bundling Hampers', 1420000, 286, 150, 48, 26400000], ['Hampers Video Unboxing', 380000, 64, 22, 3, 1050000]]]]],
      ['Menu Baru', [['Interest kopi', [['Carousel Menu Baru', 980000, 198, 96, 27, 8100000]]]]],
      ['Awareness', [['Broad Jabodetabek', [['Testimoni Pelanggan', 640000, 131, 40, 9, 2160000]]], ['Ulang tahun bulan ini', [['Kopi Gratis Ulang Tahun', 450000, 48, 20, 0, 0]]]]],
    ];
    const allAds = PF.flatMap((c) => c[1].flatMap((st) => st[1].map((a) => ({ a, camp: c[0] }))));
    const roasPill = (r, ex = {}) => pill(r >= 5 ? 'ok' : r >= 2 ? 'soft' : 'err', { justifySelf: 'end', ...ex });
    const roasT = (r) => r.toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '×';

    // ---------- overview ----------
    const DAYS = [1, 1, 7, 30, 17, 30][s.range] || 7, kf = DAYS / 7;
    const chat = Math.round(1284 * kf), lead = Math.round(642 * kf), purch = Math.round(186 * kf), val = 84720000 * kf, spend = 7120000 * kf;
    const cmp = DAYS === 1 ? 'vs kemarin' : 'vs ' + DAYS + ' hari sebelumnya';
    const kpis = [['Chat dari iklan', fmt(chat), '+12,4%', true, ic.chat], ['Lead', fmt(lead), '+8,1%', true, ic.users], ['Purchase', fmt(purch), '+21,6%', true, ic.bag], ['Nilai pembelian', rp(Math.round(val / 1000) * 1000), '+18,9%', true, ic.wallet], ['Cost per Purchase', rp(Math.round(spend / purch / 100) * 100), '−6,2%', true, ic.target, true], ['ROAS', roasT(val / spend), '+1,4', true, ic.trend, true], ['Match rate ctwa_clid', '93,8%', '+2,1 pt', true, ic.link], ['Skor EMQ', '7,8', '−0,3', false, ic.shield]]
      .map(([label, value, delta, good, icon, growth]) => ({ label, value, delta, icon, growth: !!growth, cmp, ds: pill(good ? 'ok' : 'err', { height: 22, padding: '0 8px', fontSize: 11 }) }));
    const klik = Math.round(chat * 2.66);
    const FN = [['Klik iklan', klik], ['Chat', chat], ['Lead', lead], ['Purchase', purch]];
    const funnel = FN.map(([label, v], i) => ({ label, value: fmt(v), rate: i ? (v / FN[i - 1][1] * 100).toLocaleString('id-ID', { maximumFractionDigits: 1 }) + '%' : '', bar: { height: '100%', width: (v / klik * 100) + '%', minWidth: 8, borderRadius: 999, background: i === 3 ? 'var(--accent)' : 'var(--text)', opacity: i === 3 ? 1 : 0.8 - i * 0.22 } }));
    const n = Math.max(DAYS, 7);
    const cs = Array.from({ length: n }, (_, i) => 183 * (1 + Math.sin(i * 0.8 + 1) * 0.16 + Math.sin(i * 0.23) * 0.1 + ((i * 7919) % 13) / 13 * 0.12));
    const ps = cs.map((c, i) => c * 0.145 * (1 + Math.sin(i * 1.3 + 2) * 0.22));
    const cMax = Math.ceil(Math.max(...cs) * 1.15 / 50) * 50, pMax = Math.ceil(Math.max(...ps) * 1.15 / 10) * 10;
    const path = (arr, mx) => arr.map((v, i) => (i ? 'L' : 'M') + (i / (n - 1) * 700).toFixed(1) + ',' + (219 - v / mx * 214).toFixed(1)).join(' ');
    const chatLine = path(cs, cMax);
    const xL = Array.from({ length: 6 }, (_, k) => { const i = Math.round(k / 5 * (n - 1)); return new Date(2026, 9, 17 - (n - 1 - i)).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' }); });
    const topAds = allAds.slice().sort((x, y) => y.a[4] - x.a[4]).slice(0, 5).map(({ a, camp }) => ({ name: a[0], camp, ini: ini(a[0]), thumb: thumb(adIdx(a[0])), spend: rpS(a[1]), chat: fmt(a[2]), purch: fmt(a[4]), roas: roasT(a[5] / a[1]), rs: roasPill(a[5] / a[1]) }));
    const health = [['Meta Dataset', metaOn ? 'Terhubung' : 'Terputus', metaOn ? 'ok' : 'err'], ['WhatsApp ' + waPhoneMasked, waOn ? 'Terhubung' : 'Terputus', waOn ? 'ok' : 'err'], ['Event gagal 24 jam', '3 event', 'warn'], ['Masa berlaku token Meta', tokenDays + ' hari lagi', tokenDays < 14 ? 'warn' : 'ok']]
      .map(([label, value, k]) => ({ label, value, dot: { width: 8, height: 8, borderRadius: '50%', flex: 'none', background: dotC(k) }, vs: { fontSize: 13, fontWeight: 500, color: k === 'ok' ? 'var(--text)' : 'var(--accentText)' } }));
    const steps = [['Buat workspace', 'Laundry Kilat Bersih · Jasa · Depok', true], ['Hubungkan Meta Dataset', 'Dataset & access token untuk Conversions API', true], ['Hubungkan WhatsApp', 'Agar chat dari iklan tercatat sebagai Lead', false, 'Hubungkan', 'koneksi'], ['Pilih cara kirim konversi', 'API, tandai manual, atau aturan otomatis', false, 'Atur', 'aturan'], ['Kirim event percobaan', 'Pastikan event muncul di Meta Test Events', false, 'Kirim tes', 'developer']]
      .map(([title, desc, done, cta, pg]) => ({ title, desc, todo: !done, cta, go: goP(pg), mark: done ? ic.check : '', ts: { fontSize: 14, fontWeight: 500, color: done ? 'var(--muted)' : 'var(--text)', textDecoration: done ? 'line-through' : 'none' }, dot: { width: 26, height: 26, flex: 'none', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', background: done ? 'var(--accent)' : 'var(--neutral)', color: '#fff' } }));
    const ov = { starter: s.ws === 3, full: s.ws !== 3, kpis, funnel, funnelRate: 'Klik → Purchase ' + (purch / klik * 100).toFixed(1).replace('.', ',') + '%', chatLine, chatArea: chatLine + ' L700,219 L0,219 Z', purchLine: path(ps, pMax), xL, yL: [cMax, cMax / 2, 0].map(fmt), yR: [pMax, pMax / 2, 0].map(fmt), topAds, health, steps, goPerf: goP('performa'), goKoneksi: goP('koneksi') };
    if (LV) Object.assign(ov, liveOverview(LV, { pill, thumb, ic, rp, rpS, goP, dotC, ini }));

    // ---------- leads ----------
    const LB = LV ? LV.lb : [[1, 'Andi Pratama', '628•••7890', 0, 'Kak, promo kopi susu 2.0 masih ada?', 6, 'Baru', 0], [2, 'Siti Rahmawati', '628•••4412', 1, 'Mau pesan hampers 10 box buat kantor, bisa invoice PT?', 20, 'Checkout', 3500000], [3, 'Budi Santoso', '628•••0921', 2, 'Menu baru ada yang non-kopi?', 30, 'Lead', 0], [4, 'Dewi Lestari', '628•••3378', 0, 'Bisa kirim ke Tebet hari ini?', 52, 'Purchase', 89000], [5, 'Rizky Hidayat', '628•••6650', 3, 'Kode diskonnya apa kak?', 75, 'Lead', 0], [6, 'Nur Aini', '628•••1209', 1, 'Hampers yang 350rb isinya apa aja?', 151, 'Checkout', 350000], [7, 'Fajar Nugroho', '628•••8834', 4, 'Halo, buka jam berapa ya?', 160, 'Baru', 0], [8, 'Putri Ayu', '628•••5521', 2, 'Saya mau 2 Aren Latte ya', 98, 'Purchase', 64000], [9, 'Hendra Wijaya', '628•••7716', 0, 'Kak jadi pesan 1 liter ya', 190, 'Checkout', 120000], [10, 'Maya Sari', '628•••2290', 5, 'Ultah saya besok, dapat kopi gratis?', 40, 'Hilang', 0], [11, 'Agus Salim', '628•••4057', 3, 'Sudah transfer ya kak, 215rb', 12, 'Checkout', 215000], [12, 'Lina Marlina', '628•••9183', -1, 'Halo kak, mau tanya', 3, 'Baru', 0]];
    const lov = s.leadOv || {};
    const LEADS = LB.filter(([, , phone]) => !(s.delPhones || []).includes(normPhone(phone))).map(([id, name, phone, ad, msg, clickH, status, value]) => { const o = lov[id] || {}; return { id, name, phone, ad, A: ad >= 0 ? ADS[ad] : null, msg, clickAt: new Date(+NOW - clickH * 3600e3), base: status, baseVal: value, status: o.status || status, value: o.value ?? value, events: o.events || [], hl: 168 - clickH, organic: ad < 0, clid: ad >= 0 ? (LV ? LV.clid[id] : clidOf(id)) : null }; });
    const LSP = { Baru: 'plain', Lead: 'soft', Checkout: 'accent', Purchase: 'ok', Hilang: 'neutral' };
    const lq = (s.ldQ || '').toLowerCase(), lcamp = s.ldCamp || 'Semua kampanye', lst = s.ldStatus || 'Semua';
    const lbase = LEADS.filter((l) => (lcamp === 'Semua kampanye' || (l.organic ? lcamp === 'Organik' : l.A.camp === lcamp)) && (!lq || (l.name + l.phone + (l.A ? l.A.name : 'organik')).toLowerCase().includes(lq)));
    const lrows = lbase.filter((l) => lst === 'Semua' || l.status === lst);
    const sel = s.ldSel || {}; const nSel = lrows.filter((l) => sel[l.id]).length;
    const box = (on) => ({ fontFamily: 'inherit', width: 20, height: 20, borderRadius: 6, border: 'none', padding: 0, cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', background: on ? 'var(--accent)' : 'var(--surface)', color: '#fff', boxShadow: on ? 'none' : 'inset 0 0 0 1.5px var(--borderStrong)' });
    const cdOf = (l) => l.organic ? ['Organik', pill('neutral')] : l.hl <= 0 ? ['Kedaluwarsa', pill('neutral', { textDecoration: 'line-through' })] : l.hl < 24 ? [l.hl + ' jam lagi', pill('err')] : [Math.floor(l.hl / 24) + ' hari lagi', pill('plain')];
    const openMk = (l) => set({ mk: { id: l.id, ev: s.defEv || 'Purchase', amount: l.value ? String(l.value) : '', note: '' }, ldOpen: null });
    const actOf = (l) => l.status === 'Purchase' ? ['Terkirim ✓', bN, () => set({ ldOpen: l.id })] : l.organic ? ['Tanpa ctwa_clid', bN, () => set({ ldOpen: l.id })] : l.hl <= 0 ? ['Lewat 7 hari', bN, () => openMk(l)] : ['Tandai Purchase', bS, () => openMk(l)];
    const LG = '36px minmax(180px,1.1fr) minmax(230px,1.4fr) minmax(200px,1.3fr) 110px 130px 110px 150px';
    const csv = (name, head, rows) => downloadCsv(name, head, rows);
    const ld = {
      q: s.ldQ || '', setQ: (e) => set({ ldQ: e.target.value }), camp: lcamp, setCamp: (e) => set({ ldCamp: e.target.value }),
      chips: ['Semua', 'Baru', 'Lead', 'Checkout', 'Purchase', 'Hilang'].map((x) => ({ label: x, count: x === 'Semua' ? lbase.length : lbase.filter((l) => l.status === x).length, pick: () => set({ ldStatus: x }), style: chip(lst === x), cs: { fontSize: 11, fontWeight: 500, padding: '1px 7px', borderRadius: 999, background: lst === x ? 'rgba(127,127,127,0.25)' : 'var(--surface)' } })),
      exportCsv: () => csv('convera-leads.csv', ['nama', 'nomor', 'iklan', 'kampanye', 'status', 'nilai'], lrows.map((l) => [l.name, l.phone, l.A ? l.A.name : 'Organik', l.A ? l.A.camp : '-', l.status, l.value])),
      bulk: nSel > 0, selLabel: nSel + ' lead dipilih', clearSel: () => set({ ldSel: {} }),
      bulkLost: () => { if (LV) { const ids = lrows.filter((l) => sel[l.id]).map((l) => l.id); set({ ldSel: {} }); this.__host.liveBulkStatus(ids, 'lost').then((r) => say(r.ok ? ids.length + ' lead ditandai Hilang' : 'Gagal mengubah status', r.message)); return; } const o = { ...lov }; lrows.filter((l) => sel[l.id]).forEach((l) => (o[l.id] = { ...(o[l.id] || {}), status: 'Hilang' })); set({ leadOv: o, ldSel: {} }); say(nSel + ' lead ditandai Hilang'); },
      allBox: box(lrows.length > 0 && nSel === lrows.length), allTick: lrows.length > 0 && nSel === lrows.length ? ic.check : null,
      toggleAll: () => { const all = nSel === lrows.length; const o = {}; if (!all) lrows.forEach((l) => (o[l.id] = true)); set({ ldSel: o }); },
      desktop: !mobile && lrows.length > 0, mobile: mobile && lrows.length > 0, empty: lrows.length === 0,
      rows: lrows.map((l) => { const [cd, cdS] = cdOf(l); const [actLabel, aS, act] = actOf(l); return { name: l.name, phone: l.phone, ini: ini(l.name), adName: l.A ? l.A.name : 'Chat organik', adPath: l.A ? l.A.camp + ' › ' + l.A.set : 'Tidak dari iklan', adIni: l.A ? ini(l.A.name) : '—', thumb: thumb(l.ad < 0 ? 3 : l.ad), msg: '“' + l.msg + '”', time: tfmt(l.clickAt), status: l.status, pill: pill(LSP[l.status]), pd: { width: 6, height: 6, borderRadius: '50%', flex: 'none', background: l.status === 'Baru' ? 'var(--accent)' : 'currentColor' }, cd, cs: { ...cdS, gap: 5 }, value: l.value ? rp(l.value) : '—', actLabel, act, as: { ...aS, height: 34, width: '100%' }, asLg: { ...aS, height: 48, width: '100%', fontSize: 15 }, open: () => set({ ldOpen: l.id }), tick: sel[l.id] ? ic.check : null, box: box(!!sel[l.id]), toggleSel: () => set({ ldSel: { ...sel, [l.id]: !sel[l.id] } }), rowStyle: { display: 'grid', gridTemplateColumns: LG, gap: 14, padding: '10px 18px', alignItems: 'center', borderTop: '1px solid var(--bg)', background: sel[l.id] ? 'var(--accentSoft)' : 'transparent' } }; }),
    };
    const ml = s.mk ? LEADS.find((l) => l.id === s.mk.id) : null;
    let mk = { open: false };
    if (ml) {
      const ev = s.mk.ev, needsValue = ['Purchase', 'InitiateCheckout', 'AddToCart'].includes(ev);
      const amt = Number((s.mk.amount || '').replace(/\D/g, '') || 0);
      const warn = LV && LV.mode === 'qr' ? '' : ml.organic ? 'Lead ini bukan dari iklan (tidak ada ctwa_clid), jadi tidak bisa diatribusikan ke Meta.' : ml.hl <= 0 ? 'Klik iklan terjadi ' + tfmt(ml.clickAt) + ' — sudah lewat 7 hari. Meta menolak atribusi untuk lead ini.' : '';
      const dis = !!warn || (needsValue && !amt);
      const upd = (o) => set({ mk: { ...s.mk, ...o } });
      mk = { open: true, who: ml.name + ' · ' + ml.phone + (ml.A ? ' · ' + ml.A.name : ''), close: () => set({ mk: null }), warn,
        events: (LV ? LV.eventNames : ['Purchase', 'InitiateCheckout', 'AddToCart', 'LeadSubmitted', 'ViewContent']).map((e) => ({ label: e, pick: () => upd({ ev: e }), style: { ...chip(e === ev), height: 32, fontSize: 12, fontFamily: "'Poppins',monospace" } })),
        needsValue, amount: amt ? amt.toLocaleString('id-ID') : '', setAmount: (e) => upd({ amount: e.target.value }), quick: [100000, 250000, 500000, 1000000].map((v) => ({ label: rpS(v).replace('Rp ', ''), pick: () => upd({ amount: String(v) }) })),
        note: s.mk.note, setNote: (e) => upd({ note: e.target.value }),
        preview: [['event_name', ev], ['value', needsValue ? (amt ? amt + ' IDR' : '—') : 'tidak dikirim'], ['ctwa_clid', ml.clid ? ml.clid.slice(0, 16) + '…' : '—'], ['action_source', 'business_messaging'], ['user_data.ph', 'sha256(…' + ml.phone.slice(-4) + ')']].map(([k, v]) => ({ k, v })),
        submitLabel: 'Kirim ke Meta', submitStyle: { ...bP, height: 44, padding: '0 18px', opacity: dis ? 0.4 : 1, cursor: dis ? 'not-allowed' : 'pointer' },
        submit: () => { if (dis) return;
          if (LV) { set({ mk: null }); this.__host.liveConvert(ml.id, ev, needsValue ? amt : 0).then((r) => say(r.ok ? ev + (r.queued ? ' masuk antrean' : ' tercatat') : ev + ' tidak dikirim', r.message)); return; }
          const st = ev === 'Purchase' ? 'Purchase' : ev === 'LeadSubmitted' || ev === 'ViewContent' ? 'Lead' : 'Checkout'; const prev = lov[ml.id] || {}; const v = needsValue ? amt : 0;
          const eid = uid(); this.setState({ mk: null, leadOv: { ...lov, [ml.id]: { status: st, value: v || ml.value, events: [...(prev.events || []), { ev, amt: v, id: eid }] } }, kvNew: [{ id: eid, time: 'Baru saja', name: ml.name, phone: ml.phone, event: ev, value: v, src: 'Manual', status: 'Mengirim', by: 'Rizky (Owner)' }, ...(s.kvNew || [])] });
          const fin = (ok, err) => { this.setState((x) => ({ kvNew: (x.kvNew || []).map((k) => (k.id === eid ? { ...k, status: ok ? 'Terkirim' : 'Gagal', err } : k)) })); say(ok ? ev + ' terkirim ke Meta' : ev + ' gagal dikirim', ok ? (v ? rp(v) + ' · ' : '') + ml.name + ' · ' + ml.phone : err); };
          buildEvent({ event: ev, value: v, clid: ml.clid, phone: ml.phone, eventId: eid }).then((pl) => sendEvent(pl, { datasetId: s.f.dataset, token: s.f.token })).then((r) => fin(r.ok, r.error)).catch((e) => fin(false, e.message)); } };
    }
    const dl = s.ldOpen ? LEADS.find((l) => l.id === s.ldOpen) : null;
    let ldr = { open: false };
    if (dl) {
      const add = (m) => tfmt(new Date(+dl.clickAt + m * 60e3));
      const TL = [];
      if (LV) TL.push(...liveTimeline(LV, dl, tfmt, rp)); else {
      if (dl.A) TL.push(['Klik iklan “' + dl.A.name + '”', tfmt(dl.clickAt)]);
      TL.push(['Chat masuk · “' + dl.msg + '”', add(1)]);
      if (!dl.organic && ['Lead', 'Checkout', 'Purchase'].includes(dl.base)) TL.push(['LeadSubmitted terkirim ke Meta', add(14)]);
      if (!dl.organic && ['Checkout', 'Purchase'].includes(dl.base)) TL.push(['InitiateCheckout terkirim · ' + rp(dl.baseVal), add(95)]);
      if (dl.base === 'Purchase') TL.push(['Purchase terkirim · ' + rp(dl.baseVal), add(180)]);
      dl.events.forEach((e) => TL.push([e.ev + ' terkirim manual' + (e.amt ? ' · ' + rp(e.amt) : ''), 'Baru saja']));
      if (dl.organic) TL.push(['Tidak dikirim ke Meta — chat organik tanpa ctwa_clid', '']);
      }
      const [cd] = cdOf(dl);
      const fs = (mono) => ({ fontSize: 13, fontWeight: 400, color: 'var(--text)', fontFamily: mono ? "'Poppins',monospace" : 'inherit', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' });
      ldr = { open: true, close: () => set({ ldOpen: null }), name: dl.name, phone: dl.phone, status: dl.status, pill: pill(LSP[dl.status]), thumb: thumb(dl.ad < 0 ? 3 : dl.ad, 64), adIni: dl.A ? ini(dl.A.name) : '—', head: dl.A ? dl.A.head : 'Chat organik', body: dl.A ? dl.A.body : 'Pelanggan menghubungi langsung, bukan dari iklan.', path: dl.A ? dl.A.camp + ' › ' + dl.A.set + ' › ' + dl.A.name : 'Tanpa sumber iklan',
        facts: [['CTWA_CLID', dl.clid ? dl.clid.slice(0, 18) + '…' : '—', 1], ['SISA ATRIBUSI', cd], ['CHAT PERTAMA', add(1)], ['NILAI', dl.value ? rp(dl.value) : '—']].map(([k, v, m]) => ({ k, v, style: fs(m) })),
        timeline: TL.map(([label, time], i) => ({ label, time, line: i < TL.length - 1, dot: { width: 12, height: 12, borderRadius: '50%', flex: 'none', marginTop: 3, background: i === TL.length - 1 ? 'var(--accent)' : 'var(--text)', opacity: i === TL.length - 1 ? 1 : 0.7 } })),
        canMark: !dl.organic && dl.hl > 0 && dl.status !== 'Purchase', mark: () => openMk(dl) };
    }

    // ---------- konversi ----------
    const KVB = LV ? LV.kv : [['17 Okt, 14:32', 'Dewi Lestari', '628•••3378', 'Purchase', 89000, 'Manual', 'Terkirim', 'Dina (CS)'], ['17 Okt, 13:05', 'Agus Salim', '628•••4057', 'InitiateCheckout', 215000, 'Otomatis', 'Terkirim', 'Aturan “Sudah transfer”'], ['17 Okt, 11:48', 'Siti Rahmawati', '628•••4412', 'InitiateCheckout', 3500000, 'API', 'Terkirim', 'Bot Kopi Senja'], ['17 Okt, 10:20', 'Putri Ayu', '628•••5521', 'Purchase', 64000, 'API', 'Terkirim', 'Bot Kopi Senja'], ['16 Okt, 21:14', 'Yusuf Maulana', '628•••3021', 'Purchase', 450000, 'Otomatis', 'Terkirim', 'Aturan “Sudah transfer”'], ['16 Okt, 19:02', 'Rina Kartika', '628•••7745', 'Purchase', 1250000, 'Manual', 'Retry', 'Dina (CS)'], ['16 Okt, 16:40', 'Bayu Saputra', '628•••6612', 'AddToCart', 180000, 'API', 'Terkirim', 'Bot Kopi Senja'], ['16 Okt, 13:11', 'Wulan Dari', '628•••0098', 'Purchase', 350000, 'Manual', 'Gagal', 'Arif (Admin)'], ['15 Okt, 20:55', 'Teguh Prasetyo', '628•••4410', 'Purchase', 215000, 'API', 'Terkirim', 'Bot Kopi Senja'], ['15 Okt, 18:30', 'Sari Wulandari', '628•••1187', 'LeadSubmitted', 0, 'Otomatis', 'Duplikat', 'Aturan “Tanya harga”']];
    const kvAll = [...(s.kvNew || []), ...KVB.map(([time, name, phone, event, value, src, status, by]) => ({ time, name, phone, event, value, src, status, by }))];
    const kvSrc = s.kvSrc || 'Semua';
    const kvP = kvAll.filter((r) => r.event === 'Purchase' && r.status === 'Terkirim');
    const kv = {
      sum: [['Nilai Purchase terkirim', rp(kvP.reduce((a, r) => a + r.value, 0)), kvP.length + ' purchase · 3 hari terakhir'], ['Dari API', String(kvAll.filter((r) => r.src === 'API').length), 'Bot & CRM'], ['Tandai manual', String(kvAll.filter((r) => r.src === 'Manual').length), 'Oleh tim CS'], ['Aturan otomatis', String(kvAll.filter((r) => r.src === 'Otomatis').length), 'Kata kunci & template']].map(([label, value, sub]) => ({ label, value, sub })),
      chips: ['Semua', 'API', 'Manual', 'Otomatis'].map((x) => ({ label: x === 'Semua' ? 'Semua sumber' : x, pick: () => set({ kvSrc: x }), style: chip(kvSrc === x) })),
      rows: kvAll.filter((r) => kvSrc === 'Semua' || r.src === kvSrc).map((r) => ({ ...r, value: r.value ? rp(r.value) : '—', ss: SRCB, pill: pill(EVP[r.status]) })),
    };

    // ---------- performa ----------
    const pfOpen = s.pfOpen || { c0: true, c0s0: true };
    const pfSort = s.pfSort || 'spend';
    const sumOf = (ads) => ads.reduce((a, x) => [a[0] + x[1], a[1] + x[2], a[2] + x[3], a[3] + x[4], a[4] + x[5]], [0, 0, 0, 0, 0]);
    const mv = (m) => ({ spend: m[0], purch: m[3], roas: m[0] ? m[4] / m[0] : 0, chat: m[1], val: m[4] }[pfSort] ?? m[0]);
    const PG = 'minmax(280px,2fr) repeat(9,minmax(84px,1fr))';
    const num = { textAlign: 'right', fontVariantNumeric: 'tabular-nums', color: 'var(--text)', fontSize: 13 };
    const cellsOf = (m) => { const [sp, ch, le, pu, va] = m; const r = sp ? va / sp : 0; return [{ t: rpS(sp), s: num }, { t: fmt(ch), s: num }, { t: fmt(le), s: num }, { t: fmt(pu), s: { ...num, fontWeight: 500 } }, { t: rpS(va), s: num }, { t: le ? rpS(sp / le) : '—', s: num }, { t: pu ? rpS(sp / pu) : '—', s: num }, { t: roasT(r), s: roasPill(r) }, { t: ch ? (pu / ch * 100).toFixed(1).replace('.', ',') + '%' : '—', s: num }]; };
    const pfRows = [];
    const prow = (lvl, key, name, sub, m, open) => pfRows.push({ name, sub, cells: cellsOf(m), canToggle: lvl < 2, isAd: lvl === 2, ini: ini(name), thumb: thumb(adIdx(name), 28), toggle: () => set({ pfOpen: { ...pfOpen, [key]: !open } }),
      chev: { fontFamily: 'inherit', width: 24, height: 24, flex: 'none', borderRadius: 6, border: 'none', padding: 0, background: 'transparent', color: 'var(--muted)', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', transform: open ? 'rotate(90deg)' : 'none', transition: 'transform .15s' },
      ns: { display: 'flex', alignItems: 'center', gap: 8, minWidth: 0, paddingLeft: lvl * 24 }, ts: { fontSize: 13, fontWeight: lvl === 0 ? 600 : lvl === 1 ? 500 : 400, color: 'var(--text)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' },
      rowStyle: { display: 'grid', gridTemplateColumns: PG, gap: 10, padding: '10px 18px', alignItems: 'center', borderTop: lvl === 0 ? '2px solid var(--bg)' : '1px solid var(--bg)', background: lvl === 2 ? 'rgba(127,127,127,0.05)' : 'transparent' } });
    const srt = (arr) => arr.sort((x, y) => mv(y.m) - mv(x.m));
    srt(PF.map((c, ci) => ({ c, ci, m: sumOf(c[1].flatMap((x) => x[1])) }))).forEach(({ c, ci, m }) => {
      const k = 'c' + ci, o = !!pfOpen[k];
      prow(0, k, c[0], c[1].length + ' ad set · ' + c[1].reduce((a, x) => a + x[1].length, 0) + ' iklan', m, o);
      if (o) srt(c[1].map((st, si) => ({ st, si, m: sumOf(st[1]) }))).forEach(({ st, si, m }) => {
        const k2 = k + 's' + si, o2 = !!pfOpen[k2];
        prow(1, k2, st[0], 'Ad set · ' + st[1].length + ' iklan', m, o2);
        if (o2) srt(st[1].map((a) => ({ a, m: a.slice(1) }))).forEach(({ a, m }) => prow(2, null, a[0], 'Iklan CTWA', m, false));
      });
    });
    const allOpen = PF.every((c, ci) => pfOpen['c' + ci]);
    const pf = { none: !adOn, has: adOn, connect: () => { if (LV) { goP('koneksi')(); return; } set({ disc: { ...disc, ad: false }, ws: s.ws === 1 ? 0 : s.ws }); say('Ad Account terhubung', 'act_2049381726 · Kopi Senja Ads'); }, rows: pfRows,
      expandLabel: allOpen ? 'Ciutkan semua' : 'Buka semua', expandAll: () => { const o = {}; if (!allOpen) PF.forEach((c, ci) => { o['c' + ci] = true; c[1].forEach((_, si) => (o['c' + ci + 's' + si] = true)); }); set({ pfOpen: o }); },
      cols: [['Spend', 'spend'], ['Chat', 'chat'], ['Lead'], ['Purchase', 'purch'], ['Nilai', 'val'], ['CPL'], ['CPP'], ['ROAS', 'roas'], ['Conv.']].map(([label, key]) => ({ label: label + (key && pfSort === key ? ' ↓' : ''), sort: () => key && set({ pfSort: key }), style: { fontFamily: 'inherit', border: 'none', background: 'transparent', padding: 0, textAlign: 'right', fontSize: 11, fontWeight: 500, letterSpacing: '0.06em', textTransform: 'uppercase', cursor: key ? 'pointer' : 'default', color: key && pfSort === key ? 'var(--text)' : 'var(--muted)' } })),
      lg1: pill('ok'), lg2: pill('soft'), lg3: pill('err') };

    // ---------- aturan ----------
    const EVL = LV ? Object.fromEntries(LV.eventNames.map((x) => [x, 1])) : { Contact: 1, Lead: 1, 'Hot Lead': 1, Purchase: 1, Complaint: 1, Report: 1 };
    const EVMAP = { InitiateCheckout: 'Hot Lead', AddToCart: 'Hot Lead', LeadSubmitted: 'Lead', ViewContent: 'Contact', Custom: 'Contact' };
    const fixEv = (r) => ({ ...r, ev: EVL[r.ev] ? r.ev : (EVMAP[r.ev] || 'Purchase'), val: r.val === 'tetap' ? 'tetap' : 'angka' });
    const WHO = { pelanggan: ['Pelanggan mengirim pesan', 'Cek pesan masuk dari pembeli.', 'pelanggan bilang'], admin: ['Admin / CS membalas', 'Cek pesan yang dikirim tim kamu.', 'admin membalas'], template: ['Template terkirim', 'Saat template pesan tertentu dikirim.', 'template terkirim berisi'] };
    const VALM = { angka: ['Ambil dari pesan', 'Otomatis baca nominal, mis. “Rp 450.000” atau “215rb”.', 'nilai dari pesan'], tetap: ['Nilai tetap', 'Selalu kirim angka yang sama, cocok untuk jasa.', 'nilai tetap'] };
    const RB0 = LV ? LV.rules : [{ name: 'Bukti transfer diterima', who: 'pelanggan', op: 'mengandung', kw: 'sudah transfer, udah tf', ev: 'Purchase', val: 'angka', count: 38, last: '2 jam lalu' }, { name: 'Invoice terkirim', who: 'template', op: 'mengandung', kw: 'invoice_pesanan', ev: 'Hot Lead', val: 'angka', count: 61, last: '35 menit lalu' }, { name: 'Tanya harga', who: 'pelanggan', op: 'mengandung', kw: 'harga, pricelist', ev: 'Lead', val: 'manual', count: 142, last: '5 menit lalu' }, { name: 'Total ongkir', who: 'admin', op: 'mengandung', kw: 'total ongkir', ev: 'Hot Lead', val: 'angka', count: 0, last: '—' }];
    const rulesAll = (LV ? RB0 : (s.rulesList || RB0)).map(fixEv);
    const ron = LV ? Object.fromEntries(rulesAll.map((r, i) => [i, r.active !== false])) : (s.rulesOn || { 3: false });
    const atTab = s.atTab || 'list';
    const blank = { name: '', who: 'pelanggan', op: 'mengandung', kw: 'sudah transfer, udah tf', ev: 'Purchase', val: 'angka', fixed: '' };
    const rb = fixEv(s.rb || blank);
    const uR = (k, v) => set({ rb: { ...rb, [k]: v } });
    const fixedN = Number(String(rb.fixed || '').replace(/\D/g, '') || 0);
    const kws = (rb.kw || '').split(',').map((x) => x.trim()).filter(Boolean);
    const needV = rb.ev === 'Purchase';
    const valTxt = (r) => r.ev !== 'Purchase' ? 'tanpa nilai' : r.val === 'tetap' ? rp(Number(String(r.fixed || '').replace(/\D/g, '') || 0)) : VALM[r.val][2];
    const whenTxt = (r) => WHO[r.who][2] + ' ' + (r.op === 'sama dengan' ? 'persis ' : '') + r.kw.split(',').map((x) => '“' + x.trim() + '”').join(' / ');
    const draft = s.rtMsg ?? '';
    const from = s.rtFrom || 'pelanggan';
    const simLog = s.rtLog || [{ from: 'pelanggan', text: 'Sudah transfer ya kak, total Rp 450.000. Mohon dicek' }];
    const readVal = (msg) => { let found = 0; const ma = msg.match(/rp\.?\s?([\d.,]+)/i); if (ma) found = Number(ma[1].replace(/[.,]/g, '')); else { const mb = msg.match(/(\d+(?:[.,]\d+)?)\s?(rb|ribu|k)\b/i); if (mb) found = Math.round(parseFloat(mb[1].replace(',', '.')) * 1000); else { const mc = msg.match(/\b(\d{4,})\b/); if (mc) found = Number(mc[1]); } } return found; };
    const evalMsg = (f, text) => { const m = text.toLowerCase(); const whoOk = rb.who === 'template' ? f === 'admin' : f === rb.who; const hit = kws.find((k) => rb.op === 'mengandung' ? m.includes(k.toLowerCase()) : m.trim() === k.toLowerCase()); return { matched: whoOk && !!hit, found: readVal(text) }; };
    const simSend = () => { const text = draft.trim(); if (!text) return; set({ rtLog: [...simLog, { from, text }].slice(-20), rtMsg: '' }); };
    const radio = (on) => ({ width: 18, height: 18, flex: 'none', borderRadius: '50%', background: on ? 'var(--accent)' : 'var(--surface2)', boxShadow: on ? 'inset 0 0 0 4px var(--surface)' : 'inset 0 0 0 1.5px var(--borderStrong)' });
    const oc = (on) => ({ fontFamily: 'inherit', textAlign: 'left', display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 6, padding: 14, borderRadius: 12, border: 'none', cursor: 'pointer', background: 'var(--surface)', boxShadow: on ? 'inset 0 0 0 2px var(--accent)' : 'none' });
    const canSave = kws.length > 0 && (rb.val !== 'tetap' || !needV || fixedN > 0);
    const nameAuto = rb.name || (rb.ev + ' · ' + (kws[0] || 'kata kunci'));
    const at = {
      tabs: [['list', 'Aturan saya', rulesAll.length], ['new', s.rbEdit != null ? 'Ubah aturan' : 'Buat aturan baru', '']].map(([k, label, count]) => ({ label, count, pick: () => k === 'new' ? set({ atTab: 'new', rb: s.rb || blank }) : set({ atTab: 'list', rbEdit: null }), style: { fontFamily: 'inherit', display: 'inline-flex', alignItems: 'center', gap: 6, height: 36, padding: '0 14px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 13, fontWeight: 500, whiteSpace: 'nowrap', background: atTab === k ? 'var(--surface)' : 'transparent', color: atTab === k ? 'var(--text)' : 'var(--muted)' }, cs: { display: count === '' ? 'none' : 'inline-flex', fontSize: 11, fontWeight: 500, padding: '1px 7px', borderRadius: 999, background: 'var(--neutral)', color: 'var(--muted)' } })),
      isList: atTab === 'list', isNew: atTab === 'new', startNew: () => set({ atTab: 'new', rb: blank, rbEdit: null }),
      how: [],
      activeLabel: '',
      hasRules: rulesAll.length > 0, noRules: rulesAll.length === 0,
      rules: rulesAll.map((r, i) => { const on = ron[i] ?? true; return { name: r.name, when: whenTxt(r), then: r.ev + ' · ' + valTxt(r), stLabel: on ? 'Aktif' : 'Nonaktif', stPill: pill(on ? 'ok' : 'neutral', { height: 22, fontSize: 11 }), stats: on ? (r.count ? r.count + '× terpicu 7 hari · terakhir ' + r.last : 'Belum pernah terpicu') : 'Tidak memantau pesan',
        card: { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 16, padding: '16px 18px', borderRadius: 14, background: 'var(--surface2)', opacity: on ? 1 : 0.6 },
        edit: () => set({ atTab: 'new', rb: { fixed: '', ...r }, rbEdit: i }), del: () => { if (LV) { this.__host.liveDeleteRule(r.id).then((x) => say(x.ok ? 'Aturan dihapus' : 'Gagal menghapus aturan', x.ok ? r.name : x.message)); return; } set({ rulesList: rulesAll.filter((_, j) => j !== i), rulesOn: {} }); say('Aturan dihapus', r.name); }, ...sw(on, () => (LV ? this.__host.liveSaveRule({ ...r, active: !on }).then((x) => x.ok || say('Gagal mengubah aturan', x.message)) : set({ rulesOn: { ...ron, [i]: !on } }))) }; }),
      rb, whoOpts: Object.keys(WHO).map((k) => ({ label: WHO[k][0], desc: WHO[k][1], tech: '', pick: () => uR('who', k), style: oc(rb.who === k), radio: radio(rb.who === k) })),
      setKw: (e) => uR('kw', e.target.value), kwChips: kws, kwHint: kws.length > 1 ? 'cukup salah satu muncul' : kws.length ? '' : 'Tulis minimal satu kata kunci',
      opOpts: [['mengandung', 'Pesan mengandung kata ini'], ['sama dengan', 'Pesan persis sama']].map(([k, label]) => ({ label, pick: () => uR('op', k), style: { ...chip(rb.op === k), height: 30, fontSize: 12 } })),
      evList: Object.keys(EVL), setEv: (e) => uR('ev', e.target.value),
      needV,
      valOpts: Object.keys(VALM).map((k) => ({ label: VALM[k][0], desc: VALM[k][1], tech: '', pick: () => uR('val', k), style: oc(rb.val === k), radio: radio(rb.val === k) })),
      isFixed: rb.val === 'tetap', fixedVal: fixedN ? fixedN.toLocaleString('id-ID') : '', setFixed: (e) => uR('fixed', e.target.value),
      nameStep: needV ? '5' : '4', nameVal: rb.name || '', namePh: nameAuto, setName: (e) => uR('name', e.target.value),
      fromChips: [['pelanggan', 'Dari pelanggan'], ['admin', 'Dari admin']].map(([k, label]) => ({ label, pick: () => set({ rtFrom: k }), style: { ...chip(from === k), height: 30, fontSize: 12 } })),
      sim: {
        items: simLog.map(({ from: f, text }) => { const r = evalMsg(f, text); const mine = f === 'admin'; const value = !needV ? '' : rb.val === 'angka' ? (r.found ? ' · ' + rp(r.found) : '') : ' · ' + rp(fixedN);
          return { text, reply: r.matched ? '✓ ' + rb.ev + value : '✕ Tidak ada event',
            rowStyle: { display: 'flex', justifyContent: mine ? 'flex-end' : 'flex-start' },
            bubble: { maxWidth: '80%', padding: '8px 12px', borderRadius: mine ? '14px 14px 2px 14px' : '14px 14px 14px 2px', background: mine ? 'var(--neutral)' : 'var(--surface2)', color: 'var(--text)', fontSize: 13, lineHeight: 1.5, wordBreak: 'break-word' },
            replyRowStyle: { display: 'flex', justifyContent: 'center' },
            replyBubble: { padding: '4px 12px', borderRadius: 999, fontSize: 12, background: r.matched ? 'var(--ink)' : 'transparent', color: r.matched ? 'var(--inkText)' : 'var(--muted)' } }; }),
        draft, setDraft: (e) => set({ rtMsg: e.target.value }), send: simSend,
        key: (e) => { if (e.key === 'Enter') { e.preventDefault(); simSend(); } }, clear: () => set({ rtLog: [] }),
      },
      cancel: () => set({ atTab: 'list', rbEdit: null }),
      saveLabel: s.rbEdit != null ? 'Simpan perubahan' : 'Simpan & aktifkan', saveStyle: { ...bP, height: 46, flex: 2, opacity: canSave ? 1 : 0.4, cursor: canSave ? 'pointer' : 'not-allowed' },
      save: () => { if (!canSave) return;
        if (LV) { this.__host.liveSaveRule({ ...(s.rbEdit != null ? rulesAll[s.rbEdit] : {}), ...rb, name: nameAuto }).then((x) => { if (x.ok) { set({ atTab: 'list', rbEdit: null }); say(s.rbEdit != null ? 'Aturan diperbarui' : 'Aturan aktif', nameAuto); } else say('Aturan gagal disimpan', x.message); }); return; } const r = { ...rb, name: nameAuto, count: 0, last: '—' }; if (s.rbEdit != null) { const list = rulesAll.slice(); list[s.rbEdit] = { ...rulesAll[s.rbEdit], ...rb, name: nameAuto }; set({ rulesList: list, atTab: 'list', rbEdit: null }); say('Aturan diperbarui', nameAuto); } else { set({ rulesList: [...rulesAll, r], atTab: 'list' }); say('Aturan aktif', nameAuto); } },
    };

    // ---------- log ----------
    const ST = ['Terkirim', 'Terkirim', 'Terkirim', 'Duplikat', 'Terkirim', 'Retry', 'Terkirim', 'Gagal', 'Terkirim', 'Kedaluwarsa', 'Terkirim', 'Terkirim'];
    const EV = ['Purchase', 'LeadSubmitted', 'InitiateCheckout', 'Purchase', 'AddToCart', 'ViewContent', 'LeadSubmitted', 'Purchase'];
    const SRC = ['API', 'Manual', 'Otomatis', 'API', 'API', 'Otomatis', 'Manual'];
    const CODE = { Terkirim: '200', Gagal: '400', Retry: '429', Duplikat: '—', Kedaluwarsa: '400', Menunggu: '—', Mengirim: '—' };
    const lgov = s.lgOv || {};
    const LOG = LV ? LV.log : Array.from({ length: 28 }, (_, i) => { const ev = EV[i % 8]; return { i, ev, st: lgov[i] || ST[i % 12], src: SRC[i % 7], d: new Date(+NOW - (i * 47 + 5) * 60e3), value: ['Purchase', 'InitiateCheckout', 'AddToCart'].includes(ev) ? [450000, 1250000, 89000, 215000, 350000, 64000, 180000][i % 7] : 0, clid: clidOf(i + 40), id: 'evt_' + (90412 - i), order: 'order-' + (10482 - i) }; });
    const lgSt = s.lgSt || 'Semua', lgSrc = s.lgSrc || 'Semua';
    const lgRows = LOG.filter((r) => (lgSt === 'Semua' || r.st === lgSt) && (lgSrc === 'Semua' || r.src === lgSrc));
    const lg = {
      stChips: (LV ? ['Semua', 'Terkirim', 'Gagal', 'Retry', 'Menunggu', 'Kedaluwarsa'] : ['Semua', 'Terkirim', 'Gagal', 'Retry', 'Duplikat', 'Kedaluwarsa']).map((x) => ({ label: x, pick: () => set({ lgSt: x }), style: { ...chip(lgSt === x), height: 30, fontSize: 12 } })),
      srcChips: ['Semua', 'API', 'Manual', 'Otomatis'].map((x) => ({ label: x, pick: () => set({ lgSrc: x }), style: { ...chip(lgSrc === x), height: 30, fontSize: 12 } })),
      rows: lgRows.map((r) => ({ time: tfmt(r.d), ev: r.ev, src: r.src, ss: SRCB, value: r.value ? rp(r.value) : '—', clidShort: r.clid.slice(0, 14) + '…', st: r.st, pill: pill(EVP[r.st], EVP[r.st] === 'neutral' && r.st === 'Kedaluwarsa' ? { textDecoration: 'line-through' } : {}), code: CODE[r.st], open: () => set({ lgOpen: r.i + 1, lgTab: 'payload' }), copyIcon: s.copied === 'c' + r.i ? ic.check : ic.copy, copy: (e) => { e.stopPropagation(); copy(r.clid, 'c' + r.i); } })),
      count: lgRows.length + ' dari ' + LOG.length + (LV ? ' event terbaru' : ' event · 24 jam terakhir'),
    };
    const le = s.lgOpen ? LOG[s.lgOpen - 1] : null;
    let lgd = { open: false };
    if (le) {
      const ts = Math.floor(+le.d / 1000);
      const payload = { data: [{ event_name: le.ev, event_time: ts, event_id: le.order, action_source: 'business_messaging', messaging_channel: 'whatsapp', user_data: { ph: [sha(le.i)], ctwa_clid: le.clid, page_id: '104857392018273' }, ...(le.value ? { custom_data: { currency: 'IDR', value: le.value } } : {}) }] };
      const RESP = { Terkirim: { events_received: 1, messages: [], fbtrace_id: 'A7xQ2mLp9Zt' }, Gagal: { error: { message: 'Invalid parameter', type: 'OAuthException', code: 100, error_subcode: 2804050, error_user_msg: 'user_data.ph tidak valid' } }, Retry: { error: { message: 'Application request limit reached', code: 4 } }, Kedaluwarsa: { error: { message: 'event_time is too far in the past', code: 100, error_subcode: 2804003 } }, Duplikat: { skipped: true, reason: 'duplicate event_id', original: 'evt_' + (90412 - le.i - 3) } };
      const EX = { Gagal: ['Nomor telepon pelanggan tidak valid', 'Meta menolak karena nomor kosong atau formatnya salah sebelum di-hash. Event ini tidak diulang otomatis.', ['Pastikan nomor diawali 62, tanpa spasi atau tanda +.', 'Perbaiki data kontak, lalu klik Kirim ulang.']], Retry: ['Meta sedang membatasi permintaan', 'Terlalu banyak event dalam waktu singkat. Convera mencoba lagi otomatis — kamu tidak perlu melakukan apa-apa.', ['Percobaan berikutnya dalam ±2 menit (3 dari 5).']], Kedaluwarsa: ['Lewat batas 7 hari sejak klik iklan', 'Meta hanya menerima konversi CTWA maksimal 7 hari setelah klik. Event ini tidak bisa dikirim ulang.', ['Tandai Purchase lebih cepat dari halaman Leads.', 'Aktifkan Event agar closing tercatat seketika.']], Duplikat: ['Event yang sama sudah pernah dikirim', 'event_id ini sudah terkirim sebelumnya, jadi dilewati agar konversi tidak dihitung dua kali.', ['Tidak perlu tindakan.']] }[le.st];
      const t0 = String(le.d.getHours()).padStart(2, '0') + ':' + String(le.d.getMinutes()).padStart(2, '0');
      const RET = { Terkirim: [[t0 + ':04', '200', 'Diterima Meta · events_received: 1']], Duplikat: [[t0 + ':02', '—', 'Dilewati — event_id ' + le.order + ' sudah terkirim']], Retry: [[t0 + ':03', '429', 'Rate limit dari Meta'], [t0 + ':33', '429', 'Rate limit dari Meta'], ['Terjadwal', '—', 'Percobaan 3/5 dalam 2 menit']], Gagal: [[t0 + ':03', '400', 'Invalid parameter: user_data.ph'], [t0 + ':35', '400', 'Invalid parameter: user_data.ph'], [t0 + ':58', '400', 'Dihentikan — error permanen']], Kedaluwarsa: [[t0 + ':02', '400', 'event_time lebih dari 7 hari']] }[le.st] || [];
      const tab = s.lgTab || 'payload';
      lgd = { open: true, close: () => set({ lgOpen: null }), id: le.id + ' · ' + le.order, ev: le.ev, st: le.st, pill: pill(EVP[le.st]),
        tabs: [['payload', 'Payload'], ['respons', 'Respons Meta'], ['riwayat', 'Riwayat retry']].map(([k, label]) => ({ label, pick: () => set({ lgTab: k }), style: { fontFamily: 'inherit', height: 38, padding: 0, border: 'none', borderBottom: tab === k ? '2px solid var(--accent)' : '2px solid transparent', background: 'transparent', color: tab === k ? 'var(--text)' : 'var(--muted)', fontSize: 14, fontWeight: tab === k ? 500 : 400, cursor: 'pointer', whiteSpace: 'nowrap' } })),
        explain: !!EX, exTitle: EX ? EX[0] : '', exBody: EX ? EX[1] : '', tips: EX ? EX[2] : [],
        facts: [['WAKTU', tfmt(le.d) + ' WIB'], ['SUMBER', le.src], ['NILAI', le.value ? rp(le.value) : '—'], ['CTWA_CLID', le.clid]].map(([k, v]) => ({ k, v })),
        tPre: tab !== 'riwayat', pre: JSON.stringify(tab === 'payload' ? payload : RESP[le.st], null, 2), tRetry: tab === 'riwayat',
        retry: RET.map(([t, code, msg], k) => ({ t, code, msg, dot: { width: 10, height: 10, borderRadius: '50%', flex: 'none', marginTop: 4, background: k === RET.length - 1 ? 'var(--accent)' : 'var(--borderStrong)' } })),
        canResend: le.st === 'Gagal' || le.st === 'Retry',         resend: async () => {
          const eid = uid();
          set({ lgOv: { ...lgov, [le.i]: 'Mengirim' }, lgOpen: null });
          try {
            const pl = await buildEvent({ event: le.ev, value: le.value, clid: le.clid, phone: '6281200000000', eventId: eid, time: Date.now() });
            const r = await sendEvent(pl, { datasetId: s.f.dataset, token: s.f.token });
            set({ lgOv: { ...lgov, [le.i]: r.ok ? 'Terkirim' : 'Gagal' } });
            say(r.ok ? 'Event dikirim ulang' : 'Event gagal dikirim ulang', r.ok ? 'events_received: 1' : r.error);
          } catch (e) {
            set({ lgOv: { ...lgov, [le.i]: 'Gagal' } });
            say('Event gagal dikirim ulang', e.message || 'Terjadi kesalahan');
          }
        } };
      if (LV) {
        const tabL = s.lgTab || 'payload';
        lgd = { ...lgd, ...liveEventDetail(LV, le, tabL, tfmt, rp), tPre: tabL !== 'riwayat', tRetry: tabL === 'riwayat',
          resend: async () => { set({ lgOpen: null }); const r = await this.__host.liveRetry(le.eid); say(r.ok ? 'Event dikirim ulang' : 'Event gagal dikirim ulang', r.message); } };
      }
    }

    // ---------- laporan ----------
    const LPP = LV ? [[LV.range.label, LV.range.hint, 1]] : [['Minggu ini', '11–17 Okt 2026', 1], ['Minggu lalu', '4–10 Okt 2026', 0.88], ['Bulan ini', '1–17 Okt 2026', 2.4], ['September', '1–30 Sep 2026', 4.1]];
    const lpi = s.lpP || 0, lpk = LPP[lpi][2];
    const ch2 = s.lpCh || { email: true, wa: true };
    const lp = { periods: LPP.map(([label], i) => ({ label, pick: () => set({ lpP: i }), style: chip(lpi === i) })), range: LPP[lpi][1] + ' · dibuat 17 Okt 2026, 14:40 WIB',
      kpis: [['Chat dari iklan', fmt(1284 * lpk), '+12,4%'], ['Purchase', fmt(186 * lpk), '+21,6%'], ['Nilai pembelian', rpS(84720000 * lpk), '+18,9%'], ['ROAS', '11,9×', '+1,4']].map(([label, value, delta]) => ({ label, value, delta: delta + ' vs periode lalu' })),
      top: allAds.slice().sort((x, y) => y.a[5] / y.a[1] - x.a[5] / x.a[1]).slice(0, 3).map(({ a }, i) => ({ n: i + 1, name: a[0], purch: fmt(a[4] * lpk) + ' purchase', roas: roasT(a[5] / a[1]) })),
      pdf: () => { const ok = printReport({ title: WS[s.ws][0], range: lp.range, kpis: lp.kpis, top: lp.top, note: 'Laporan dibuat otomatis oleh Convera.' }); say(ok ? 'Laporan PDF disiapkan' : 'Pop-up diblokir', ok ? 'Pilih “Simpan sebagai PDF” di dialog cetak' : 'Izinkan pop-up untuk mengunduh PDF'); }, csv: () => csv('convera-laporan.csv', ['iklan', 'kampanye', 'spend', 'chat', 'lead', 'purchase', 'nilai'], allAds.map(({ a, camp }) => [a[0], camp, a[1], a[2], a[3], a[4], a[5]])),
      sw: sw(s.lpOn ?? true, () => set({ lpOn: !(s.lpOn ?? true) })), channels: [['email', 'Email'], ['wa', 'WhatsApp']].map(([k, label]) => ({ label, pick: () => set({ lpCh: { ...ch2, [k]: !ch2[k] } }), style: chip(ch2[k]) })),
      to: s.lpTo ?? [P.profileEmail, P.profilePhone].filter(Boolean).join(', '), setTo: (e) => set({ lpTo: e.target.value }), day: s.lpDay || 'Senin', setDay: (e) => set({ lpDay: e.target.value }), hour: s.lpHour || '08.00 WIB', setHour: (e) => set({ lpHour: e.target.value }),
      save: () => { const r = parseRecipients(s.lpTo ?? 'rizky@kopisenja.id, 0812-3456-7890'); if (!ch2.email && !ch2.wa) { say('Pilih minimal satu kanal'); return; } if (r.invalid.length) { say('Penerima tidak valid', r.invalid.join(', ')); return; } if (ch2.email && !r.emails.length) { say('Isi alamat email', 'Kanal Email butuh minimal satu email'); return; } if (ch2.wa && !r.phones.length) { say('Isi nomor WhatsApp', 'Kanal WhatsApp butuh minimal satu nomor'); return; } set({ lpSaved: { at: Date.now(), emails: r.emails, phones: r.phones } }); say('Jadwal laporan disimpan', 'Setiap ' + (s.lpDay || 'Senin') + ' ' + (s.lpHour || '08.00 WIB')); } };

    if (LV) Object.assign(lp, liveReport(LV, { rp, rpS, csv, tfmt }));
    // ---------- koneksi ----------
    const stP = (on) => pill(on ? 'ok' : 'err');
    const kRow = (k, v, hi) => ({ k, v, vs: { fontWeight: 500, color: hi ? 'var(--accentText)' : 'var(--text)', textAlign: 'right' } });
    const A = (label, go, st) => ({ label, go, style: st === 'p' ? bP : st === 'd' ? bD : bW });
    const cfOpen = (k) => set({ confirm: k });
    const kn = { tips: [[true, 'Nomor telepon dikirim dalam format 62… (sudah)'], [false, 'Tambahkan email pelanggan bila ada (+0,6 EMQ)'], [false, 'Kirim nama depan & belakang (fn, ln)']].map(([d, text]) => ({ text, mark: d ? '✓' : '+', dot: { width: 18, height: 18, flex: 'none', borderRadius: '50%', fontSize: 11, fontWeight: 600, display: 'flex', alignItems: 'center', justifyContent: 'center', background: d ? 'var(--accent)' : 'var(--neutral)', color: d ? '#fff' : 'var(--text)' } })),
      cards: [
        { icon: ic.meta, title: 'Meta Dataset', sub: 'Kopi Senja – WhatsApp · 1029384756102938', status: metaOn ? 'Terhubung' : 'Terputus', pill: stP(metaOn), emq: metaOn, alert: metaOn ? '' : 'Event baru tidak dikirim ke Meta sampai Dataset dihubungkan lagi.',
          rows: metaOn ? [kRow('Event diterima 7 hari', '1.204'), kRow('Masa berlaku token', tokenDays + ' hari lagi', tokenDays < 14), kRow('Test event code', 'TEST48213')] : [kRow('Terakhir aktif', '17 Okt, 14:40')],
          actions: metaOn ? [A('Ganti token', () => set({ tokModal: true, tokVal: '' }), 'p'), A('Tes koneksi', async () => { say('Menguji koneksi Meta…'); const r = await testConnection({ datasetId: s.f.dataset, token: s.f.token }); say(r.ok ? 'Koneksi Meta OK' : 'Meta menolak koneksi', r.ok ? (r.demo ? 'Mode demo · Dataset ID & token valid' : r.name) : r.error); }), A('Putuskan', () => cfOpen('meta'), 'd')] : [A('Hubungkan', () => { set({ disc: { ...disc, meta: false } }); say('Meta Dataset terhubung'); }, 'p')] },
        { icon: ic.chat, title: 'WhatsApp', sub: waPhoneMasked + ' · Cloud API', status: waOn ? 'Terhubung' : 'Terputus', pill: stP(waOn), emq: false, alert: waOn ? '' : 'Terputus sejak 12:31. 12 event ditahan dan akan dikirim otomatis setelah terhubung. Biasanya karena token System User dicabut.',
          rows: [kRow('Mode', 'Cloud API (resmi)'), kRow('Terakhir aktif', waOn ? 'Baru saja' : '17 Okt, 12:31', !waOn), kRow('Lead hari ini', '37'), kRow('Webhook ctwa_clid', waOn ? 'Aktif' : 'Tertunda', !waOn)],
          actions: waOn ? [A('Tambah nomor', () => set({ screen: 'onboarding', step: 2 })), A('Putuskan', () => cfOpen('wa'), 'd')] : [A('Hubungkan ulang', () => { set({ waOn: true, disc: { ...disc, wa: false }, banners: { ...s.banners, wa: false } }); say('WhatsApp terhubung lagi', '12 event tertahan sedang dikirim'); }, 'p'), A('Putuskan', () => cfOpen('wa'), 'd')] },
        { icon: ic.bars, title: 'Ad Account', sub: adOn ? 'act_2049381726 · Kopi Senja Ads' : 'Belum terhubung', status: adOn ? 'Terhubung' : 'Belum terhubung', pill: adOn ? stP(true) : pill('neutral'), emq: false, alert: '',
          rows: adOn ? [kRow('Spend 7 hari', 'Rp 7.120.000'), kRow('Sinkron terakhir', '14:05 · tiap jam'), kRow('Mata uang', 'IDR')] : [kRow('Dibutuhkan untuk', 'Spend, CPP, ROAS')],
          actions: adOn ? [A('Ganti akun', () => say('Pilih Ad Account', 'Membuka Meta Business…')), A('Putuskan', () => cfOpen('ad'), 'd')] : [A('Hubungkan Ad Account', () => { set({ disc: { ...disc, ad: false }, ws: s.ws === 1 ? 0 : s.ws }); say('Ad Account terhubung'); }, 'p')] },
      ] };
    const CF = { meta: ['Putuskan Meta Dataset?', 'Event baru tidak akan dikirim ke Meta dan optimasi iklan kehilangan sinyal konversi. Data yang sudah terkirim tetap aman.'], wa: ['Putuskan nomor WhatsApp?', 'Chat baru dari iklan tidak lagi tercatat sebagai Lead. Riwayat lead tetap tersimpan.'], ad: ['Putuskan Ad Account?', 'Spend, Cost per Purchase, dan ROAS tidak lagi diperbarui.'] };
    const cf = s.confirm ? { open: true, title: CF[s.confirm][0], body: CF[s.confirm][1], cta: 'Ya, putuskan', close: () => set({ confirm: null }), ok: () => { const k = s.confirm; set({ confirm: null, disc: { ...disc, [k]: true } }); say(CF[k][0].replace('Putuskan ', '').replace('?', '') + ' diputuskan'); } } : { open: false };
    const tk = s.tokModal ? { open: true, val: s.tokVal || '', set: (e) => set({ tokVal: e.target.value }), close: () => set({ tokModal: false }), save: async () => { const t = (s.tokVal || '').trim(); if (t.length < 20) { say('Token terlalu pendek', 'Salin ulang dari Events Manager'); return; } say('Menguji token…'); const r = await testConnection({ datasetId: s.f.dataset, token: t }); if (!r.ok) { say('Token ditolak', r.error); return; } set({ tokModal: false, tokenDays: 60, f: { ...s.f, token: t }, banners: { ...s.banners, token: false } }); say('Token diperbarui', 'Berlaku 60 hari · koneksi OK'); } } : { open: false };

    // ---------- integrasi ----------
    const ion = s.intOn || { 0: true };
    const ig = { items: [['Webhook / API', 'Kirim event dari bot, CRM, atau sistem kasir lewat endpoint Convera.', 'ok'], ['Google Sheets', 'Tandai Purchase dari baris spreadsheet — cocok untuk rekap order manual.', 'off'], ['n8n', 'Otomasi tanpa kode: trigger dari payment gateway, form, atau toko online.', 'off'], ['Form order', 'Form checkout ringan; setiap submit dikirim sebagai InitiateCheckout.', 'off'], ['Instagram DM', 'Lacak chat dari iklan Click-to-Instagram Direct.', 'soon'], ['TikTok', 'Kirim konversi ke TikTok Events API.', 'soon'], ['Messenger', 'Lacak chat dari iklan Click-to-Messenger.', 'soon']]
      .map(([name, desc, st], i) => { const on = st === 'ok' || ion[i]; const soon = st === 'soon'; return { name, desc, ini: ini(name), logo: thumb(i, 44), badge: soon ? 'SEGERA' : on ? 'TERHUBUNG' : 'TERSEDIA', bs: pill(soon ? 'neutral' : on ? 'ok' : 'plain', { height: 22, fontSize: 10, letterSpacing: '0.04em' }), btn: soon ? 'Kabari saya' : on ? 'Atur' : 'Hubungkan', btnStyle: { ...(soon ? bW : on ? bW : bP), alignSelf: 'flex-start' }, act: () => { if (soon) say('Kami akan kabari', name + ' segera hadir'); else if (on) set({ page: 'developer' }); else { set({ intOn: { ...ion, [i]: true } }); say(name + ' terhubung'); } } }; }) };

    // ---------- developer ----------
    const keys = s.keys || [{ label: 'Bot WhatsApp (produksi)', key: 'cvr_live_8f3k••••••••q2Lm', created: '2 Okt 2026', used: '2 menit lalu' }, { label: 'Google Sheets sync', key: 'cvr_live_a91x••••••••77Hd', created: '9 Okt 2026', used: 'Kemarin, 21.04' }];
    const sbox = !!s.sandbox;
    const URL_ = (sbox ? 'https://sandbox.convera.id' : 'https://api.convera.id') + '/v1/events';
    const KEYS = sbox ? 'cvr_test_…' : 'cvr_live_…';
    const CODES = {
      curl: 'curl -X POST ' + URL_ + ' \\\n  -H "Authorization: Bearer ' + KEYS + '" \\\n  -H "Content-Type: application/json" \\\n  -d \'{\n    "event_name": "Purchase",\n    "ctwa_clid": "ARAkLm3x9QpZ7vB2",\n    "event_id": "order-10482",\n    "value": 450000,\n    "currency": "IDR",\n    "customer": { "phone": "6281234567890" }\n  }\'',
      node: 'await fetch("' + URL_ + '", {\n  method: "POST",\n  headers: {\n    Authorization: "Bearer ' + KEYS + '",\n    "Content-Type": "application/json",\n  },\n  body: JSON.stringify({\n    event_name: "Purchase",\n    ctwa_clid: "ARAkLm3x9QpZ7vB2",\n    event_id: "order-10482",\n    value: 450000,\n    currency: "IDR",\n    customer: { phone: "6281234567890" },\n  }),\n});',
      python: 'import requests\n\nrequests.post(\n    "' + URL_ + '",\n    headers={"Authorization": "Bearer ' + KEYS + '"},\n    json={\n        "event_name": "Purchase",\n        "ctwa_clid": "ARAkLm3x9QpZ7vB2",\n        "event_id": "order-10482",\n        "value": 450000,\n        "currency": "IDR",\n        "customer": {"phone": "6281234567890"},\n    },\n)',
      php: '$ch = curl_init("' + URL_ + '");\ncurl_setopt_array($ch, [\n  CURLOPT_POST => true,\n  CURLOPT_HTTPHEADER => [\n    "Authorization: Bearer ' + KEYS + '",\n    "Content-Type: application/json",\n  ],\n  CURLOPT_POSTFIELDS => json_encode([\n    "event_name" => "Purchase",\n    "ctwa_clid"  => "ARAkLm3x9QpZ7vB2",\n    "value"      => 450000,\n    "currency"   => "IDR",\n  ]),\n]);\ncurl_exec($ch);',
    };
    const dc = s.devCode || 'curl';
    const whE = s.whEv || { sent: true, failed: true, quota: false, lead: false };
    const dv = { sbDesc: sbox ? 'Aktif — event dikirim dengan test_event_code dan tidak memengaruhi iklan.' : 'Nonaktif — event dikirim ke Meta dan dipakai untuk optimasi iklan.', sb: sw(sbox, () => set({ sandbox: !sbox })),
      newKey: () => set({ keyModal: { step: 'ask', label: '' } }),
      keys: keys.map((k, i) => ({ ...k, revoke: () => { set({ keys: keys.filter((_, j) => j !== i) }); say('API key dicabut', k.label); } })),
      url: URL_, copyUrl: () => copy(URL_, 'url'), copyUrlLabel: s.copied === 'url' ? 'Tersalin' : 'Salin',
      tabs: [['curl', 'cURL'], ['node', 'Node.js'], ['python', 'Python'], ['php', 'PHP']].map(([k, label]) => ({ label, pick: () => set({ devCode: k }), style: { fontFamily: 'inherit', height: 30, padding: '0 10px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12, fontWeight: 400, whiteSpace: 'nowrap', background: dc === k ? 'rgba(255,255,255,0.12)' : 'transparent', color: dc === k ? '#f3f0ea' : '#a39e95' } })),
      code: CODES[dc], copyCode: () => copy(CODES[dc], 'code'), copyCodeLabel: s.copied === 'code' ? 'Tersalin' : 'Salin',
      schema: [['event_name', 'Wajib', 'LeadSubmitted, Purchase, AddToCart, InitiateCheckout, atau ViewContent'], ['ctwa_clid', 'Wajib', 'Dari webhook pesan pertama (referral.ctwa_clid)'], ['value', 'Wajib*', 'Wajib untuk Purchase, AddToCart, InitiateCheckout'], ['event_id', 'Opsional', 'Untuk deduplikasi, mis. nomor order'], ['currency', 'Opsional', 'Default IDR'], ['customer.phone', 'Disarankan', 'Format 62…, di-hash SHA-256 otomatis'], ['customer.email', 'Opsional', 'Menaikkan EMQ'], ['event_time', 'Opsional', 'Unix detik, maksimal 7 hari ke belakang']].map(([k, req, d]) => ({ k, req, d, rs: { fontSize: 11, fontWeight: 500, color: req.startsWith('Wajib') ? 'var(--accentText)' : 'var(--muted)' } })),
      wh: s.wh ?? 'https://n8n.kopisenja.id/webhook/convera', setWh: (e) => set({ wh: e.target.value }),
      whEv: [['sent', 'event.sent'], ['failed', 'event.failed'], ['lead', 'lead.created'], ['quota', 'quota.warning']].map(([k, label]) => ({ label, pick: () => set({ whEv: { ...whE, [k]: !whE[k] } }), style: { ...chip(whE[k]), height: 30, fontSize: 12, fontFamily: "'Poppins',monospace" } })),
      secretMask: (s.whSecret || 'whsec_5Tq9xVb').slice(0, 9) + '…' + (s.whSecret || 'whsec_5Tq9xVb').slice(-4),
      testWh: async () => { const url = s.wh ?? 'https://n8n.kopisenja.id/webhook/convera'; say('Mengirim webhook tes…', url.replace('https://', '').split('/')[0]); let secret = s.whSecret; if (!secret) { secret = 'whsec_' + randomToken(24); set({ whSecret: secret }); } const r = await testWebhook(url, secret); say(r.ok ? 'Webhook tes terkirim' : 'Webhook gagal', r.ok ? 'HTTP ' + r.status : r.error); },
      saveWh: () => { const url = (s.wh ?? 'https://n8n.kopisenja.id/webhook/convera').trim(); if (!isHttps(url)) { say('URL webhook tidak valid', 'Harus diawali https://'); return; } if (!s.whSecret) set({ whSecret: 'whsec_' + randomToken(24) }); say('Webhook disimpan', url.replace('https://', '').split('/')[0]); } };
    const kmS = s.keyModal;
    const newKey = 'cvr_' + (sbox ? 'test' : 'live') + '_' + clidOf(keys.length + 7).slice(2, 26);
    const km = kmS ? { open: true, ask: kmS.step === 'ask', show: kmS.step === 'show', label: kmS.label, setLabel: (e) => set({ keyModal: { ...kmS, label: e.target.value } }), close: () => set({ keyModal: null }),
      create: () => set({ keyModal: { ...kmS, step: 'show', gen: 'cvr_' + (sbox ? 'test' : 'live') + '_' + randomToken(26) } }), key: kmS.gen || newKey, copy: () => copy(kmS.gen || newKey, 'nk'), copyLabel: s.copied === 'nk' ? 'Tersalin' : 'Salin',
      done: () => { set({ keyModal: null, keys: [...keys, { label: kmS.label || 'Key baru', key: (kmS.gen || newKey).slice(0, 13) + '••••••••' + (kmS.gen || newKey).slice(-4), created: new Date().toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' }), used: 'Belum pernah' }] }); say('API key dibuat', kmS.label || 'Key baru'); } } : { open: false };

    // ---------- pengaturan ----------
    const tabK = ['umum', 'notif', 'keamanan'].includes(s.setTab) ? s.setTab : 'umum';
    const roleP = (r) => pill(r === 'Owner' ? 'ok' : r === 'Admin' ? 'accent' : r === 'CS' ? 'soft' : 'neutral');
    const ownerName = P.profileName || 'Pemilik akun';
    const ownerEmail = P.profileEmail || 'Email belum diisi';
    const members = P.backendEnabled
      ? [[ownerName, ownerEmail, 'Owner', 'Aktif']]
      : [[ownerName, ownerEmail, 'Owner', 'Aktif'], ['Dina Maharani', 'dina@kopisenja.id', 'CS', 'Aktif · 5 mnt lalu'], ['Arif Rahman', 'arif@kopisenja.id', 'Admin', 'Aktif · kemarin'], ...(s.invites || [])];
    const bill = s.bill || 'bulanan';
    const PLANS = [['Starter', 149000, '2.000 event / bulan', ['1 workspace & 1 nomor WhatsApp', 'Tandai manual & API', 'Log event 30 hari']], ['Growth', 399000, '10.000 event / bulan', ['3 workspace & 3 nomor WhatsApp', 'Aturan otomatis & ROAS', 'Laporan terjadwal', 'Log event 90 hari']], ['Agency', 1290000, '50.000 event / bulan', ['Workspace tak terbatas', 'Dashboard Semua Workspace', 'Laporan white-label', 'Prioritas support WhatsApp']]];
    const curPlan = s.plan || null;
    const priceOf = (p) => bill === 'tahunan' ? p * 10 : p;
    const NT = [['WhatsApp / Meta terputus', 'Segera, agar event tidak tertahan', 1, 1], ['Token Meta hampir kedaluwarsa', '14, 7, dan 1 hari sebelumnya', 1, 1], ['Event gagal > 10 dalam 1 jam', 'Ringkasan error & saran perbaikan', 1, 0], ['Kuota event 80% & 100%', 'Agar tidak ada konversi yang terlewat', 1, 1], ['Ringkasan harian', 'Chat, Purchase, dan nilai kemarin', 0, 1], ['Laporan mingguan', 'Setiap Senin pagi', 1, 0], ['Anggota tim baru', 'Saat undangan diterima', 1, 0]];
    const nts = s.nt || {};
    const demoSessions = [[ic.monitor, 'Chrome · Windows', 'Jakarta · 103.47.x.x · sekarang', true], [ic.phone, 'Safari · iPhone 15', 'Jakarta · 2 jam lalu', false], [ic.monitor, 'Edge · Windows', 'Bandung · 3 hari lalu', false]];
    const formatSession = (item) => {
      const ua = item.user_agent || 'Perangkat tidak dikenal';
      const mobileDevice = /android|iphone|ipad|mobile/i.test(ua);
      const browser = /edg/i.test(ua) ? 'Edge' : /chrome/i.test(ua) ? 'Chrome' : /safari/i.test(ua) && !/chrome/i.test(ua) ? 'Safari' : 'Browser';
      const os = /windows/i.test(ua) ? 'Windows' : /android/i.test(ua) ? 'Android' : /iphone|ipad/i.test(ua) ? 'iOS' : /mac/i.test(ua) ? 'macOS' : 'Perangkat';
      const at = item.last_used_at || item.created_at;
      const when = item.current ? 'sekarang' : at ? new Date(at).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' }) : 'belum digunakan';
      return [mobileDevice ? ic.phone : ic.monitor, browser + ' · ' + os, (item.ip || 'IP tidak tersedia') + ' · ' + when, !!item.current, item.id];
    };
    const sess = P.backendEnabled ? (P.security?.sessions || []).map(formatSession) : demoSessions.filter((_, i) => !(s.sessOut || {})[i]);
    const auditAction = (action) => ({ 'auth.login': 'Masuk ke akun', 'auth.logout': 'Keluar dari akun', 'password.change': 'Mengubah password' }[action] || action || 'Aktivitas akun');
    const audit = P.backendEnabled
      ? (P.security?.audit || []).map((item) => ({ t: item.at ? new Date(item.at).toLocaleString('id-ID', { dateStyle: 'medium', timeStyle: 'short' }) : '—', who: item.actor_label || ownerName, what: auditAction(item.action), ip: item.ip || '—' }))
      : [['17 Okt, 14:32', 'Dina (CS)', 'Menandai Purchase Rp 89.000 · Dewi Lestari', '103.47.12.8'], ['17 Okt, 12:31', 'Sistem', 'WhatsApp +62 812-•••-7890 terputus', '—'], ['16 Okt, 20:10', 'Arif (Admin)', 'Mengubah aturan “Sudah transfer → Purchase”', '36.72.4.19'], ['16 Okt, 09:02', 'Rizky (Owner)', 'Mengundang ads@lumbungdigital.id sebagai Viewer', '103.47.12.8'], ['15 Okt, 18:44', 'Rizky (Owner)', 'Membuat API key “Google Sheets sync”', '103.47.12.8'], ['14 Okt, 08:15', 'Arif (Admin)', 'Mengganti access token Meta', '36.72.4.19']].map(([t, who, what, ip]) => ({ t, who, what, ip }));
    const generalF = { ...s.f, ...(P.activeWorkspace ? { biz: P.activeWorkspace.name, industry: P.activeWorkspace.category || s.f.industry } : {}) };
    const delOk = (s.delName || '') === WS[s.ws][0];
    const se = {
      tabs: [['umum', 'Umum'], ['notif', 'Notifikasi'], ['keamanan', 'Keamanan']].map(([k, label]) => ({ label, pick: () => set({ setTab: k }), style: { fontFamily: 'inherit', height: 34, padding: '0 14px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 13, fontWeight: 500, whiteSpace: 'nowrap', background: tabK === k ? 'var(--surface)' : 'transparent', color: tabK === k ? 'var(--text)' : 'var(--muted)' } })),
      umum: tabK === 'umum', tim: tabK === 'tim', paket: tabK === 'paket', notif: tabK === 'notif', keamanan: tabK === 'keamanan', privasi: tabK === 'privasi',
      defEv: s.defEv || 'Purchase', setDefEv: (e) => set({ defEv: e.target.value }), defVal: s.defVal ?? '150.000', setDefVal: (e) => { const d = e.target.value.replace(/\D/g, ''); set({ defVal: d ? Number(d).toLocaleString('id-ID') : '' }); }, saveUmum: async () => { if (!(generalF.biz || '').trim()) { say('Nama bisnis wajib diisi'); return; } try { if (P.saveWorkspaceSettings) await P.saveWorkspaceSettings({ name: generalF.biz.trim(), category: generalF.industry }); set({ f: { ...s.f, biz: generalF.biz, industry: generalF.industry } }); say('Pengaturan disimpan', generalF.biz); } catch (error) { say('Pengaturan gagal disimpan', error.message || 'Coba lagi.'); } },
      retention: s.retention || '90 hari', setRetention: (e) => set({ retention: e.target.value }),
      invEmail: s.invEmail ?? '', setInvEmail: (e) => set({ invEmail: e.target.value }), invRole: s.invRole || 'CS', setInvRole: (e) => set({ invRole: e.target.value }),
      invite: () => { const em = (s.invEmail || '').trim(); if (!/.+@.+\..+/.test(em)) { say('Email belum valid'); return; } set({ invites: [...(s.invites || []), [em.split('@')[0], em, s.invRole || 'CS', 'Undangan terkirim']], invEmail: '' }); say('Undangan terkirim', em); },
      members: members.map(([name, email, role, state]) => ({ name, email, role, state, ini: ini(name), ps: roleP(role) })),
      perms: [['Lihat dashboard & laporan', 1, 1, 1, 1], ['Tandai Purchase di Leads', 1, 1, 1, 0], ['Kelola aturan otomatis', 1, 1, 0, 0], ['Kelola koneksi & API key', 1, 1, 0, 0], ['Undang & atur anggota', 1, 1, 0, 0], ['Paket, tagihan & hapus workspace', 1, 0, 0, 0]].map(([label, ...c]) => ({ label, cells: c.map((v) => ({ t: v ? '✓' : '—', s: { textAlign: 'center', fontWeight: 600, color: v ? 'var(--accentText)' : 'var(--borderStrong)' } })) })),
      planName: curPlan ? 'Paket ' + curPlan : 'Trial Growth', planState: curPlan ? 'Aktif' : 'Sisa 9 hari', planPill: pill(curPlan ? 'ok' : 'soft'), planSub: curPlan ? 'Diperpanjang otomatis 17 Nov 2026 · QRIS' : 'Berakhir 26 Okt 2026. Pilih paket agar ROAS & Event tetap aktif.',
      billTabs: [['bulanan', 'Bulanan'], ['tahunan', 'Tahunan · hemat 2 bulan']].map(([k, label]) => ({ label, pick: () => set({ bill: k }), style: { fontFamily: 'inherit', height: 32, padding: '0 12px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12, fontWeight: 500, whiteSpace: 'nowrap', background: bill === k ? 'var(--surface)' : 'transparent', color: bill === k ? 'var(--text)' : 'var(--muted)' } })),
      plans: PLANS.map(([name, price, quota, feats]) => { const hi = name === 'Growth'; const cur = curPlan === name; return { name, quota, price: rp(priceOf(price)), per: bill === 'tahunan' ? '/ tahun' : '/ bulan', tag: hi ? 'POPULER' : '', tagStyle: { fontSize: 10, fontWeight: 600, letterSpacing: '0.04em', padding: '3px 8px', borderRadius: 999, background: 'var(--accent)', color: '#fff' },
        features: feats.map((t) => ({ t, ts: { color: hi ? 'var(--accent)' : 'var(--accentText)', fontWeight: 600 } })),
        card: { display: 'flex', flexDirection: 'column', gap: 14, padding: 22, borderRadius: 14, background: hi ? 'var(--ink)' : 'var(--surface2)', color: hi ? 'var(--inkText)' : 'var(--text)', minHeight: 320 },
        btnLabel: cur ? 'Paket aktif' : 'Pilih ' + name, btn: { ...BTN, height: 44, width: '100%', background: cur ? 'transparent' : hi ? 'var(--accent)' : 'var(--surface)', color: cur ? 'inherit' : hi ? '#fff' : 'var(--text)', cursor: cur ? 'default' : 'pointer' },
        choose: () => !cur && set({ checkout: { plan: name, price: priceOf(price), step: 'pay', method: 'qris' } }) }; }),
      invoices: [['INV-2026-0912', '12 Sep 2026', 'Starter · Bulanan', 149000], ['INV-2026-0812', '12 Agu 2026', 'Starter · Bulanan', 149000], ['INV-2026-0712', '12 Jul 2026', 'Starter · Bulanan', 149000]].map(([no, date, plan, a]) => ({ no, date, plan, amount: rp(a * 1.11), status: 'Lunas', pill: pill('ok'), dl: () => { const ok = printInvoice({ number: no, date, plan, amount: rp(a * 1.11), workspace: WS[s.ws][0] }); say(ok ? 'Invoice disiapkan' : 'Pop-up diblokir', ok ? 'Pilih “Simpan sebagai PDF” di dialog cetak' : 'Izinkan pop-up untuk mencetak invoice'); } })),
      notifs: NT.map(([label, desc, e, w], i) => { const em = nts['e' + i] ?? !!e, wa = nts['w' + i] ?? !!w; return { label, desc, em: sw(em, () => set({ nt: { ...nts, ['e' + i]: !em } })), wa: sw(wa, () => set({ nt: { ...nts, ['w' + i]: !wa } })) }; }),
      pw1: s.pw1 || '', pw2: s.pw2 || '', pw3: s.pw3 || '', setPw1: (e) => set({ pw1: e.target.value }), setPw2: (e) => set({ pw2: e.target.value }), setPw3: (e) => set({ pw3: e.target.value }),
      savePw: async () => { const err = checkPassword(s.pw1 || '', s.pw2 || '', s.pw3 || ''); if (err) { say(err); return; } try { if (P.changeSecurityPassword) await P.changeSecurityPassword(s.pw1, s.pw2); set({ pw1: '', pw2: '', pw3: '', pwUpdated: Date.now(), sessOut: {} }); say('Password diperbarui', 'Sesi lain sudah dikeluarkan'); } catch (error) { say('Password gagal diperbarui', error.message || 'Password saat ini salah.'); } },
      sessions: sess.map(([icon, device, meta, current, id], i) => ({ icon, device, meta, current, other: !current, ps: pill('soft'), out: async () => { try { if (P.revokeSecuritySession && id) await P.revokeSecuritySession(id); else set({ sessOut: { ...(s.sessOut || {}), [i + 1]: true } }); say('Sesi dikeluarkan', device); } catch (error) { say('Sesi gagal dikeluarkan', error.message || 'Coba lagi.'); } } })),
      audit,
      exportAll: () => { downloadJson('convera-export-' + new Date().toISOString().slice(0, 10) + '.json', { exported_at: new Date().toISOString(), workspace: WS[s.ws][0], leads: LEADS.map((l) => ({ id: l.id, name: l.name, phone: l.phone, campaign: l.A ? l.A.camp : null, ad: l.A ? l.A.name : null, first_message: l.msg, status: l.status, value: l.value, events: l.events })), manual_events: s.kvNew || [], rules: s.rulesList || null, settings: { default_event: s.defEv || 'Purchase', default_value: s.defVal ?? '150.000', retention: s.retention || '90 hari' } }); say('Data diekspor', 'File JSON sudah diunduh'); },
      delPhone: s.delPhone || '', setDelPhone: (e) => set({ delPhone: e.target.value }), delContact: () => { const ph = normPhone(s.delPhone || ''); if (ph.length < 9) { say('Isi nomor terlebih dahulu', 'Contoh: 0812-3456-7890'); return; }
        if (LV) { set({ delPhone: '' }); this.__host.liveErasePhone(ph).then((r) => say(r.ok ? 'Data kontak dihapus' : 'Gagal menghapus', r.message)); return; } const found = LEADS.filter((l) => normPhone(l.phone) === ph).length; if (!found) { say('Nomor tidak ditemukan', 'Tidak ada lead dengan nomor ini'); return; } set({ delPhone: '', delPhones: [...(s.delPhones || []), ph], kvNew: (s.kvNew || []).filter((e) => normPhone(e.phone) !== ph) }); say('Data kontak dihapus', found + ' lead & riwayat chat dihapus permanen'); },
      delName: s.delName || '', setDelName: (e) => set({ delName: e.target.value }), delStyle: { ...BTN, height: 44, background: '#fff', color: '#b8530f', opacity: delOk ? 1 : 0.5, cursor: delOk ? 'pointer' : 'not-allowed' },
      delWs: () => { if (!delOk) return; set({ delName: '', wsDelAt: Date.now() + 864e5 }); say('Permintaan hapus diterima', 'Workspace dihapus dalam 24 jam — bisa dibatalkan lewat email'); },
    };
    const coS = s.checkout;
    let co = { open: false };
    if (coS) {
      const tax = Math.round(coS.price * 0.11), total = rp(coS.price + tax);
      const left = coS.start ? Math.max(0, 900 - Math.floor((Date.now() - coS.start) / 1000)) : 900;
      const uC = (o) => set({ checkout: { ...coS, ...o } });
      co = { open: true, title: coS.step === 'done' ? 'Paket ' + coS.plan : 'Checkout · Paket ' + coS.plan, close: () => { clearInterval(this.iv); set({ checkout: null }); }, total,
        sPay: coS.step === 'pay', sQris: coS.step === 'qris', sVa: coS.step === 'va', sDone: coS.step === 'done',
        lines: [['Paket ' + coS.plan + ' · ' + bill, rp(coS.price)], ['PPN 11%', rp(tax)], ['Total', total]].map(([k, v], i) => ({ k, v, s: { display: 'flex', justifyContent: 'space-between', gap: 12, color: 'var(--text)', fontWeight: i === 2 ? 600 : 400, fontSize: i === 2 ? 15 : 13, paddingTop: i === 2 ? 8 : 0, borderTop: i === 2 ? '1px solid var(--bg)' : 'none' } })),
        methods: [['qris', 'QRIS', 'GoPay, OVO, DANA, ShopeePay, m-banking'], ['va', 'Virtual Account', 'BCA, Mandiri, BNI, BRI'], ['card', 'Kartu kredit / debit', 'Visa, Mastercard, JCB']].map(([k, label, desc]) => { const on = coS.method === k; return { label, desc, pick: () => uC({ method: k }), style: { fontFamily: 'inherit', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '14px 16px', borderRadius: 12, border: 'none', cursor: 'pointer', background: on ? 'var(--accentSoft)' : 'var(--surface2)', boxShadow: on ? 'inset 0 0 0 2px var(--accent)' : 'none' }, radio: { width: 18, height: 18, flex: 'none', borderRadius: '50%', background: on ? 'var(--accent)' : 'var(--surface)', boxShadow: on ? 'inset 0 0 0 4px var(--surface)' : 'inset 0 0 0 1.5px var(--borderStrong)' } }; }),
        pay: () => { if (coS.method === 'qris') { uC({ step: 'qris', start: Date.now() }); clearInterval(this.iv); this.iv = setInterval(() => this.forceUpdate(), 1000); } else if (coS.method === 'va') uC({ step: 'va' }); else { set({ checkout: { ...coS, step: 'done' }, plan: coS.plan }); } },
        timer: String(Math.floor(left / 60)).padStart(2, '0') + ':' + String(left % 60).padStart(2, '0'), back: () => { clearInterval(this.iv); uC({ step: 'pay' }); },
        paid: () => { clearInterval(this.iv); set({ checkout: { ...coS, step: 'done' }, plan: coS.plan, banners: { ...s.banners, trial: false } }); },
        copyVa: () => copy('8277081234567890', 'va'), copyVaLabel: s.copied === 'va' ? 'Tersalin' : 'Salin', doneText: 'Paket ' + coS.plan + ' aktif sampai ' + (bill === 'tahunan' ? '17 Okt 2027' : '17 Nov 2026') + '. Invoice dikirim ke email Owner.' };
    }

    // ---------- agency ----------
    const AGD = [[186, 84720000, 'err', 'WhatsApp terputus'], [64, 51200000, 'warn', 'Ad Account belum terhubung'], [92, 18400000, 'ok', 'Semua koneksi sehat'], [0, 0, 'neutral', 'Setup belum selesai'], [41, 12300000, 'ok', 'Semua koneksi sehat'], [128, 38900000, 'warn', 'Kuota 92% terpakai']];
    const agQ = (s.agQ || '').toLowerCase();
    const agItems = WS.map(([name, meta], i) => ({ name, meta, i })).filter((w) => !agQ || w.name.toLowerCase().includes(agQ));
    const ag = { q: s.agQ || '', setQ: (e) => set({ agQ: e.target.value }), summary: WS.length + ' workspace · ' + AGD.filter((d) => d[2] === 'err' || d[2] === 'warn').length + ' butuh perhatian',
      items: agItems.map((w) => { const d = AGD[w.i]; return { name: w.name, meta: w.meta, ini: ini(w.name), logo: thumb(w.i, 44), status: d[3], ss: pill(d[2] === 'ok' ? 'plain' : d[2] === 'neutral' ? 'neutral' : d[2] === 'err' ? 'err' : 'soft', { alignSelf: 'flex-start' }), kpis: [['Purchase', d[0] ? fmt(d[0]) : '—'], ['Nilai', d[1] ? rpS(d[1]) : '—'], ['ROAS', d[1] ? roasT(d[1] / (d[1] / (6 + w.i * 1.7))) : '—']].map(([label, value]) => ({ label, value })), open: () => set({ ws: w.i, page: 'overview' }) }; }) };

    const pg = {}; ['overview', 'leads', 'konversi', 'performa', 'aturan', 'log', 'laporan', 'koneksi', 'integrasi', 'developer', 'pengaturan', 'agency'].forEach((k) => (pg[k] = s.page === k));
    const tst = s.toast ? { open: true, title: s.toast.title, sub: s.toast.sub, style: { position: 'fixed', zIndex: 90, left: mobile ? 12 : 'auto', right: mobile ? 12 : 24, bottom: mobile ? 84 : 24, width: mobile ? 'auto' : 380, display: 'flex', alignItems: 'center', gap: 12, padding: '12px 14px', borderRadius: 12, background: 'var(--ink)', color: 'var(--inkText)', boxShadow: '0 18px 40px -16px rgba(0,0,0,0.5)' } } : { open: false };
    return { pg, ov, ld, mk, ldr, kv, pf, at, lg, lgd, lp, kn, cf, tk, ig, dv, km, se, co, ag, tst };
  }

  renderVals() {
    const s = this.state, P = this.props;
    const dark = s.dark ?? P.dark ?? false;
    const screen = s.screen ?? P.startScreen ?? 'app';
    const ic = this.icons();
    const rootStyle = { ...this.theme(dark, P.accent ?? 'orange', P.statusStyle ?? 'brand'), background: 'var(--bg)', color: 'var(--text)', minHeight: '100vh', fontFamily: "'Poppins',sans-serif" };
    const W = s.width, mobile = W < 768, wide = W >= 1024;
    const ownerPhone = P.profilePhone ? normPhone(P.profilePhone) : '';
    const ownerPhoneMasked = ownerPhone ? ownerPhone.slice(0, 4) + '-•••-' + ownerPhone.slice(-4) : 'nomor belum diisi';
    // Nomor WA yang terdaftar di koneksi (dari wa_sessions via health), fallback ke nomor pemilik akun
    const liveHealth = P.live?.raw?.overview?.health || null;
    const waPhoneMasked = displayPhone(liveHealth?.wa_phone || liveHealth?.wa_phone_masked) || ownerPhoneMasked;
    const metaConnected = !s.disc?.meta && !!s.f?.dataset && !!s.f?.token;
    const waConnected = liveWaConnected(liveHealth, !!s.waOn && !s.disc?.wa);
    const healthIssues = [metaConnected, waConnected].filter((connected) => !connected).length;
    const healthSummary = healthIssues ? healthIssues + ' koneksi bermasalah' : 'Semua koneksi sehat';
    const healthMetaStatus = metaConnected ? 'Terhubung · token berlaku ' + (s.tokenDays ?? 0) + ' hari lagi' : 'Terputus · hubungkan Meta Dataset';
    const healthWaTitle = 'WhatsApp ' + waPhoneMasked;
    const healthWaStatus = waConnected ? 'Terhubung · Cloud API' : 'Terputus · Cloud API';
    const common = { rootStyle, ic, f: { ...s.f, ...(P.activeWorkspace ? { biz: P.activeWorkspace.name, industry: P.activeWorkspace.category || s.f.industry } : {}) }, toggleDark: () => this.setState({ dark: !dark, menu: null }), darkLabel: dark ? 'Mode terang' : 'Mode gelap', darkIcon: dark ? ic.sun : ic.moon, isOnboarding: screen === 'onboarding', isApp: screen === 'app' };

    // ---------- onboarding ----------
    const STEPS = [['Workspace', 'Profil bisnis'], ['Hubungkan Meta', 'Dataset & token'], ['Hubungkan WhatsApp', 'Cloud API atau QR'], ['Cara kirim konversi', 'API, manual, otomatis'], ['Selesai', 'Cek & kirim event uji']];
    const HEAD = [
      ['WORKSPACE', 'Ceritakan bisnismu', 'Satu workspace = satu bisnis. Kamu bisa menambah workspace lain kapan saja — cocok untuk agency.'],
      ['META CONVERSIONS API', 'Hubungkan Meta Dataset', 'Convera mengirim event ke Dataset (Pixel) yang dipakai iklan Click-to-WhatsApp kamu.'],
      ['WHATSAPP', 'Hubungkan nomor WhatsApp', 'Chat dari iklan otomatis tercatat sebagai Lead beserta kampanye, ad set, dan iklan asalnya.'],
      ['KONVERSI', 'Bagaimana closing dicatat?', 'Pilih cara tim kamu menandai pembelian. Semua dikirim ke Meta maksimal 7 hari sejak klik iklan.'],
      ['SIAP', 'Semua hampir siap', 'Kirim satu event percobaan untuk memastikan data sampai di Meta Test Events.'],
    ];
    const obSteps = STEPS.map(([label, desc], i) => {
      const done = i < s.step, cur = i === s.step;
      return { label, desc, num: done ? '✓' : String(i + 1), line: i < 4, go: () => this.setState({ step: i }),
        numStyle: { width: 28, height: 28, borderRadius: '50%', flex: 'none', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 12, fontWeight: 600, background: done ? 'var(--accent)' : cur ? 'var(--text)' : 'var(--neutral)', color: done || cur ? 'var(--surface)' : 'var(--muted)', border: 'none' },
        lineStyle: { width: 2, flex: 1, minHeight: 22, background: done ? 'var(--accent)' : 'var(--border)', margin: '4px 0' },
        labelStyle: { fontSize: 14, fontWeight: cur ? 600 : 400, color: cur || done ? 'var(--text)' : 'var(--muted)' } };
    });
    const setF = (k) => (e) => { const v = e.target.value; this.setState((st) => ({ f: { ...st.f, [k]: v }, test: k === 'token' || k === 'dataset' ? 'idle' : st.test })); };
    const on = {}; Object.keys(s.f).forEach((k) => (on[k] = setF(k)));
    const card = (sel) => ({ fontFamily: 'inherit', textAlign: 'left', display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 10, padding: 18, borderRadius: 14, cursor: 'pointer', background: sel ? 'var(--accentSoft)' : 'var(--surface2)', border: 'none', boxShadow: sel ? 'inset 0 0 0 2px var(--accent)' : 'none' });
    const N = 25; let qr = '';
    for (let y = 0; y < N; y++) for (let x = 0; x < N; x++) {
      const fx = x < 8 ? x : x >= N - 8 ? x - (N - 7) : -9, fy = y < 8 ? y : y >= N - 8 ? y - (N - 7) : -9;
      const inF = (x < 8 && y < 8) || (x >= N - 8 && y < 8) || (x < 8 && y >= N - 8);
      const o = inF ? (fx >= 0 && fy >= 0 && fx < 7 && fy < 7 && (fx === 0 || fx === 6 || fy === 0 || fy === 6 || (fx >= 2 && fx <= 4 && fy >= 2 && fy <= 4))) : ((x * 73 + y * 151 + x * y * 7) % 11) < 5;
      if (o) qr += `M${x} ${y}h1v1h-1z`;
    }
    const M = [['api', 'API dari bot / CRM', 'Sistem kamu mengirim event otomatis lewat endpoint Convera. Paling akurat.', false, ic.code], ['manual', 'Tandai manual oleh CS', 'Tim CS klik “Tandai Purchase” di daftar Leads setelah transfer masuk.', false, ic.hand], ['auto', 'Aturan otomatis', 'Kirim event saat pesan mengandung kata kunci, mis. “sudah transfer”.', true, ic.zap]];
    const methods = M.map(([id, title, desc, growth, icon]) => { const sel = s.methods[id]; return { title, desc, growth, icon, tick: sel ? ic.check : null,
      toggle: () => this.setState((st) => ({ methods: { ...st.methods, [id]: !st.methods[id] } })),
      card: { fontFamily: 'inherit', textAlign: 'left', display: 'flex', alignItems: 'flex-start', gap: 14, padding: 18, borderRadius: 14, cursor: 'pointer', background: sel ? 'var(--accentSoft)' : 'var(--surface2)', border: 'none', boxShadow: sel ? 'inset 0 0 0 2px var(--accent)' : 'none' },
      box: { width: 22, height: 22, flex: 'none', borderRadius: 6, marginTop: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', background: sel ? 'var(--accent)' : 'var(--neutral)', border: 'none', color: '#fff' } }; });
    const nMethods = Object.values(s.methods).filter(Boolean).length;
    const metaOk = s.test === 'ok', waOk = s.wa === 'cloud' ? !!(s.f.phoneId && s.f.waba && s.f.waToken) : false;
    const ck = (title, desc, state, step) => ({ title, desc, fix: state !== 'ok', go: () => this.setState({ step }), mark: state === 'ok' ? ic.check : state === 'pending' ? '…' : '!',
      dot: { width: 28, height: 28, flex: 'none', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 13, fontWeight: 600, background: state === 'ok' ? 'var(--okBg)' : state === 'pending' ? 'var(--warnBg)' : 'var(--errBg)', color: state === 'ok' ? 'var(--okFg)' : state === 'pending' ? 'var(--warnFg)' : 'var(--errFg)' } });
    const checklist = [
      ck('Workspace “' + (s.f.biz || 'Tanpa nama') + '”', s.f.industry + ' · IDR · ' + s.f.tz.split(' ')[0], s.f.biz ? 'ok' : 'err', 0),
      ck('Meta Dataset', metaOk ? 'Terhubung · EMQ 7,8' : 'Belum dites — klik Tes Koneksi', metaOk ? 'ok' : 'err', 1),
      ck('WhatsApp', s.wa === 'qr' ? 'Menunggu scan QR' : waOk ? 'Cloud API · +62 812-•••-7890' : 'Data Cloud API belum lengkap', s.wa === 'qr' ? 'pending' : waOk ? 'ok' : 'err', 2),
      ck('Cara kirim konversi', nMethods ? M.filter(([id]) => s.methods[id]).map(([, t]) => t.split(' ')[0] === 'API' ? 'API' : t.split(' ')[0] === 'Tandai' ? 'Manual' : 'Otomatis').join(' · ') : 'Belum dipilih', nMethods ? 'ok' : 'err', 3),
    ];
    const sendDone = s.send === 'done';
    const btnBase = { fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 8, height: 46, padding: '0 20px', borderRadius: 10, fontSize: 15, fontWeight: 500, cursor: 'pointer', whiteSpace: 'nowrap' };
    const nextDisabled = s.step === 3 && nMethods === 0;
    const onboarding = {
      wide, narrowOb: !wide, obSteps, stepNo: s.step + 1, obKicker: HEAD[s.step][0], obTitle: HEAD[s.step][1], obSub: HEAD[s.step][2],
      progressStyle: { height: '100%', width: ((s.step + 1) / 5) * 100 + '%', background: 'var(--accent)', transition: 'width .3s' },
      s0: s.step === 0, s1: s.step === 1, s2: s.step === 2, s3: s.step === 3, s4: s.step === 4, f: s.f, on,
      openGuide: () => this.setState({ guide: true }), closeGuide: () => this.setState({ guide: false }), guideOpen: s.guide,
      runTest: async () => { this.setState({ test: 'loading' }); const r = await testConnection({ datasetId: this.state.f.dataset, token: this.state.f.token }); this.setState({ test: r.ok ? 'ok' : 'fail' }); },
      testLabel: s.test === 'loading' ? 'Menguji koneksi…' : s.test === 'idle' ? 'Tes Koneksi' : 'Tes ulang', testIcon: s.test === 'loading' ? this.spin() : ic.plug,
      testOk: s.test === 'ok', testFail: s.test === 'fail',
      pickCloud: () => this.setState({ wa: 'cloud' }), pickQr: () => this.setState({ wa: 'qr' }), cloudCard: card(s.wa === 'cloud'), qrCard: card(s.wa === 'qr'),
      waCloud: s.wa === 'cloud', waQr: s.wa === 'qr', qrPath: qr, spinnerDark: this.spin(),
      methods, checklist, sentDone: sendDone,
      sendTitle: sendDone ? 'Event percobaan diterima Meta' : 'Kirim event percobaan',
      sendDesc: s.sendErr ? 'Gagal: ' + s.sendErr : sendDone ? 'Purchase · Rp 1 · test_event_code TEST48213 — cek di Events Manager → Test Events.' : 'Kami kirim Purchase bernilai Rp 1 dengan test_event_code, tidak memengaruhi iklan.',
      sendLabel: s.send === 'loading' ? 'Mengirim…' : sendDone ? 'Kirim lagi' : 'Kirim event percobaan', sendIcon: s.send === 'loading' ? this.spin() : ic.send,
      sendTest: async () => { this.setState({ send: 'loading', pop: false, sendErr: '' }); const pl = await buildEvent({ event: 'Purchase', value: 1, clid: 'test_' + randomToken(16), phone: '6281200000000', testCode: 'TEST48213' }); const r = await sendEvent(pl, { datasetId: this.state.f.dataset, token: this.state.f.token }); if (!r.ok) { this.setState({ send: 'idle', sendErr: r.error }); return; } this.setState({ send: 'done' }); setTimeout(() => this.setState({ pop: true }), 30); },
      popStyle: { width: 52, height: 52, flex: 'none', borderRadius: '50%', background: 'var(--accent)', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', transform: s.pop ? 'scale(1)' : 'scale(0.4)', opacity: s.pop ? 1 : 0, transition: 'transform .45s cubic-bezier(.34,1.56,.64,1), opacity .25s', boxShadow: s.pop ? '0 0 0 8px var(--accentSoft)' : 'none' },
      prev: () => this.setState({ step: Math.max(0, s.step - 1) }),
      next: () => (s.step < 4 ? this.setState({ step: s.step + 1 }) : this.setState({ screen: 'app', page: 'overview' })),
      nextLabel: s.step < 4 ? 'Lanjut' : 'Buka dashboard', nextDisabled,
      nextStyle: { ...btnBase, border: 'none', background: 'var(--accent)', color: '#fff', opacity: nextDisabled ? 0.4 : 1, cursor: nextDisabled ? 'not-allowed' : 'pointer' },
      backStyle: { ...btnBase, padding: '0 10px', border: 'none', background: 'transparent', color: 'var(--muted)', visibility: s.step === 0 ? 'hidden' : 'visible' },
      saveLater: () => this.setState({ screen: 'app' }),
    };

    // ---------- app shell ----------
    const collapsed = mobile ? false : (s.collapsed ?? W < 1100);
    const expanded = !collapsed;
    const NAV = [
      ['overview', 'Overview', 'dash', null, null, '', 3],
      ['leads', 'Leads', 'users', null, null, '', 4],
      ['konversi', 'Konversi', 'bag', null, null, '', 4],
      ['aturan', 'Event', 'zap', 'GROWTH', null, '', 7],
      ['log', 'Log Event', 'list', null, 'DATA', '', 5],
      ['laporan', 'Laporan', 'file', 'GROWTH', null, '', 7],
      ['koneksi', 'Koneksi', 'link', null, 'PENGATURAN', '', 5],
      ['pengaturan', 'Pengaturan', 'sliders', null, null, '', 6],
      ['agency', 'Semua Workspace', 'grid', 'AGENCY', null, '', 7],
    ];
    const navItem = (id, label, iconKey, badge, group) => {
      const a = s.page === id;
      const locked = P.subExpired && id !== 'overview';
      return { label, icon: ic[iconKey], badge: expanded ? badge : null, group: expanded ? group : (group ? ' ' : null), showLabel: expanded,
        alert: id === 'koneksi', alertStyle: { width: 7, height: 7, borderRadius: '50%', background: 'var(--errDotStrong)', flex: 'none', ...(collapsed ? { position: 'absolute', top: 8, right: 10 } : {}) },
        groupStyle: expanded ? { fontSize: 11, fontWeight: 500, letterSpacing: '0.08em', color: 'var(--muted)', padding: '14px 10px 6px' } : { display: 'block', height: 1, background: 'var(--border)', margin: '10px 8px' },
        locked,
        go: () => { if (locked) return; this.setState({ page: id, menu: null, more: false }); },
        style: { fontFamily: 'inherit', position: 'relative', display: 'flex', alignItems: 'center', gap: 12, width: '100%', height: 40, padding: collapsed ? 0 : '0 10px', justifyContent: collapsed ? 'center' : 'flex-start', borderRadius: 10, border: 'none', cursor: locked ? 'not-allowed' : 'pointer', fontSize: 14, fontWeight: a ? 500 : 400, textAlign: 'left', background: a ? 'var(--accentSoft)' : 'transparent', color: a ? 'var(--accentText)' : locked ? 'var(--border)' : 'var(--muted)', flex: 'none', opacity: locked ? 0.45 : 1 } };
    };
    const nav = NAV.filter(([id]) => id !== 'agency').map(([id, l, i, b, g]) => navItem(id, l, i, b, g));
    const sidebarStyle = { width: collapsed ? 76 : 264, flex: 'none', position: 'sticky', top: 0, height: '100vh', display: 'flex', flexDirection: 'column', gap: 16, padding: collapsed ? '18px 12px' : '18px 14px', background: 'var(--surface)', borderRight: '1px solid var(--border)', transition: 'width .2s', zIndex: 32 };
    const WS = [['Kopi Senja', 'F&B · Jakarta'], ['Klinik Gigi Senyum', 'Klinik · Bandung'], ['Rumah Pijat Sehati', 'Jasa · Surabaya'], ['Laundry Kilat Bersih', 'Jasa · Depok'], ['Bimbel Cerdas Mandiri', 'Pendidikan · Medan'], ['Toko Hijab Amira', 'Fashion · Yogyakarta']];
    // Mode backend: daftar workspace nyata dari server (App meneruskannya lewat props.workspaces); selain itu daftar contoh.
    const WSP = Array.isArray(this.props.workspaces) ? this.props.workspaces : null;
    if (WSP) { WS.length = 0; WSP.forEach((w) => WS.push([w.name, w.meta, w.slug])); if (!WS.length) WS.push(['Belum ada workspace', 'Tambahkan workspace', '']); } else WS[0][0] = s.f.biz || 'Kopi Senja';
    const q = s.wsQuery.trim().toLowerCase();
    const wsList = WS.map(([name, meta], i) => ({ name, meta, i })).filter((w) => !q || w.name.toLowerCase().includes(q)).map((w) => ({ name: w.name, meta: w.meta, active: w.i === s.ws, ini: w.name.split(' ').map((x) => x[0]).join('').slice(0, 2).toUpperCase(),
      pick: () => this.setState({ ws: w.i, menu: null, more: false, wsQuery: '' }),
      style: { fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 10, width: '100%', padding: '8px 10px', borderRadius: 8, border: 'none', cursor: 'pointer', textAlign: 'left', background: w.i === s.ws ? 'var(--surface2)' : 'transparent' } }));
    const menu = (m) => () => this.setState({ menu: s.menu === m ? null : m });
    const customFrom = s.customFrom || '', customTo = s.customTo || '';
    const setCustomDate = (key) => (e) => {
      e.stopPropagation();
      const value = e.target.value;
      this.setState((st) => ({ [key]: value, range: key === 'customFrom' ? (value && st.customTo ? 6 : st.range) : (st.customFrom && value ? 6 : st.range) }));
    };
    const customLabel = React.createElement('span', { style: { display: 'flex', flexDirection: 'column', gap: 6, flex: 1 } },
      React.createElement('span', null, 'Pilih tanggal'),
      React.createElement('span', { style: { display: 'flex', gap: 6 } },
        React.createElement('input', { type: 'date', value: customFrom, onClick: (e) => e.stopPropagation(), onChange: setCustomDate('customFrom'), 'aria-label': 'Tanggal mulai', style: { minWidth: 0, width: '50%', height: 30, border: '1px solid var(--border)', borderRadius: 6, padding: '0 5px', background: 'var(--surface2)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11 } }),
        React.createElement('input', { type: 'date', value: customTo, onClick: (e) => e.stopPropagation(), onChange: setCustomDate('customTo'), 'aria-label': 'Tanggal akhir', style: { minWidth: 0, width: '50%', height: 30, border: '1px solid var(--border)', borderRadius: 6, padding: '0 5px', background: 'var(--surface2)', color: 'var(--text)', fontFamily: 'inherit', fontSize: 11 } })),
    );
    const PRE = [0, 1, 2, 3, 4, 5].map((i) => { const r = rangeFor(i); return [r.label, r.hint]; }).concat([['Kustom', customFrom && customTo ? customFrom + ' – ' + customTo : 'Pilih rentang']]);
    const presets = PRE.map(([label, hint], i) => ({ label: i === 6 ? customLabel : label, hint: i === 6 ? '' : hint, pick: () => { if (i < 6) this.setState({ range: i, menu: null }); }, style: { fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 8, padding: '9px 10px', borderRadius: 8, border: 'none', cursor: i === 6 ? 'default' : 'pointer', textAlign: 'left', fontSize: 13, fontWeight: i === s.range ? 500 : 400, background: i === s.range ? 'var(--accentSoft)' : 'transparent', color: i === s.range ? 'var(--accentText)' : 'var(--text)' } }));
    // Mode backend: notifikasi nyata dari API; mode demo tetap memakai data contoh.
    const agoId = (iso) => { const m = Math.max(0, Math.round((Date.now() - new Date(iso)) / 60000)); return m < 1 ? 'Baru saja' : m < 60 ? m + ' mnt lalu' : m < 1440 ? Math.floor(m / 60) + ' jam lalu' : Math.floor(m / 1440) + ' hari lalu'; };
    const realNotifs = P.live ? ((P.notifs && P.notifs.items) || []).filter((n) => !(waOn && n.type === 'wa_disconnected')) : null;
    // Notif trial: inject di posisi pertama jika status trial atau expired
    const trialNotif = (() => {
      const rs = P.realSub;
      if (!rs) return null;
      const daysLeft = rs.trial_until ? Math.max(0, Math.ceil((new Date(rs.trial_until) - Date.now()) / 864e5)) : 0;
      if (rs.status === 'trial' && daysLeft > 0) {
        const tone = daysLeft <= 3 ? 'err' : 'warn';
        const icon = daysLeft <= 3 ? 'alert' : 'clock';
        return [tone, icon, 'Trial tersisa ' + daysLeft + ' hari · Upgrade sekarang agar semua fitur tetap aktif.', 'Sekarang', true];
      }
      if (rs.status === 'expired' || rs.status === 'stopped' || (rs.status === 'trial' && daysLeft === 0)) {
        return ['err', 'alert', 'Masa trial berakhir · Semua fitur terkunci kecuali Overview. Upgrade untuk membuka akses penuh.', 'Sekarang', true];
      }
      return null;
    })();
    const NOT = realNotifs
      ? [...(trialNotif ? [trialNotif] : []), ...realNotifs.map((n) => {
        const ty = String(n.type || '');
        const tn = /reject|invalid|fail|disconnect|expired/.test(ty) ? 'err' : /remind|expir|token|trial|soon/.test(ty) ? 'warn' : /approv|activ|paid|success|verified/.test(ty) ? 'ok' : 'info';
        return [tn, tn === 'err' ? 'alert' : tn === 'warn' ? 'clock' : tn === 'ok' ? 'bag' : 'users', n.body ? n.title + ' · ' + n.body : n.title, agoId(n.created_at), !n.read_at];
      })]
      : [['err', 'alert', 'WhatsApp ' + waPhoneMasked + ' terputus. Event dari nomor ini ditahan.', '2 jam lalu'], ['warn', 'clock', 'Token Meta kedaluwarsa dalam 5 hari.', '5 jam lalu'], ['ok', 'bag', '48 Purchase terkirim kemarin · Rp 21.300.000', 'Kemarin, 08.00'], ['info', 'users', 'Dina (CS) bergabung ke workspace Kopi Senja.', '2 hari lalu']];
    const isUnread = (k, flag) => (realNotifs ? !!flag : !s.read && k < 3);
    const tone = { err: ['var(--errBg)', 'var(--errFg)'], warn: ['var(--warnBg)', 'var(--warnFg)'], ok: ['var(--okBg)', 'var(--okFg)'], info: ['var(--neutral)', 'var(--text)'] };
    const notifs = NOT.map(([t, i, title, time, unreadFlag], k) => ({ title, time, icon: ic[i], unread: isUnread(k, unreadFlag),
      row: { display: 'flex', gap: 12, padding: '14px 16px', borderBottom: '1px solid var(--border)', background: isUnread(k, unreadFlag) ? 'var(--surface2)' : 'transparent' },
      iconStyle: { width: 30, height: 30, flex: 'none', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', background: tone[t][0], color: tone[t][1] } }));
    const BAN = [
      ['wa', 'err', 'WhatsApp ' + waPhoneMasked + ' terputus.', 'Event dari nomor ini ditahan sampai terhubung lagi.', 'Hubungkan ulang', 'koneksi'],
      ['token', 'warn', 'Token Meta kedaluwarsa dalam 5 hari.', 'Event berhenti terkirim setelah 22 Okt.', 'Ganti token', 'koneksi'],
      ['trial', 'info', 'Trial Growth tersisa 9 hari.', 'Pilih paket agar fitur ROAS & Event tetap aktif.', 'Pilih paket', 'pengaturan'],
    ].filter(([k]) => s.banners[k] && (k !== 'wa' || !waConnected));
    const bStyle = { err: ['var(--errBg)', 'var(--text)'], warn: ['var(--accentSoft)', 'var(--accentText)'], info: ['var(--neutral)', 'var(--text)'] };
    const banners = BAN.slice(0, 1).map(([k, t, title, body, cta, page]) => ({ title, body: '', cta, icon: t === 'info' ? ic.clock : ic.alert,
      act: () => this.setState({ page }), close: () => this.setState((st) => ({ banners: { ...st.banners, [k]: false } })),
      style: { display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 12, padding: '10px 10px 10px 14px', borderRadius: 12, background: bStyle[t][0], ...(t === 'err' ? { '--text': '#1d1c1a', '--surface': '#ffffff', '--muted': '#1d1c1a' } : {}) },
      iconStyle: { display: 'flex', flex: 'none', color: bStyle[t][1] } }));
    const page = NAV.find(([id]) => id === s.page) || NAV[0];
    const MOB = [['overview', 'Overview', 'dash'], ['leads', 'Leads', 'users'], ['konversi', 'Konversi', 'bag'], ['more', 'Lainnya', 'more']];
    const bottomNav = MOB.map(([id, label, i]) => {
      const a = id === 'more' ? s.more || !['overview', 'leads', 'konversi'].includes(s.page) : s.page === id;
      const locked = P.subExpired && id !== 'overview' && id !== 'more';
      return { label, icon: ic[i], locked,
        go: () => { if (locked) return; id === 'more' ? this.setState({ more: true }) : this.setState({ page: id, more: false }); },
        style: { fontFamily: 'inherit', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 4, border: 'none', background: 'transparent', cursor: locked ? 'not-allowed' : 'pointer', fontSize: 11, fontWeight: a ? 600 : 400, color: locked ? 'var(--border)' : a ? 'var(--accentText)' : 'var(--muted)', opacity: locked ? 0.45 : 1 } };
    });
    const moreNav = NAV.filter(([id]) => !['overview', 'leads', 'konversi'].includes(id)).map(([id, label, i, badge]) => {
      const locked = P.subExpired && id !== 'overview';
      return { label, badge, icon: ic[i], locked,
        go: () => { if (locked) return; this.setState({ page: id, more: false }); },
        style: { fontFamily: 'inherit', display: 'flex', alignItems: 'center', gap: 10, height: 48, padding: '0 12px', borderRadius: 10, border: '1px solid var(--border)', background: s.page === id ? 'var(--accentSoft)' : 'var(--surface)', color: locked ? 'var(--border)' : 'var(--text)', fontSize: 13, fontWeight: 400, cursor: locked ? 'not-allowed' : 'pointer', opacity: locked ? 0.45 : 1 } };
    });
    const app = {
      mobile, notMobile: !mobile, wideTop: W >= 1180, showSidebar: !mobile, expanded, collapsedQuota: collapsed, sidebarStyle,
      subExpired: !!P.subExpired,
      sbHeadStyle: { display: 'flex', alignItems: 'center', justifyContent: collapsed ? 'center' : 'space-between', flexDirection: collapsed ? 'column' : 'row', gap: 10, padding: collapsed ? 0 : '2px 4px 2px 6px' },
      toggleCollapse: () => this.setState({ collapsed: !collapsed }),
      wsName: WS[s.ws][0], wsMeta: WS[s.ws][1] + ' · Growth', wsInitial: WS[s.ws][0].charAt(0),
      wsBtnStyle: { fontFamily: 'inherit', width: '100%', display: 'flex', alignItems: 'center', justifyContent: collapsed ? 'center' : 'flex-start', gap: 10, padding: collapsed ? 6 : '8px 10px', borderRadius: 10, border: '1px solid var(--border)', background: 'var(--surface2)', cursor: 'pointer' },
      openWs: menu('ws'), wsMenu: s.menu === 'ws', wsQuery: s.wsQuery, onWsQuery: (e) => this.setState({ wsQuery: e.target.value }), wsList, wsAll: WS.map(([name, meta, slug]) => ({ name, meta, slug })), wsEmpty: wsList.length === 0,
      newWs: () => this.setState({ screen: 'onboarding', step: 0, menu: null, test: 'idle', send: 'idle' }), allWs: () => this.setState({ page: 'agency', menu: null }),
      nav, menuOpen: false, closeMenus: () => this.setState({ menu: null }),
      openDate: menu('date'), dateMenu: s.menu === 'date', dateLabel: PRE[s.range][0] + (W >= 1280 ? ' · ' + PRE[s.range][1] : ''), presets,
      openHealth: menu('health'), healthMenu: s.menu === 'health', healthSummary, healthMetaStatus, healthWaTitle, healthWaStatus, goKoneksi: () => this.setState({ page: 'koneksi', menu: null }),
      openNotif: menu('notif'), notifMenu: s.menu === 'notif', notifs, unread: realNotifs ? ((P.notifs.unread || 0) + (trialNotif ? 1 : 0)) : (s.read ? 0 : 3), readAll: () => (realNotifs ? P.readAllNotifs && P.readAllNotifs() : this.setState({ read: true })),
      notifPanelStyle: { position: mobile ? 'fixed' : 'absolute', top: mobile ? 70 : 'calc(100% + 6px)', right: mobile ? 12 : 0, left: mobile ? 12 : 'auto', width: mobile ? 'auto' : 360, background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 12, boxShadow: '0 20px 44px -18px rgba(0,0,0,0.35)', display: 'flex', flexDirection: 'column', overflow: 'hidden' },
      openHelp: menu('help'), helpMenu: s.menu === 'help', openUser: menu('user'), userMenu: s.menu === 'user',
      goOnboarding: () => this.setState({ screen: 'onboarding', menu: null }),
      banners, moreBanners: BAN.length > 1 ? BAN.length - 1 : 0,
      pageTitle: page[1], pageBadge: 'v1.1', pageDesc: page[5], pagePhase: page[6],
      mainStyle: { padding: mobile ? '18px 16px 96px' : '24px 28px 48px', display: 'flex', flexDirection: 'column', gap: 18, width: '100%', maxWidth: 1440 },
      skel4: [1, 2, 3, 4], skelRows: [1, 2, 3, 4, 5].map((k) => ({ a: { width: 120 + ((k * 37) % 80), height: 12, borderRadius: 6, background: 'var(--neutral)' }, b: { width: 70 + ((k * 23) % 40), height: 12, borderRadius: 6, background: 'var(--neutral)' } })),
      bottomNav, moreOpen: s.more, openMore: () => this.setState({ more: true }), closeMore: () => this.setState({ more: false }), moreNav,
    };
    return { ...common, ...onboarding, ...app, ...this.pagesVals(s, P, ic, W, mobile, WS) };
  }
}

export default Component;
