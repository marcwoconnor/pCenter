import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { execSync } from 'child_process'

// Dev proxy target — override with VITE_API_TARGET env var.
// Default: production pCenter instance.
const apiTarget = process.env.VITE_API_TARGET || 'http://10.31.11.50:8080'
const wsTarget = apiTarget.replace(/^http/, 'ws')

// Get version from git tag or package.json
const version = process.env.PCENTER_VERSION ||
  (() => { try { return execSync('git describe --tags --always 2>/dev/null').toString().trim() } catch { return '0.1.0-dev' } })()

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    __APP_VERSION__: JSON.stringify(version),
  },
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
