// Klien API Convera Admin. Sesi lewat cookie httpOnly (cv_admin); untuk POST/PUT/DELETE header X-CSRF-Token
// diambil dari cookie csrf_admin. Base URL kosong = satu origin (dev: proxy Vite, produksi: Caddy).
const BASE = String(import.meta.env.VITE_API_BASE_URL || '').replace(/\/+$/, '');
const PREFIX = '/api/admin/v1';

export class ApiError extends Error {
  constructor(message, status = 0, code = '') { super(message); this.status = status; this.code = code; }
}

let onUnauthorized = () => {};
export const setUnauthorizedHandler = (fn) => { onUnauthorized = fn; };

const csrf = () => {
  try { const m = document.cookie.match(/(?:^|;\s*)csrf_admin=([^;]*)/); return m ? decodeURIComponent(m[1]) : ''; } catch { return ''; }
};

async function request(method, path, body, raw = false) {
  const headers = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (method !== 'GET') headers['X-CSRF-Token'] = csrf();
  let res;
  try {
    res = await fetch(BASE + PREFIX + path, { method, headers, credentials: 'include', body: body !== undefined ? JSON.stringify(body) : undefined });
  } catch { throw new ApiError('Server tidak terjangkau. Pastikan API admin berjalan.'); }
  if (raw && res.ok) return res;
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith('/auth/login')) onUnauthorized();
    throw new ApiError(data?.message || `Permintaan gagal (${res.status})`, res.status, data?.code);
  }
  return data;
}

export const api = {
  get: (p) => request('GET', p),
  post: (p, b = {}) => request('POST', p, b),
  put: (p, b = {}) => request('PUT', p, b),
  del: (p) => request('DELETE', p),
};

// Unduh berkas (CSV) dengan sesi yang sama.
export async function download(path, filename) {
  const res = await request('GET', path, undefined, true);
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement('a');
  a.href = url; a.download = filename; a.click();
  URL.revokeObjectURL(url);
}

// Ambil berkas sebagai Blob (mis. bukti transfer) dengan sesi yang sama.
export async function fetchBlob(path) {
  const res = await request('GET', path, undefined, true);
  return res.blob();
}

// qs({a: 1, b: ''}) -> "?a=1" (nilai kosong dibuang)
export const qs = (o) => {
  const p = new URLSearchParams();
  Object.entries(o).forEach(([k, v]) => { if (v !== '' && v != null) p.set(k, v); });
  const s = p.toString();
  return s ? '?' + s : '';
};
