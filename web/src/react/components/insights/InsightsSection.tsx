import { useState, useMemo } from 'react';
import { Tabs } from 'radix-ui';
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts';
import type { SegmentPerformance, ChannelVelocity, SaleChannel } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';
import { TabNavigation, Section } from '../../ui';
import { usePortfolioInsights, usePortfolioChannelVelocity } from '../../queries/useCampaignQueries';
import { saleChannelLabels, normalizeChannel } from '../../utils/campaignConstants';
import type { Tab } from '../../ui';

type SortField = 'label' | 'purchaseCount' | 'soldCount' | 'sellThroughPct' | 'roi' | 'netProfitCents' | 'avgDaysToSell' | 'bestChannel';
type SortDir = 'asc' | 'desc';

function sortSegments(segments: SegmentPerformance[], field: SortField, dir: SortDir): SegmentPerformance[] {
  return [...segments].sort((a, b) => {
    const av = a[field];
    const bv = b[field];
    if (field === 'bestChannel' || typeof av === 'string' || typeof bv === 'string') {
      const sa = String(av ?? '');
      const sb = String(bv ?? '');
      return dir === 'asc' ? sa.localeCompare(sb) : sb.localeCompare(sa);
    }
    const aNull = av == null;
    const bNull = bv == null;
    if (aNull && bNull) return 0;
    if (aNull) return 1;
    if (bNull) return -1;
    return dir === 'asc' ? (Number(av)) - (Number(bv)) : (Number(bv)) - (Number(av));
  });
}

type TabKey = 'character' | 'grade' | 'era' | 'priceTier' | 'characterGrade' | 'channel';

const insightTabs: readonly Tab<TabKey>[] = [
  { id: 'character', label: 'By Character' },
  { id: 'grade', label: 'By Grade' },
  { id: 'era', label: 'By Era' },
  { id: 'priceTier', label: 'By Price Tier' },
  { id: 'characterGrade', label: 'Char x Grade' },
  { id: 'channel', label: 'By Channel' },
];

const channelColors: Record<string, string> = {
  ebay: 'var(--channel-ebay)', website: 'var(--channel-website)', inperson: 'var(--channel-inperson)',
};

const DEFAULT_ROW_LIMIT = 5;

