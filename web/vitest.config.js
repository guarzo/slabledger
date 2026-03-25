import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./tests/setup.js'],
    // Exclude Playwright E2E tests from Vitest
    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/*.spec.js',  // Playwright E2E tests (JS)
      '**/*.spec.ts',  // Playwright E2E tests (TS)
      '**/*.e2e.js',
      '**/*.e2e.ts',
    ],
    coverage: {
      reporter: ['text', 'json', 'html'],
      exclude: [
        'node_modules/',
        'src/js/core/example.html',
        '**/*.config.js',
        'tests/*.spec.js',
        'tests/*.e2e.js',
      ],
      thresholds: {
        statements: 30,
        branches: 25,
        functions: 25,
        lines: 30,
      },
    },
  },
});
