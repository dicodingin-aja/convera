const env = import.meta.env;

export const appConfig = Object.freeze({
  apiBaseUrl: String(env.VITE_API_BASE_URL || '').replace(/\/+$/, ''),
  backendEnabled: env.VITE_BACKEND_ENABLED === 'true',
  webhookBase: String(env.VITE_WEBHOOK_BASE || 'https://api.convera.id/webhook').replace(/\/+$/, ''),
  liveMetaApi: env.VITE_LIVE_API === 'true',
  csPhone: String(env.VITE_CS_PHONE || '+62 812-9573-1556').trim(), // nomor CS yang tampil di halaman pembayaran diproses
  requestTimeoutMs: Number(env.VITE_REQUEST_TIMEOUT_MS || 10000),
});
