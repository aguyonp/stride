import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// VITE_MOCK=1 (npm run dev:mock) : l'app tourne sur les fixtures de src/mocks
// au lieu du réseau. La bascule est faite dans src/api/client.ts ; en build
// normal, le code des mocks est éliminé (import dynamique sous condition statique).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  preview: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
    sourcemap: false,
    chunkSizeWarningLimit: 600,
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
