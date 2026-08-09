/**
 * Test Setup File
 *
 * Configures the testing environment for Vitest + React Testing Library
 */
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';

// Track every window.setTimeout so afterEach can sweep the ones that outlive
// their test. Unmounting does NOT clear a component's pending timers unless the
// component clears them itself, and at least one dependency does not:
// @radix-ui/react-toast arms its auto-dismiss with `window.setTimeout(handleClose,
// duration)` in an effect that returns no cleanup function. Our toasts run a
// 5000ms duration, so any test that fires one and finishes sooner leaves a live
// timer behind. When the file's jsdom is torn down, that callback runs against a
// destroyed environment and throws `ReferenceError: document is not defined` —
// an *unhandled* error, which fails the whole vitest run with exit code 1 while
// still reporting every test as passed. It only reproduces under CPU contention
// (CI's 4-vCPU runner), because on a fast machine the process usually exits
// before the stray timer's turn comes up.
//
// Sweeping is safe: any timer still pending after a test finished and the trees
// were unmounted is by definition unowned.
const pendingTimeouts = new Set();
const realSetTimeout = window.setTimeout.bind(window);
const realClearTimeout = window.clearTimeout.bind(window);

// jsdom's `window` and the worker's `globalThis` can hold separate bindings for
// these, so patch both: Radix calls `window.setTimeout` explicitly, while most
// app and library code calls the bare global.
for (const target of new Set([window, globalThis])) {
  target.setTimeout = function trackedSetTimeout(...args) {
    const id = realSetTimeout(...args);
    pendingTimeouts.add(id);
    return id;
  };
  target.clearTimeout = function trackedClearTimeout(id) {
    pendingTimeouts.delete(id);
    return realClearTimeout(id);
  };
}

// Cleanup after each test
afterEach(() => {
  cleanup();
  for (const id of pendingTimeouts) realClearTimeout(id);
  pendingTimeouts.clear();
});

// Mock localStorage
const localStorageMock = {
  getItem: vi.fn(),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
};

global.localStorage = localStorageMock;

// Mock matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// Mock ResizeObserver
global.ResizeObserver = class ResizeObserver {
  constructor(callback) {
    this.callback = callback;
  }
  observe() {}
  unobserve() {}
  disconnect() {}
};

// Mock IntersectionObserver
global.IntersectionObserver = class IntersectionObserver {
  constructor(callback) {
    this.callback = callback;
  }
  observe() {}
  unobserve() {}
  disconnect() {}
};

console.warn('✅ Test environment configured');
