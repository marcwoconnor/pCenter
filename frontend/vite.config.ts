import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Dev proxy target — override with VITE_API_TARGET env var.
// Default: production pCenter instance.
const apiTarget = process.env.VITE_API_TARGET || 'http://10.31.11.50:8080'
const wsTarget = apiTarget.replace(/^http/, 'ws')

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': apiTarget,
      '/health': apiTarget,
      '/ws': {
        target: wsTarget,
        ws: true,
      },
    },
  },
  build: {
    target: 'esnext',
  },
})
