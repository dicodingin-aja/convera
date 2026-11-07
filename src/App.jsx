import React from 'react';
import Logic from './Logic.js';
import Template from './Template.jsx';
import defaults from './defaults.json';
import { load, save, reset } from './services/storage.js';
import ProfilePage from './ProfilePage.jsx';
import { LaporanPage, DashboardExtras } from './ReportViews.jsx';
import PlansModal from './PlansModal.jsx';
import { updateProfile, changePassword, ensureWorkspace, getSubscription, listWorkspaces, updateWorkspace, listSessions, revokeSession, listAuditLogs } from './services/backend.js';
import AddWorkspaceCard from './CreateWorkspace.jsx';
import QrScanPage from './QrScanPage.jsx';
import { fetchLive, emptyLive, liveApi, notifApi, rangeFor } from './services/live.js';
import { appConfig } from './services/config.js';
import PaymentModal from './PaymentModal.jsx';
import LoginPage from './LoginPage.jsx';
import { getSession, logout, initials, clearLocalSession } from './services/auth.js';
import { getSub, subInfo, activate } from './plans.js';
import KoneksiSettings, { CONN_DEFAULT } from './KoneksiSettings.jsx';

const IDLE_LIMIT_MS = 30 * 60 * 1000;

// Host: owns the design's logic instance (state, timers, handlers) and renders the template with its values.
export default class App extends React.Component {
  constructor(props) {
    super(props);
    this.props_ = { ...defaults, ...props };
    this.logic = new Logic(this.props_);
    this.logic.__host = this;
    // pulihkan data tersimpan; event yang masih "Mengirim" saat halaman ditutup dianggap gagal
    const saved = load();
    if (saved.kvNew) saved.kvNew = saved.kvNew.map((k) => (k.status === 'Mengirim' ? { ...k, status: 'Gagal', err: 'Terputus sebelum terkirim' } : k));
    if (['integrasi', 'developer', 'performa'].includes(saved.page)) saved.page = 'overview';
    this.logic.state = { ...this.logic.state, ...saved };
    // route: null = dashboard, 'profile' = halaman Profil Saya. plansFor = indeks workspace yang sedang memilih paket.
    this.state = { v: 0, route: null, plansFor: null, session: getSession(), realSub: null, workspaces: null, live: null, createWsOpen: false };
    window.convera = { reset, state: () => this.logic.state };
  }
  __setLogicState(update, cb) {
    const prev = this.logic.state;
    const patch = typeof update === 'function' ? update(prev) : update;
    this.logic.state = { ...prev, ...patch };
    save(this.logic.state);
    this.setState((s) => ({ v: s.v + 1 }), cb);
  }
  openProfile = (event) => {
    if (event) event.preventDefault();
    this.logic.state = { ...this.logic.state, menu: null };
    this.setState({ route: 'profile' });
  };
  openPlans = (index) => this.setState({ plansFor: typeof index === 'number' ? index : this.logic.state.ws });
  pickPlan = (plan) => {
    const i = this.state.plansFor;
    const subs = this.logic.state.subs || {};
    this.__setLogicState({ subs: { ...subs, [i]: activate(plan, getSub(subs, i)) } });
    this.setState({ plansFor: null });
  };
  openWorkspace = (i) => {
    this.__setLogicState({ ws: i, menu: null, more: false, wsQuery: '' });
    this.setState({ route: null });
  };
  handleShellClick = (event) => {
    const target = event.target instanceof Element ? event.target : null;
    const link = target ? target.closest('a') : null;
    if (!link) return;
    const label = link.textContent.trim();
    if (label === 'Profil saya') {
      this.openProfile(event);
    } else if (label === 'Keluar') {
      event.preventDefault();
      this.handleLogout();
    }
  };
  handleLogin = (session) => {
    const p = this.logic.state.profile || {};
    const sameUser = p.username === session.username;
    const profile = {
      ...(sameUser ? p : {}),
      name: session.name || (sameUser ? p.name : ''),
      username: session.username,
      email: session.email || (sameUser ? p.email : ''),
      phone: session.phone || (sameUser ? p.phone : ''),
    };
    this.logic.state = { ...this.logic.state, profile, menu: null };
    save(this.logic.state);
    try {
      const current = JSON.parse(localStorage.getItem('convera:auth') || '{}');
      localStorage.setItem('convera:auth', JSON.stringify({ ...current, ...session, phone: profile.phone }));
    } catch {}
    this._lastTouch = 0; this.touchActivity();
    this.setState({ session, workspaces: null });
    this.loadWorkspaces(session);
  };
  handleLogout = async () => {
    const support = this.state.session?.support;
    await logout();
    if (support) { try { sessionStorage.setItem('convera:notice', 'Sesi dukungan diakhiri. Anda bisa menutup tab ini.'); } catch { /* abaikan */ } }
    this.logic.state = { ...this.logic.state, menu: null };
    this.setState({ session: null, route: null, plansFor: null, workspaces: null, realSub: null, live: null });
  };
  // Server menjawab 401 (sesi berakhir/dicabut): kembali ke halaman masuk.
  handleUnauthorized = () => {
    if (!this.state.session) return;
    const support = this.state.session.support;
    clearLocalSession();
    try { sessionStorage.setItem('convera:notice', support ? 'Sesi dukungan berakhir.' : 'Sesi berakhir. Silakan masuk lagi.'); } catch { /* abaikan */ }
    this.setState({ session: null, route: null, plansFor: null, workspaces: null, realSub: null, live: null });
  };
  // Mode backend: daftar workspace di UI = workspace milik akun ini di server (bukan daftar contoh).
  loadWorkspaces = async (session = this.state.session) => {
    if (!appConfig.backendEnabled || !session) return;
    try {
      const list = (await listWorkspaces()).map((w) => ({ name: w.name, slug: w.slug, category: w.category || '', city: w.city || '', meta: [w.category, w.city].filter(Boolean).join(' · ') || 'Workspace' }));
      const ws = this.logic.state.ws;
      if (ws >= list.length) this.__setLogicState({ ws: 0 });
      this.setState({ workspaces: list });
    } catch { this.setState({ workspaces: [] }); }
  };
  // Setelah workspace baru dibuat dari dialog: muat ulang daftar, pindah ke workspace itu, kosongkan data lama.
  handleWorkspaceCreated = async () => {
    await this.loadWorkspaces();
    const n = (this.state.workspaces || []).length;
    this.__setLogicState({ ws: Math.max(0, n - 1), page: 'overview', menu: null });
    this._liveKey = null;
    this.setState({ createWsOpen: false, live: null, realSub: null, route: null });
  };
  saveProfile = async (profile, password) => {
    const savedProfile = await updateProfile(profile);
    if (password) await changePassword(password.current, password.next);
    this.__setLogicState({ profile: savedProfile });
  };
  loadSecurity = async () => {
    if (!appConfig.backendEnabled || !this.state.session) return;
    try {
      const [sessions, audit] = await Promise.all([listSessions(), listAuditLogs()]);
      this.setState({ security: { sessions, audit } });
    } catch (error) {
      this.setState({ securityError: error.message || 'Data keamanan tidak dapat dimuat.' });
    }
  };
  saveSecurityPassword = async (current, next) => {
    await changePassword(current, next);
    await this.loadSecurity();
  };
  revokeSecuritySession = async (id) => {
    await revokeSession(id);
    await this.loadSecurity();
  };
  saveWorkspaceSettings = async (body) => {
    const active = this.state.workspaces?.[this.logic.state.ws];
    if (!appConfig.backendEnabled || !active?.slug) return;
    const updated = await updateWorkspace(active.slug, body);
    const workspaces = (this.state.workspaces || []).map((w) => w.slug === active.slug
      ? { ...w, name: updated.name, category: updated.category || '', city: updated.city || '', meta: [updated.category, updated.city].filter(Boolean).join(' · ') || 'Workspace' }
      : w);
    this.setState({ workspaces });
    this.logic.state = { ...this.logic.state, f: { ...this.logic.state.f, biz: updated.name, industry: updated.category || this.logic.state.f.industry } };
  };
  // Mode backend: status langganan workspace aktif diambil dari server (bukan data contoh).
  loadRealSub = async () => {
    const name = this._activeWsName;
    if (!appConfig.backendEnabled || !this.state.session || !name || this._noWs) return;
    try {
      const slug = this._activeWsSlug || (await ensureWorkspace(name));
      const sub = await getSubscription(slug);
      if (name === this._activeWsName) this.setState({ realSub: { name, ...sub } });
    } catch { /* biarkan label sebelumnya; kesalahan jaringan tidak mengganggu dashboard */ }
  };
  // ---------- Data nyata dashboard (mode backend) ----------
  rangeIdx = () => this.logic.state.range ?? 2;
  // Memuat overview, leads, event, dan aturan untuk workspace aktif + rentang tanggal yang dipilih.
  refreshLive = async () => {
    const slug = this._activeWsSlug, range = this.rangeIdx();
    const customFrom = this.logic.state.customFrom || '', customTo = this.logic.state.customTo || '';
    if (!appConfig.backendEnabled || !this.state.session || !slug) return;
    try {
      const next = await fetchLive(slug, range, customFrom, customTo);
      if (slug !== this._activeWsSlug || range !== this.rangeIdx() || customFrom !== (this.logic.state.customFrom || '') || customTo !== (this.logic.state.customTo || '')) return; // sudah pindah workspace/rentang
      this.setState({ live: next });
    } catch { /* pertahankan data sebelumnya; 401 ditangani oleh handleUnauthorized */ }
  };
  refreshNotifs = async () => {
    if (!appConfig.backendEnabled || !this.state.session) return;
    try { this.setState({ notifs: await notifApi.list() }); } catch { /* biarkan daftar sebelumnya */ }
  };
  readAllNotifs = async () => {
    const cur = this.state.notifs;
    if (cur) this.setState({ notifs: { items: cur.items.map((n) => ({ ...n, read_at: n.read_at || new Date().toISOString() })), unread: 0 } });
    try { await notifApi.readAll(); } catch { this.refreshNotifs(); }
  };
  // Detail lead/event diambil saat panelnya dibuka (timeline lead, payload & respons Meta).
  fetchOpenDetails = () => {
    const slug = this._activeWsSlug, live = this.state.live, st = this.logic.state;
    if (!slug || !live) return;
    const lead = st.ldOpen;
    if (lead !== this._ldOpen) {
      this._ldOpen = lead;
      if (lead) liveApi.leadDetail(slug, lead).then((d) => { live.leadDetail[lead] = d; this.forceUpdate(); }).catch(() => {});
    }
    const ev = st.lgOpen ? live.log[st.lgOpen - 1] : null;
    const evKey = ev ? ev.eid : null;
    if (evKey !== this._evOpen) {
      this._evOpen = evKey;
      if (evKey) liveApi.eventDetail(slug, evKey).then((d) => { live.eventDetail[evKey] = d; this.forceUpdate(); }).catch(() => {});
    }
  };
  // Aksi dari Logic.js: semuanya mengembalikan { ok, message } dan menyegarkan data setelahnya.
  liveAction = async (fn, okMsg) => {
    try { const r = await fn(); await this.refreshLive(); return { ok: true, message: okMsg ? okMsg(r) : '' }; } catch (e) { return { ok: false, message: e.message || 'Terjadi kesalahan' }; }
  };
  liveConvert = async (leadId, ev, value) => {
    const slug = this._activeWsSlug;
    try {
      const r = await liveApi.convert(slug, leadId, ev, value);
      await this.refreshLive();
      const msg = { queued: 'Sistem akan mengirimnya ke Meta sebentar lagi.', pending_value: 'Menunggu nilai pembelian diisi.', failed: r.reason || 'Tidak bisa dikirim ke Meta.', skipped: r.reason || 'Tidak dikirim.', duplicate: 'Event serupa sudah pernah dicatat.' }[r.duplicate ? 'duplicate' : r.status] || '';
      return { ok: !r.duplicate && ['queued', 'pending_value'].includes(r.status), queued: r.status === 'queued', message: msg };
    } catch (e) { return { ok: false, message: e.message || 'Terjadi kesalahan' }; }
  };
  liveBulkStatus = (ids, status) => this.liveAction(() => liveApi.bulkStatus(this._activeWsSlug, ids, status));
  liveRetry = (eventId) => { this.state.live && delete this.state.live.eventDetail[eventId]; return this.liveAction(() => liveApi.retry(this._activeWsSlug, eventId), () => 'Masuk antrean pengiriman'); };
  liveSaveRule = (rule) => this.liveAction(() => liveApi.saveRule(this._activeWsSlug, rule));
  liveErasePhone = async (phone) => {
    try {
      const r = await liveApi.erase(this._activeWsSlug, phone);
      await this.refreshLive();
      return { ok: true, message: r.leads ? r.leads + ' lead dan ' + r.events + ' event dihapus permanen' : 'Tidak ada data dengan nomor ini' };
    } catch (e) { return { ok: false, message: e.message || 'Terjadi kesalahan' }; }
  };
  liveDeleteRule = (id) => this.liveAction(() => liveApi.deleteRule(this._activeWsSlug, id));

