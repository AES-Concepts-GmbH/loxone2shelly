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
    environment: 'jsdom',
    setupFiles: ['src/__tests__/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      all: true,
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/__tests__/**', 'src/**/*.d.ts', 'src/main.tsx', 'src/types/**'],
      reporter: ['text', 'text-summary'],
      // CI gate: fails the run when statement or line coverage drops below 90 %.
      thresholds: { statements: 90, lines: 90 },
    },
  },
})
