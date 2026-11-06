import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发时:BACKEND_PORT=18079 npm run dev 可指定后端端口,默认 8079
const backendPort = process.env.BACKEND_PORT || '8079'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': `http://localhost:${backendPort}`,
      '/webhook': `http://localhost:${backendPort}`
    }
  },
  build: {
    outDir: 'dist'
  }
})
