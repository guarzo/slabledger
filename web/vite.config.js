import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'
import { visualizer } from 'rollup-plugin-visualizer'

export default defineConfig(({ mode }) => ({
  envDir: '..',  // Load .env from web/ directory (parent of src/)
  plugins: [
    // @vitejs/plugin-react automatically handles JSX/TSX transformation
    // No need for custom esbuild loaders - Vite handles this out of the box
    react(),
    // Bundle analyzer - only in analyze mode
    mode === 'analyze' && visualizer({
      open: true,
      filename: '../dist/stats.html',
      gzipSize: true,
      brotliSize: true,
    })
  ].filter(Boolean),
  root: 'src',
  publicDir: '../public',
  build: {
    outDir: '../dist',
    emptyOutDir: true,
    rolldownOptions: {
      input: {
        // HTML entry points - Vite will automatically process linked CSS/JS
        // Single entry point for SPA (collection and pricing are now part of index.html)
        main: resolve(__dirname, 'src/index.html')
      },
      output: {
        entryFileNames: 'js/[name].[hash].js',
        chunkFileNames: 'js/[name].[hash].js',
        assetFileNames: (assetInfo) => {
          const name = assetInfo.name ?? assetInfo.names?.[0] ?? '';
          // Keep CSS in /css/ directory
          if (name.endsWith('.css')) {
            return 'css/[name].[hash][extname]';
          }
          // Keep JS in /js/ directory (handled by entryFileNames)
          // Everything else goes to /assets/
          return 'assets/[name].[hash][extname]';
        }
      }
    },
    sourcemap: true,
    target: 'es2015',
    chunkSizeWarningLimit: 800
  },
  server: {
    port: 5173,
    // Disable proxy in Playwright test mode to allow route mocking
    proxy: process.env.PLAYWRIGHT_TEST ? {} : {
      // Proxy API requests to the Go server in dev mode
      '/api': {
        target: 'http://localhost:8081',
        changeOrigin: true
      },
      // Proxy auth routes to the Go server
      '/auth': {
        target: 'http://localhost:8081',
        changeOrigin: true
      }
    }
  }
}))
