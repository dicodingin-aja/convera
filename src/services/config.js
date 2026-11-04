const env = import.meta.env;

export const appConfig = Object.freeze({
  apiBaseUrl: String(env.VITE_API_BASE_URL || '').replace(/\/+$/, ''),
  backendEnabled: env.VITE_BACKEND_ENABLED === 'true',
  webhookBase: String(env.VITE_WEBHOOK_BASE || 'https://app.convera.id/webhook').replace(/\/+$/, ''),
  liveMetaApi: env.VITE_LIVE_API === 'true',
  requestTimeoutMs: Number(env.VITE_REQUEST_TIMEOUT_MS || 10000),
});
