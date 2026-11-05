import React from 'react';
import Logic from './Logic.js';
import Template from './Template.jsx';
import defaults from './defaults.json';
import { load, save, reset } from './services/storage.js';
import ProfilePage from './ProfilePage.jsx';
import PlansModal from './PlansModal.jsx';
import { updateProfile, ensureWorkspace, getSubscription, listWorkspaces } from './services/backend.js';
import CreateWorkspace from './CreateWorkspace.jsx';
import QrScanPage from './QrScanPage.jsx';
import { fetchLive, emptyLive, liveApi } from './services/live.js';
import { appConfig } from './services/config.js';
import PaymentModal from './PaymentModal.jsx';
import LoginPage from './LoginPage.jsx';
import { getSession, logout, initials, clearLocalSession } from './services/auth.js';
import { getSub, subInfo, activate } from './plans.js';
import KoneksiSettings, { CONN_DEFAULT } from './KoneksiSettings.jsx';

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
    this.state = { v: 0, route: null, plansFor: null, session: getSession(), realSub: null, workspaces: null, live: null };
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
    const profile = p.email === session.email ? p : { ...p, name: session.name, email: session.email };
    this.logic.state = { ...this.logic.state, profile, menu: null };
    save(this.logic.state);
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
      const list = (await listWorkspaces()).map((w) => ({ name: w.name, slug: w.slug, meta: [w.category, w.city].filter(Boolean).join(' · ') || 'Workspace' }));
      const ws = this.logic.state.ws;
      if (ws >= list.length) this.__setLogicState({ ws: 0 });
      this.setState({ workspaces: list });
    } catch { this.setState({ workspaces: [] }); }
  };
  saveProfile = async (profile) => {
    const savedProfile = await updateProfile(profile);
    this.__setLogicState({ profile: savedProfile });
  };
  // Mode backend: status langganan workspace aktif diambil dari server (bukan data contoh).
  loadRealSub = async () => {
    const name = this._activeWsName;
    if (!appConfig.backendEnabled || !this.state.session || !name) return;
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
    if (!appConfig.backendEnabled || !this.state.session || !slug) return;
    try {
      const next = await fetchLive(slug, range);
      if (slug !== this._activeWsSlug || range !== this.rangeIdx()) return; // sudah pindah workspace/rentang
      this.setState({ live: next });
    } catch { /* pertahankan data sebelumnya; 401 ditangani oleh handleUnauthorized */ }
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

  componentDidMount() {
    this.logic.componentDidMount();
    window.addEventListener('convera:unauthorized', this.handleUnauthorized);
    this.loadWorkspaces();
    this._liveTimer = setInterval(() => { if (document.visibilityState === 'visible') this.refreshLive(); }, 30000);
  }
  componentDidUpdate() {
    if (appConfig.backendEnabled && this.state.session && this.state.workspaces && this._activeWsSlug) {
      const key = this._activeWsSlug + '|' + this.rangeIdx();
      if (this._liveKey !== key) { this._liveKey = key; this.refreshLive(); }
      this.fetchOpenDetails();
    }
    const ws = this._activeWsName;
    if (appConfig.backendEnabled && this.state.session && ws && (this._subWs !== ws || this._subSession !== this.state.session)) {
      this._subWs = ws; this._subSession = this.state.session;
      this.loadRealSub();
    }
  }
  componentWillUnmount() { this.logic.componentWillUnmount(); window.removeEventListener('convera:unauthorized', this.handleUnauthorized); clearInterval(this._liveTimer); }
  render() {
    this.logic.props = appConfig.backendEnabled && this.state.workspaces
      ? { ...this.props_, workspaces: this.state.workspaces, live: this.state.live || emptyLive(this.logic.state.range ?? 2) }
      : this.props_;
    const vals = { ...this.props_, ...(this.logic.renderVals() || {}) };
    if (!this.state.session) return <LoginPage themeStyle={vals.rootStyle} onLogin={this.handleLogin} />;
    if (appConfig.backendEnabled) {
      if (this.state.workspaces === null) return <div style={{ ...vals.rootStyle, minHeight: '100vh', padding: 40, background: 'var(--bg)', color: 'var(--muted)' }}>Memuat workspace…</div>;
      if (this.state.workspaces.length === 0) return <CreateWorkspace themeStyle={vals.rootStyle} userName={this.state.session.name} onCreated={() => this.loadWorkspaces()} onLogout={this.handleLogout} />;
    }
    const st = this.logic.state;
    const me = st.profile || {};
    vals.userName = me.name || this.state.session.name;
    vals.userEmail = me.email || this.state.session.email;
    vals.userInitials = initials(vals.userName);
    // langganan workspace yang sedang dibuka (pengganti kotak kuota event di sidebar)
    const activeWs = vals.wsAll[st.ws] || vals.wsAll[0];
    const activeInfo = subInfo(getSub(st.subs, st.ws));
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
    }
    vals.openPlans = () => this.openPlans(st.ws);
    // halaman Koneksi: setting per workspace (key = reset form saat pindah workspace)
    vals.koneksiNode = <KoneksiSettings key={st.ws} workspace={activeWs} value={(st.conn || {})[st.ws] || CONN_DEFAULT}
      onSave={(data) => this.__setLogicState({ conn: { ...(this.logic.state.conn || {}), [st.ws]: data } })} onScanQr={() => this.setState({ route: 'scan-qr' })} />;
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
          <ProfilePage themeStyle={vals.rootStyle} profile={me} onSave={this.saveProfile} workspaces={vals.wsAll} activeIndex={st.ws} subs={st.subs}
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
