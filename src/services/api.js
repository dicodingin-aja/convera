import { appConfig } from './config.js';

function createError(message, status = 0) {
  const error = new Error(message);
  error.status = status;
  return error;
}

// Cookie csrf_token diset backend saat login; dikirim balik lewat header untuk POST/PUT/PATCH/DELETE.
function csrfToken() {
  try {
    const m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]*)/);
    return m ? decodeURIComponent(m[1]) : '';
  } catch { return ''; }
}

// options.workspace = slug workspace di backend (dikirim sebagai header X-Workspace).
export async function requestJson(path, options = {}) {
  if (!appConfig.apiBaseUrl) throw createError('VITE_API_BASE_URL belum dikonfigurasi');
  const { workspace, ...init } = options;
  const method = String(init.method || 'GET').toUpperCase();
  const controller = new AbortController();
  // Unggahan berkas (FormData) diberi waktu lebih lama dari permintaan JSON biasa.
  const timeoutMs = init.body instanceof FormData ? Math.max(appConfig.requestTimeoutMs, 60000) : appConfig.requestTimeoutMs;
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const headers = {
    Accept: 'application/json',
    // FormData: biarkan browser mengisi Content-Type + boundary multipart.
    ...(init.body && !(init.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}),
    ...(method !== 'GET' && method !== 'HEAD' && csrfToken() ? { 'X-CSRF-Token': csrfToken() } : {}),
    ...(workspace ? { 'X-Workspace': workspace } : {}),
    ...(init.headers || {}),
  };
  try {
    const response = await fetch(`${appConfig.apiBaseUrl}${path}`, {
      ...init,
      headers,
      credentials: init.credentials || 'include',
      signal: controller.signal,
    });
    const data = await response.json().catch(() => null);
    // Sesi berakhir atau dicabut (mis. sesi dukungan habis 2 jam): beri tahu aplikasi agar kembali ke halaman masuk.
    if (response.status === 401 && !path.includes('/auth/')) window.dispatchEvent(new Event('convera:unauthorized'));
    if (!response.ok) throw createError(data?.message || `Request gagal (${response.status})`, response.status);
    return data;
  } catch (error) {
    if (error.name === 'AbortError') throw createError('Request timeout');
    if (error instanceof TypeError) throw createError('Server tidak terjangkau. Pastikan backend berjalan.');
    throw error;
  } finally {
    clearTimeout(timer);
  }
}
