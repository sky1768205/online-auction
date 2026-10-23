import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const backend = process.env.BACKEND_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    // Один origin для фронтенда и API: httpOnly-cookie и WebSocket работают без CORS.
    proxy: {
      '/api': backend,
      '/uploads': backend,
      '/ws': { target: backend, ws: true },
    },
  },
})
