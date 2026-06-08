import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@locales': path.resolve(__dirname, 'locales'),
    },
  },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/healthz': 'http://localhost:8080',
      '/v1': 'http://localhost:8080',
    },
  },
})
