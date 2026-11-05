import { appConfig } from './config.js';
import { requestJson } from './api.js';
import { ENDPOINTS, clearWorkspaceCache } from './backend.js';
import { isEmail } from './validators.js';

// Sesi login. Mode demo (VITE_BACKEND_ENABLED=false): sesi disimpan di localStorage dan
// kredensial TIDAK diverifikasi ke server. Mode backend: POST /api/v1/auth/login dan POST /api/v1/auth/logout.
const KEY = 'convera:auth';

export function getSession() {
  try { return JSON.parse(localStorage.getItem(KEY)) || null; } catch { return null; }
}

function nameFromEmail(email) {
  const local = email.split('@')[0].replace(/[._-]+/g, ' ').trim();
  return local.replace(/\b\w/g, (c) => c.toUpperCase()) || 'Pengguna';
}

export async function login(email, password) {
  email = String(email).trim().toLowerCase();
  if (!isEmail(email)) throw new Error('Format email tidak valid');
  if (!password) throw new Error('Password wajib diisi');
  let user;
  if (appConfig.backendEnabled) {
    const res = await requestJson(ENDPOINTS.login, { method: 'POST', body: JSON.stringify({ email, password }) });
    user = res?.data?.user || res?.user || { email };
  } else {
    if (password.length < 6) throw new Error('Password minimal 6 karakter');
    user = { email, name: nameFromEmail(email) };
  }
  const session = { email: user.email || email, name: user.name || nameFromEmail(email), at: new Date().toISOString() };
  try { localStorage.setItem(KEY, JSON.stringify(session)); } catch {}
  return session;
}

// Daftar akun baru (hanya mode backend). Server langsung membuat sesi login.
export async function register(name, email, password) {
  email = String(email).trim().toLowerCase();
  if (!String(name).trim()) throw new Error('Nama wajib diisi');
  if (!isEmail(email)) throw new Error('Format email tidak valid');
  if (String(password).length < 8) throw new Error('Password minimal 8 karakter');
  const res = await requestJson(ENDPOINTS.register, { method: 'POST', body: JSON.stringify({ name: String(name).trim(), email, password }) });
  const user = res?.data?.user || { email, name };
  const session = { email: user.email || email, name: user.name || name, at: new Date().toISOString() };
  try { localStorage.setItem(KEY, JSON.stringify(session)); } catch {}
  return session;
}

// Sesi dukungan: admin Convera membantu customer. Kode sekali pakai dari Convera Admin ditukar jadi sesi terbatas.
export async function supportLogin(code) {
  const res = await requestJson(ENDPOINTS.supportLogin, { method: 'POST', body: JSON.stringify({ code }) });
  const u = res?.data?.user || {};
  const sup = res?.data?.support || {};
  const session = {
    email: u.email, name: u.name, at: new Date().toISOString(),
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

// ---- Lupa password, reset, dan verifikasi email (hanya mode backend) ----

export async function forgotPassword(email) {
  email = String(email).trim().toLowerCase();
  if (!isEmail(email)) throw new Error('Format email tidak valid');
  const res = await requestJson(ENDPOINTS.forgot, { method: 'POST', body: JSON.stringify({ email }) });
  return res?.data?.message || 'Jika email terdaftar, tautan reset password sudah dikirim.';
}

export async function resetPassword(token, password) {
  if (String(password).length < 8) throw new Error('Password minimal 8 karakter');
  await requestJson(ENDPOINTS.reset, { method: 'POST', body: JSON.stringify({ token, password }) });
}

export async function verifyEmail(token) {
  await requestJson(ENDPOINTS.verify, { method: 'POST', body: JSON.stringify({ token }) });
}
