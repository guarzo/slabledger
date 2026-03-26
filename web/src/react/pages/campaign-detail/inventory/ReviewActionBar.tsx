import { useState } from 'react';
import { Button } from '../../../ui';
import { formatCents, dollarsToCents } from '../../../utils/formatters';

interface QuickPick {
  label: string;
  priceCents: number;
  source: string;
}

interface ReviewActionBarProps {
  quickPicks: QuickPick[];
  onConfirm: (priceCents: number, source: string) => void;
  onFlag: () => void;
  isSubmitting?: boolean;
}

export default function ReviewActionBar({ quickPicks, onConfirm, onFlag, isSubmitting }: ReviewActionBarProps) {
  const [customValue, setCustomValue] = useState('');
  const [selectedPick, setSelectedPick] = useState<QuickPick | null>(null);

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
    setSelectedPick(pick);
    setCustomValue('');
  };

  const handleCustomChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setCustomValue(e.target.value);
    setSelectedPick(null);
  };

  const hasSelection = selectedPick !== null || (customValue !== '' && dollarsToCents(customValue) > 0);
  const visiblePicks = quickPicks.filter(p => p.priceCents > 0);

  return (
    <div className="flex items-center gap-3 flex-wrap">
      <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>

      {visiblePicks.map(pick => (
        <button
          key={`${pick.source}-${pick.priceCents}`}
          type="button"
          onClick={() => handlePickClick(pick)}
          disabled={isSubmitting}
          className={`text-xs px-2.5 py-1.5 rounded-md border transition-colors ${
            selectedPick?.source === pick.source && selectedPick?.priceCents === pick.priceCents
              ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
              : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
          } disabled:opacity-40 disabled:cursor-not-allowed`}
        >
          {pick.label} {formatCents(pick.priceCents)}
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
