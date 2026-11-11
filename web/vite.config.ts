import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // Proxy the API (including OAuth callbacks) so cookies land on the Vite origin.
    proxy: { '/api': 'http://localhost:8080' },
  },
  build: {
    outDir: '../internal/webembed/dist',
    emptyOutDir: true,
  },
})
