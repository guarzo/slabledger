import { useState, useEffect } from 'react';
import { Button } from '../../../ui';
import { formatCents, dollarsToCents, centsToDollars } from '../../../utils/formatters';

export interface QuickPick {
  label: string;
  priceCents: number;
  source: string;
}

interface ReviewActionBarProps {
  quickPicks: QuickPick[];
  selectedPick: QuickPick | null;
  onPickSelect: (pick: QuickPick | null) => void;
  onConfirm: (priceCents: number, source: string) => void;
  onFlag: () => void;
  isSubmitting?: boolean;
}

export default function ReviewActionBar({ quickPicks, selectedPick, onPickSelect, onConfirm, onFlag, isSubmitting }: ReviewActionBarProps) {
  const [customValue, setCustomValue] = useState('');

  useEffect(() => {
    if (selectedPick) {
      setCustomValue(centsToDollars(selectedPick.priceCents));
    }
  }, [selectedPick]);

  const handleConfirm = () => {
    if (selectedPick) {
      onConfirm(selectedPick.priceCents, selectedPick.source);
      return;
    }
    const cents = dollarsToCents(customValue);
    if (cents > 0) {
      onConfirm(cents, 'manual');
    }
  };

  const handlePickClick = (pick: QuickPick) => {
    onPickSelect(pick);
  };

  const handleCustomChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setCustomValue(e.target.value);
    onPickSelect(null);
  };

  const hasSelection = selectedPick !== null || (customValue !== '' && dollarsToCents(customValue) > 0);

  return (
    <div className="flex items-center gap-3 flex-wrap">
      <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>

      {quickPicks.map(pick => (
        <button
          key={pick.source}
          type="button"
          onClick={() => handlePickClick(pick)}
          disabled={isSubmitting || pick.priceCents === 0}
          className={`text-xs px-2.5 py-1.5 rounded-md border transition-colors ${
            selectedPick?.source === pick.source
              ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
              : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
          } disabled:opacity-40 disabled:cursor-not-allowed`}
        >
          {pick.label} {pick.priceCents > 0 ? formatCents(pick.priceCents) : '\u2014'}
        </button>
      ))}

      <div className="flex items-center gap-1.5">
        <span className="text-[var(--text-muted)] text-xs">$</span>
        <input
          type="text"
          inputMode="decimal"
          placeholder="0.00"
          value={customValue}
          onChange={handleCustomChange}
          disabled={isSubmitting}
          className="w-20 px-2 py-1.5 text-xs rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)] disabled:opacity-40"
        />
      </div>

      <Button
        variant="success"
        size="sm"
        onClick={handleConfirm}
        disabled={!hasSelection || isSubmitting}
        loading={isSubmitting}
      >
        Confirm
      </Button>

      <div className="ml-auto">
        <Button
          variant="danger"
          size="sm"
          onClick={onFlag}
          disabled={isSubmitting}
        >
          Flag Price Issue
        </Button>
      </div>
    </div>
  );
}
