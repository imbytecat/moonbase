import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [
    // The router plugin must run before the React plugin.
    tanstackRouter(),
    react(),
    tailwindcss(),
  ],
  // Build the SPA straight into the Go server's embed dir (apps/server/internal/web/dist).
  build: {
    outDir: '../server/internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      // Forward API calls to the Go server during development (no CORS needed).
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      // Permanent file URLs (/f/{file_id}) are served by the Go server too.
      '/f': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
