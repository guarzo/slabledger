import { useEffect, useRef, useState } from 'react';
import { formatCents, centsToDollars, dollarsToCents } from '../../../utils/formatters';

interface InlinePriceEditProps {
  purchaseId: string;
  currentCents: number;
  costBasisCents: number;
  onSave: (purchaseId: string, priceCents: number) => Promise<void>;
}

export default function InlinePriceEdit({ purchaseId, currentCents, costBasisCents, onSave }: InlinePriceEditProps) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState('');
  const [saving, setSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (editing) inputRef.current?.select();
  }, [editing]);

  const beginEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    setValue(currentCents > 0 ? centsToDollars(currentCents) : '');
    setEditing(true);
  };

  const cancel = () => {
    setEditing(false);
    setValue('');
  };

  const commit = async () => {
    const cents = dollarsToCents(value);
    if (cents <= 0 || cents === currentCents) {
      cancel();
      return;
    }
    setSaving(true);
    try {
      await onSave(purchaseId, cents);
      setEditing(false);
      setValue('');
    } catch {
      // toast handled upstream; keep editing mode so user can retry
    } finally {
      setSaving(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      void commit();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      cancel();
    }
  };

  // Live P/L delta preview while editing.
  const previewCents = dollarsToCents(value);
  const plPreview = previewCents > 0 && costBasisCents > 0 ? previewCents - costBasisCents : null;

  if (editing) {
    return (
      <div className="flex flex-col items-end gap-[1px]" onClick={e => e.stopPropagation()}>
        <input
          ref={inputRef}
          type="text"
          inputMode="decimal"
          value={value}
          onChange={e => setValue(e.target.value)}
          onKeyDown={handleKeyDown}
          onBlur={() => void commit()}
          disabled={saving}
          className="w-20 text-right tabular-nums text-sm bg-[var(--surface-2)] border border-[var(--brand-500)] rounded px-1.5 py-0.5 text-[var(--text)] focus:outline-none"
          aria-label="List price"
          placeholder="0.00"
        />
        {plPreview != null && (
          <span className={`text-[10px] tabular-nums leading-none ${plPreview >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
            {plPreview >= 0 ? '+' : ''}{formatCents(plPreview)}
          </span>
        )}
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={beginEdit}
      className="text-right tabular-nums text-[var(--text)] hover:bg-[rgba(255,255,255,0.04)] rounded px-1 py-0.5 -mx-1 transition-colors"
      title="Click to edit list price"
    >
      {currentCents > 0 ? formatCents(currentCents) : <span className="text-[var(--text-muted)]">set price</span>}
    </button>
  );
}
