import { useState, useEffect } from 'react';
import { useCardLadderStatus, useSaveCardLadderConfig, useTriggerCardLadderRefresh } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';

interface CLLastRun {
  lastRunAt: string;
  durationMs: number;
  updated: number;
  mapped: number;
  skipped: number;
  totalCLCards: number;
}

function RunStatRow({ label, value, accent }: { label: string; value: string | number; accent?: 'green' | 'red' | 'yellow' }) {
  const color =
    accent === 'green' ? 'text-emerald-400' :
    accent === 'red'   ? 'text-red-400' :
    accent === 'yellow'? 'text-yellow-400' :
    'text-[var(--text)]';
  return (
    <div className="flex justify-between items-center py-1 border-b border-[var(--surface-2)] last:border-0">
      <span className="text-xs text-[var(--text-muted)]">{label}</span>
      <span className={`text-xs font-medium tabular-nums ${color}`}>{value}</span>
    </div>
  );
}

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

  const lastRun: CLLastRun | undefined = status?.lastRun;

  return (
    <div className="space-y-4 mt-4">
      {/* Connection Status */}
      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">Connection Status</h3>
        {status?.configured ? (
          <div className="space-y-2">
            <div className="flex items-center gap-3">
              <div className="w-2 h-2 rounded-full bg-emerald-400" />
              <span className="text-sm text-[var(--text)]">
                Connected as <strong>{status.email}</strong>
              </span>
            </div>
            <p className="text-xs text-[var(--text-muted)]">
              Collection: {status.collectionId}
            </p>
            {status.cardsMapped !== undefined && (
              <p className="text-xs text-[var(--text-muted)]">
                Cards mapped: {status.cardsMapped}
              </p>
            )}
          </div>
        ) : (
          <div className="flex items-center gap-3">
            <div className="w-2 h-2 rounded-full bg-gray-500" />
            <span className="text-sm text-[var(--text-muted)]">Not connected</span>
          </div>
        )}
      </CardShell>

      {/* Last Run Stats */}
      {lastRun && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-3">Last Refresh Run</h3>
          <div className="space-y-0">
            <RunStatRow label="Ran at" value={new Date(lastRun.lastRunAt).toLocaleString()} />
            <RunStatRow label="Duration" value={`${(lastRun.durationMs / 1000).toFixed(1)}s`} />
            <RunStatRow label="CL cards fetched" value={lastRun.totalCLCards} />
            <RunStatRow
              label="Updated"
              value={lastRun.updated}
              accent={lastRun.updated > 0 ? 'green' : undefined}
            />
            <RunStatRow
              label="New mappings"
              value={lastRun.mapped}
              accent={lastRun.mapped > 0 ? 'green' : undefined}
            />
            <RunStatRow label="Skipped (no match)" value={lastRun.skipped} />
          </div>
        </CardShell>
      )}

      {/* Configuration Form */}
      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">
          {status?.configured ? 'Update Credentials' : 'Connect Card Ladder'}
        </h3>
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
              Stable project identifier — does not rotate.
            </p>
          </div>
          <Button type="submit" variant="primary" size="sm" loading={saveMutation.isPending}>
            {status?.configured ? 'Update' : 'Connect'}
          </Button>
        </form>
      </CardShell>

      {/* Manual Refresh */}
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
