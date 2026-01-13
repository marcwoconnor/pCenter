import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': 'http://10.31.11.50:8080',
      '/health': 'http://10.31.11.50:8080',
      '/ws': {
        target: 'ws://10.31.11.50:8080',
        ws: true,
      },
    },
  },
  build: {
    target: 'esnext',
  },
})
