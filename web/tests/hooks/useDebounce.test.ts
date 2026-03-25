import { renderHook, act } from '@testing-library/react';
import { useDebounce } from '../../src/react/hooks/useDebounce';

// Type declaration for global in test environment
declare const global: typeof globalThis;

describe('useDebounce Hook', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('Initial State', () => {
    it('should return initial value immediately', () => {
      const { result } = renderHook(() => useDebounce('initial', 300));
      expect(result.current).toBe('initial');
    });

    it('should work with different types', () => {
      // String
      const { result: stringResult } = renderHook(() => useDebounce('test', 100));
      expect(stringResult.current).toBe('test');

      // Number
      const { result: numberResult } = renderHook(() => useDebounce(42, 100));
      expect(numberResult.current).toBe(42);

      // Object
      const obj = { name: 'test', value: 123 };
      const { result: objectResult } = renderHook(() => useDebounce(obj, 100));
      expect(objectResult.current).toEqual(obj);

      // Array
      const arr = [1, 2, 3];
      const { result: arrayResult } = renderHook(() => useDebounce(arr, 100));
      expect(arrayResult.current).toEqual(arr);

      // Boolean
      const { result: boolResult } = renderHook(() => useDebounce(true, 100));
      expect(boolResult.current).toBe(true);

      // Null
      const { result: nullResult } = renderHook(() => useDebounce(null, 100));
      expect(nullResult.current).toBeNull();
    });
  });

  describe('Debounce Behavior', () => {
    it('should not update value before delay', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'updated' });

      // Before delay, should still show initial
      expect(result.current).toBe('initial');

      // Advance time but not past delay
      act(() => {
        vi.advanceTimersByTime(200);
      });

      expect(result.current).toBe('initial');
    });

    it('should update value after delay', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'updated' });

      act(() => {
        vi.advanceTimersByTime(300);
      });

      expect(result.current).toBe('updated');
    });

    it('should use default delay of 300ms when not specified', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'updated' });

      // At 299ms, should still be initial
      act(() => {
        vi.advanceTimersByTime(299);
      });
      expect(result.current).toBe('initial');

      // At 300ms, should update
      act(() => {
        vi.advanceTimersByTime(1);
      });
      expect(result.current).toBe('updated');
    });
  });

  describe('Rapid Updates', () => {
    it('should only update to final value after multiple rapid changes', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' } }
      );

      // Simulate rapid typing
      rerender({ value: 'a' });
      act(() => vi.advanceTimersByTime(100));

      rerender({ value: 'ab' });
      act(() => vi.advanceTimersByTime(100));

      rerender({ value: 'abc' });
      act(() => vi.advanceTimersByTime(100));

      rerender({ value: 'abcd' });
      act(() => vi.advanceTimersByTime(100));

      // Still showing initial because delay hasn't passed since last change
      expect(result.current).toBe('initial');

      // Complete the delay after last change
      act(() => vi.advanceTimersByTime(300));

      // Should show final value
      expect(result.current).toBe('abcd');
    });

    it('should reset timer on each value change', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' } }
      );

      // First change
      rerender({ value: 'first' });
      act(() => vi.advanceTimersByTime(200));
      expect(result.current).toBe('initial');

      // Second change resets timer
      rerender({ value: 'second' });
      act(() => vi.advanceTimersByTime(200));
      expect(result.current).toBe('initial');

      // Complete timer
      act(() => vi.advanceTimersByTime(100));
      expect(result.current).toBe('second');
    });
  });

  describe('Cleanup', () => {
    it('should clear timeout on unmount', () => {
      const clearTimeoutSpy = vi.spyOn(global, 'clearTimeout');

      const { unmount, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'updated' });
      unmount();

      expect(clearTimeoutSpy).toHaveBeenCalled();
      clearTimeoutSpy.mockRestore();
    });

    it('should clear previous timeout when value changes', () => {
      const clearTimeoutSpy = vi.spyOn(global, 'clearTimeout');

      const { rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'second' });
      rerender({ value: 'third' });

      // Should have cleared timeouts on value changes
      expect(clearTimeoutSpy.mock.calls.length).toBeGreaterThan(0);
      clearTimeoutSpy.mockRestore();
    });
  });

  describe('Delay Changes', () => {
    it('should reset debounce when delay changes', () => {
      const { result, rerender } = renderHook(
        ({ value, delay }) => useDebounce(value, delay),
        { initialProps: { value: 'test', delay: 300 } }
      );

      rerender({ value: 'updated', delay: 300 });
      act(() => vi.advanceTimersByTime(200));

      // Change delay - should reset the timer
      rerender({ value: 'updated', delay: 500 });

      // Advance by old remaining time (100ms) + some more
      act(() => vi.advanceTimersByTime(400));
      expect(result.current).toBe('test'); // Still not updated

      // Complete new delay
      act(() => vi.advanceTimersByTime(100));
      expect(result.current).toBe('updated');
    });

    it('should work with zero delay', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 0),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'updated' });

      // Even with 0 delay, setTimeout is async
      act(() => vi.advanceTimersByTime(0));

      expect(result.current).toBe('updated');
    });
  });

  describe('Edge Cases', () => {
    it('should handle undefined value', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'initial' as string | undefined } }
      );

      rerender({ value: undefined });
      act(() => vi.advanceTimersByTime(300));

      expect(result.current).toBeUndefined();
    });

    it('should handle same value rerenders', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: 'same' } }
      );

      // Rerender with same value multiple times
      rerender({ value: 'same' });
      act(() => vi.advanceTimersByTime(100));
      rerender({ value: 'same' });
      act(() => vi.advanceTimersByTime(100));
      rerender({ value: 'same' });
      act(() => vi.advanceTimersByTime(300));

      expect(result.current).toBe('same');
    });

    it('should handle object reference changes', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 300),
        { initialProps: { value: { id: 1 } } }
      );

      // New object with same content but different reference
      rerender({ value: { id: 1 } });
      act(() => vi.advanceTimersByTime(300));

      // Should still update because it's a new reference
      expect(result.current).toEqual({ id: 1 });
    });

    it('should handle very long delay', () => {
      const { result, rerender } = renderHook(
        ({ value }) => useDebounce(value, 10000),
        { initialProps: { value: 'initial' } }
      );

      rerender({ value: 'updated' });

      act(() => vi.advanceTimersByTime(9999));
      expect(result.current).toBe('initial');

      act(() => vi.advanceTimersByTime(1));
      expect(result.current).toBe('updated');
    });
  });
});
