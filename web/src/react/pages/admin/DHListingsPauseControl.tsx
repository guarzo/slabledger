import { useState } from 'react';
import { useDHPushConfig, useSaveDHPushConfig } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';

/**
 * DHListingsPauseControl — the DoubleHolo listing kill switch.
 *
 * This is deliberately always visible. It holds state an operator acts on
 * regularly (before a card show, during a pricing incident), so it must never
 * live behind a disclosure widget: a paused marketplace that reads as normal
 * is the exact failure this control exists to prevent.
 */
export function DHListingsPauseControl({ enabled = true }: { enabled?: boolean }) {
  const toast = useToast();
  const { data: config } = useDHPushConfig({ enabled });
  const saveMutation = useSaveDHPushConfig();

  // Optimistic override: null means "trust the server value". Cleared on both
  // settle paths so a failed save rolls straight back to the fetched state.
  const [optimistic, setOptimistic] = useState<boolean | null>(null);

  if (!config) return null;

  const paused = optimistic ?? config.listingsPaused;

  const toggle = () => {
    if (saveMutation.isPending) return;
    const next = !paused;
    setOptimistic(next);
    // Send ONLY the key this control owns. useSaveDHPushConfig merges it into a
    // freshly fetched server config, so a threshold edit saved moments earlier
    // is never rolled back by this write, and vice versa.
    saveMutation.mutate(
      { listingsPaused: next },
      {
        onSuccess: () => {
          setOptimistic(null);
          toast.success(next ? 'DH listings paused' : 'DH listings resumed');
        },
        onError: () => {
          setOptimistic(null);
          toast.error('Failed to update DH listing pause');
        },
      },
    );
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium text-[var(--text-muted)]">
          {paused ? 'Listings paused' : 'Listings live'}
        </span>
        <button
          type="button"
          role="switch"
          aria-checked={paused}
          aria-label="Pause DH listings"
          aria-busy={saveMutation.isPending}
          disabled={saveMutation.isPending}
          onClick={toggle}
          className={`relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors disabled:opacity-60 ${
            paused ? 'bg-[var(--warning)]' : 'bg-[var(--surface-3)]'
          }`}
        >
          <span
            className={`inline-block h-4 w-4 transform rounded-full bg-[var(--text)] transition-transform ${
              paused ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
      </div>
    </div>
  );
}
