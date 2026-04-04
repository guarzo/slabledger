import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { useSellSheet } from './useSellSheet';

// The global setup.js installs a vi.fn() localStorage stub that doesn't
// persist data between calls. This test suite needs real storage behaviour,
// so we swap in a Map-backed implementation for the duration of the suite.
function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => { store.set(key, value); },
    removeItem: (key: string) => { store.delete(key); },
    clear: () => { store.clear(); },
    get length() { return store.size; },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}

describe('useSellSheet', () => {
  let storageMock: ReturnType<typeof makeLocalStorageMock>;

  beforeEach(() => {
    storageMock = makeLocalStorageMock();
    vi.stubGlobal('localStorage', storageMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('initializes with empty set when localStorage is empty', () => {
    const { result } = renderHook(() => useSellSheet());
    expect(result.current.count).toBe(0);
    expect(result.current.has('abc')).toBe(false);
  });

  it('initializes from existing localStorage data', () => {
    localStorage.setItem('sellSheetIds', JSON.stringify(['id1', 'id2']));
    const { result } = renderHook(() => useSellSheet());
    expect(result.current.count).toBe(2);
    expect(result.current.has('id1')).toBe(true);
    expect(result.current.has('id2')).toBe(true);
  });

  it('adds items', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b']));
    expect(result.current.count).toBe(2);
    expect(result.current.has('a')).toBe(true);
    expect(result.current.has('b')).toBe(true);
    expect(JSON.parse(localStorage.getItem('sellSheetIds')!)).toEqual(['a', 'b']);
  });

  it('does not duplicate existing items on add', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b']));
    act(() => result.current.add(['b', 'c']));
    expect(result.current.count).toBe(3);
  });

  it('removes items', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b', 'c']));
    act(() => result.current.remove(['b']));
    expect(result.current.count).toBe(2);
    expect(result.current.has('b')).toBe(false);
    expect(result.current.has('a')).toBe(true);
  });

  it('clears all items', () => {
    const { result } = renderHook(() => useSellSheet());
    act(() => result.current.add(['a', 'b']));
    act(() => result.current.clear());
    expect(result.current.count).toBe(0);
    expect(JSON.parse(localStorage.getItem('sellSheetIds')!)).toEqual([]);
  });

  it('handles corrupted localStorage gracefully', () => {
    localStorage.setItem('sellSheetIds', 'not-json');
    const { result } = renderHook(() => useSellSheet());
    expect(result.current.count).toBe(0);
  });

  it('syncs across hooks via storage event', () => {
    const { result: hook1 } = renderHook(() => useSellSheet());
    const { result: hook2 } = renderHook(() => useSellSheet());
    act(() => hook1.current.add(['x']));
    expect(hook2.current.has('x')).toBe(true);
  });
});
