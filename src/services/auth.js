import { appConfig } from './config.js';
import { requestJson } from './api.js';
import { ENDPOINTS } from './backend.js';
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

export async function logout() {
  if (appConfig.backendEnabled) { try { await requestJson(ENDPOINTS.logout, { method: 'POST' }); } catch {} }
  try { localStorage.removeItem(KEY); } catch {}
}

export const initials = (name) => String(name || '?').trim().split(/\s+/).slice(0, 2).map((w) => w[0]).join('').toUpperCase() || '?';
