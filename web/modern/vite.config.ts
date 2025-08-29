/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createHash } from 'node:crypto'
const __dirname = path.dirname(fileURLToPath(import.meta.url))

// Helper function to generate short SHA256 hash from chunk content with date and time
//
// This is a better approach than using the default hash provided by Vite (I'm not sure what algorithm they use).
// Now includes date and time for dynamic file naming and better cache busting.
function generateContentHash(content: string): string {
  const currentDateTime = new Date().toISOString() // Full ISO timestamp format (YYYY-MM-DDTHH:mm:ss.sssZ)
  const contentWithDateTime = `${content}-${currentDateTime}`
  return createHash('sha256').update(contentWithDateTime).digest('hex').substring(0, 8)
}

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
    rollupOptions: {
      output: {
        // Use both name and hash (Use SHA256 digest) for chunk file names to aid debugging and cache busting
        chunkFileNames: (chunkInfo) => {
          // Use moduleIds as a proxy for chunk content since code is not available at this stage
          const moduleContent = chunkInfo.moduleIds?.join('') || chunkInfo.name || 'chunk'
          const contentHash = generateContentHash(moduleContent)
          return `[name].${contentHash}.js`
        },
        // Use SHA256 digest for entry file names
        entryFileNames: (chunkInfo) => {
          // Use moduleIds as a proxy for entry content
          const moduleContent = chunkInfo.moduleIds?.join('') || chunkInfo.name || 'entry'
          const contentHash = generateContentHash(moduleContent)
          return `[name].${contentHash}.js`
        },
        // Use SHA256 digest for asset file names (CSS, images, etc.)
        assetFileNames: (assetInfo) => {
          // Use the asset name and type for content hashing
          const assetContent = assetInfo.name || 'asset'
          const contentHash = generateContentHash(assetContent)
          const extension = assetInfo.name?.split('.').pop() || 'asset'
          return `[name].${contentHash}.${extension}`
        },
        manualChunks: {
          vendor: ['react', 'react-dom'],
          router: ['react-router-dom'],
          ui: ['@radix-ui/react-dialog', '@radix-ui/react-dropdown-menu'],
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
