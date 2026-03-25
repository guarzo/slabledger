/**
 * Tests for useLocalStorage hook
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLocalStorage } from '../../src/react/hooks/useLocalStorage';

describe('useLocalStorage', () => {
  beforeEach(() => {
    // Clear localStorage before each test
    localStorage.clear();
    localStorage.getItem.mockClear();
    localStorage.setItem.mockClear();
    localStorage.removeItem.mockClear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('initialization', () => {
    it('should initialize with default value when localStorage is empty', () => {
      localStorage.getItem.mockReturnValue(null);

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      expect(result.current[0]).toBe('default');
      expect(localStorage.getItem).toHaveBeenCalledWith('test-key');
    });

    it('should initialize with stored value when localStorage has data', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('stored'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      expect(result.current[0]).toBe('stored');
      expect(localStorage.getItem).toHaveBeenCalledWith('test-key');
    });

    it('should handle invalid JSON gracefully', () => {
      localStorage.getItem.mockReturnValue('invalid-json{');

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      expect(result.current[0]).toBe('default');
    });

    it('should work with objects', () => {
      const obj = { count: 5, name: 'test' };
      localStorage.getItem.mockReturnValue(JSON.stringify(obj));

      const { result } = renderHook(() => useLocalStorage('test-key', { count: 0 }));

      expect(result.current[0]).toEqual(obj);
    });

    it('should work with arrays', () => {
      const arr = ['item1', 'item2', 'item3'];
      localStorage.getItem.mockReturnValue(JSON.stringify(arr));

      const { result } = renderHook(() => useLocalStorage('test-key', []));

      expect(result.current[0]).toEqual(arr);
    });

    it('should work with numbers', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify(42));

      const { result } = renderHook(() => useLocalStorage('test-key', 0));

      expect(result.current[0]).toBe(42);
    });

    it('should work with booleans', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify(true));

      const { result } = renderHook(() => useLocalStorage('test-key', false));

      expect(result.current[0]).toBe(true);
    });
  });

  describe('setValue', () => {
    it('should update state and localStorage', () => {
      localStorage.getItem.mockReturnValue(null);
      localStorage.setItem.mockImplementation(() => {});

      const { result } = renderHook(() => useLocalStorage('test-key', 'initial'));

      act(() => {
        result.current[1]('updated');
      });

      expect(result.current[0]).toBe('updated');
      expect(localStorage.setItem).toHaveBeenCalledWith('test-key', JSON.stringify('updated'));
    });

    it('should support functional updates', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('initial'));
      localStorage.setItem.mockImplementation(() => {});

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      act(() => {
        result.current[1]((prev) => prev + '-updated');
      });

      expect(result.current[0]).toBe('initial-updated');
      expect(localStorage.setItem).toHaveBeenCalledWith('test-key', JSON.stringify('initial-updated'));
    });

    it('should dispatch custom event for same-tab sync', () => {
      localStorage.getItem.mockReturnValue(null);
      localStorage.setItem.mockImplementation(() => {});

      const eventSpy = vi.fn();
      window.addEventListener('local-storage', eventSpy);

      const { result } = renderHook(() => useLocalStorage('test-key', 'initial'));

      act(() => {
        result.current[1]('updated');
      });

      expect(eventSpy).toHaveBeenCalled();
      expect(eventSpy.mock.calls[0][0].detail).toEqual({
        key: 'test-key',
        value: 'updated',
        removed: false,
      });

      window.removeEventListener('local-storage', eventSpy);
    });

    it('should handle errors gracefully', () => {
      localStorage.getItem.mockReturnValue(null);
      localStorage.setItem.mockImplementation(() => {
        throw new Error('Storage full');
      });

      const { result } = renderHook(() => useLocalStorage('test-key', 'initial'));

      // Should not throw even when localStorage.setItem fails
      act(() => {
        result.current[1]('updated');
      });

      // State updates even when persistence fails — UI should reflect user intent
      expect(result.current[0]).toBe('updated');
    });
  });

  describe('removeValue', () => {
    it('should reset to initial value and remove from localStorage', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('stored'));
      localStorage.removeItem.mockImplementation(() => {});

      const { result } = renderHook(() => useLocalStorage('test-key', 'initial'));

      expect(result.current[0]).toBe('stored');

      act(() => {
        result.current[2](); // Call removeValue
      });

      expect(result.current[0]).toBe('initial');
      expect(localStorage.removeItem).toHaveBeenCalledWith('test-key');
    });

    it('should dispatch custom event when removing', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('stored'));
      localStorage.removeItem.mockImplementation(() => {});

      const eventSpy = vi.fn();
      window.addEventListener('local-storage', eventSpy);

      const { result } = renderHook(() => useLocalStorage('test-key', 'initial'));

      act(() => {
        result.current[2]();
      });

      expect(eventSpy).toHaveBeenCalled();
      expect(eventSpy.mock.calls[0][0].detail).toEqual({
        key: 'test-key',
        value: null,
        removed: true,
      });

      window.removeEventListener('local-storage', eventSpy);
    });
  });

  describe('cross-tab synchronization', () => {
    it('should update when storage event is fired from another tab', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('initial'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      expect(result.current[0]).toBe('initial');

      // Simulate storage event from another tab
      act(() => {
        window.dispatchEvent(
          new StorageEvent('storage', {
            key: 'test-key',
            newValue: JSON.stringify('from-another-tab'),
            oldValue: JSON.stringify('initial'),
          })
        );
      });

      expect(result.current[0]).toBe('from-another-tab');
    });

    it('should reset to initial value when storage event has null newValue', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('stored'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'initial'));

      expect(result.current[0]).toBe('stored');

      act(() => {
        window.dispatchEvent(
          new StorageEvent('storage', {
            key: 'test-key',
            newValue: null,
            oldValue: JSON.stringify('stored'),
          })
        );
      });

      expect(result.current[0]).toBe('initial');
    });

    it('should ignore storage events for different keys', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('initial'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      expect(result.current[0]).toBe('initial');

      act(() => {
        window.dispatchEvent(
          new StorageEvent('storage', {
            key: 'other-key',
            newValue: JSON.stringify('should-not-update'),
          })
        );
      });

      expect(result.current[0]).toBe('initial');
    });

    it('should handle invalid JSON in storage events', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('initial'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      act(() => {
        window.dispatchEvent(
          new StorageEvent('storage', {
            key: 'test-key',
            newValue: 'invalid-json{',
          })
        );
      });

      expect(result.current[0]).toBe('initial'); // Should remain unchanged
    });
  });

  describe('same-tab synchronization', () => {
    it('should update when local-storage event is fired', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('initial'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      expect(result.current[0]).toBe('initial');

      act(() => {
        window.dispatchEvent(
          new CustomEvent('local-storage', {
            detail: { key: 'test-key', value: 'from-same-tab' },
          })
        );
      });

      expect(result.current[0]).toBe('from-same-tab');
    });

    it('should ignore local-storage events for different keys', () => {
      localStorage.getItem.mockReturnValue(JSON.stringify('initial'));

      const { result } = renderHook(() => useLocalStorage('test-key', 'default'));

      act(() => {
        window.dispatchEvent(
          new CustomEvent('local-storage', {
            detail: { key: 'other-key', value: 'should-not-update' },
          })
        );
      });

      expect(result.current[0]).toBe('initial');
    });
  });

  describe('cleanup', () => {
    it('should remove event listeners on unmount', () => {
      const removeEventListenerSpy = vi.spyOn(window, 'removeEventListener');

      const { unmount } = renderHook(() => useLocalStorage('test-key', 'default'));

      unmount();

      expect(removeEventListenerSpy).toHaveBeenCalledWith('storage', expect.any(Function));
      expect(removeEventListenerSpy).toHaveBeenCalledWith('local-storage', expect.any(Function));

      removeEventListenerSpy.mockRestore();
    });
  });
});
