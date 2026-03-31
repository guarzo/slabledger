import { useState, useEffect } from 'react';
import { Button } from '../ui';
import { formatCents, dollarsToCents, centsToDollars } from '../utils/formatters';

export interface PriceSource {
  label: string;
  priceCents: number;
  source: string;
}

export interface PriceDecisionBarProps {
  sources: PriceSource[];
  preSelected?: string;
  onConfirm: (priceCents: number, source: string) => void;
  onSkip?: () => void;
  onFlag?: () => void;
  status?: 'pending' | 'accepted' | 'skipped';
  disabled?: boolean;
  isSubmitting?: boolean;
  confirmLabel?: string;
  onReset?: () => void;
}

export default function PriceDecisionBar({
  sources,
  preSelected,
  onConfirm,
  onSkip,
  onFlag,
  status = 'pending',
  disabled = false,
  isSubmitting = false,
  confirmLabel = 'Confirm',
  onReset,
}: PriceDecisionBarProps) {
  const [selectedSource, setSelectedSource] = useState<string | null>(null);
  const [customValue, setCustomValue] = useState('');

  useEffect(() => {
    if (preSelected) {
      const match = sources.find(s => s.source === preSelected && s.priceCents > 0);
      if (match) {
        setSelectedSource(match.source);
        setCustomValue(centsToDollars(match.priceCents));
      }
    }
  }, [preSelected, sources]);

  const handleSourceClick = (src: PriceSource) => {
    setSelectedSource(src.source);
    setCustomValue(centsToDollars(src.priceCents));
  };

  const handleCustomChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setCustomValue(e.target.value);
    setSelectedSource(null);
  };

  const getConfirmValues = (): { priceCents: number; source: string } | null => {
    if (selectedSource) {
      const match = sources.find(s => s.source === selectedSource);
      if (match && match.priceCents > 0) {
        return { priceCents: match.priceCents, source: match.source };
      }
    }
    const cents = dollarsToCents(customValue);
    if (cents > 0) {
      return { priceCents: cents, source: 'manual' };
    }
    return null;
  };

  const handleConfirm = () => {
    const values = getConfirmValues();
    if (values) {
      onConfirm(values.priceCents, values.source);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      handleConfirm();
    }
  };

  const hasSelection = selectedSource !== null || (customValue !== '' && dollarsToCents(customValue) > 0);
  const allDisabled = disabled || isSubmitting;

  // --- Accepted state ---
  if (status === 'accepted') {
    const values = getConfirmValues();
    const displayCents = values?.priceCents ?? 0;
    return (
      <div className="flex items-center gap-3 flex-wrap opacity-60">
        <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>
        {sources.map(src => (
          <button
            key={src.source}
            type="button"
            disabled
            className={`text-xs px-2.5 py-1.5 rounded-md border transition-colors ${
              selectedSource === src.source
                ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
                : 'border-[var(--border)] text-[var(--text-muted)]'
            } disabled:cursor-not-allowed`}
          >
            {src.label} {src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}
          </button>
        ))}
        <span className="text-xs px-2.5 py-1.5 rounded-md bg-[var(--success)]/15 text-[var(--success)] font-medium border border-[var(--success)]/30">
          &#10003; {formatCents(displayCents)}
        </span>
        {onReset && (
          <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled}>
            Change
          </Button>
        )}
      </div>
    );
  }

  // --- Skipped state ---
  if (status === 'skipped') {
    return (
      <div className="flex items-center gap-3 flex-wrap opacity-50">
        <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>
        {sources.map(src => (
          <button key={src.source} type="button" disabled
            className="text-xs px-2.5 py-1.5 rounded-md border border-[var(--border)] text-[var(--text-muted)] disabled:cursor-not-allowed">
            {src.label} {src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}
          </button>
        ))}
        <span className="text-xs text-[var(--text-muted)] italic">Skipped</span>
        {onReset && (
          <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled}>
            Undo
          </Button>
        )}
      </div>
    );
  }

  // --- Pending state (default) ---
  return (
    <div className="flex items-center gap-3 flex-wrap">
      <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>

      {sources.map(src => (
        <button
          key={src.source}
          type="button"
          onClick={() => handleSourceClick(src)}
          disabled={allDisabled || src.priceCents === 0}
          className={`text-xs px-2.5 py-1.5 rounded-md border transition-colors ${
            selectedSource === src.source
              ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
              : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
          } disabled:opacity-40 disabled:cursor-not-allowed`}
        >
          {src.label} {src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}
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
          onKeyDown={handleKeyDown}
          disabled={allDisabled}
          className="w-20 px-2 py-1.5 text-xs rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)] disabled:opacity-40"
        />
      </div>

      <Button
        variant="success"
        size="sm"
        onClick={handleConfirm}
        disabled={!hasSelection || allDisabled}
        loading={isSubmitting}
      >
        {confirmLabel}
      </Button>

      {onSkip && (
        <Button variant="ghost" size="sm" onClick={onSkip} disabled={allDisabled}>
          Skip
        </Button>
      )}

      {onFlag && (
        <div className="ml-auto">
          <Button variant="danger" size="sm" onClick={onFlag} disabled={allDisabled}>
            Flag Price Issue
          </Button>
        </div>
      )}
    </div>
  );
}
