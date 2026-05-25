import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api/replay': { target: 'http://dataset-replay:8081', changeOrigin: true },
      '/api/recommendations': { target: 'http://recommendation:8082', changeOrigin: true },
      '/api/dashboard': { target: 'http://engagement:8083', changeOrigin: true },
      '/api/activity': { target: 'http://engagement:8083', changeOrigin: true },
      '/api/analytics': { target: 'http://retention-analytics:8084', changeOrigin: true },
      '/api/ai': { target: 'http://ai-insights:8085', changeOrigin: true },
      '/ws': { target: 'ws://engagement:8083', ws: true },
    },
  },
})
