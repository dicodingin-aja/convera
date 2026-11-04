import React from 'react';
import Logic from './Logic.js';
import Template from './Template.jsx';
import defaults from './defaults.json';
import { load, save, reset } from './services/storage.js';
import ProfilePage from './ProfilePage.jsx';
import PlansModal from './PlansModal.jsx';
import { updateProfile } from './services/backend.js';
import LoginPage from './LoginPage.jsx';
import { getSession, logout, initials } from './services/auth.js';
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
    this.state = { v: 0, route: null, plansFor: null, session: getSession() };
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
    this.setState({ session });
  };
  handleLogout = async () => {
    await logout();
    this.logic.state = { ...this.logic.state, menu: null };
    this.setState({ session: null, route: null, plansFor: null });
  };
  saveProfile = async (profile) => {
    const savedProfile = await updateProfile(profile);
    this.__setLogicState({ profile: savedProfile });
  };
  componentDidMount() { this.logic.componentDidMount(); }
  componentWillUnmount() { this.logic.componentWillUnmount(); }
  render() {
    this.logic.props = this.props_;
    const vals = { ...this.props_, ...(this.logic.renderVals() || {}) };
    if (!this.state.session) return <LoginPage themeStyle={vals.rootStyle} onLogin={this.handleLogin} />;
    const st = this.logic.state;
    const me = st.profile || {};
    vals.userName = me.name || this.state.session.name;
    vals.userEmail = me.email || this.state.session.email;
    vals.userInitials = initials(vals.userName);
    // langganan workspace yang sedang dibuka (pengganti kotak kuota event di sidebar)
    const activeWs = vals.wsAll[st.ws] || vals.wsAll[0];
    const activeInfo = subInfo(getSub(st.subs, st.ws));
    vals.subName = activeWs.name;
    vals.subStatus = activeInfo.label;
    vals.subBtn = activeInfo.btn;
    vals.openPlans = () => this.openPlans(st.ws);
    // halaman Koneksi: setting per workspace (key = reset form saat pindah workspace)
    vals.koneksiNode = <KoneksiSettings key={st.ws} workspace={activeWs} value={(st.conn || {})[st.ws] || CONN_DEFAULT}
      onSave={(data) => this.__setLogicState({ conn: { ...(this.logic.state.conn || {}), [st.ws]: data } })} />;
    const { plansFor } = this.state;
    const modal = plansFor !== null ? (
      <PlansModal themeStyle={vals.rootStyle} workspace={vals.wsAll[plansFor].name} renew={subInfo(getSub(st.subs, plansFor)).active} onPick={this.pickPlan} onClose={() => this.setState({ plansFor: null })} />
    ) : null;
    if (this.state.route === 'profile') {
      return (
        <div className="sc-host">
          <ProfilePage themeStyle={vals.rootStyle} profile={me} onSave={this.saveProfile} workspaces={vals.wsAll} activeIndex={st.ws} subs={st.subs}
            onOpen={this.openWorkspace} onPlans={this.openPlans} onBack={() => this.setState({ route: null })} onLogout={this.handleLogout} />
          {modal}
        </div>
      );
    }
    return (
      <div className="sc-host" onClickCapture={this.handleShellClick}>
        <Template v={vals} />
        {modal}
      </div>
    );
  }
}
