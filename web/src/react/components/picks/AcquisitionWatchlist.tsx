/**
 * AcquisitionWatchlist
 *
 * Shows the acquisition watchlist with add/remove functionality.
 */
import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import type { WatchlistItem } from '../../../types/picks';

const glassStyle = { background: 'var(--glass-bg)', backdropFilter: 'blur(12px)' } as const;
const inputClass = 'w-full px-3 py-1.5 text-sm rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] text-[var(--text)] placeholder-[var(--text-muted)] focus:outline-none focus:border-[var(--brand-500)]/60';

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function sourceBadgeClass(source: WatchlistItem['source']): string {
  switch (source) {
    case 'manual':
      return 'bg-[var(--surface-2)]/60 text-[var(--text-muted)] border border-[var(--surface-2)]';
    case 'auto_from_pick':
      return 'bg-[var(--brand-500)]/15 text-[var(--brand-500)] border border-[var(--brand-500)]/25';
  }
}

function sourceLabel(source: WatchlistItem['source']): string {
  switch (source) {
    case 'manual':         return 'Manual';
    case 'auto_from_pick': return 'AI Pick';
  }
}

/* ------------------------------------------------------------------ */
/*  AddCardForm                                                        */
/* ------------------------------------------------------------------ */

interface AddCardFormProps {
  onCancel: () => void;
  onSuccess: () => void;
}

function AddCardForm({ onCancel, onSuccess }: AddCardFormProps) {
  const queryClient = useQueryClient();
  const [cardName, setCardName] = useState('');
  const [setName, setSetName]   = useState('');
  const [grade, setGrade]       = useState('10');

  const addMutation = useMutation({
    mutationFn: () => api.addToAcquisitionWatchlist(cardName.trim(), setName.trim(), grade.trim()),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['picks', 'watchlist'] });
      onSuccess();
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!cardName.trim() || !setName.trim() || !grade.trim()) return;
    addMutation.mutate();
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="mt-3 p-4 rounded-xl border border-[var(--brand-500)]/20 space-y-3"
      style={glassStyle}
    >
      <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide">Add to Watchlist</p>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
        <div className="sm:col-span-1">
          <label htmlFor="watchlist-card-name" className="block text-xs text-[var(--text-muted)] mb-1">Card Name</label>
          <input
            id="watchlist-card-name"
            type="text"
            value={cardName}
            onChange={e => setCardName(e.target.value)}
            placeholder="Charizard"
            className={inputClass}
            required
          />
        </div>
        <div className="sm:col-span-1">
          <label htmlFor="watchlist-set-name" className="block text-xs text-[var(--text-muted)] mb-1">Set Name</label>
          <input
            id="watchlist-set-name"
            type="text"
            value={setName}
            onChange={e => setSetName(e.target.value)}
            placeholder="Base Set"
            className={inputClass}
            required
          />
        </div>
        <div>
          <label htmlFor="watchlist-grade" className="block text-xs text-[var(--text-muted)] mb-1">Grade</label>
          <input
            id="watchlist-grade"
            type="text"
            value={grade}
            onChange={e => setGrade(e.target.value)}
            placeholder="10"
            className={inputClass}
            required
          />
        </div>
      </div>

      {addMutation.isError && (
        <p className="text-xs text-red-400">Failed to add card. Please try again.</p>
      )}

      <div className="flex items-center gap-2 pt-1">
        <button
          type="submit"
          disabled={addMutation.isPending || !cardName.trim() || !setName.trim() || !grade.trim()}
          className="px-4 py-1.5 text-xs font-semibold rounded-lg bg-[var(--brand-500)] text-white disabled:opacity-50 disabled:cursor-not-allowed hover:opacity-90 transition-opacity"
        >
          {addMutation.isPending ? 'Adding…' : 'Add Card'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="px-4 py-1.5 text-xs font-medium rounded-lg border border-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

/* ------------------------------------------------------------------ */
/*  WatchlistRow                                                       */
/* ------------------------------------------------------------------ */

function WatchlistRow({ item }: { item: WatchlistItem }) {
  const queryClient = useQueryClient();

  const removeMutation = useMutation({
    mutationFn: () => api.removeFromAcquisitionWatchlist(item.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['picks', 'watchlist'] });
    },
  });

  return (
    <div className="flex items-start justify-between gap-3 py-3 border-b border-[var(--surface-2)]/40 last:border-b-0">
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex items-center gap-2 flex-wrap">
          <p className="text-sm font-semibold text-[var(--text)] leading-snug">
            {item.card_name}
          </p>
          <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${sourceBadgeClass(item.source)}`}>
            {sourceLabel(item.source)}
          </span>
        </div>
        <p className="text-xs text-[var(--text-muted)]">
          {item.set_name} &middot; PSA {item.grade}
        </p>
        {item.latest_assessment?.buy_thesis && (
          <p className="text-xs text-[var(--text-secondary)] leading-relaxed line-clamp-2">
            {item.latest_assessment.buy_thesis}
          </p>
        )}
      </div>
      <div className="shrink-0 flex flex-col items-end gap-1">
        <button
          onClick={() => removeMutation.mutate()}
          disabled={removeMutation.isPending}
          className="text-xs text-[var(--text-muted)] hover:text-red-400 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          aria-label={`Remove ${item.card_name} from watchlist`}
        >
          {removeMutation.isPending ? 'Removing…' : 'Remove'}
        </button>
        {removeMutation.isError && (
          <span className="text-xs text-[var(--danger)]">Failed to remove</span>
        )}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */

export default function AcquisitionWatchlist() {
  const [showForm, setShowForm] = useState(false);

  const { data, isLoading, isError } = useQuery({
    queryKey: ['picks', 'watchlist'],
    queryFn: () => api.getAcquisitionWatchlist(),
    staleTime: 2 * 60 * 1000,
  });

  const items = (data?.items ?? []).filter(item => item.active);

  return (
    <div
      className="p-4 rounded-2xl border border-[var(--surface-2)]/50 space-y-1"
      style={glassStyle}
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-lg font-semibold text-[var(--text)]">Acquisition Watchlist</h2>
        <button
          onClick={() => setShowForm(prev => !prev)}
          className="text-xs font-semibold px-3 py-1.5 rounded-lg bg-[var(--brand-500)]/15 text-[var(--brand-400)] border border-[var(--brand-500)]/25 hover:bg-[var(--brand-500)]/25 transition-colors"
        >
          {showForm ? 'Cancel' : '+ Add Card'}
        </button>
      </div>

      {/* Add form */}
      {showForm && (
        <AddCardForm
          onCancel={() => setShowForm(false)}
          onSuccess={() => setShowForm(false)}
        />
      )}

      {/* Loading */}
      {isLoading && (
        <div className="space-y-2 pt-2">
          {[1, 2].map(n => (
            <div key={n} className="h-14 rounded-lg animate-pulse bg-[var(--surface-2)]/30" />
          ))}
        </div>
      )}

      {/* Error */}
      {isError && (
        <div className="p-3 rounded-lg border border-[var(--danger-border)] bg-[var(--danger-bg)] text-sm text-[var(--danger)]">
          Failed to load watchlist.
        </div>
      )}

      {/* Items */}
      {!isLoading && !isError && items.length === 0 && (
        <p className="text-sm text-[var(--text-muted)] py-4 text-center">
          No cards on your watchlist yet. Add one to start tracking.
        </p>
      )}

      {!isLoading && !isError && items.length > 0 && (
        <div>
          {items.map(item => (
            <WatchlistRow key={item.id} item={item} />
          ))}
        </div>
      )}
    </div>
  );
}
