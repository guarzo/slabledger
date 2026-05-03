import { type ReactNode } from 'react';
import { AlertDialog } from 'radix-ui';
import Button from './Button';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'primary';
  loading?: boolean;
  disabled?: boolean;
  children?: ReactNode;
  onConfirm: () => void;
  onCancel: () => void;
}

// Uses Radix AlertDialog (role=alertdialog) so screen readers announce this as
// an interrupt that demands a decision, and so focus is trapped until the user
// chooses. Destructive confirms (Delete, etc.) and high-stakes confirms should
// always use this rather than a plain Dialog.
export default function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'danger',
  loading = false,
  disabled = false,
  children,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <AlertDialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen && !loading) onCancel(); }}>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <AlertDialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-sm w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out]"
          onOpenAutoFocus={(e) => {
            const cancel = (e.target as HTMLElement).querySelector<HTMLButtonElement>('[data-cancel]');
            if (cancel && !cancel.disabled) {
              e.preventDefault();
              cancel.focus();
            }
          }}
        >
          <AlertDialog.Title className="text-lg font-semibold text-[var(--text)] mb-2">
            {title}
          </AlertDialog.Title>
          <AlertDialog.Description className="text-sm text-[var(--text-muted)] mb-4">
            {message}
          </AlertDialog.Description>
          {children}
          <div className="flex justify-end gap-3 mt-6">
            <AlertDialog.Cancel asChild>
              <Button data-cancel variant="ghost" size="sm" disabled={loading}>
                {cancelLabel}
              </Button>
            </AlertDialog.Cancel>
            <AlertDialog.Action asChild>
              <Button variant={variant} size="sm" onClick={onConfirm} loading={loading} disabled={disabled}>
                {confirmLabel}
              </Button>
            </AlertDialog.Action>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}
