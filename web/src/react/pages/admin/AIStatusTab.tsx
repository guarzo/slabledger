import type { AIOperationSummary } from '../../../types/apiStatus';
import PokeballLoader from '../../PokeballLoader';
import { useAIUsage } from '../../queries/useAdminQueries';
import { formatTokens, formatLatency } from '../../utils/formatters';
import { SummaryCard } from './shared';
import { formatAdminDate } from './adminUtils';

function formatCost(cents: number): string {
  if (cents === 0) return '$0';
  return `$${(cents / 100).toFixed(2)}`;
}

const operationLabels: Record<string, string> = {
  digest: 'Weekly Digest',
  campaign_analysis: 'Campaign Analysis',
  liquidation: 'Liquidation Analysis',
  purchase_assessment: 'Purchase Assessment',
  social_caption: 'Social Captions',
  social_suggestion: 'Social Post Suggestions',
};

function OperationRow({ op }: { op: AIOperationSummary }) {
  const label = operationLabels[op.operation] ?? op.operation;
  return (
    <tr className="border-b border-[var(--surface-2)] last:border-b-0">
      <td className="py-2 pr-4 text-sm text-[var(--text)]">{label}</td>
      <td className="py-2 pr-4 text-sm text-[var(--text)] text-right">{op.calls}</td>
      <td className="py-2 pr-4 text-sm text-right">
        <span className={op.errors > 0 ? 'text-[var(--danger)]' : 'text-[var(--text)]'}>{op.errors}</span>
      </td>
      <td className="py-2 pr-4 text-sm text-right">
        <span className={op.successRate < 90 && op.calls > 0 ? 'text-[var(--warning)]' : 'text-[var(--text)]'}>
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

  if (isLoading) return <div className="py-8"><PokeballLoader /></div>;
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

      {/* Summary cards — 5 cards: 2 cols → 3 cols on sm → 5 on lg */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        <SummaryCard
          label="Total Calls (7d)"
          value={summary.totalCalls}
          sub={`${summary.callsLast24h} in last 24h`}
        />
        <SummaryCard
          label="Success Rate"
          value={summary.totalCalls > 0 ? `${summary.successRate.toFixed(1)}%` : '-'}
          sub={summary.rateLimitHits > 0 ? `${summary.rateLimitHits} rate limited` : undefined}
          color={summary.totalCalls > 0 && summary.successRate < 90 ? 'var(--warning)' : undefined}
        />
        <SummaryCard
          label="Total Tokens"
          value={formatTokens(summary.totalTokens)}
          sub={summary.totalTokens > 0 ? `${inputPct}% in / ${outputPct}% out` : undefined}
        />
        <SummaryCard
          label="Avg Latency"
          value={summary.totalCalls > 0 ? formatLatency(summary.avgLatencyMs) : '-'}
          sub={summary.lastCallAt ? `Last: ${formatAdminDate(summary.lastCallAt)}` : 'No calls yet'}
        />
        <SummaryCard
          label="Est. Cost (7d)"
          value={formatCost(summary.totalCostCents)}
        />
      </div>

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
          <p className="text-xs mt-1">AI usage metrics will appear here once the advisor or social caption generation runs.</p>
        </div>
      )}

      {/* Timestamp */}
      <div className="text-xs text-[var(--text-muted)] text-right">
        Updated: {formatAdminDate(data.timestamp)}
      </div>
    </div>
  );
}