  // Auto-logout bila tidak ada aktivitas pengguna selama 30 menit. Hanya sesi login di browser ini yang berakhir;
  // proses di server (sesi WhatsApp, pengiriman event ke Meta) tetap berjalan. Aktivitas dibagi antar-tab lewat localStorage.
  _idleKey = 'convera:lastActive';
  touchActivity = () => {
    const now = Date.now();
    if (now - (this._lastTouch || 0) < 5000) return; // batasi tulis
    this._lastTouch = now;
    try { localStorage.setItem(this._idleKey, String(now)); } catch { /* abaikan */ }
  };
  checkIdle = () => {
    if (!this.state.session || this.state.session.support) return; // sesi dukungan punya batas waktu sendiri
    let last = 0;
    try { last = Number(localStorage.getItem(this._idleKey)) || 0; } catch { /* abaikan */ }
    if (!last) { this.touchActivity(); return; }
    if (Date.now() - last >= IDLE_LIMIT_MS) this.idleLogout();
  };
  idleLogout = async () => {
    try { sessionStorage.setItem('convera:notice', 'Anda keluar otomatis karena tidak ada aktivitas selama 30 menit. Silakan masuk lagi.'); } catch { /* abaikan */ }
    try { await logout(); } catch { /* tetap keluar di sisi browser */ }
    try { localStorage.removeItem(this._idleKey); } catch { /* abaikan */ }
    this.setState({ session: null, route: null, plansFor: null, workspaces: null, realSub: null, live: null });
  };

