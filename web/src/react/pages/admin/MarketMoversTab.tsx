import { useState, useEffect, useRef } from 'react';
import { useMarketMoversStatus, useSaveMarketMoversConfig, useTriggerMarketMoversRefresh } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { formatAdminDate } from './adminUtils';

function StatLine({ label, value, accent }: { label: string; value: string | number; accent?: 'green' | 'red' | 'yellow' }) {
  const color =
    accent === 'green' ? 'text-emerald-400' :
    accent === 'red'   ? 'text-red-400' :
    accent === 'yellow'? 'text-yellow-400' :
    'text-[var(--text)]';
  return (
    <p className="text-xs text-[var(--text-muted)]">
      {label}: <span className={color}>{value}</span>
    </p>
  );
}

export function MarketMoversTab({ enabled = true }: { enabled?: boolean }) {
  const { data: status, isLoading, error } = useMarketMoversStatus({ enabled });
  const saveMutation = useSaveMarketMoversConfig();
  const refreshMutation = useTriggerMarketMoversRefresh();
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
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Market Movers integration is not enabled.</p>
      </CardShell>
    );
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

  if (isLoading) {
    return (
      <CardShell padding="lg">
        <p className="text-[var(--text-muted)]">Loading Market Movers status...</p>
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
      <div className="space-y-4 mt-4">
        <CardShell padding="lg">
          <p className="text-red-400 text-sm mb-4">Failed to load Market Movers status.</p>
          <h3 className="text-base font-semibold text-[var(--text)] mb-4">Connect Market Movers</h3>
          <div className="flex items-center gap-3 mb-4">
            <div className="w-2 h-2 rounded-full bg-gray-500" />
            <span className="text-sm text-[var(--text-muted)]">Not connected</span>
          </div>
          {credentialForm}
        </CardShell>
      </div>
    );
  }

  const lastRun = status?.lastRun;
  const priceStats = status?.priceStats;

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
            <span className="text-xs text-[var(--text-muted)]">{status.username}</span>
          </div>

          {/* Info rows */}
          <div className="space-y-1 mb-3">
            {status.cardsMapped !== undefined && (
              <p className="text-xs text-[var(--text-muted)]">Cards mapped: {status.cardsMapped}</p>
            )}
          </div>

          {/* Price Coverage — compact inline */}
          {priceStats && (
            <div className="space-y-1 mb-3">
              <StatLine
                label="With MM price"
                value={`${priceStats.withMMPrice} / ${priceStats.unsoldTotal}`}
                accent={priceStats.withMMPrice > 0 ? 'green' : undefined}
              />
              <StatLine
                label="Synced to collection"
                value={priceStats.syncedCount}
                accent={priceStats.syncedCount > 0 ? 'green' : undefined}
              />
              <StatLine
                label="Stale (>7 days)"
                value={priceStats.staleCount}
                accent={priceStats.staleCount > 0 ? 'yellow' : undefined}
              />
            </div>
          )}

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
                Ran at {formatAdminDate(lastRun.lastRunAt)} · {Number.isFinite(lastRun.durationMs) ? (lastRun.durationMs / 1000).toFixed(1) : '?'}s
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
      ) : (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-4">Connect Market Movers</h3>
          <div className="flex items-center gap-3 mb-4">
            <div className="w-2 h-2 rounded-full bg-gray-500" />
            <span className="text-sm text-[var(--text-muted)]">Not connected</span>
          </div>
          {credentialForm}
        </CardShell>
      )}

      {/* Manual Refresh — separate action card */}
      {status?.configured && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-2">Manual Refresh</h3>
          <p className="text-sm text-[var(--text-muted)] mb-3">
            Trigger a Market Movers value sync. Searches for each unsold card by name/grade and fetches the 30-day avg sale price.
          </p>
          <Button variant="secondary" size="sm" onClick={handleRefresh} loading={refreshMutation.isPending}>
            Trigger Refresh
          </Button>
        </CardShell>
      )}
    </div>
  );
}
