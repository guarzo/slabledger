import { useState, useEffect, useRef } from 'react';
import { useMarketMoversStatus, useSaveMarketMoversConfig, useTriggerMarketMoversRefresh } from '../../queries/useAdminQueries';
import { useToast } from '../../contexts/ToastContext';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import type { MMLastRun } from '../../../types/admin';

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
          <p className="text-xs text-[var(--text-muted)] mb-3">
            Use your <strong>Sports Card Investor</strong> (sportscardinvestor.com) login credentials.
          </p>
          <form onSubmit={handleSave} className="space-y-3">
            <div>
              <label htmlFor="mm-username" className="block text-xs text-[var(--text-muted)] mb-1">Email / Username</label>
              <input
                id="mm-username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="your@email.com"
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
              Connect
            </Button>
          </form>
        </CardShell>
      </div>
    );
  }

  const lastRun: MMLastRun | undefined = status?.lastRun;
  const priceStats = status?.priceStats;

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
                Connected as <strong>{status.username}</strong>
              </span>
            </div>
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

      {/* Price Coverage Stats */}
      {priceStats && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-3">Price Coverage</h3>
          <div className="space-y-0">
            <RunStatRow label="Unsold inventory" value={priceStats.unsoldTotal} />
            <RunStatRow
              label="With MM price"
              value={`${priceStats.withMMPrice} / ${priceStats.unsoldTotal}`}
              accent={priceStats.withMMPrice > 0 ? 'green' : undefined}
            />
            <RunStatRow
              label="Synced to collection"
              value={priceStats.syncedCount}
              accent={priceStats.syncedCount > 0 ? 'green' : undefined}
            />
            <RunStatRow
              label="Stale (>7 days)"
              value={priceStats.staleCount}
              accent={priceStats.staleCount > 0 ? 'yellow' : undefined}
            />
            {priceStats.newestUpdate && (
              <RunStatRow label="Latest update" value={new Date(priceStats.newestUpdate).toLocaleString()} />
            )}
            {priceStats.oldestUpdate && (
              <RunStatRow
                label="Oldest update"
                value={new Date(priceStats.oldestUpdate).toLocaleString()}
                accent={priceStats.staleCount > 0 ? 'red' : undefined}
              />
            )}
          </div>
        </CardShell>
      )}

      {/* Last Run Stats */}
      {lastRun && (
        <CardShell padding="lg">
          <h3 className="text-base font-semibold text-[var(--text)] mb-3">Last Refresh Run</h3>
          <div className="space-y-0">
            <RunStatRow label="Ran at" value={new Date(lastRun.lastRunAt).toLocaleString()} />
            <RunStatRow label="Duration" value={`${(lastRun.durationMs / 1000).toFixed(1)}s`} />
            <RunStatRow label="Total inventory" value={lastRun.totalPurchases} />
            <RunStatRow
              label="Updated"
              value={lastRun.updated}
              accent={lastRun.updated > 0 ? 'green' : undefined}
            />
            <RunStatRow
              label="New mappings"
              value={lastRun.newMappings}
              accent={lastRun.newMappings > 0 ? 'green' : undefined}
            />
            <RunStatRow label="Skipped (no match)" value={lastRun.skipped} />
            <RunStatRow
              label="Search errors"
              value={lastRun.searchFailed}
              accent={lastRun.searchFailed > 0 ? 'red' : undefined}
            />
          </div>
        </CardShell>
      )}

      {/* Configuration Form */}
      <CardShell padding="lg">
        <h3 className="text-base font-semibold text-[var(--text)] mb-4">
          {status?.configured ? 'Update Credentials' : 'Connect Market Movers'}
        </h3>
        <p className="text-xs text-[var(--text-muted)] mb-3">
          Use your <strong>Sports Card Investor</strong> (sportscardinvestor.com) login credentials.
        </p>
        <form onSubmit={handleSave} className="space-y-3">
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
      </CardShell>

      {/* Manual Refresh */}
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
