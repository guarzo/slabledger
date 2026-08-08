import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./tests/setup.js'],
    // These two settings exist to stop the suite from failing on CPU contention
    // rather than on product behavior. The tests here are not slow: the worst of
    // them renders in ~1.0s in isolation. Under an unbounded worker pool it
    // inflated to 3.9s, i.e. 78% of the old 5000ms default, so whichever handful
    // of the ~10 near-limit tests lost the CPU lottery that run failed with
    // "Test timed out in 5000ms" — a different set each time. See SLA-50.
    //
    // Cap the pool so jsdom environments stop oversubscribing the box. Measured
    // on a 24-core machine: peak test 3910ms -> ~1300ms and aggregate
    // `environment` 148-186s -> 63-72s, at no wall-clock cost (~18-20s either
    // way). Capping to 4 lowers the peak further but costs ~40% wall clock.
    // On CI (ubuntu-latest, 4 vCPU) this is inert — the pool is already smaller.
    maxWorkers: 8,
    // The cap only binds where cores > 8, so it does nothing for CI. 5000ms left
    // only 1.28x headroom over the observed worst case on a fast idle machine;
    // 15000ms keeps a genuine hang failing promptly while removing the lottery
    // on hardware we do not measure here.
    testTimeout: 15000,
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
