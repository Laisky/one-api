/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: '../build/modern',
    // This is now correct; source maps should only be generated for development mode, not production
    sourcemap: mode === 'development',
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        // Use both name and hash for chunk file names to aid debugging and cache busting
        chunkFileNames: '[name].[hash].js',
        manualChunks: (id) => {
          if (id.includes('node_modules')) {
            if (id.includes('react/') || id.includes('react-dom/')) {
              return 'vendor'
            }
            if (id.includes('react-router-dom')) {
              return 'router'
            }
            if (id.includes('@radix-ui/react-dialog') || id.includes('@radix-ui/react-dropdown-menu') ||
              id.includes('@radix-ui/react-popover') || id.includes('@radix-ui/react-tooltip')) {
              return 'ui-overlay'
            }
            if (id.includes('@radix-ui/react-select') || id.includes('@radix-ui/react-checkbox') ||
              id.includes('@radix-ui/react-switch') || id.includes('@radix-ui/react-slider')) {
              return 'ui-form'
            }
            if (id.includes('@radix-ui')) {
              return 'ui'
            }
            if (id.includes('react-markdown') || id.includes('rehype') || id.includes('remark') || id.includes('marked')) {
              return 'markdown'
            }
            if (id.includes('recharts') || id.includes('d3-')) {
              return 'charts'
            }
            if (id.includes('@tanstack/react-query')) {
              return 'query'
            }
            if (id.includes('@tanstack/react-table') || id.includes('@tanstack/table-core')) {
              return 'table'
            }
            if (id.includes('katex')) {
              return 'katex'
            }
            if (id.includes('react-hook-form') || id.includes('zod') || id.includes('@hookform')) {
              return 'forms'
            }
            if (id.includes('axios')) {
              return 'http'
            }
            if (id.includes('i18next') || id.includes('i18next-browser-languagedetector') || id.includes('react-i18next')) {
              return 'i18n'
            }
            if (id.includes('highlight.js')) {
              return 'highlight'
            }
            if (id.includes('lucide-react')) {
              return 'icons'
            }
            if (id.includes('qrcode')) {
              return 'qrcode'
            }
            if (id.includes('zustand')) {
              return 'state'
            }
            if (id.includes('cmdk')) {
              return 'command'
            }
            if (id.includes('scheduler')) {
              return 'vendor'
            }
            return 'vendor-misc'
          }
        },
      },
    },
  },
  server: {
    port: 3001,
    proxy: {
      '/api': { target: 'http://localhost:3000', changeOrigin: true },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
}))
