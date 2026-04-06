import { useState, useCallback, useEffect, useRef } from 'react';
import type { PriceHint } from '../types/pricing';
import { api } from '../js/api';
import { useToast } from './contexts/ToastContext';

interface PriceHintDialogProps {
  cardName: string;
  setName: string;
  cardNumber: string;
  onClose: () => void;
  onSaved: () => void;
}

export default function PriceHintDialog({
  cardName,
  setName,
  cardNumber,
  onClose,
  onSaved,
}: PriceHintDialogProps) {
  const [provider, setProvider] = useState<PriceHint['provider']>('pricecharting');
  const [externalId, setExternalId] = useState('');
  const [saving, setSaving] = useState(false);
  const toast = useToast();
  const panelRef = useRef<HTMLDivElement>(null);

  // Focus management: focus the panel on mount
  useEffect(() => {
    panelRef.current?.focus();
  }, []);

  // Escape key handler
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const handleSave = useCallback(async () => {
    if (!externalId.trim()) return;
    setSaving(true);
    try {
      await api.savePriceHint({
        cardName,
        setName,
        cardNumber,
        provider,
        externalId: externalId.trim(),
      });
      toast.success('Price hint saved');
      onSaved();
      onClose();
    } catch {
      toast.error('Failed to save price hint');
    } finally {
      setSaving(false);
    }
  }, [cardName, setName, cardNumber, provider, externalId, onClose, onSaved, toast]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="price-hint-dialog-title"
        tabIndex={-1}
        className="bg-[var(--surface-1)] rounded-lg shadow-xl p-6 max-w-md w-full mx-4"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 id="price-hint-dialog-title" className="text-lg font-semibold mb-4">Fix Pricing</h3>

        <div className="space-y-3 text-sm">
          <div>
            <div className="block text-[var(--text-muted)] mb-1">Card</div>
            <div className="font-medium">{cardName}</div>
          </div>
          <div>
            <div className="block text-[var(--text-muted)] mb-1">Set</div>
            <div className="font-medium">{setName}</div>
          </div>
          {cardNumber && (
            <div>
              <div className="block text-[var(--text-muted)] mb-1">Number</div>
              <div className="font-medium">{cardNumber}</div>
            </div>
          )}

          <div>
            <label htmlFor="price-hint-provider" className="block text-[var(--text-muted)] mb-1">Provider</label>
            <select
              id="price-hint-provider"
              value={provider}
              onChange={(e) => setProvider(e.target.value as PriceHint['provider'])}
              className="w-full px-3 py-2 rounded bg-[var(--surface-2)] border border-[var(--surface-2)] text-[var(--text)]"
            >
              <option value="pricecharting">PriceCharting</option>
              <option value="doubleholo">DoubleHolo</option>
            </select>
          </div>

          <div>
            <label htmlFor="price-hint-external-id" className="block text-[var(--text-muted)] mb-1">
              External ID
            </label>
            <input
              id="price-hint-external-id"
              type="text"
              value={externalId}
              onChange={(e) => setExternalId(e.target.value)}
              placeholder={provider === 'pricecharting' ? 'Product ID (e.g. 12345)' : 'DoubleHolo Card ID'}
              className="w-full px-3 py-2 rounded bg-[var(--surface-2)] border border-[var(--surface-2)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
              autoFocus
            />
            {provider === 'pricecharting' && (
              <p className="text-xs text-[var(--text-muted)] mt-1">
                Find the product ID in the PriceCharting URL (e.g. /game/pokemon-.../card-name)
              </p>
            )}
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm rounded bg-[var(--surface-2)] hover:bg-[var(--surface-3)] transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleSave}
            disabled={saving || !externalId.trim()}
            className="px-4 py-2 text-sm rounded bg-[var(--accent)] text-white hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save Hint'}
          </button>
        </div>
      </div>
    </div>
  );
}