  componentDidMount() {
    this.logic.componentDidMount();
    window.addEventListener('convera:unauthorized', this.handleUnauthorized);
    this._idleEvents = ['mousedown', 'keydown', 'touchstart', 'scroll', 'wheel', 'click'];
    this._idleEvents.forEach((e) => window.addEventListener(e, this.touchActivity, { passive: true, capture: true }));
    document.addEventListener('visibilitychange', this.checkIdle);
    this._idleTimer = setInterval(this.checkIdle, 30000);
    this.checkIdle(); // tab dibuka lagi setelah >30 menit tanpa aktivitas: langsung keluar
    this._lastTouch = 0; this.touchActivity();
    this.loadWorkspaces();
    this._liveTimer = setInterval(() => { if (document.visibilityState === 'visible') { this.refreshLive(); this.refreshNotifs(); } }, 30000);
  }
  componentDidUpdate() {
    if (appConfig.backendEnabled && this.state.session && this.state.workspaces && this._activeWsSlug) {
      const key = this._activeWsSlug + '|' + this.rangeIdx() + '|' + (this.logic.state.customFrom || '') + '|' + (this.logic.state.customTo || '');
      if (this._liveKey !== key) { this._liveKey = key; this.refreshLive(); }
      this.fetchOpenDetails();
    }
    if (appConfig.backendEnabled && this.state.session && this.logic.state.page === 'pengaturan') {
      const key = this.state.session.username + '|' + this._activeWsSlug;
      if (this._securityKey !== key) { this._securityKey = key; this.loadSecurity(); }
    }
    if (appConfig.backendEnabled && this.state.session && this._notifSession !== this.state.session) { this._notifSession = this.state.session; this.refreshNotifs(); }
    const ws = this._activeWsName;
    if (appConfig.backendEnabled && this.state.session && ws && (this._subWs !== ws || this._subSession !== this.state.session)) {
      this._subWs = ws; this._subSession = this.state.session;
      this.loadRealSub();
    }
  }
  componentWillUnmount() { this.logic.componentWillUnmount(); window.removeEventListener('convera:unauthorized', this.handleUnauthorized); clearInterval(this._liveTimer); clearInterval(this._idleTimer); (this._idleEvents || []).forEach((e) => window.removeEventListener(e, this.touchActivity, true)); document.removeEventListener('visibilitychange', this.checkIdle); }
  render() {
    this.logic.props = appConfig.backendEnabled && this.state.workspaces
      ? { ...this.props_, backendEnabled: true, workspaces: this.state.workspaces, activeWorkspace: this.state.workspaces[this.logic.state.ws], security: this.state.security, live: this.state.live || emptyLive(this.logic.state.range ?? 2), notifs: this.state.notifs || { items: [], unread: 0 }, readAllNotifs: this.readAllNotifs, saveWorkspaceSettings: this.saveWorkspaceSettings, changeSecurityPassword: this.saveSecurityPassword, revokeSecuritySession: this.revokeSecuritySession, profileName: this.logic.state.profile?.name || this.state.session.name || '', profileEmail: this.logic.state.profile?.email || this.state.session.email || '', profilePhone: this.logic.state.profile?.phone || this.state.session.phone || '', realSub: this.state.realSub }
      : { ...this.props_, backendEnabled: false, profileName: this.logic.state.profile?.name || this.state.session?.name || '', profileEmail: this.logic.state.profile?.email || this.state.session?.email || '', profilePhone: this.logic.state.profile?.phone || this.state.session?.phone || '', realSub: null };
    const vals = { ...this.props_, ...(this.logic.renderVals() || {}) };
    if (!this.state.session) return <LoginPage themeStyle={vals.rootStyle} onLogin={this.handleLogin} />;
    if (appConfig.backendEnabled) {
      if (this.state.workspaces === null) return <div style={{ ...vals.rootStyle, minHeight: '100vh', padding: 40, background: 'var(--bg)', color: 'var(--muted)' }}>Memuat workspace…</div>;
    }
    const st = this.logic.state;
    const me = st.profile || {};
    vals.userName = me.name || this.state.session.name;
    vals.userHandle = me.username || this.state.session.username;
    vals.userInitials = initials(vals.userName);
    // langganan workspace yang sedang dibuka (pengganti kotak kuota event di sidebar)
    const activeWs = vals.wsAll[st.ws] || vals.wsAll[0];
    const activeInfo = subInfo(getSub(st.subs, st.ws));
    this._noWs = appConfig.backendEnabled && (this.state.workspaces || []).length === 0;
    this._activeWsName = activeWs.name;
    this._activeWsSlug = activeWs.slug;
    vals.subName = activeWs.name;
    vals.subStatus = activeInfo.label;
    vals.subBtn = activeInfo.btn;
    const rs = this.state.realSub;
    if (appConfig.backendEnabled && rs && rs.name === activeWs.name) {
      const d = (x) => new Date(x).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
      const left = (x) => Math.max(0, Math.ceil((new Date(x) - Date.now()) / 864e5));
      vals.subStatus = { active: 'Aktif · s/d ' + d(rs.current_period_end), trial: 'Trial · ' + left(rs.trial_until) + ' hari', pending: 'Menunggu verifikasi', expired: 'Berakhir', stopped: 'Dihentikan' }[rs.status] || 'Belum berlangganan';
      vals.subBtn = rs.status === 'active' ? 'Perpanjang' : 'Upgrade';
      const isExpired = rs.status === 'expired' || rs.status === 'stopped' || rs.status === 'pending' ||
        (rs.status === 'trial' && left(rs.trial_until) === 0);
      vals.subExpired = isExpired;
      // Redirect ke overview jika halaman saat ini terkunci
      if (isExpired && st.page !== 'overview') {
        this.__setLogicState({ page: 'overview' });
      }
      // Lock semua pg kecuali overview
      if (isExpired) {
        vals.pg = Object.fromEntries(Object.entries(vals.pg || {}).map(([k, v]) => [k, k === 'overview' ? v : false]));
      }
    } else {
      vals.subExpired = false;
    }
    vals.openPlans = () => this.openPlans(st.ws);
    // halaman Koneksi: setting per workspace (key = reset form saat pindah workspace)
    vals.koneksiNode = <KoneksiSettings key={st.ws} workspace={activeWs} value={(st.conn || {})[st.ws] || CONN_DEFAULT}
      onSave={(data) => this.__setLogicState({ conn: { ...(this.logic.state.conn || {}), [st.ws]: data } })} onScanQr={() => this.setState({ route: 'scan-qr' })} />;
    if (appConfig.backendEnabled) {
      vals.laporanNode = <LaporanPage key={'lap' + activeWs.slug} slug={activeWs.slug} workspaceName={activeWs.name} />;
      vals.dashExtraNode = <DashboardExtras key={'dash' + activeWs.slug} slug={activeWs.slug} range={rangeFor(this.rangeIdx())} />;
    }
    if (appConfig.backendEnabled) {
      // "Tambah workspace" (pilihan workspace / tombol kosong) membuka kartu isian nama di atas Dashboard, seperti PixelAD.
      vals.newWs = () => { this.__setLogicState({ menu: null, page: 'overview' }); this.setState({ route: null, createWsOpen: true }); };
      if (this._noWs) {
        // Akun baru tanpa workspace: langsung di Dashboard, isi nama untuk membuat workspace pertama (bukan layar penuh).
        vals.pg = Object.fromEntries(Object.keys(vals.pg || {}).map((k) => [k, false]));
        vals.laporanNode = null; vals.dashExtraNode = null;
      }
      if (this._noWs || this.state.createWsOpen) {
        vals.noWsNode = <AddWorkspaceCard first={this._noWs} userName={this.state.session.name} onCancel={() => this.setState({ createWsOpen: false })} onCreated={this.handleWorkspaceCreated} />;
      }
    }
    const { plansFor } = this.state;
    const modal = plansFor !== null && appConfig.backendEnabled ? (
      <PaymentModal themeStyle={vals.rootStyle} workspace={vals.wsAll[plansFor]} onClose={() => this.setState({ plansFor: null })} onDone={this.loadRealSub} />
    ) : plansFor !== null ? (
      <PlansModal themeStyle={vals.rootStyle} workspace={vals.wsAll[plansFor].name} renew={subInfo(getSub(st.subs, plansFor)).active} onPick={this.pickPlan} onClose={() => this.setState({ plansFor: null })} />
    ) : null;
    const support = this.state.session.support;
    const banner = support ? (
      <div role="status" style={{ ...vals.rootStyle, position: 'fixed', top: 0, left: 0, right: 0, zIndex: 300, minHeight: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 14, flexWrap: 'wrap', padding: '6px 12px', background: '#b4440f', color: '#fff', fontSize: 13, fontWeight: 600 }}>
        <span>Mode dukungan · {support.workspaceName} · admin {support.admin} · berakhir {support.expiresAt ? new Date(support.expiresAt).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' }) : '—'} · tindakan Anda dicatat</span>
        <button type="button" onClick={this.handleLogout} style={{ height: 28, padding: '0 12px', border: '1px solid rgba(255,255,255,.7)', borderRadius: 8, background: 'transparent', color: '#fff', fontFamily: 'inherit', fontSize: 12, fontWeight: 600, cursor: 'pointer' }}>Akhiri sesi dukungan</button>
      </div>
    ) : null;
    if (this.state.route === 'scan-qr') {
      const back = () => this.setState({ route: null });
      return (
        <div className="sc-host" style={support ? { paddingTop: 44 } : undefined}>
          {banner}
          <QrScanPage key={st.ws} themeStyle={vals.rootStyle} workspace={activeWs} onDone={back} onBack={back} />
        </div>
      );
    }
    if (this.state.route === 'profile') {
      return (
        <div className="sc-host" style={support ? { paddingTop: 44 } : undefined}>
          {banner}
          <ProfilePage themeStyle={vals.rootStyle} profile={me} onSave={this.saveProfile} workspaces={this._noWs ? [] : vals.wsAll} activeIndex={st.ws} subs={st.subs}
            onOpen={this.openWorkspace} onPlans={this.openPlans} onBack={() => this.setState({ route: null })} onLogout={this.handleLogout} />
          {modal}
        </div>
      );
    }
    return (
      <div className="sc-host" onClickCapture={this.handleShellClick} style={support ? { paddingTop: 44 } : undefined}>
        {banner}
        <Template v={vals} />
        {modal}
      </div>
    );
  }
}
