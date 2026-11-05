// Sesi admin lewat API. Cookie httpOnly tidak terbaca JavaScript, jadi status login ditanyakan ke server.
import { api } from './api.js';

export async function getSession() {
  try { return (await api.get('/auth/session')).data.admin; } catch (e) { if (e.status === 401) return null; throw e; }
}

export async function login(username, password) {
  if (!String(username).trim() || !password) throw new Error('Username dan password wajib diisi');
  return (await api.post('/auth/login', { username: String(username).trim(), password })).data.admin;
}

export async function logout() {
  try { await api.post('/auth/logout'); } catch { /* sesi sudah berakhir */ }
}
