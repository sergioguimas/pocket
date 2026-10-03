import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Backend Go local, para dev no PC (CONTRATO §8: POCKET_LISTEN=127.0.0.1:9104
// com cert autoassinado). secure:false porque o cert de dev não é confiável
// para o Node/Vite.
const BACKEND_DEV_URL = 'https://127.0.0.1:9104'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // O binário Go (main.go, //go:embed all:dist) espera o build aqui.
    outDir: '../dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': { target: BACKEND_DEV_URL, secure: false, changeOrigin: true },
      '/health': { target: BACKEND_DEV_URL, secure: false, changeOrigin: true },
    },
  },
})