function VelocityChart({ velocity }: { velocity: ChannelVelocity[] }) {
  const chartData = useMemo(() => {
    return velocity.map(cv => {
      const normalized = normalizeChannel(cv.channel);
      return {
        label: saleChannelLabels[normalized] ?? normalized,
        channel: normalized,
        avgDaysToSell: Number.isFinite(cv.avgDaysToSell) ? Math.round(cv.avgDaysToSell) : null,
        saleCount: cv.saleCount,
        revenueCents: cv.revenueCents,
      };
    });
  }, [velocity]);

  if (chartData.length === 0) return null;

  return (
    <div className="mt-4 pt-4 border-t border-[var(--surface-2)]">
      <div className="text-xs font-medium text-[var(--text-muted)] mb-2">Recovery Velocity by Channel</div>
      <ResponsiveContainer width="100%" height={chartData.length * 30 + 10}>
        <BarChart layout="vertical" data={chartData} margin={{ top: 0, right: 4, bottom: 0, left: 0 }}>
          <XAxis type="number" hide />
          <YAxis type="category" dataKey="label" width={80} tick={{ fontSize: 11, fill: 'var(--text-muted)' }} axisLine={false} tickLine={false} />
          <Tooltip
            contentStyle={{ background: 'var(--surface-2)', border: '1px solid var(--surface-3, #444)', borderRadius: 8, fontSize: 12 }}
            labelStyle={{ color: 'var(--text)', fontWeight: 600 }}
            formatter={(_value: unknown, _name: unknown, props: { payload?: { saleCount: number; revenueCents: number } }) => {
              const d = props.payload;
              if (!d) return [String(_value), ''];
              return [`${d.saleCount} sales \u00b7 ${formatCents(d.revenueCents)}`, 'Avg days'];
            }}
          />
          <Bar dataKey="avgDaysToSell" radius={[0, 4, 4, 0]} barSize={14}>
            {chartData.map(d => (
              <Cell key={d.channel} fill={channelColors[d.channel] ?? '#6b7280'} />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

export default function InsightsSection() {
  const { data: insights, isLoading, error } = usePortfolioInsights();
  const { data: velocity } = usePortfolioChannelVelocity();
  const [sortField, setSortField] = useState<SortField>('netProfitCents');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [expandedTabs, setExpandedTabs] = useState<Set<TabKey>>(new Set());

  const segmentData: Record<string, SegmentPerformance[]> = useMemo(() => {
    if (!insights) return {} as Record<string, SegmentPerformance[]>;
    return {
      character: insights.byCharacter ?? [],
      grade: insights.byGrade ?? [],
      era: insights.byEra ?? [],
      priceTier: insights.byPriceTier ?? [],
      characterGrade: insights.byCharacterGrade ?? [],
    };
  }, [insights]);

  // Smart default tab: tab with most segments that have soldCount > 0, fallback to 'grade'
  const smartDefaultTab = useMemo<TabKey>(() => {
    if (!insights) return 'grade';
    const segmentKeys: TabKey[] = ['character', 'grade', 'era', 'priceTier', 'characterGrade'];
    let bestTab: TabKey = 'grade';
    let bestCount = -1;
    for (const key of segmentKeys) {
      const segments = segmentData[key] ?? [];
      const withSales = segments.filter(s => s.soldCount > 0).length;
      if (withSales > bestCount) {
        bestCount = withSales;
        bestTab = key;
      }
    }
    // If no sales at all, default to 'grade' (more compact than 'character')
    if (bestCount <= 0) return 'grade';
    return bestTab;
  }, [insights, segmentData]);

  const [activeTab, setActiveTab] = useState<TabKey | null>(null);
  const effectiveTab = activeTab ?? smartDefaultTab;

  if (isLoading) return <div className="text-center text-[var(--text-muted)] py-4">Loading insights...</div>;
  if (error && !insights) return <div className="p-3 rounded-lg bg-[var(--danger-bg)] border border-[var(--danger-border)] text-[var(--danger)] text-sm">Failed to load insights</div>;
  if (!insights || insights.dataSummary.totalPurchases === 0) return null;

  function handleSort(field: SortField) {
    if (sortField === field) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortDir('desc');
    }
  }

  const sortIndicator = (field: SortField) => sortField === field ? (sortDir === 'asc' ? ' \u2191' : ' \u2193') : '';
  const ariaSort = (field: SortField): 'ascending' | 'descending' | 'none' =>
    sortField === field ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none';

  // Filter segments: only show rows where purchaseCount > 0
  const getFilteredSegments = (tabKey: TabKey): SegmentPerformance[] => {
    const segments = segmentData[tabKey] ?? [];
    return segments.filter(s => s.purchaseCount > 0);
  };

  const getDisplaySegments = (tabKey: TabKey): { rows: SegmentPerformance[]; total: number; isExpanded: boolean } => {
    const filtered = getFilteredSegments(tabKey);
    const sorted = sortSegments(filtered, sortField, sortDir);
    const isExpanded = expandedTabs.has(tabKey);
    const limited = !isExpanded && sorted.length > DEFAULT_ROW_LIMIT ? sorted.slice(0, DEFAULT_ROW_LIMIT) : sorted;
    return { rows: limited, total: sorted.length, isExpanded };
  };

  const toggleExpand = (tabKey: TabKey) => {
    setExpandedTabs(prev => {
      const next = new Set(prev);
      if (next.has(tabKey)) {
        next.delete(tabKey);
      } else {
        next.add(tabKey);
      }
      return next;
    });
  };

  const { dataSummary } = insights;
  const hasSales = dataSummary.totalSales > 0;

  return (
    <div className="mb-6">
      <h2 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Portfolio Insights</h2>

      {/* Summary strip */}
      <div className="flex flex-wrap gap-3 text-xs text-[var(--text-muted)] mb-3 px-1">
        <span>{dataSummary.totalPurchases} purchases</span>
        <span className="text-[var(--surface-3)]">/</span>
        <span>{dataSummary.totalSales} sold</span>
        <span className="text-[var(--surface-3)]">/</span>
        <span>{dataSummary.campaignsAnalyzed} campaign{dataSummary.campaignsAnalyzed !== 1 ? 's' : ''}</span>
      </div>

      <Tabs.Root value={effectiveTab} onValueChange={v => setActiveTab(v as TabKey)}>
        <TabNavigation tabs={insightTabs} ariaLabel="Insight dimension tabs" />

        <Tabs.Content value="channel">
          <Section title="Channel Performance">
            <div className="glass-table">
              <table className="w-full text-sm">
                <thead>
                  <tr className="glass-table-header">
                    <th className="glass-table-th text-left">Channel</th>
                    <th className="glass-table-th text-right">Sales</th>
                    <th className="glass-table-th text-right">Revenue</th>
                    <th className="glass-table-th text-right">Fees</th>
                    <th className="glass-table-th text-right">Net Profit</th>
                    <th className="glass-table-th text-right">Avg Days</th>
                  </tr>
                </thead>
                <tbody>
                  {(() => {
                    const channels = insights.byChannel ?? [];
                    if (channels.length === 0) {
                      return <tr><td colSpan={6} className="py-4 text-center text-[var(--text-muted)]">No data</td></tr>;
                    }
                    return channels.map(ch => (
                      <tr key={ch.channel} className="glass-table-row">
                        <td className="glass-table-td font-medium text-[var(--text)]">{saleChannelLabels[ch.channel as SaleChannel] ?? ch.channel}</td>
                        <td className="glass-table-td text-right text-[var(--text)]">{ch.saleCount}</td>
                        <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatCents(ch.revenueCents)}</td>
                        <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">{formatCents(ch.feesCents)}</td>
                        <td className={`glass-table-td text-right font-semibold tabular-nums ${ch.netProfitCents == null ? 'text-[var(--text-muted)]' : ch.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                          {ch.netProfitCents == null ? '-' : formatCents(ch.netProfitCents)}
                        </td>
                        <td className="glass-table-td text-right text-[var(--text-muted)]">{typeof ch.avgDaysToSell === 'number' ? `${ch.avgDaysToSell.toFixed(0)}d` : '-'}</td>
                      </tr>
                    ));
                  })()}
                </tbody>
              </table>
            </div>
          </Section>
        </Tabs.Content>

        {(['character', 'grade', 'era', 'priceTier', 'characterGrade'] as const).map(tabKey => {
          const { rows, total, isExpanded } = getDisplaySegments(tabKey);
          const hasNoSalesInTab = rows.length > 0 && rows.every(s => s.soldCount === 0);
          return (
            <Tabs.Content key={tabKey} value={tabKey}>
              <Section title={insightTabs.find(t => t.id === tabKey)?.label ?? ''}>
                <div className="glass-table">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="glass-table-header">
                        <th className="glass-table-th text-left" aria-sort={ariaSort('label')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('label')}>
                            Segment{sortIndicator('label')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('purchaseCount')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('purchaseCount')}>
                            Purchases{sortIndicator('purchaseCount')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('soldCount')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('soldCount')}>
                            Sold{sortIndicator('soldCount')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('sellThroughPct')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('sellThroughPct')}>
                            Sell-Thru{sortIndicator('sellThroughPct')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('avgDaysToSell')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('avgDaysToSell')}>
                            Avg Days{sortIndicator('avgDaysToSell')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('roi')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('roi')}>
                            ROI{sortIndicator('roi')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('netProfitCents')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('netProfitCents')}>
                            Net Profit{sortIndicator('netProfitCents')}
                          </button>
                        </th>
                        <th className="glass-table-th text-right" aria-sort={ariaSort('bestChannel')}>
                          <button type="button" className="cursor-pointer bg-transparent border-none p-0 text-inherit hover:text-[var(--text)] transition-colors" onClick={() => handleSort('bestChannel')}>
                            Best Ch.{sortIndicator('bestChannel')}
                          </button>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((seg, idx) => (
                        <tr key={`${tabKey}-${seg.label}-${idx}`} className="glass-table-row">
                          <td className="glass-table-td font-medium text-[var(--text)]">{seg.label}</td>
                          <td className="glass-table-td text-right text-[var(--text)]">{seg.purchaseCount}</td>
                          <td className="glass-table-td text-right text-[var(--text)]">{seg.soldCount}</td>
                          <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatPct(seg.sellThroughPct)}</td>
                          <td className="glass-table-td text-right text-[var(--text-muted)]">{seg.avgDaysToSell != null ? `${seg.avgDaysToSell.toFixed(0)}d` : '-'}</td>
                          <td className={`glass-table-td text-right font-semibold tabular-nums ${seg.roi != null && Number.isFinite(seg.roi) && seg.roi >= 0 ? 'text-[var(--success)]' : seg.roi != null && Number.isFinite(seg.roi) ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'}`}>
                            {seg.roi != null && Number.isFinite(seg.roi) ? formatPct(seg.roi) : '--'}
                          </td>
                          <td className={`glass-table-td text-right font-semibold tabular-nums ${seg.netProfitCents != null && Number.isFinite(seg.netProfitCents) && seg.netProfitCents >= 0 ? 'text-[var(--success)]' : seg.netProfitCents != null && Number.isFinite(seg.netProfitCents) ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]'}`}>
                            {seg.netProfitCents != null && Number.isFinite(seg.netProfitCents) ? formatCents(seg.netProfitCents) : '--'}
                          </td>
                          <td className="glass-table-td text-right text-[var(--text-muted)]">{seg.bestChannel || '-'}</td>
                        </tr>
                      ))}
                      {rows.length === 0 && (
                        <tr><td colSpan={8} className="py-4 text-center text-[var(--text-muted)]">No data</td></tr>
                      )}
                    </tbody>
                  </table>
                </div>
                {hasNoSalesInTab && !hasSales && (
                  <div className="text-xs text-[var(--text-muted)] text-center mt-2 italic">No sales recorded yet</div>
                )}
                {total > DEFAULT_ROW_LIMIT && (
                  <button
                    type="button"
                    onClick={() => toggleExpand(tabKey)}
                    className="mt-2 text-xs text-[var(--brand-500)] hover:underline w-full text-center"
                  >
                    {isExpanded ? 'Show less' : `Show all ${total}`}
                  </button>
                )}
              </Section>
            </Tabs.Content>
          );
        })}
      </Tabs.Root>

      {/* Recovery Velocity chart */}
      {velocity && velocity.length > 0 && <VelocityChart velocity={velocity} />}
    </div>
  );
}
