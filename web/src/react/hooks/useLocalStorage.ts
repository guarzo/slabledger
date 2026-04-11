import { useState, useEffect, useCallback, useRef, Dispatch, SetStateAction } from 'react';
import { reportError } from '../../js/errors';

/**
 * Custom hook for localStorage with React state synchronization
 * Provides type-safe localStorage access with cross-tab synchronization
 *
 * @template T
 * @param key - localStorage key
 * @param initialValue - Default value if key doesn't exist
 * @returns [value, setValue, removeValue]
 *
 * @example
 * const [theme, setTheme, removeTheme] = useLocalStorage('theme', 'dark');
 * setTheme('light'); // Updates state and localStorage
 * removeTheme(); // Clears from localStorage and resets to initial value
 */
export function useLocalStorage<T>(
  key: string,
  initialValue: T
): [T, Dispatch<SetStateAction<T>>, () => void] {
  const initialValueRef = useRef(initialValue);
  useEffect(() => { initialValueRef.current = initialValue; }, [initialValue]);

  const [storedValue, setStoredValue] = useState<T>(() => {
    if (typeof window === 'undefined') {
      return initialValue;
    }

    try {
      const item = window.localStorage.getItem(key);
      // Parse stored json or return initialValue
      return item ? (JSON.parse(item) as T) : initialValue;
    } catch {
      return initialValue;
    }
  });

  const setValue = useCallback<Dispatch<SetStateAction<T>>>(
    (value) => {
      // Use functional updater to avoid stale closure over storedValue
      setStoredValue((prev) => {
        const valueToStore = value instanceof Function ? value(prev) : value;

        // Save to local storage
        try {
          if (typeof window !== 'undefined') {
            window.localStorage.setItem(key, JSON.stringify(valueToStore));

            // Dispatch custom event for cross-tab synchronization
            window.dispatchEvent(
              new CustomEvent('local-storage', {
                detail: { key, value: valueToStore, removed: false },
              })
            );
          }
        } catch (err) {
          if (import.meta.env.DEV) {
            reportError('useLocalStorage/write', err);
          }
          // State still updates; only persistence failed
        }

        return valueToStore;
      });
    },
    [key]
  );

  // Remove value from localStorage
  const removeValue = useCallback(() => {
    try {
      if (typeof window !== 'undefined') {
        window.localStorage.removeItem(key);

        // Dispatch custom event for cross-tab synchronization
        window.dispatchEvent(
          new CustomEvent('local-storage', {
            detail: { key, value: null, removed: true },
          })
        );
      }
    } catch {
      // localStorage removal failed — still reset state below
    } finally {
      setStoredValue(initialValueRef.current);
    }
  }, [key]);

  // Listen for changes from other tabs/windows
  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }

    // Handler for storage event (cross-tab sync)
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === key && e.newValue !== null) {
        try {
          setStoredValue(JSON.parse(e.newValue) as T);
        } catch {
          // Ignore malformed storage events
        }
      } else if (e.key === key && e.newValue === null) {
        setStoredValue(initialValueRef.current);
      }
    };

    // Handler for custom local-storage event (same-tab sync)
    const handleLocalStorageEvent = (e: Event) => {
      const customEvent = e as CustomEvent<{ key: string; value: T | null; removed?: boolean }>;
      if (customEvent.detail.key === key) {
        if (customEvent.detail.removed) {
          setStoredValue(initialValueRef.current);
        } else {
          setStoredValue(customEvent.detail.value as T);
        }
      }
    };

    window.addEventListener('storage', handleStorageChange);
    window.addEventListener('local-storage', handleLocalStorageEvent);

    return () => {
      window.removeEventListener('storage', handleStorageChange);
      window.removeEventListener('local-storage', handleLocalStorageEvent);
    };
  }, [key]);

  return [storedValue, setValue, removeValue];
}

export default useLocalStorage;
