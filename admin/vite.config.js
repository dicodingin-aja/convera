import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Dev: /api diteruskan ke API admin (go run ./cmd/admin, :8081) sehingga browser memakai satu origin,
// cookie sesi (SameSite=Strict) jalan tanpa CORS. Produksi: Caddy melakukan hal yang sama (deploy/Caddyfile).
const target = process.env.ADMIN_API_URL || 'http://localhost:8081';
export default defineConfig({
  plugins: [react()],
  server: { port: 5174, proxy: { '/api': { target, changeOrigin: false } } },
});
