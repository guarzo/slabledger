import { useState, useCallback, useEffect, useRef } from 'react';
import { api, isAPIError } from '../js/api';
import { useToast } from './contexts/ToastContext';
import { formatCents } from './utils/formatters';

interface PriceOverrideDialogProps {
  purchaseId: string;
  cardName: string;
  costBasisCents: number;
  currentPriceCents: number;
  currentOverrideCents?: number;
  currentOverrideSource?: string;
  aiSuggestedCents?: number;
  onClose: () => void;
  onSaved: () => void;
}

export default function PriceOverrideDialog({
  purchaseId,
  cardName,
  costBasisCents,
  currentPriceCents,
  currentOverrideCents,
  currentOverrideSource,
  aiSuggestedCents,
  onClose,
  onSaved,
}: PriceOverrideDialogProps) {
  const [priceInput, setPriceInput] = useState(
    currentOverrideCents ? (currentOverrideCents / 100).toFixed(2) : ''
  );
  const [source, setSource] = useState<string>(currentOverrideSource || 'manual');
  const [saving, setSaving] = useState(false);
  const toast = useToast();
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !saving) onClose();
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose, saving]);

  const markupPrice = Math.round(costBasisCents * 1.12);

  const handleCostMarkup = useCallback(() => {
    setPriceInput((markupPrice / 100).toFixed(2));
    setSource('cost_markup');
  }, [markupPrice]);

  function errorMessage(e: unknown, fallback: string): string {
    if (isAPIError(e)) return e.message;
    return fallback;
  }

  const handleAcceptAI = useCallback(async () => {
    setSaving(true);
    try {
      await api.acceptAISuggestion(purchaseId);
      toast.success('AI suggestion accepted');
      onSaved();
      onClose();
    } catch (e) {
      toast.error(errorMessage(e, 'Failed to accept AI suggestion'));
    } finally {
      setSaving(false);
    }
  }, [purchaseId, onClose, onSaved, toast]);

  const handleDismissAI = useCallback(async () => {
    setSaving(true);
    try {
      await api.dismissAISuggestion(purchaseId);
      toast.success('AI suggestion dismissed');
      onSaved();
      onClose();
    } catch (e) {
      toast.error(errorMessage(e, 'Failed to dismiss AI suggestion'));
    } finally {
      setSaving(false);
    }
  }, [purchaseId, onClose, onSaved, toast]);

  const handleSave = useCallback(async () => {
    const cents = Math.round(parseFloat(priceInput) * 100);
    if (isNaN(cents) || cents <= 0) return;
    setSaving(true);
    try {
      await api.setPriceOverride(purchaseId, cents, source);
      toast.success('Price override saved');
      onSaved();
      onClose();
    } catch (e) {
      toast.error(errorMessage(e, 'Failed to save price override'));
    } finally {
      setSaving(false);
    }
  }, [purchaseId, priceInput, source, onClose, onSaved, toast]);

  const handleClear = useCallback(async () => {
    setSaving(true);
    try {
      await api.clearPriceOverride(purchaseId);
      toast.success('Price override cleared');
      onSaved();
      onClose();
    } catch (e) {
      toast.error(errorMessage(e, 'Failed to clear price override'));
    } finally {
      setSaving(false);
    }
  }, [purchaseId, onClose, onSaved, toast]);

  const parsedCents = Math.round(parseFloat(priceInput) * 100);
  const isValid = !isNaN(parsedCents) && parsedCents > 0;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[var(--surface-overlay)]"
      onClick={() => { if (!saving) onClose(); }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="price-override-dialog-title"
        tabIndex={-1}
        className="bg-[var(--surface-1)] rounded-lg shadow-xl p-6 max-w-md w-full mx-4"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 id="price-override-dialog-title" className="text-lg font-semibold mb-4">Set Price</h3>

        <div className="space-y-3 text-sm">
          <div>
            <div className="text-[var(--text-muted)] mb-1">Card</div>
            <div className="font-medium truncate">{cardName}</div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <div className="text-[var(--text-muted)] mb-1">Cost Basis</div>
              <div className="font-medium">{formatCents(costBasisCents)}</div>
            </div>
            <div>
              <div className="text-[var(--text-muted)] mb-1">Computed Price</div>
              <div className="font-medium">{currentPriceCents > 0 ? formatCents(currentPriceCents) : '-'}</div>
            </div>
          </div>

          {currentOverrideCents && currentOverrideCents > 0 && (
            <div className="p-2 rounded bg-[var(--brand-500)]/10 border border-[var(--brand-500)]/20">
              <div className="flex items-center justify-between">
                <span className="text-xs text-[var(--text-muted)]">
                  Current override ({currentOverrideSource})
                </span>
                <span className="font-medium">{formatCents(currentOverrideCents)}</span>
              </div>
            </div>
          )}

          {aiSuggestedCents && aiSuggestedCents > 0 && (
            <div className="p-2 rounded bg-purple-500/10 border border-purple-500/20">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-purple-400">AI Suggestion</span>
                <span className="font-medium">{formatCents(aiSuggestedCents)}</span>
              </div>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={handleAcceptAI}
                  disabled={saving}
                  className="flex-1 text-xs px-2 py-1 rounded bg-purple-500/20 text-purple-300 hover:bg-purple-500/40 transition-colors disabled:opacity-50"
                >
                  Accept
                </button>
                <button
                  type="button"
                  onClick={handleDismissAI}
                  disabled={saving}
                  className="flex-1 text-xs px-2 py-1 rounded bg-[var(--surface-2)] hover:bg-[var(--surface-3)] transition-colors disabled:opacity-50"
                >
                  Dismiss
                </button>
              </div>
            </div>
          )}

          <div>
            <div className="flex items-center justify-between mb-1">
              <label htmlFor="price-override-input" className="text-[var(--text-muted)]">
                Override Price ($)
              </label>
              <button
                type="button"
                onClick={handleCostMarkup}
                className="text-xs px-2 py-0.5 rounded bg-[var(--success)]/20 text-[var(--success)] hover:bg-[var(--success)]/40 transition-colors"
              >
                12% Over Cost ({formatCents(markupPrice)})
              </button>
            </div>
            <input
              id="price-override-input"
              type="number"
              step="0.01"
              min="0"
              value={priceInput}
              onChange={(e) => { setPriceInput(e.target.value); setSource('manual'); }}
              placeholder="e.g. 125.00"
              className="w-full px-3 py-2 rounded bg-[var(--surface-2)] border border-[var(--surface-2)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
              autoFocus
            />
          </div>
        </div>

        <div className="flex justify-between mt-6">
          <div>
            {currentOverrideCents && currentOverrideCents > 0 && (
              <button
                type="button"
                onClick={handleClear}
                disabled={saving}
                className="px-4 py-2 text-sm rounded text-[var(--danger)] bg-[var(--danger)]/10 hover:bg-[var(--danger)]/20 transition-colors disabled:opacity-50"
              >
                Clear Override
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={saving}
              aria-disabled={saving}
              className="px-4 py-2 text-sm rounded bg-[var(--surface-2)] hover:bg-[var(--surface-3)] transition-colors disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={saving || !isValid}
              className="px-4 py-2 text-sm rounded bg-[var(--accent)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save Price'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
