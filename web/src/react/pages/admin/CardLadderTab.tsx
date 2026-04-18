import { useState, useEffect, useRef } from 'react';
import { useCardLadderStatus, useSaveCardLadderConfig, useTriggerCardLadderRefresh, useSyncCardLadderCollection } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { formatAdminDate } from './adminUtils';

export function CardLadderTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useCardLadderStatus({ enabled });
  const saveMutation = useSaveCardLadderConfig();
  const refreshMutation = useTriggerCardLadderRefresh();
  const syncMutation = useSyncCardLadderCollection();
  const toast = useToast();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [collectionId, setCollectionId] = useState('');
  const [firebaseApiKey, setFirebaseApiKey] = useState('');
  const hasHydrated = useRef(false);

  useEffect(() => {
    if (hasHydrated.current || !status?.configured) return;
    hasHydrated.current = true;
    setEmail(status.email ?? '');
    setCollectionId(status.collectionId ?? '');
  }, [status]);

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

  const handleSyncCollection = async () => {
    try {
      const result = await syncMutation.mutateAsync();
      if (result.failed > 0) {
        toast.warning(`CL sync: ${result.synced} synced, ${result.skipped} skipped, ${result.failed} failed`);
      } else if (result.synced === 0 && result.skipped === 0) {
        toast.info('All items already synced to Card Ladder');
      } else {
        toast.success(`CL sync: ${result.synced} cards synced to Card Ladder`);
      }
    } catch {
      toast.error('Failed to sync Card Ladder collection');
    }
  };

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading Card Ladder status…</p>
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

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)] text-sm mb-4">No credentials saved — enter your Card Ladder email and password to connect.</p>
        <div className="flex items-center gap-3 mb-4">
          <div className="w-2 h-2 rounded-full bg-gray-500" />
          <span className="text-sm text-[var(--text-muted)]">Not connected</span>
        </div>
        {credentialForm}
      </CardShell>
    );
  }

  if (!status?.configured) {
    return (
      <CardShell padding="lg">
        <div className="flex items-center gap-3 mb-4">
          <div className="w-2 h-2 rounded-full bg-gray-500" />
          <span className="text-sm text-[var(--text-muted)]">Not connected</span>
        </div>
        {credentialForm}
      </CardShell>
    );
  }

  const lastRun = status.lastRun;

  return (
    <CardShell padding="lg">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-emerald-400 shrink-0" />
          <span className="text-sm font-semibold text-[var(--text)]">Connected</span>
        </div>
        <span className="text-xs text-[var(--text-muted)]">{status.email}</span>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-3">
        <Stat label="Mapped" value={status.cardsMapped ?? 0} />
        <Stat label="Collection" value={status.collectionId ?? '—'} />
      </div>

      <div className="flex flex-wrap items-center gap-2 pt-3 border-t border-[var(--surface-2)]">
        <Button variant="secondary" size="sm" onClick={handleSyncCollection} loading={syncMutation.isPending}>
          Sync collection
        </Button>
        <Button variant="secondary" size="sm" onClick={handleRefresh} loading={refreshMutation.isPending}>
          Refresh values
        </Button>
      </div>

      <details className="mt-3">
        <summary className="text-xs text-[var(--brand-400)] cursor-pointer select-none">Update credentials</summary>
        <div className="mt-3">{credentialForm}</div>
      </details>

      {lastRun && (
        <div className="mt-4 pt-3 border-t border-[var(--surface-2)] space-y-1">
          <p className="text-[10px] font-semibold text-[var(--text-muted)] uppercase tracking-wider">Last Refresh</p>
          <p className="text-xs text-[var(--text-muted)]">
            {formatAdminDate(lastRun.lastRunAt)} · {Number.isFinite(lastRun.durationMs) ? (lastRun.durationMs / 1000).toFixed(1) : '?'}s
          </p>
          <p className="text-xs text-[var(--text-muted)]">
            {(lastRun.updated ?? 0) > 0
              ? <span className="text-[var(--success)]">{lastRun.updated} updated</span>
              : <span>0 updated</span>} · {lastRun.resolved ?? 0} newly resolved · {lastRun.totalPurchases ?? 0} purchases
          </p>
          {((lastRun.cardsPushed ?? 0) > 0 || (lastRun.cardsRemoved ?? 0) > 0) && (
            <p className="text-xs text-[var(--text-muted)]">
              {(lastRun.cardsPushed ?? 0) > 0 && (
                <span className="text-[var(--success)]">{lastRun.cardsPushed} pushed</span>
              )}
              {(lastRun.cardsPushed ?? 0) > 0 && (lastRun.cardsRemoved ?? 0) > 0 && ' · '}
              {(lastRun.cardsRemoved ?? 0) > 0 && (
                <span className="text-amber-400">{lastRun.cardsRemoved} removed</span>
              )}
            </p>
          )}
        </div>
      )}
    </CardShell>
  );
}

function Stat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="min-w-0">
      <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">{label}</div>
      <div className="text-sm font-semibold tabular-nums text-[var(--text)] truncate" title={String(value)}>{value}</div>
    </div>
  );
}
