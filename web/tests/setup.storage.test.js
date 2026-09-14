import { expect, it, vi } from 'vitest';

it('preserves shared localStorage when tests restore their own global stubs', () => {
  const storage = localStorage;
  vi.stubGlobal('fetch', vi.fn());
  vi.unstubAllGlobals();

  expect(localStorage).toBe(storage);
  expect(window.localStorage).toBe(storage);
});
