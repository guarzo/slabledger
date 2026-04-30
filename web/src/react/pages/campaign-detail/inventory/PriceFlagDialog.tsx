import { useState } from 'react';
import { Button } from '../../../ui';
import { PRICE_FLAG_LABELS, type PriceFlagReason } from '../../../../types/campaigns/priceReview';

interface PriceFlagDialogProps {
  cardName: string;
  grade: number;
  onSubmit: (reason: PriceFlagReason) => void;
  onCancel: () => void;
  isSubmitting?: boolean;
}

const REASONS = Object.keys(PRICE_FLAG_LABELS) as PriceFlagReason[];

export default function PriceFlagDialog({ cardName, grade, onSubmit, onCancel, isSubmitting }: PriceFlagDialogProps) {
  const [selected, setSelected] = useState<PriceFlagReason | null>(null);

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onCancel();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--surface-overlay)]"
      onClick={handleBackdropClick}
    >
      <div className="w-full max-w-md mx-4 rounded-lg border border-[var(--border)] bg-[var(--surface-raised)] shadow-xl">
        {/* Header */}
        <div className="px-5 pt-5 pb-3">
          <h3 className="text-sm font-semibold text-[var(--text)]">Flag Price Issue</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {cardName} &middot; PSA {grade}
          </p>
        </div>

        {/* Reason options */}
        <div className="px-5 pb-4 space-y-2">
          {REASONS.map(reason => {
            const reasonId = `price-flag-reason-${reason}`;
            return (
            <label
              key={reason}
              htmlFor={reasonId}
              className={`flex items-center gap-3 px-3 py-2.5 rounded-md border cursor-pointer transition-colors ${
                selected === reason
                  ? 'border-[var(--accent)] bg-[var(--accent)]/10'
                  : 'border-[var(--border)] hover:border-[var(--text-muted)]'
              }`}
            >
              <span
                className={`flex-shrink-0 w-4 h-4 rounded-full border-2 flex items-center justify-center ${
                  selected === reason
                    ? 'border-[var(--accent)]'
                    : 'border-[var(--text-muted)]'
                }`}
              >
                {selected === reason && (
                  <span className="w-2 h-2 rounded-full bg-[var(--accent)]" />
                )}
              </span>
              <input
                id={reasonId}
                type="radio"
                name="flag-reason"
                value={reason}
                checked={selected === reason}
                onChange={() => setSelected(reason)}
                className="sr-only"
              />
              <span className="text-sm text-[var(--text)]">{PRICE_FLAG_LABELS[reason]}</span>
            </label>
            );
          })}
        </div>

        {/* Actions */}
        <div className="flex items-center justify-end gap-2 px-5 pb-5">
          <Button variant="ghost" size="sm" onClick={onCancel} disabled={isSubmitting}>
            Cancel
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => selected && onSubmit(selected)}
            disabled={!selected || isSubmitting}
            loading={isSubmitting}
          >
            Submit Flag
          </Button>
        </div>
      </div>
    </div>
  );
}
