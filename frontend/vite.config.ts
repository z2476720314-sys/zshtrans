import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 构建产物直接落到 Go 的 embed 目录，打包时一并嵌进 exe
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
  },
  server: {
    port: 5273,
    proxy: {
      '/api': 'http://127.0.0.1:8765',
    },
  },
})
