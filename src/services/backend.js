import { appConfig } from './config.js';
import { requestJson } from './api.js';

export const ENDPOINTS = Object.freeze({
  profile: '/api/v1/me/profile',
  session: '/api/v1/auth/session',
  login: '/api/v1/auth/login',
  register: '/api/v1/auth/register',
  logout: '/api/v1/auth/logout',
  supportLogin: '/api/v1/auth/support-login',
  workspaces: '/api/v1/workspaces',
  leads: '/api/v1/leads',
  events: '/api/v1/events',
  reports: '/api/v1/reports',
  metaConnection: '/api/v1/connections/meta',
  metaTest: '/api/v1/connections/meta/test',
});

export async function updateProfile(profile) {
  if (!appConfig.backendEnabled) return profile;
  const response = await requestJson(ENDPOINTS.profile, { method: 'PATCH', body: JSON.stringify(profile) });
  return response?.data || response;
}

export async function changePassword(current, next) {
  if (!appConfig.backendEnabled) return { changed: true };
  return requestJson('/api/v1/me/password', {
    method: 'POST',
    body: JSON.stringify({ current, new: next }),
  });
}

export const listSessions = async () => (await requestJson('/api/v1/me/sessions'))?.data || [];
export const revokeSession = async (id) =>
  (await requestJson(`/api/v1/me/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }))?.data;
export const listAuditLogs = async () => (await requestJson('/api/v1/me/audit-logs'))?.data || [];

export const updateWorkspace = async (slug, body) =>
  (await requestJson(`/api/v1/workspaces/${encodeURIComponent(slug)}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  }))?.data;

// ---- Workspace: daftar workspace di UI dicocokkan dengan backend lewat nama ----

const slugCache = new Map(); // nama workspace -> slug backend (per sesi halaman)

// Mengembalikan slug workspace di backend untuk nama ini; dibuat bila belum ada.
export async function ensureWorkspace(name, { category, city } = {}) {
  if (slugCache.has(name)) return slugCache.get(name);
  const list = (await requestJson(ENDPOINTS.workspaces))?.data || [];
  let ws = list.find((w) => w.name === name);
  if (!ws) {
    const created = (await requestJson(ENDPOINTS.workspaces, { method: 'POST', body: JSON.stringify({ name, category, city }) }))?.data;
    ws = created;
  }
  slugCache.set(name, ws.slug);
  return ws.slug;
}

export function clearWorkspaceCache() { slugCache.clear(); }

// ---- Koneksi Meta ----
// Token dan app secret hanya dikirim ke server; server tidak pernah mengembalikannya utuh.

export const getMetaConnection = async (workspace) => (await requestJson(ENDPOINTS.metaConnection, { workspace }))?.data;

export const saveMetaConnection = async (workspace, body) =>
  (await requestJson(ENDPOINTS.metaConnection, { method: 'PUT', workspace, body: JSON.stringify(body) }))?.data;

export const testMetaConnection = async (workspace) =>
  (await requestJson(ENDPOINTS.metaTest, { method: 'POST', workspace }))?.data;

// ---- Mode QR (sesi WhatsApp Web) ----

export const startQr = async (workspace) => (await requestJson('/api/v1/connections/whatsapp/qr', { method: 'POST', workspace }))?.data;
export const getQrStatus = async (workspace) => (await requestJson('/api/v1/connections/whatsapp/status', { workspace }))?.data;
export const disconnectQr = async (workspace) => (await requestJson('/api/v1/connections/whatsapp', { method: 'DELETE', workspace }))?.data;

// ---- Koneksi Meta Ads (Marketing API, hanya baca) ----

export const getAdsConnection = async (workspace) => (await requestJson('/api/v1/connections/ads', { workspace }))?.data;
export const saveAdsConnection = async (workspace, body) =>
  (await requestJson('/api/v1/connections/ads', { method: 'PUT', workspace, body: JSON.stringify(body) }))?.data;
export const syncAdsConnection = async (workspace, full = false) =>
  (await requestJson('/api/v1/connections/ads/sync' + (full ? '?full=1' : ''), { method: 'POST', workspace }))?.data;
export const deleteAdsConnection = async (workspace) =>
  (await requestJson('/api/v1/connections/ads', { method: 'DELETE', workspace }))?.data;

// ---- Langganan & bukti transfer ----

export const getPlans = async () => (await requestJson('/api/v1/plans'))?.data || [];
export const getBankAccounts = async (workspace) => (await requestJson('/api/v1/bank-accounts', { workspace }))?.data || [];
export const getSubscription = async (workspace) => (await requestJson('/api/v1/subscription', { workspace }))?.data;
export const getMyProofs = async (workspace) => (await requestJson('/api/v1/payment-proofs', { workspace }))?.data || [];

// formData: plan, months, bank, sender, ref?, amount?, bank_account_id?, message?, file (JPG/PNG/PDF, maks. 5 MB)
export const uploadProof = async (workspace, formData) =>
  (await requestJson('/api/v1/payment-proofs', { method: 'POST', workspace, body: formData }))?.data;

// ---- Daftar workspace nyata (menggantikan daftar contoh di UI) ----

export async function listWorkspaces() {
  return (await requestJson(ENDPOINTS.workspaces))?.data || [];
}

export async function createWorkspace(body) {
  const ws = (await requestJson(ENDPOINTS.workspaces, { method: 'POST', body: JSON.stringify(body) }))?.data;
  clearWorkspaceCache();
  return ws;
}
