import { appConfig } from './config.js';
import { requestJson } from './api.js';
import { ENDPOINTS, clearWorkspaceCache } from './backend.js';
import { isUsername, isEmail, isPhone } from './validators.js';

// Sesi login. Mode demo (VITE_BACKEND_ENABLED=false): sesi disimpan di localStorage dan
// kredensial TIDAK diverifikasi ke server. Mode backend: POST /api/v1/auth/login dan POST /api/v1/auth/logout.
const KEY = 'convera:auth';

export function getSession() {
  try { return JSON.parse(localStorage.getItem(KEY)) || null; } catch { return null; }
}

const USERNAME_HINT = 'Username 3-30 karakter: huruf kecil, angka, titik, garis bawah, atau strip; diawali huruf atau angka';

export async function login(username, password) {
  username = String(username).trim().toLowerCase();
  if (!username) throw new Error('Username wajib diisi');
  if (!password) throw new Error('Password wajib diisi');
  let user;
  if (appConfig.backendEnabled) {
    const res = await requestJson(ENDPOINTS.login, { method: 'POST', body: JSON.stringify({ username, password }) });
    user = res?.data?.user || res?.user || { username };
  } else {
    if (password.length < 6) throw new Error('Password minimal 6 karakter');
    user = { username, name: username };
  }
  const session = { username: user.username || username, email: user.email || '', name: user.name || username, phone: user.phone || '', at: new Date().toISOString() };
  try { localStorage.setItem(KEY, JSON.stringify(session)); } catch {}
  return session;
}

// Daftar akun baru (hanya mode backend). Server langsung membuat sesi login.
export async function register({ name, username, email, phone, password }) {
  name = String(name || '').trim();
  username = String(username || '').trim().toLowerCase();
  email = String(email || '').trim().toLowerCase();
  phone = String(phone || '').trim();
  if (!name) throw new Error('Nama wajib diisi');
  if (!isUsername(username)) throw new Error(USERNAME_HINT);
  if (!isEmail(email)) throw new Error('Format email tidak valid');
  if (!isPhone(phone)) throw new Error('Nomor telepon tidak valid');
  if (String(password).length < 8) throw new Error('Password minimal 8 karakter');
  const res = await requestJson(ENDPOINTS.register, { method: 'POST', body: JSON.stringify({ name, username, email, phone, password }) });
  const user = res?.data?.user || res?.user || {};
  const session = {
    username: user.username || username,
    email: user.email || email,
    name: user.name || name,
    phone: user.phone || normPhone(phone),
    at: new Date().toISOString(),
  };
  try { localStorage.setItem(KEY, JSON.stringify(session)); } catch {}
  return session;
}

// Sesi dukungan: admin Convera membantu customer. Kode sekali pakai dari Convera Admin ditukar jadi sesi terbatas.
export async function supportLogin(code) {
  const res = await requestJson(ENDPOINTS.supportLogin, { method: 'POST', body: JSON.stringify({ code }) });
  const u = res?.data?.user || {};
  const sup = res?.data?.support || {};
  const session = {
    username: u.username, email: u.email, name: u.name, phone: u.phone || '', at: new Date().toISOString(),
    support: { admin: sup.admin, workspace: sup.workspace, workspaceName: sup.workspace_name, expiresAt: sup.expires_at },
  };
  try { localStorage.setItem(KEY, JSON.stringify(session)); } catch {}
  return session;
}

// Hapus sesi di browser saja (dipakai saat server menjawab 401: sesi berakhir/dicabut).
export function clearLocalSession() {
  clearWorkspaceCache();
  try { localStorage.removeItem(KEY); } catch {}
}

export async function logout() {
  clearWorkspaceCache();
  if (appConfig.backendEnabled) { try { await requestJson(ENDPOINTS.logout, { method: 'POST' }); } catch {} }
  try { localStorage.removeItem(KEY); } catch {}
}

export const initials = (name) => String(name || '?').trim().split(/\s+/).slice(0, 2).map((w) => w[0]).join('').toUpperCase() || '?';
