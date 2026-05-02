import type { AIOperationSummary } from '../../../types/apiStatus';
import PokeballLoader from '../../PokeballLoader';
import { useAIUsage } from '../../queries/useAdminQueries';
import { formatTokens, formatLatency } from '../../utils/formatters';
import { formatAdminDate } from './adminUtils';

function formatCost(cents: number): string {
  if (cents === 0) return '$0';
  return `$${(cents / 100).toFixed(2)}`;
}

function SupportStat({ label, value, caption, color }: { label: string; value: string; caption?: string; color?: string }) {
  return (
    <div className="flex flex-col gap-1 min-w-[80px]">
      <div className="text-[10px] font-medium uppercase tracking-[0.08em] text-[var(--text-muted)]">{label}</div>
      <div className="text-lg font-bold tabular-nums" style={color ? { color } : { color: 'var(--text)' }}>{value}</div>
      {caption && <div className="text-[10px] text-[var(--text-muted)] tracking-[0.06em] opacity-75">{caption}</div>}
    </div>
  );
}

const operationLabels: Record<string, string> = {
  digest: 'Weekly Digest',
  campaign_analysis: 'Campaign Analysis',
  liquidation: 'Liquidation Analysis',
};

function OperationRow({ op }: { op: AIOperationSummary }) {
  const label = operationLabels[op.operation] ?? op.operation;
  return (
    <tr className="border-b border-[var(--surface-2)] last:border-b-0">
      <td className="py-2 pr-4 text-sm text-[var(--text)]">{label}</td>
      <td className="py-2 pr-4 text-sm text-[var(--text)] text-right">{op.calls}</td>
      <td className="py-2 pr-4 text-sm text-right">
        <span className={op.errors > 0 ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'}>{op.errors}</span>
      </td>
      <td className="py-2 pr-4 text-sm text-right">
        <span className={op.calls > 0 ? (op.successRate < 50 ? 'text-[var(--danger)]' : op.successRate < 90 ? 'text-[var(--warning)]' : 'text-[var(--text)]') : 'text-[var(--text)]'}>
          {op.calls > 0 ? `${op.successRate.toFixed(0)}%` : '-'}
        </span>
      </td>
      <td className="py-2 pr-4 text-sm text-[var(--text)] text-right">{op.calls > 0 ? formatLatency(op.avgLatencyMs) : '-'}</td>
      <td className="py-2 pr-4 text-sm text-[var(--text)] text-right">{formatTokens(op.totalTokens)}</td>
      <td className="py-2 text-sm text-[var(--text)] text-right">{op.calls > 0 ? formatCost(op.totalCostCents) : '-'}</td>
    </tr>
  );
}

export function AIStatusTab({ enabled = true }: { enabled?: boolean }) {
  const { data, error, isLoading } = useAIUsage({ enabled });

  if (isLoading) {
    return (
      <div className="py-8" role="status" aria-live="polite" aria-atomic="true">
        <span className="sr-only">Loading AI usage stats…</span>
        <PokeballLoader />
      </div>
    );
  }
  if (error && !data) return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">Failed to load AI usage stats</div>;
  if (!data) return null;

  if (!data.configured) {
    return (
      <div className="text-center py-8 text-[var(--text-muted)]">
        <p className="text-sm">AI tracking is not configured.</p>
        <p className="text-xs mt-1">Set AZURE_AI_ENDPOINT and AZURE_AI_API_KEY to enable AI features.</p>
      </div>
    );
  }

  const { summary, operations } = data;
  const inputPct = summary.totalTokens > 0 ? ((summary.totalInputTokens / summary.totalTokens) * 100).toFixed(0) : '0';
  const outputPct = summary.totalTokens > 0 ? ((summary.totalOutputTokens / summary.totalTokens) * 100).toFixed(0) : '0';

  return (
    <div className="space-y-6">
      {error && data && (
        <div className="p-3 rounded-lg bg-[var(--warning-bg)] border border-[var(--warning-border)] text-[var(--warning)] text-sm">
          Failed to refresh stats — showing cached data
        </div>
      )}

      {/* Summary as typography row: Success Rate (hero) + supporting stats with hairline divider. */}
      <section
        className="flex flex-wrap items-end gap-x-7 gap-y-3 pb-5 border-b border-[var(--surface-2)]/40"
        aria-label="AI usage summary"
      >
        <div className="flex flex-col gap-1.5">
          <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--brand-400)]">Success Rate</div>
          <div
            className="text-[clamp(28px,3.5vw,40px)] font-extrabold leading-none tabular-nums"
            style={{
              color: summary.totalCalls > 0
                ? summary.successRate >= 90
                  ? 'var(--success)'
                  : summary.successRate >= 75
                    ? 'var(--warning)'
                    : 'var(--danger)'
                : 'var(--text-muted)',
            }}
          >
            {summary.totalCalls > 0 ? `${summary.successRate.toFixed(1)}%` : '—'}
          </div>
          {summary.rateLimitHits > 0 && (
            <div className="text-[11px] text-[var(--text-muted)] tabular-nums">{summary.rateLimitHits} rate limited</div>
          )}
        </div>
        <div className="w-px self-stretch bg-[rgba(255,255,255,0.05)] my-1" aria-hidden />
        <div className="flex flex-wrap items-end gap-x-8 gap-y-3 pb-1">
          <SupportStat
            label="Total Calls (7d)"
            value={String(summary.totalCalls)}
            caption={`${summary.callsLast24h} in last 24h`}
          />
          <SupportStat
            label="Total Tokens"
            value={formatTokens(summary.totalTokens)}
            caption={summary.totalTokens > 0 ? `${inputPct}% in / ${outputPct}% out` : undefined}
          />
          <SupportStat
            label="Avg Latency"
            value={summary.totalCalls > 0 ? formatLatency(summary.avgLatencyMs) : '—'}
            caption={summary.lastCallAt ? `Last: ${formatAdminDate(summary.lastCallAt)}` : 'No calls yet'}
            color={
              summary.totalCalls > 0
                ? summary.avgLatencyMs > 60000
                  ? 'var(--danger)'
                  : summary.avgLatencyMs > 30000
                    ? 'var(--warning)'
                    : undefined
                : undefined
            }
          />
          <SupportStat
            label="Est. Cost (7d)"
            value={formatCost(summary.totalCostCents)}
          />
        </div>
      </section>

      {/* Operations breakdown */}
      {operations.length > 0 && (
        <div className="space-y-3">
          <h3 className="text-sm font-medium text-[var(--text)]">By Operation</h3>
          <div className="rounded-xl bg-[var(--surface-1)] border border-[var(--surface-2)] overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-[var(--surface-2)] bg-[var(--surface-2)]">
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-left">Operation</th>
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-right">Calls</th>
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-right">Errors</th>
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-right">Success</th>
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-right">Latency</th>
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-right">Tokens</th>
                  <th className="py-2 px-4 text-xs font-medium text-[var(--text-muted)] text-right">Cost</th>
                </tr>
              </thead>
              <tbody className="px-4">
                {operations.map(op => <OperationRow key={op.operation} op={op} />)}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Empty state */}
      {summary.totalCalls === 0 && (
        <div className="text-center py-8 text-[var(--text-muted)]">
          <p className="text-sm">No AI calls recorded yet.</p>
          <p className="text-xs mt-1">AI usage metrics will appear here once the advisor runs.</p>
        </div>
      )}

      {/* Timestamp */}
      <div className="text-xs text-[var(--text-muted)] text-right">
        Updated: {formatAdminDate(data.timestamp)}
      </div>
    </div>
  );
}
