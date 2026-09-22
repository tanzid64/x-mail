import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 5180,
    strictPort: true,
    // Proxying keeps the API same-origin during development, so the session
    // cookie behaves exactly as it will in production behind Nginx — no CORS
    // preflights and no SameSite surprises.
    proxy: {
      '/api': { target: 'http://localhost:8090', changeOrigin: true },
      '/ws': { target: 'ws://localhost:8090', ws: true },
    },
  },
})
