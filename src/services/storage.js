// Penyimpanan lokal (localStorage). Hanya state "permanen" yang disimpan; modal/menu/toast tidak.
const KEY = 'convera:v1';
export const PERSIST = ['f', 'page', 'ws', 'range', 'dark', 'screen', 'step', 'collapsed', 'banners', 'read', 'leadOv', 'kvNew', 'lgOv', 'rulesList', 'rulesOn',
  'ldQ', 'ldCamp', 'ldStatus', 'ldSel', 'kvSrc', 'pfOpen', 'pfSort', 'atTab', 'rb', 'rbEdit', 'rtMsg', 'rtFrom', 'lgSt', 'lgSrc', 'lgTab',
  'keys', 'invites', 'wh', 'whEv', 'whSecret', 'lpOn', 'lpCh', 'lpP', 'lpTo', 'lpDay', 'lpHour', 'lpSaved', 'defEv', 'defVal', 'disc', 'intOn', 'sandbox', 'sessOut',
  'wa', 'methods', 'plan', 'bill', 'tokenDays', 'nt', 'retention', 'delPhones', 'pwUpdated', 'wsDelAt', 'events', 'profile', 'subs', 'conn'];
export function load() { try { return JSON.parse(localStorage.getItem(KEY)) || {}; } catch { return {}; } }
let t;
export function save(state) {
  clearTimeout(t);
  t = setTimeout(() => { try { const o = {}; PERSIST.forEach((k) => { if (state[k] !== undefined) o[k] = state[k]; }); localStorage.setItem(KEY, JSON.stringify(o)); } catch {} }, 150);
}
export const reset = () => { try { localStorage.removeItem(KEY); } catch {} location.reload(); };
