import { useEffect, useRef, useState } from 'react';
import { formatCents, centsToDollars, dollarsToCents } from '../../../utils/formatters';

interface InlinePriceEditProps {
  purchaseId: string;
  currentCents: number;
  costBasisCents: number;
  onSave: (purchaseId: string, priceCents: number) => Promise<void>;
  /** Optional: called after a successful save so the parent can flash the row. */
  onSaveComplete?: () => void;
}

/**
 * Inline price editor for the row's List/Rec column.
 *
 * Commit semantics: Enter saves; Escape reverts; blur commits **only when
 * the input has a valid, positive value that differs from the current price**.
 * Anything else (empty, zero, unchanged) cancels silently. This kills the old
 * silent-no-op behavior where blurring a typo-then-corrected field would
 * still flash a "saved" toast.
 */
export default function InlinePriceEdit({
  purchaseId,
  currentCents,
  costBasisCents,
  onSave,
  onSaveComplete,
}: InlinePriceEditProps) {
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
    if (saving) return;
    const cents = dollarsToCents(value);
    // Silent revert when nothing changed or input is invalid/zero.
    // Previously this still rendered the "Price saved" toast, which made
    // typos and accidental no-ops indistinguishable from real saves.
    if (cents <= 0 || cents === currentCents) {
      cancel();
      return;
    }
    setSaving(true);
    try {
      await onSave(purchaseId, cents);
      setEditing(false);
      setValue('');
      onSaveComplete?.();
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

  // Visible delta vs current price — answers "is this an actual change?"
  const dirty = previewCents > 0 && previewCents !== currentCents;

  if (editing) {
    return (
      <div className="flex flex-col items-end gap-[1px]" onClick={e => e.stopPropagation()}>
        <div className="flex items-center gap-1">
          <span className="text-[10px] text-[var(--text-muted)]" aria-hidden="true">$</span>
          <input
            ref={inputRef}
            type="text"
            inputMode="decimal"
            value={value}
            onChange={e => setValue(e.target.value)}
            onKeyDown={handleKeyDown}
            onBlur={() => void commit()}
            disabled={saving}
            className={`w-20 text-right tabular-nums text-sm bg-[var(--surface-2)] border rounded px-1.5 py-0.5 text-[var(--text)] focus:outline-none ${dirty ? 'border-[var(--brand-500)]' : 'border-[var(--surface-2)]'}`}
            aria-label="List price"
            placeholder="0.00"
          />
        </div>
        {plPreview != null && (
          <span className={`text-[10px] tabular-nums leading-none ${plPreview >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
            vs cost {plPreview >= 0 ? '+' : ''}{formatCents(plPreview)}
          </span>
        )}
        <span className="text-[10px] text-[var(--text-subtle)] leading-none">enter saves · esc cancels</span>
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={beginEdit}
      className="group text-right tabular-nums text-[var(--text)] hover:bg-[rgba(255,255,255,0.04)] rounded px-1 py-0.5 -mx-1 transition-colors inline-flex items-center gap-1 border border-dashed border-transparent hover:border-[var(--surface-2)]"
      title="Click to edit list price"
    >
      {currentCents > 0 ? formatCents(currentCents) : <span className="text-[var(--text-muted)]">set price</span>}
      <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true" className="opacity-0 group-hover:opacity-60 text-[var(--text-muted)] transition-opacity">
        <path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 0 0 0-1.41l-2.34-2.34a1 1 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z"/>
      </svg>
    </button>
  );
}
