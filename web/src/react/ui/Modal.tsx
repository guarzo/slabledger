import { type ReactNode, type RefObject, useEffect, useRef } from 'react';
import { Dialog } from 'radix-ui';

export type ModalSize = 'sm' | 'md' | 'lg';

export interface ModalProps {
  /** Accessible name for the dialog. Rendered as the visible heading. */
  title: string;
  /** Called when the user dismisses via Escape, an outside click, or the close control. */
  onClose: () => void;
  /**
   * Suppresses every dismissal path while an action is in flight, so a pending
   * save cannot be abandoned halfway. Mirrors the `saving` / `isSubmitting`
   * guards the hand-rolled dialogs carried individually.
   */
  busy?: boolean;
  size?: ModalSize;
  /** Screen-reader-only description, wired to aria-describedby. */
  description?: string;
  /** Renders a ✕ control in the header. Off by default: most dialogs close via their own Cancel button. */
  showClose?: boolean;
  /**
   * Focus this element on open instead of the panel's first tabbable node.
   * For dialogs whose primary field sits below other controls.
   */
  initialFocusRef?: RefObject<HTMLElement | null>;
  /** Caps the panel height and scrolls the body under a sticky header. For long content. */
  scrollable?: boolean;
  children: ReactNode;
}

const SIZES: Record<ModalSize, string> = {
  sm: 'max-w-sm',
  md: 'max-w-md',
  lg: 'max-w-3xl',
};

/**
 * Centered modal dialog built on Radix Dialog.
 *
 * Radix supplies most of what was hand-rolled per dialog: `role="dialog"`,
 * focus into the panel on open, and a Tab loop via FocusScope. Two gaps are
 * closed here rather than at each call site:
 *
 * 1. Radix Dialog does not emit `aria-modal`; it relies on `aria-hidden` on
 *    sibling nodes. Every dialog this primitive replaces set `aria-modal`
 *    itself, so it is set explicitly to preserve that behaviour.
 * 2. `DialogContentModal` preventDefaults FocusScope's restore and focuses
 *    `Dialog.Trigger` instead. These dialogs are mounted conditionally from a
 *    row action, with no `Dialog.Trigger` in the tree, so that ref is null and
 *    focus would silently fall to `<body>` on close. `onCloseAutoFocus` below
 *    restores to whatever was focused when the dialog mounted.
 *
 * The capture is mount-only on purpose. Re-reading `document.activeElement` on
 * a later render would capture a node *inside* the panel — the failure mode
 * SLA-98 hit — so a save in flight cannot corrupt the restore target.
 *
 * `open` is intentionally not a prop. Call sites mount this conditionally and
 * Radix stays controlled-open, so dismissal always routes through `onClose`
 * and the `busy` guard.
 *
 * Destructive or high-stakes confirmations should use ConfirmDialog instead —
 * it renders role="alertdialog", which screen readers announce as an interrupt.
 */
export default function Modal({
  title,
  onClose,
  busy = false,
  size = 'md',
  description,
  showClose = false,
  initialFocusRef,
  scrollable = false,
  children,
}: ModalProps) {
  const restoreFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    restoreFocusRef.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    // Empty deps: capture once, on mount. See the note above.
  }, []);

  return (
    <Dialog.Root
      open
      onOpenChange={(isOpen) => {
        if (!isOpen && !busy) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          aria-modal="true"
          onOpenAutoFocus={(event) => {
            if (!initialFocusRef?.current) return;
            event.preventDefault();
            initialFocusRef.current.focus();
          }}
          onCloseAutoFocus={(event) => {
            // Owns the restore outright: preventDefault stops both FocusScope's
            // fallback and Radix's own trigger-focus, which would be a no-op here.
            event.preventDefault();
            restoreFocusRef.current?.focus();
          }}
          // Radix auto-wires aria-describedby when a Description is rendered, and
          // warns when one is missing. Opt out explicitly for the undescribed case
          // rather than forcing every dialog to invent prose.
          {...(description ? {} : { 'aria-describedby': undefined })}
          className={[
            'fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2',
            'bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl shadow-xl',
            'w-[calc(100%-2rem)]',
            SIZES[size],
            scrollable ? 'max-h-[80vh] overflow-auto' : 'p-6',
            'data-[state=open]:animate-[scaleIn_150ms_ease-out]',
          ].join(' ')}
        >
          <div
            className={
              scrollable
                ? 'flex items-center justify-between gap-4 p-4 border-b border-[var(--surface-2)] sticky top-0 bg-[var(--surface-1)]'
                : 'flex items-center justify-between gap-4 mb-4'
            }
          >
            <Dialog.Title className="text-lg font-semibold text-[var(--text)]">
              {title}
            </Dialog.Title>
            {showClose && (
              <Dialog.Close asChild>
                <button
                  type="button"
                  aria-label="Close"
                  disabled={busy}
                  className="text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-50"
                >
                  ✕
                </button>
              </Dialog.Close>
            )}
          </div>

          {description && <Dialog.Description className="sr-only">{description}</Dialog.Description>}

          <div className={scrollable ? 'p-4' : undefined}>{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
