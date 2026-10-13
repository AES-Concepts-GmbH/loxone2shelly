/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true }
    }
  },
  define: {
    '__APP_VERSION__': JSON.stringify(process.env.VITE_APP_VERSION || 'dev'),
    '__BUILD_DATE__': JSON.stringify(process.env.VITE_BUILD_DATE || ''),
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
