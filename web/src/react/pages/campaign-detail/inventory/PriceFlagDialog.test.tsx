import { describe, it, expect, vi } from 'vitest';
import { useState } from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import PriceFlagDialog from './PriceFlagDialog';

function renderDialog(overrides: Partial<React.ComponentProps<typeof PriceFlagDialog>> = {}) {
  const onCancel = overrides.onCancel ?? vi.fn();
  const onSubmit = overrides.onSubmit ?? vi.fn();
  render(
    <PriceFlagDialog
      cardName="Charizard"
      grade={10}
      onSubmit={onSubmit}
      onCancel={onCancel}
      {...overrides}
    />,
  );
  return { onCancel, onSubmit };
}

describe('PriceFlagDialog accessibility', () => {
  it('exposes dialog semantics labelled by its heading', () => {
    renderDialog();
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Flag Price Issue');
  });

  it('closes on Escape', () => {
    const { onCancel } = renderDialog();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('does not close on Escape while a flag is in flight', () => {
    const { onCancel } = renderDialog({ isSubmitting: true });
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('moves focus into the dialog on mount', () => {
    renderDialog();
    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true);
  });

  it('restores focus to the invoking trigger on unmount', async () => {
    // The dialog is rendered conditionally next to its trigger, mirroring
    // InventoryTab's `{flagTarget && <PriceFlagDialog .../>}`.
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Flag
          </button>
          {open && (
            <PriceFlagDialog
              cardName="Charizard"
              grade={10}
              onSubmit={vi.fn()}
              onCancel={() => setOpen(false)}
            />
          )}
        </>
      );
    }
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Flag' });
    trigger.focus();
    fireEvent.click(trigger);
    expect(trigger).not.toHaveFocus();

    fireEvent.keyDown(document, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    // Radix restores focus from a post-unmount task, so this settles async.
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  // Tab trapping is Radix FocusScope's job and depends on real browser tab
  // sequencing plus focus sentinels, neither of which jsdom implements.
  // Covered instead by the @a11y Playwright spec in tests/e2e/accessibility.spec.ts.
});
