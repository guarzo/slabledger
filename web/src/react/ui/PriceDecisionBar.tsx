import { useState, useEffect, useRef } from 'react';
import { Button } from '../ui';
import { formatCents, dollarsToCents, centsToDollars } from '../utils/formatters';
import { PricePill } from './PricePill';
import { MarginBadge } from './MarginBadge';
import styles from './PriceDecisionBar.module.css';
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

  const preSelectedKey = preSelected
    ? preSelected.kind === 'source'
      ? `source:${preSelected.source}`
      : preSelected.kind === 'manual'
        ? `manual:${preSelected.priceCents}`
        : 'none'
    : 'undefined';
  const appliedKeyRef = useRef<string | null>(null);

  useEffect(() => {
    if (!preSelected) return;
    if (appliedKeyRef.current === preSelectedKey) return;
    if (preSelected.kind === 'source') {
      const match = sources.find(s => s.source === preSelected.source && s.priceCents > 0);
      if (match) {
        setSelectedSource(match.source);
        setCustomValue(centsToDollars(match.priceCents));
        appliedKeyRef.current = preSelectedKey;
      }
    } else if (preSelected.kind === 'manual') {
      setSelectedSource(null);
      setCustomValue(centsToDollars(preSelected.priceCents));
      appliedKeyRef.current = preSelectedKey;
    }
  }, [preSelectedKey, preSelected, sources]);

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
    if (e.key === 'Enter') handleConfirm();
  };

  const hasSelection = selectedSource !== null || (customValue !== '' && dollarsToCents(customValue) > 0);
  const allDisabled = disabled || isSubmitting;
  const noPriceData = sources.every(s => s.priceCents === 0) && dollarsToCents(customValue) === 0;

  const currentCents = dollarsToCents(customValue);
  const marginCents = costBasisCents != null && costBasisCents > 0 && currentCents > 0
    ? currentCents - costBasisCents : null;

  if (status === 'accepted') {
    const displayCents = acceptedPriceCents || lastConfirmedCents || (getConfirmValues()?.priceCents ?? 0);
    const acceptedMargin = costBasisCents != null && costBasisCents > 0 && displayCents > 0
      ? displayCents - costBasisCents : null;
    return <AcceptedView
      displayCents={displayCents}
      marginCents={acceptedMargin}
      onReset={onReset}
      disabled={disabled}
    />;
  }

  if (status === 'skipped') {
    return <SkippedView onReset={onReset} disabled={disabled} />;
  }

  if (noPriceData) {
    return <NoPriceDataView onFlag={onFlag} allDisabled={allDisabled} />;
  }

  return (
    <div className={styles.bar}>
      <span className={styles.prompt}>Set Price:</span>

      {sources.map(src => (
        <PricePill
          key={src.source}
          label={src.label}
          priceCents={src.priceCents}
          selected={selectedSource === src.source}
          recommended={recommendedSource === src.source}
          disabled={allDisabled}
          onClick={() => handleSourceClick(src)}
        />
      ))}

      <div className={styles.inputGroup}>
        <span className={styles.inputPrefix}>$</span>
        <input
          type="text"
          inputMode="decimal"
          placeholder="0.00"
          aria-label="Custom price in dollars"
          value={customValue}
          onChange={handleCustomChange}
          onKeyDown={handleKeyDown}
          disabled={allDisabled}
          className={styles.input}
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

      {marginCents != null && <MarginBadge cents={marginCents} />}

      {onFlag && (
        <div className={styles.flag}>
          <Button variant="secondary" size="sm" onClick={onFlag} disabled={allDisabled}>
            Flag Price Issue
          </Button>
        </div>
      )}
    </div>
  );
}

function AcceptedView({ displayCents, marginCents, onReset, disabled }: {
  displayCents: number;
  marginCents?: number | null;
  onReset?: () => void;
  disabled?: boolean;
}) {
  return (
    <div className={styles.accepted}>
      <span className={styles.acceptedChip}>&#10003; {formatCents(displayCents)}</span>
      {marginCents != null && <MarginBadge cents={marginCents} />}
      {onReset && (
        <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled}>Change</Button>
      )}
    </div>
  );
}

function SkippedView({ onReset, disabled }: { onReset?: () => void; disabled?: boolean }) {
  return (
    <div className={styles.skipped}>
      <span className={styles.skippedLabel}>Skipped</span>
      {onReset && (
        <Button variant="ghost" size="sm" onClick={onReset} disabled={disabled}>Undo</Button>
      )}
    </div>
  );
}

function NoPriceDataView({ onFlag, allDisabled }: { onFlag?: () => void; allDisabled: boolean }) {
  return (
    <div className={styles.warning}>
      <div className={styles.warningBody}>
        <span className={styles.warningIcon} aria-hidden="true">&#9888;</span>
        <div className={styles.warningText}>
          <span className={styles.warningTitle}>No price data</span>
          <span className={styles.warningDesc}>CL, DH, and last-sold signals are all missing — investigate before pricing.</span>
        </div>
      </div>
      {onFlag && (
        <div className={styles.warningAction}>
          <Button variant="danger" size="sm" onClick={onFlag} disabled={allDisabled}>
            Flag for Fix
          </Button>
        </div>
      )}
    </div>
  );
}
