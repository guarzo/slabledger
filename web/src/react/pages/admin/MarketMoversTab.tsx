import { useState, useEffect, useRef } from 'react';
import { useMarketMoversStatus, useSaveMarketMoversConfig, useTriggerMarketMoversRefresh, useSyncMarketMoversCollection } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { formatAdminDate } from './adminUtils';

export function MarketMoversTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useMarketMoversStatus({ enabled });
  const saveMutation = useSaveMarketMoversConfig();
  const refreshMutation = useTriggerMarketMoversRefresh();
  const syncMutation = useSyncMarketMoversCollection();
  const toast = useToast();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const hasHydrated = useRef(false);

  useEffect(() => {
    if (!hasHydrated.current && status?.configured) {
      setUsername(status.username ?? '');
      hasHydrated.current = true;
    }
  }, [status?.configured, status?.username]);

  if (!enabled) {
    return <NotConfigured message="Market Movers integration is not enabled." />;
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await saveMutation.mutateAsync({ username, password });
      toast.success('Market Movers connected');
      setPassword('');
    } catch {
      toast.error('Failed to connect Market Movers — check credentials');
    }
  };

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync();
      toast.success('Market Movers refresh complete');
    } catch {
      toast.error('Market Movers refresh failed');
    }
  };

  const handleSyncCollection = async () => {
    try {
      const result = await syncMutation.mutateAsync();
      if (result.failed > 0) {
        toast.warning(`MM sync: ${result.synced} synced, ${result.skipped} skipped, ${result.failed} failed`);
      } else if (result.synced === 0 && result.skipped === 0) {
        toast.info('All items already synced to Market Movers');
      } else {
        toast.success(`MM sync: ${result.synced} items added to Market Movers collection`);
      }
    } catch {
      toast.error('Failed to sync Market Movers collection');
    }
  };

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading Market Movers status…</p>
      </CardShell>
    );
  }

  const credentialForm = (
    <form onSubmit={handleSave} className="space-y-3">
      <p className="text-xs text-[var(--text-muted)]">
        Use your <strong>Sports Card Investor</strong> (sportscardinvestor.com) login credentials.
      </p>
      <div>
        <label htmlFor="mm-username" className="block text-xs text-[var(--text-muted)] mb-1">Email / Username</label>
        <input
          id="mm-username"
          type="text"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder={status?.username ?? 'your@email.com'}
          required
          autoComplete="username"
          className="w-full rounded-md bg-[var(--surface-2)] border border-[var(--surface-2)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
      </div>
      <div>
        <label htmlFor="mm-password" className="block text-xs text-[var(--text-muted)] mb-1">Password</label>
        <input
          id="mm-password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
          autoComplete="current-password"
          className="w-full rounded-md bg-[var(--surface-2)] border border-[var(--surface-2)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
      </div>
      <Button type="submit" variant="primary" size="sm" loading={saveMutation.isPending}>
        {status?.configured ? 'Update' : 'Connect'}
      </Button>
    </form>
  );

  if (error && !status) {
    return (
      <CardShell padding="lg">
        <p className="text-red-400 text-sm mb-4">Failed to load Market Movers status.</p>
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
  const priceStats = status.priceStats;

  return (
    <CardShell padding="lg">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-emerald-400 shrink-0" />
          <span className="text-sm font-semibold text-[var(--text)]">Connected</span>
        </div>
        <span className="text-xs text-[var(--text-muted)]">{status.username}</span>
      </div>

      {priceStats && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-3">
          <Stat label="Mapped" value={status.cardsMapped ?? 0} />
          <Stat label="Priced" value={`${priceStats.withMMPrice}/${priceStats.unsoldTotal}`} tone={priceStats.withMMPrice > 0 ? 'success' : 'default'} />
          <Stat label="In collection" value={priceStats.syncedCount} />
          <Stat label="Stale >7d" value={priceStats.staleCount} tone={priceStats.staleCount > 0 ? 'warning' : 'default'} />
        </div>
      )}

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
            {lastRun.updated > 0
              ? <span className="text-[var(--success)]">{lastRun.updated} updated</span>
              : <span>0 updated</span>} · {lastRun.newMappings} new · {lastRun.skipped} skipped
            {lastRun.searchFailed > 0 && (
              <> · <span className="text-red-400">{lastRun.searchFailed} errors</span></>
            )}
          </p>
        </div>
      )}
    </CardShell>
  );
}

function NotConfigured({ message }: { message: string }) {
  return (
    <CardShell padding="lg">
      <p className="text-[var(--text-muted)]">{message}</p>
    </CardShell>
  );
}

function Stat({ label, value, tone = 'default' }: { label: string; value: number | string; tone?: 'default' | 'warning' | 'success' }) {
  const valueColor =
    tone === 'warning' ? 'text-[var(--warning)]' :
    tone === 'success' ? 'text-[var(--success)]' :
    'text-[var(--text)]';
  return (
    <div className="min-w-0">
      <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wider">{label}</div>
      <div className={`text-sm font-semibold tabular-nums ${valueColor}`}>{value}</div>
    </div>
  );
}
