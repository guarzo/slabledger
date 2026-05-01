import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest';
import { useRepriceKeyboard } from './useRepriceKeyboard';

function dispatchKey(key: string, opts: KeyboardEventInit = {}) {
  document.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...opts }));
}

describe('useRepriceKeyboard', () => {
  let onAcceptFocused: Mock<(index: number) => void>;
  let onToggleFocused: Mock<(index: number) => void>;
  let onJumpToInput: Mock<() => void>;
  let onShowShortcuts: Mock<() => void>;
  let onSubmit: Mock<() => void>;
  let onDeselectAll: Mock<() => void>;

  beforeEach(() => {
    onAcceptFocused = vi.fn<(index: number) => void>();
    onToggleFocused = vi.fn<(index: number) => void>();
    onJumpToInput = vi.fn<() => void>();
    onShowShortcuts = vi.fn<() => void>();
    onSubmit = vi.fn<() => void>();
    onDeselectAll = vi.fn<() => void>();
  });

  function setup(itemCount = 5, selectedCount = 0, isModalOpen = false) {
    return renderHook(() =>
      useRepriceKeyboard({
        itemCount,
        selectedCount,
        isModalOpen,
        onAcceptFocused,
        onToggleFocused,
        onJumpToInput,
        onShowShortcuts,
        onSubmit,
        onDeselectAll,
      })
    );
  }

  it('starts with focusedIndex null', () => {
    const { result } = setup();
    expect(result.current.focusedIndex).toBeNull();
  });

  it('j moves focus forward; first j sets index 0', () => {
    const { result } = setup();
    act(() => dispatchKey('j'));
    expect(result.current.focusedIndex).toBe(0);
    act(() => dispatchKey('j'));
    expect(result.current.focusedIndex).toBe(1);
  });

  it('k moves focus backward; clamps at 0', () => {
    const { result } = setup();
    act(() => result.current.setFocusedIndex(2));
    act(() => dispatchKey('k'));
    expect(result.current.focusedIndex).toBe(1);
    act(() => dispatchKey('k'));
    act(() => dispatchKey('k'));
    expect(result.current.focusedIndex).toBe(0);
  });

  it('ArrowDown / ArrowUp behave like j / k', () => {
    const { result } = setup();
    act(() => dispatchKey('ArrowDown'));
    expect(result.current.focusedIndex).toBe(0);
    act(() => dispatchKey('ArrowDown'));
    expect(result.current.focusedIndex).toBe(1);
    act(() => dispatchKey('ArrowUp'));
    expect(result.current.focusedIndex).toBe(0);
  });

  it('j clamps at itemCount - 1', () => {
    const { result } = setup(3);
    act(() => result.current.setFocusedIndex(2));
    act(() => dispatchKey('j'));
    expect(result.current.focusedIndex).toBe(2);
  });

  it('Enter calls onAcceptFocused with current index', () => {
    const { result } = setup();
    act(() => result.current.setFocusedIndex(2));
    act(() => dispatchKey('Enter'));
    expect(onAcceptFocused).toHaveBeenCalledWith(2);
  });

  it('Enter is no-op when focusedIndex is null', () => {
    setup();
    act(() => dispatchKey('Enter'));
    expect(onAcceptFocused).not.toHaveBeenCalled();
  });

  it('Space calls onToggleFocused with current index', () => {
    const { result } = setup();
    act(() => result.current.setFocusedIndex(1));
    act(() => dispatchKey(' '));
    expect(onToggleFocused).toHaveBeenCalledWith(1);
  });

  it('? calls onShowShortcuts', () => {
    setup();
    act(() => dispatchKey('?'));
    expect(onShowShortcuts).toHaveBeenCalledTimes(1);
  });

  it('/ calls onJumpToInput', () => {
    setup();
    act(() => dispatchKey('/'));
    expect(onJumpToInput).toHaveBeenCalledTimes(1);
  });

  it('Cmd+Enter calls onSubmit', () => {
    setup();
    act(() => dispatchKey('Enter', { metaKey: true }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onAcceptFocused).not.toHaveBeenCalled();
  });

  it('Ctrl+Enter calls onSubmit (cross-OS)', () => {
    setup();
    act(() => dispatchKey('Enter', { ctrlKey: true }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it('Esc cascade: clears focus when focused', () => {
    const { result } = setup(5, 0);
    act(() => result.current.setFocusedIndex(2));
    act(() => dispatchKey('Escape'));
    expect(result.current.focusedIndex).toBeNull();
    expect(onDeselectAll).not.toHaveBeenCalled();
  });

  it('Esc cascade: deselects all when no focus and selection exists', () => {
    setup(5, 3);
    act(() => dispatchKey('Escape'));
    expect(onDeselectAll).toHaveBeenCalledTimes(1);
  });

  it('Esc cascade: no-op when no focus and no selection', () => {
    setup(5, 0);
    act(() => dispatchKey('Escape'));
    expect(onDeselectAll).not.toHaveBeenCalled();
  });

  it('Ignores typing keys while an input is focused (e.g. j inside an input)', () => {
    const { result } = setup();
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => dispatchKey('j'));
    expect(result.current.focusedIndex).toBeNull();
    document.body.removeChild(input);
  });

  it('Allows j/k navigation when a checkbox is focused', () => {
    const { result } = setup();
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    document.body.appendChild(checkbox);
    checkbox.focus();
    act(() => dispatchKey('j'));
    expect(result.current.focusedIndex).toBe(0);
    document.body.removeChild(checkbox);
  });

  it('Esc inside an input blurs the input and does NOT cascade to clear focus', () => {
    const { result } = setup(5, 0);
    act(() => result.current.setFocusedIndex(2));
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => dispatchKey('Escape'));
    // Input is blurred (focus moves off) but focusedIndex remains
    expect(document.activeElement).not.toBe(input);
    expect(result.current.focusedIndex).toBe(2);
    document.body.removeChild(input);
  });

  it('Enter inside an input still triggers onAcceptFocused (after blur)', () => {
    const { result } = setup();
    act(() => result.current.setFocusedIndex(0));
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    act(() => dispatchKey('Enter'));
    expect(onAcceptFocused).toHaveBeenCalledWith(0);
    document.body.removeChild(input);
  });

  it('Cleans up the document keydown listener on unmount', () => {
    const removeSpy = vi.spyOn(document, 'removeEventListener');
    const { unmount } = setup();
    unmount();
    expect(removeSpy).toHaveBeenCalledWith('keydown', expect.any(Function));
    removeSpy.mockRestore();
  });

  it('Resets focusedIndex when itemCount shrinks below current focus', () => {
    const { result, rerender } = renderHook(
      ({ itemCount }) =>
        useRepriceKeyboard({
          itemCount,
          selectedCount: 0,
          isModalOpen: false,
          onAcceptFocused,
          onToggleFocused,
          onJumpToInput,
          onShowShortcuts,
          onSubmit,
          onDeselectAll,
        }),
      { initialProps: { itemCount: 5 } }
    );
    act(() => result.current.setFocusedIndex(4));
    rerender({ itemCount: 2 });
    expect(result.current.focusedIndex).toBe(1);
  });

  it('Skips all keystrokes when isModalOpen is true', () => {
    const { result } = setup(5, 3, true);
    act(() => dispatchKey('j'));
    expect(result.current.focusedIndex).toBeNull();
    act(() => dispatchKey('Enter'));
    expect(onAcceptFocused).not.toHaveBeenCalled();
    act(() => dispatchKey('Escape'));
    expect(onDeselectAll).not.toHaveBeenCalled();
    act(() => dispatchKey('?'));
    expect(onShowShortcuts).not.toHaveBeenCalled();
  });
});
