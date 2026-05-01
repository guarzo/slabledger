import { Dialog } from 'radix-ui';
import Button from '../../ui/Button';

interface RepriceShortcutSheetProps {
  open: boolean;
  onClose: () => void;
}

const bindings: ReadonlyArray<{ keys: string; action: string }> = [
  { keys: 'j / ↓',         action: 'Next row' },
  { keys: 'k / ↑',         action: 'Previous row' },
  { keys: 'Enter',         action: 'Accept current row' },
  { keys: 'Space',         action: 'Toggle selection on current row' },
  { keys: 'Esc',           action: 'Blur input, clear focus, deselect (cascade)' },
  { keys: '/',             action: 'Focus first price input' },
  { keys: '?',             action: 'Show this sheet' },
  { keys: '⌘/Ctrl+Enter',  action: 'Open Apply confirm' },
];

export default function RepriceShortcutSheet({ open, onClose }: RepriceShortcutSheetProps) {
  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2 bg-[var(--surface-1)] border border-[var(--surface-2)] rounded-xl p-6 max-w-md w-[calc(100%-2rem)] shadow-xl data-[state=open]:animate-[scaleIn_150ms_ease-out]">
          <Dialog.Title className="text-lg font-semibold text-[var(--text)] mb-4">
            Keyboard shortcuts
          </Dialog.Title>
          <Dialog.Description className="sr-only">
            Key bindings for the Reprice page.
          </Dialog.Description>
          <table className="w-full text-sm">
            <tbody>
              {bindings.map(b => (
                <tr key={b.keys} className="border-b border-[var(--surface-2)] last:border-b-0">
                  <td className="py-2 pr-4 font-mono text-xs text-[var(--text)] whitespace-nowrap">{b.keys}</td>
                  <td className="py-2 text-[var(--text-muted)]">{b.action}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="flex justify-end mt-4">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm">Close</Button>
            </Dialog.Close>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
