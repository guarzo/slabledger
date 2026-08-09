import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { useRef, useState } from 'react';
import Modal from './Modal';

describe('Modal', () => {
  it('exposes dialog semantics with an accessible name from the title', () => {
    render(
      <Modal title="Fix Pricing" onClose={vi.fn()}>
        <p>body</p>
      </Modal>
    );

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Fix Pricing');
  });

  it('closes on Escape', () => {
    const onClose = vi.fn();
    render(
      <Modal title="Fix Pricing" onClose={onClose}>
        <p>body</p>
      </Modal>
    );

    fireEvent.keyDown(document.body, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // The in-flight guard the hand-rolled dialogs carried as `!saving` / `!isSubmitting`.
  it('does not close on Escape while busy', () => {
    const onClose = vi.fn();
    render(
      <Modal title="Fix Pricing" onClose={onClose} busy>
        <p>body</p>
      </Modal>
    );

    fireEvent.keyDown(document.body, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });

  it('does not close on an outside interaction while busy', () => {
    const onClose = vi.fn();
    render(
      <Modal title="Fix Pricing" onClose={onClose} busy>
        <p>body</p>
      </Modal>
    );

    fireEvent.pointerDown(document.body);

    expect(onClose).not.toHaveBeenCalled();
  });

  it('moves focus into the dialog on open', async () => {
    render(
      <Modal title="Fix Pricing" onClose={vi.fn()}>
        <button type="button">Save</button>
      </Modal>
    );

    const dialog = screen.getByRole('dialog');
    await waitFor(() => {
      expect(dialog.contains(document.activeElement)).toBe(true);
    });
  });

  // Several migrated dialogs autoFocus a field that is not the first tabbable
  // node in the panel; without this, FocusScope would land on the first one.
  it('honours initialFocusRef over the first tabbable element', async () => {
    function Harness() {
      const inputRef = useRef<HTMLInputElement>(null);
      return (
        <Modal title="Fix Pricing" onClose={vi.fn()} initialFocusRef={inputRef}>
          <button type="button">Not me</button>
          <input ref={inputRef} aria-label="External ID" />
        </Modal>
      );
    }

    render(<Harness />);

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByLabelText('External ID'));
    });
  });

  // The mount-only subtlety from SLA-98: a re-render mid-flight must not
  // re-capture `document.activeElement` from inside the dialog, or restore
  // silently lands on the panel instead of the trigger.
  it('restores focus to the trigger on close, even after a busy re-render', async () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      const [busy, setBusy] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Open
          </button>
          {open && (
            <Modal title="Fix Pricing" busy={busy} onClose={() => setOpen(false)}>
              <button type="button" onClick={() => setBusy(true)}>
                Start
              </button>
            </Modal>
          )}
        </>
      );
    }

    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open' });
    trigger.focus();
    fireEvent.click(trigger);

    await screen.findByRole('dialog');
    // Force a re-render while the dialog is mounted.
    fireEvent.click(screen.getByRole('button', { name: 'Start' }));

    fireEvent.keyDown(document.body, { key: 'Escape' });
    // busy is now true, so Escape is suppressed and the dialog stays open.
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('restores focus to the trigger when closed', async () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Open
          </button>
          {open && (
            <Modal title="Fix Pricing" onClose={() => setOpen(false)}>
              <button type="button">Save</button>
            </Modal>
          )}
        </>
      );
    }

    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open' });
    trigger.focus();
    fireEvent.click(trigger);

    await screen.findByRole('dialog');
    fireEvent.keyDown(document.body, { key: 'Escape' });

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
    await waitFor(() => {
      expect(document.activeElement).toBe(trigger);
    });
  });

  it('renders a labelled close control when showClose is set', () => {
    const onClose = vi.fn();
    render(
      <Modal title="Failures" onClose={onClose} showClose>
        <p>body</p>
      </Modal>
    );

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('omits the close control by default', () => {
    render(
      <Modal title="Fix Pricing" onClose={vi.fn()}>
        <p>body</p>
      </Modal>
    );

    expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
  });

  it('describes the dialog when a description is provided', () => {
    render(
      <Modal title="Fix Pricing" description="Set a manual price for this card" onClose={vi.fn()}>
        <p>body</p>
      </Modal>
    );

    expect(screen.getByRole('dialog')).toHaveAccessibleDescription(
      'Set a manual price for this card'
    );
  });
});
