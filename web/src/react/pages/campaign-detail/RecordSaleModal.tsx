import { Dialog } from 'radix-ui';
import type { AgingItem } from '../../../types/campaigns';
import RecordSaleForm from './RecordSaleForm';

interface RecordSaleModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  items: [AgingItem];
}

export default function RecordSaleModal({ open, onClose, onSuccess, items }: RecordSaleModalProps) {
  const item = items[0];
  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed right-0 top-0 bottom-0 z-50 w-[min(520px,calc(100%-2rem))] bg-[var(--surface-1)] border-l border-[var(--surface-2)] p-6 shadow-2xl data-[state=open]:animate-[slideInFromRight_200ms_cubic-bezier(0.4,0,0.2,1)] overflow-y-auto"
        >
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-4">
            Record Sale
          </Dialog.Title>
          <Dialog.Description className="sr-only">
            Enter sale details including channel, date, and price
          </Dialog.Description>
          <RecordSaleForm
            item={item}
            onSuccess={() => { onSuccess?.(); onClose(); }}
            onCancel={onClose}
          />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
