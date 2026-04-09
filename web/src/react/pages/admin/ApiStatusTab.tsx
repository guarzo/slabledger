import type { ProviderStatus } from '../../../types/apiStatus';
import { useAdminApiUsage } from '../../queries/useAdminQueries';
import { ProgressBar } from './shared';
import { formatAdminDate } from './adminUtils';

function UsageBar({ used, limit }: { used: number; limit: number }) {
  return <ProgressBar value={used} max={limit} warningThreshold={80} dangerThreshold={95} />;
}

const providerLabels: Record<string, string> = {
  doubleholo: 'DoubleHolo',
};

function ProviderCard({ provider }: { provider: ProviderStatus }) {
  const { today } = provider;
  const label = providerLabels[provider.name] ?? provider.name;
  const hasLimit = today.limit != null;

  let statusText = 'Healthy';
  let statusColor = 'text-[var(--success)]';
  if (provider.blocked) {
    statusText = 'Blocked'; statusColor = 'text-[var(--danger)]';
  } else if (today.calls > 0 && today.successRate < 90) {
    statusText = 'Degraded'; statusColor = 'text-[var(--warning)]';
  } else if (hasLimit && today.limit! > 0 && today.remaining != null && today.remaining / today.limit! < 0.2) {
    statusText = 'Low Budget'; statusColor = 'text-[var(--warning)]';
  }

  return (
    <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] p-5 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-base font-semibold text-[var(--text)]">{label}</h3>
        <span className={`text-xs font-medium ${statusColor}`}>{statusText}</span>
      </div>
      {hasLimit && (
        <div className="space-y-1.5">
          <div className="flex justify-between text-xs text-[var(--text-muted)]">
            <span>{today.calls.toLocaleString()} / {today.limit!.toLocaleString()} calls</span>
            <span>{today.remaining?.toLocaleString() ?? 0} remaining</span>
          </div>
          <UsageBar used={today.calls} limit={today.limit!} />
        </div>
      )}
      <div className="grid grid-cols-2 gap-3 text-sm">
        <div>
          <div className="text-[var(--text-muted)] text-xs">Calls (24h)</div>
          <div className="text-[var(--text)] font-medium">{today.calls.toLocaleString()}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)] text-xs">Success Rate</div>
          <div className="text-[var(--text)] font-medium">{today.calls > 0 ? `${today.successRate.toFixed(1)}%` : '-'}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)] text-xs">Avg Latency</div>
          <div className="text-[var(--text)] font-medium">{today.calls > 0 ? `${Math.round(today.avgLatencyMs)}ms` : '-'}</div>
        </div>
        <div>
          <div className="text-[var(--text-muted)] text-xs">Rate Limit Hits</div>
          <div className="text-[var(--text)] font-medium">{today.rateLimitHits}</div>
        </div>
      </div>
    </div>
  );
}

export function ApiStatusTab({ enabled = true }: { enabled?: boolean }) {
  const { data, error } = useAdminApiUsage({ enabled });
  const errorMessage = error instanceof Error ? error.message : error ? 'Failed to load status' : null;

  return (
    <div className="space-y-4">
      {errorMessage && (
        <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">{errorMessage}</div>
      )}
      {data ? (
        <>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {data.providers.map(p => <ProviderCard key={p.name} provider={p} />)}
          </div>
          <div className="text-xs text-[var(--text-muted)] text-right">
            Updated: {formatAdminDate(data.timestamp)}
          </div>
        </>
      ) : !errorMessage ? (
        <div className="text-center text-[var(--text-muted)] py-8">Loading...</div>
      ) : null}
    </div>
  );
}
