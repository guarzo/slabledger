import { useCallback, useEffect, useState } from 'react';

interface RepriceKeyboardDeps {
  itemCount: number;
  selectedCount: number;
  onAcceptFocused: (index: number) => void;
  onToggleFocused: (index: number) => void;
  onJumpToInput: () => void;
  onShowShortcuts: () => void;
  onSubmit: () => void;
  onDeselectAll: () => void;
}

interface RepriceKeyboardResult {
  focusedIndex: number | null;
  setFocusedIndex: (i: number | null) => void;
}

function isEditableTarget(el: EventTarget | null): el is HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement {
  if (!el || !(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  if (tag === 'INPUT') {
    // Checkbox/radio/button-style inputs don't consume nav keys, so let them through.
    const type = (el as HTMLInputElement).type;
    return type !== 'checkbox' && type !== 'radio' && type !== 'button' && type !== 'submit' && type !== 'reset';
  }
  return tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable;
}

export function useRepriceKeyboard(deps: RepriceKeyboardDeps): RepriceKeyboardResult {
  const [focusedIndex, setFocusedIndex] = useState<number | null>(null);

  // If item list shrinks below current focus, clamp it.
  useEffect(() => {
    setFocusedIndex(prev => {
      if (prev === null) return prev;
      if (deps.itemCount === 0) return null;
      if (prev >= deps.itemCount) return deps.itemCount - 1;
      return prev;
    });
  }, [deps.itemCount]);

  const moveFocus = useCallback((delta: 1 | -1) => {
    setFocusedIndex(prev => {
      if (deps.itemCount === 0) return null;
      if (prev === null) return delta > 0 ? 0 : 0;
      const next = prev + delta;
      if (next < 0) return 0;
      if (next >= deps.itemCount) return deps.itemCount - 1;
      return next;
    });
  }, [deps.itemCount]);

  useEffect(() => {
    function handler(e: KeyboardEvent) {
      const editable = isEditableTarget(document.activeElement);

      // Cmd/Ctrl+Enter is the one combo that wins over editable focus
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        deps.onSubmit();
        return;
      }

      // Modifier-keys other than Cmd/Ctrl+Enter: ignore
      if (e.metaKey || e.ctrlKey || e.altKey) return;

      // Inside an editable element: only Enter and Escape leak through
      if (editable) {
        if (e.key === 'Enter') {
          (document.activeElement as HTMLElement).blur();
          if (focusedIndex !== null) deps.onAcceptFocused(focusedIndex);
        } else if (e.key === 'Escape') {
          (document.activeElement as HTMLElement).blur();
        }
        return;
      }

      switch (e.key) {
        case 'j':
        case 'ArrowDown':
          moveFocus(1);
          e.preventDefault();
          break;
        case 'k':
        case 'ArrowUp':
          moveFocus(-1);
          e.preventDefault();
          break;
        case 'Enter':
          if (focusedIndex !== null) deps.onAcceptFocused(focusedIndex);
          break;
        case ' ':
          if (focusedIndex !== null) deps.onToggleFocused(focusedIndex);
          e.preventDefault();
          break;
        case 'Escape':
          if (focusedIndex !== null) {
            setFocusedIndex(null);
          } else if (deps.selectedCount > 0) {
            deps.onDeselectAll();
          }
          break;
        case '/':
          deps.onJumpToInput();
          e.preventDefault();
          break;
        case '?':
          deps.onShowShortcuts();
          break;
      }
    }
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [deps, focusedIndex, moveFocus]);

  return { focusedIndex, setFocusedIndex };
}
