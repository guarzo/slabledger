import { type ReactNode } from 'react';
import { Dialog } from 'radix-ui';
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
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen && !loading) onCancel(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/50 data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-sm w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out]"
          onOpenAutoFocus={(e) => {
            const cancel = (e.target as HTMLElement).querySelector<HTMLButtonElement>('[data-cancel]');
            if (cancel && !cancel.disabled) {
              e.preventDefault();
              cancel.focus();
            }
          }}
        >
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-2">
            {title}
          </Dialog.Title>
          <Dialog.Description className="text-sm text-[var(--text-muted)] mb-4">
            {message}
          </Dialog.Description>
          {children}
          <div className="flex justify-end gap-3 mt-6">
            <Dialog.Close asChild>
              <Button data-cancel variant="ghost" size="sm" disabled={loading}>
                {cancelLabel}
              </Button>
            </Dialog.Close>
            <Button variant={variant} size="sm" onClick={onConfirm} loading={loading} disabled={disabled}>
              {confirmLabel}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
