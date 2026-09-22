import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const apiTarget = process.env.API_PROXY_TARGET ?? 'http://localhost:8090'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': new URL('./src', import.meta.url).pathname },
  },
  server: {
    // The end-to-end suite runs its own isolated stack on different ports, so
    // it can run alongside `make dev` without either disturbing the other.
    port: Number(process.env.WEB_PORT ?? 5180),
    strictPort: true,
    // Proxying keeps the API same-origin during development, so the session
    // cookie behaves exactly as it will in production behind Nginx — no CORS
    // preflights and no SameSite surprises.
    proxy: {
      // ws: true is required — the realtime endpoint lives at /api/v1/ws, so
      // the same rule has to carry the upgrade as well as ordinary requests.
      '/api': { target: apiTarget, changeOrigin: true, ws: true },
    },
  },
})
