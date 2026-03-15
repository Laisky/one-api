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
    // Increase chunk size warning limit to reduce noise for legitimate large chunks
    chunkSizeWarningLimit: 500, // Reduced from 1000 to encourage better chunking
    // Enable advanced minification and optimization
    minify: 'esbuild',
    target: 'esnext',
    // Additional build optimizations
    cssCodeSplit: true, // Enable CSS code splitting
    assetsInlineLimit: 4096, // Inline assets smaller than 4KB
    reportCompressedSize: true, // Report compressed sizes in build output
    // Enable advanced esbuild optimizations
    esbuild: {
      legalComments: 'none',
      treeShaking: true,
      minifyIdentifiers: true,
      minifySyntax: true,
      minifyWhitespace: true,
    },
    rollupOptions: {
      // Improve tree shaking and dead code elimination
      treeshake: {
        preset: 'recommended',
        moduleSideEffects: 'no-external',
        propertyReadSideEffects: false,
        tryCatchDeoptimization: false,
      },
      // Optimize external dependencies
      external: [],
      output: {
        // Use both name and hash for chunk file names to aid debugging and cache busting
        chunkFileNames: '[name].[hash].js',
        manualChunks: {
          // Core React libraries - keep small and essential
          vendor: ['react', 'react-dom'],
          router: ['react-router-dom'],

          // TanStack libraries split for better caching
          'tanstack-query': ['@tanstack/react-query'],
          'tanstack-table': ['@tanstack/react-table'],

          // Split Radix UI into logical groups to reduce chunk sizes
          'radix-ui-core': [
            '@radix-ui/react-dialog',
            '@radix-ui/react-dropdown-menu',
            '@radix-ui/react-popover',
            '@radix-ui/react-tooltip'
          ],
          'radix-ui-forms': [
            '@radix-ui/react-checkbox',
            '@radix-ui/react-label',
            '@radix-ui/react-select',
            '@radix-ui/react-switch'
          ],
          'radix-ui-layout': [
            '@radix-ui/react-scroll-area',
            '@radix-ui/react-separator',
            '@radix-ui/react-slot',
            '@radix-ui/react-tabs',
            '@radix-ui/react-toast',
            '@radix-ui/react-hover-card'
          ],

          // Markdown processing and syntax highlighting - heavy libraries
          'markdown-core': ['react-markdown', 'marked'],
          // Split markdown plugins into smaller chunks
          'markdown-remark': ['remark-gfm', 'remark-math', 'remark-emoji'],
          'markdown-rehype-highlight': ['rehype-highlight'],
          'markdown-rehype-katex': ['rehype-katex', 'katex'],
          'markdown-rehype-sanitize': ['rehype-sanitize'],

          // Chart and visualization libraries
          charts: ['recharts'],

          // Icons and UI utilities
          'ui-utils': [
            'lucide-react',
            'class-variance-authority',
            'clsx',
            'tailwind-merge',
            'cmdk'
          ],

          // Form handling
          forms: ['react-hook-form', '@hookform/resolvers', 'zod'],

          // Internationalization
          //
          // Note: This currently unused
          //i18n: ['i18next', 'react-i18next', 'i18next-browser-languagedetector'],

          // Network and external services
          network: ['axios'],

          // Specialized utilities
          'misc-utils': [
            'qrcode',
            'zustand'
          ],
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
