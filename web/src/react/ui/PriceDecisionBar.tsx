import { useState, useEffect } from 'react';
import { Button } from '../ui';
import { formatCents, dollarsToCents, centsToDollars } from '../utils/formatters';
import type { PreSelection } from './priceDecisionHelpers';

export interface PriceSource {
  label: string;
  priceCents: number;
  source: string;
}

export interface PriceDecisionBarProps {
  sources: PriceSource[];
  preSelected?: PreSelection;
  onConfirm: (priceCents: number, source: string) => void;
  onSkip?: () => void;
  onFlag?: () => void;
  status?: 'pending' | 'accepted' | 'skipped';
  disabled?: boolean;
  isSubmitting?: boolean;
  confirmLabel?: string;
  onReset?: () => void;
  /** Price to display in accepted state when set externally (e.g. Accept All). */
  acceptedPriceCents?: number;
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
  acceptedPriceCents,
}: PriceDecisionBarProps) {
  const [selectedSource, setSelectedSource] = useState<string | null>(null);
  const [customValue, setCustomValue] = useState('');
  const [lastConfirmedCents, setLastConfirmedCents] = useState(0);

  useEffect(() => {
    if (!preSelected) return;
    if (preSelected.kind === 'source') {
      const match = sources.find(s => s.source === preSelected.source && s.priceCents > 0);
      if (match) {
        setSelectedSource(match.source);
        setCustomValue(centsToDollars(match.priceCents));
      }
    } else if (preSelected.kind === 'manual') {
      setSelectedSource(null);
      setCustomValue(centsToDollars(preSelected.priceCents));
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
      setLastConfirmedCents(values.priceCents);
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

  if (status === 'accepted') {
    const displayCents = acceptedPriceCents || lastConfirmedCents || (getConfirmValues()?.priceCents ?? 0);
    return (
      <div className="flex items-center gap-3 flex-wrap opacity-60">
        <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>
        {sources.map(src => (
          <button
            key={src.source}
            type="button"
            disabled
            aria-pressed={selectedSource === src.source}
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

  return (
    <div className="flex items-center gap-3 flex-wrap">
      <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>

      {sources.map(src => (
        <button
          key={src.source}
          type="button"
          onClick={() => handleSourceClick(src)}
          disabled={allDisabled || src.priceCents === 0}
          aria-pressed={selectedSource === src.source}
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
          aria-label="Custom price in dollars"
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
