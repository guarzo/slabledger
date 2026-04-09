import { useState, useEffect } from 'react';
import { useCardLadderStatus, useSaveCardLadderConfig, useTriggerCardLadderRefresh } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { formatAdminDate } from './shared';

export function CardLadderTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useCardLadderStatus({ enabled });
  const saveMutation = useSaveCardLadderConfig();
  const refreshMutation = useTriggerCardLadderRefresh();
  const toast = useToast();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [collectionId, setCollectionId] = useState('');
  const [firebaseApiKey, setFirebaseApiKey] = useState('');

  useEffect(() => {
    if (status?.configured) {
      setEmail(status.email ?? '');
      setCollectionId(status.collectionId ?? '');
    }
  }, [status?.configured, status?.email, status?.collectionId]);

  if (!enabled) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Card Ladder integration is not enabled.</p>
      </CardShell>
    );
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await saveMutation.mutateAsync({ email, password, collectionId, firebaseApiKey });
      toast.success('Card Ladder connected');
      setPassword('');
    } catch {
      toast.error('Failed to connect Card Ladder — check credentials');
    }
  };

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync();
      toast.success('Card Ladder refresh complete');
    } catch {
      toast.error('Card Ladder refresh failed');
    }
  };

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading Card Ladder status...</p>
      </CardShell>
    );
  }

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-red-400 text-sm">Failed to load Card Ladder status.</p>
      </CardShell>
    );
  }

  const credentialForm = (
    <form onSubmit={handleSave} className="space-y-3">
      <div>
        <label htmlFor="cl-email" className="block text-xs text-[var(--text-muted)] mb-1">Email</label>
        <input
          id="cl-email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder={status?.email ?? 'your@email.com'}
          required
          autoComplete="email"
          className="w-full rounded-md bg-[var(--surface-2)] border border-[var(--surface-2)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
      </div>
      <div>
        <label htmlFor="cl-password" className="block text-xs text-[var(--text-muted)] mb-1">Password</label>
        <input
          id="cl-password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          autoComplete="current-password"
          className="w-full rounded-md bg-[var(--surface-2)] border border-[var(--surface-2)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
      </div>
      <div>
        <label htmlFor="cl-collection-id" className="block text-xs text-[var(--text-muted)] mb-1">Collection ID</label>
        <input
          id="cl-collection-id"
          type="text"
          value={collectionId}
          onChange={(e) => setCollectionId(e.target.value)}
          placeholder={status?.collectionId ?? 'abc123'}
          required
          className="w-full rounded-md bg-[var(--surface-2)] border border-[var(--surface-2)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
      </div>
      <div>
        <label htmlFor="cl-firebase-key" className="block text-xs text-[var(--text-muted)] mb-1">Firebase API Key</label>
        <input
          id="cl-firebase-key"
          type="password"
          value={firebaseApiKey}
          onChange={(e) => setFirebaseApiKey(e.target.value)}
          placeholder="AIza..."
          required
          autoComplete="off"
          className="w-full rounded-md bg-[var(--surface-2)] border border-[var(--surface-2)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
        <p className="text-xs text-[var(--text-muted)] mt-1">
          Your Card Ladder project key — available in the Card Ladder Firebase console settings.
        </p>
      </div>
      <Button type="submit" variant="primary" size="sm" loading={saveMutation.isPending}>
        {status?.configured ? 'Update' : 'Connect'}
      </Button>
    </form>
  );

  // Type for lastRun stats — matches CLRunStats JSON from backend
  interface CLLastRun {
    lastRunAt: string;
    durationMs: number;
    updated: number;
    mapped: number;
    skipped: number;
    totalCLCards: number;
    cardsPushed: number;
    cardsRemoved: number;
  }
  const lastRun = (status as (typeof status & { lastRun?: CLLastRun }) | undefined)?.lastRun;

  return (
    <div className="space-y-4 mt-4">
      {status?.configured ? (
        <CardShell padding="lg">
          {/* Header row */}
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-emerald-400 shrink-0" />
              <span className="text-sm font-semibold text-[var(--text)]">Connected</span>
            </div>
            <span className="text-xs text-[var(--text-muted)]">{status.email}</span>
          </div>

          {/* Info rows */}
          <div className="space-y-1 mb-3">
            <p className="text-xs text-[var(--text-muted)]">Collection: {status.collectionId}</p>
            {status.cardsMapped !== undefined && (
              <p className="text-xs text-[var(--text-muted)]">Cards mapped: {status.cardsMapped}</p>
            )}
          </div>

          {/* Collapsible credentials update */}
          <details>
            <summary className="text-xs text-[var(--brand-400)] cursor-pointer mt-3 select-none">Update credentials</summary>
            <div className="mt-3">
              {credentialForm}
            </div>
          </details>

          {/* Last Refresh block */}
          {lastRun && (
            <div className="mt-4 pt-4 border-t border-[var(--surface-2)] space-y-1">
              <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide">Last Refresh</p>
              <p className="text-xs text-[var(--text-muted)]">
                Ran at {formatAdminDate(lastRun.lastRunAt)} · {(lastRun.durationMs / 1000).toFixed(1)}s
              </p>
              <p className="text-xs text-[var(--text-muted)]">
                {lastRun.updated > 0
                  ? <span className="text-[var(--success)]">{lastRun.updated} updated</span>
                  : <span>0 updated</span>} · {lastRun.mapped} mapped · {lastRun.skipped} skipped · {lastRun.totalCLCards} total CL cards
              </p>
              {(lastRun.cardsPushed > 0 || lastRun.cardsRemoved > 0) && (
                <p className="text-xs text-[var(--text-muted)]">
                  {lastRun.cardsPushed > 0 && (
                    <span className="text-[var(--success)]">{lastRun.cardsPushed} pushed</span>
                  )}
                  {lastRun.cardsPushed > 0 && lastRun.cardsRemoved > 0 && ' · '}
                  {lastRun.cardsRemoved > 0 && (
                    <span className="text-amber-400">{lastRun.cardsRemoved} removed</span>
                  )}
                </p>
              )}
            </div>
          )}
        </CardShell>
      ) : (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-4">Connect Card Ladder</h3>
          <div className="flex items-center gap-3 mb-4">
            <div className="w-2 h-2 rounded-full bg-gray-500" />
            <span className="text-sm text-[var(--text-muted)]">Not connected</span>
          </div>
          {credentialForm}
        </CardShell>
      )}

      {/* Trigger Refresh — separate action card */}
      {status?.configured && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-2">Manual Refresh</h3>
          <p className="text-sm text-[var(--text-muted)] mb-3">
            Trigger a Card Ladder value sync. This fetches your collection and updates CL values for matched cards.
          </p>
          <Button variant="secondary" size="sm" onClick={handleRefresh} loading={refreshMutation.isPending}>
            Trigger Refresh
          </Button>
        </CardShell>
      )}
    </div>
  );
}
