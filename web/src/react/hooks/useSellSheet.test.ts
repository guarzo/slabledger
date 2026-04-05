import { renderHook, act, waitFor } from '@testing-library/react';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { useSellSheet } from './useSellSheet';

// Shared state so mocks can track mutations
let serverItems: string[] = [];

// Mock the API module
vi.mock('../../js/api', () => ({
  api: {
    getSellSheetItems: vi.fn().mockImplementation(async () => ({ purchaseIds: [...serverItems] })),
    addSellSheetItems: vi.fn().mockImplementation(async (ids: string[]) => {
      const set = new Set(serverItems);
      for (const id of ids) set.add(id);
      serverItems = Array.from(set);
    }),
    removeSellSheetItems: vi.fn().mockImplementation(async (ids: string[]) => {
      const removeSet = new Set(ids);
      serverItems = serverItems.filter(id => !removeSet.has(id));
    }),
    clearSellSheetItems: vi.fn().mockImplementation(async () => {
      serverItems = [];
    }),
  },
}));

import { api } from '../../js/api';

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

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
}

function resetMockImplementations() {
  (api.getSellSheetItems as ReturnType<typeof vi.fn>).mockImplementation(
    async () => ({ purchaseIds: [...serverItems] }),
  );
  (api.addSellSheetItems as ReturnType<typeof vi.fn>).mockImplementation(
    async (ids: string[]) => {
      const set = new Set(serverItems);
      for (const id of ids) set.add(id);
      serverItems = Array.from(set);
    },
  );
  (api.removeSellSheetItems as ReturnType<typeof vi.fn>).mockImplementation(
    async (ids: string[]) => {
      const removeSet = new Set(ids);
      serverItems = serverItems.filter(id => !removeSet.has(id));
    },
  );
  (api.clearSellSheetItems as ReturnType<typeof vi.fn>).mockImplementation(
    async () => { serverItems = []; },
  );
}

describe('useSellSheet', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', makeLocalStorageMock());
    vi.clearAllMocks();
    serverItems = [];
    resetMockImplementations();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('initializes with empty set', async () => {
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.count).toBe(0);
    expect(result.current.has('abc')).toBe(false);
  });

  it('loads items from server', async () => {
    serverItems = ['id1', 'id2'];
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.count).toBe(2);
    expect(result.current.has('id1')).toBe(true);
  });

  it('adds items optimistically', async () => {
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    act(() => result.current.add(['a', 'b']));
    await waitFor(() => expect(result.current.count).toBe(2));
    expect(result.current.has('a')).toBe(true);
    expect(api.addSellSheetItems).toHaveBeenCalledWith(['a', 'b']);
  });

  it('removes items optimistically', async () => {
    serverItems = ['a', 'b', 'c'];
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.count).toBe(3));
    act(() => result.current.remove(['b']));
    await waitFor(() => expect(result.current.count).toBe(2));
    expect(result.current.has('b')).toBe(false);
    expect(api.removeSellSheetItems).toHaveBeenCalledWith(['b']);
  });

  it('clears all items optimistically', async () => {
    serverItems = ['a', 'b'];
    const { result } = renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.count).toBe(2));
    act(() => result.current.clear());
    await waitFor(() => expect(result.current.count).toBe(0));
    expect(api.clearSellSheetItems).toHaveBeenCalled();
  });

  it('migrates from localStorage when server is empty', async () => {
    localStorage.setItem('sellSheetIds', JSON.stringify(['legacy1', 'legacy2']));
    renderHook(() => useSellSheet(), { wrapper: createWrapper() });
    await waitFor(() => expect(api.addSellSheetItems).toHaveBeenCalledWith(['legacy1', 'legacy2']));
    expect(localStorage.getItem('sellSheetIds')).toBeNull();
  });
});
