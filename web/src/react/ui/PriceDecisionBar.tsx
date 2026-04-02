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
  /** Source key to highlight as recommended (thicker border). */
  recommendedSource?: string;
  /** Cost basis in cents — shows live margin badge when set. */
  costBasisCents?: number;
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
  recommendedSource,
  costBasisCents,
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

  // Compute live margin from current selection vs cost basis
  const currentCents = dollarsToCents(customValue);
  const marginCents = costBasisCents != null && costBasisCents > 0 && currentCents > 0
    ? currentCents - costBasisCents : null;

  const marginBadge = marginCents != null ? (
    <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
      marginCents >= 0
        ? 'text-[var(--success)] bg-[var(--success)]/10'
        : 'text-red-400 bg-red-400/10'
    }`}>
      {marginCents >= 0 ? '+' : ''}{formatCents(marginCents)} margin
    </span>
  ) : null;

  const pillClass = (src: PriceSource, isDisabled: boolean) => {
    const isSelected = selectedSource === src.source;
    const isRecommended = recommendedSource === src.source;
    const base = 'text-xs rounded-md border transition-colors flex flex-col items-center min-w-[68px] px-2 py-1.5 disabled:opacity-40 disabled:cursor-not-allowed';
    if (isSelected) return `${base} border-[var(--accent)] ${isRecommended ? 'border-2' : ''} bg-[var(--accent)]/10 text-[var(--accent)]`;
    if (isRecommended && !isDisabled) return `${base} border-2 border-[var(--accent)]/50 text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)]`;
    return `${base} border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]`;
  };

  if (status === 'accepted') {
    const displayCents = acceptedPriceCents || lastConfirmedCents || (getConfirmValues()?.priceCents ?? 0);
    const acceptedMargin = costBasisCents != null && costBasisCents > 0 && displayCents > 0
      ? displayCents - costBasisCents : null;
    return (
      <div className="flex items-center gap-3 flex-wrap">
        <span className="text-xs px-2.5 py-1.5 rounded-md bg-[var(--success)]/15 text-[var(--success)] font-semibold border border-[var(--success)]/30">
          &#10003; {formatCents(displayCents)}
        </span>
        {acceptedMargin != null && (
          <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
            acceptedMargin >= 0
              ? 'text-[var(--success)] bg-[var(--success)]/10'
              : 'text-red-400 bg-red-400/10'
          }`}>
            {acceptedMargin >= 0 ? '+' : ''}{formatCents(acceptedMargin)} margin
          </span>
        )}
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
    <div className="flex items-center gap-2.5 flex-wrap">
      <span className="text-xs font-medium text-[var(--text-muted)] whitespace-nowrap">Set Price:</span>

      {sources.map(src => (
        <button
          key={src.source}
          type="button"
          onClick={() => handleSourceClick(src)}
          disabled={allDisabled || src.priceCents === 0}
          aria-pressed={selectedSource === src.source}
          className={pillClass(src, allDisabled || src.priceCents === 0)}
        >
          <span className="leading-none text-[9px] uppercase tracking-wide opacity-70">{src.label}</span>
          <span className="leading-tight font-semibold text-xs">{src.priceCents > 0 ? formatCents(src.priceCents) : '\u2014'}</span>
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

      {marginBadge}

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
