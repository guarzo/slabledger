import { useMemo } from 'react';
import PokeballLoader from '../../PokeballLoader';
import { formatCents, formatPct } from '../../utils/formatters';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import StatCard from '../../ui/StatCard';
import { saleChannelLabels } from '../../utils/campaignConstants';
import { useCampaignPNL, useChannelPNL, useFillRate, useDaysToSell } from '../../queries/useCampaignQueries';
import type { ChannelPNL } from '../../../types/campaigns';

function ChannelMobileCard({ ch }: { ch: ChannelPNL }) {
  return (
    <div className="p-3 rounded-xl border border-[var(--surface-2)] transition-colors duration-150 hover:border-[var(--surface-3)]"
      style={{ background: 'var(--glass-bg)', backdropFilter: 'blur(8px)' }}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-medium text-[var(--text)]">{saleChannelLabels[ch.channel] || ch.channel}</span>
        <span className={`text-sm font-semibold tabular-nums ${ch.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {formatCents(ch.netProfitCents)}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        <div><span className="text-[var(--text-muted)]">Sales:</span> <span className="text-[var(--text)]">{ch.saleCount}</span></div>
        <div><span className="text-[var(--text-muted)]">Revenue:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(ch.revenueCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Fees:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(ch.feesCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Avg Days:</span> <span className="text-[var(--text)]">{ch.avgDaysToSell.toFixed(1)}</span></div>
      </div>
    </div>
  );
}

function ChannelDesktopRow({ ch }: { ch: ChannelPNL }) {
  return (
    <tr className="glass-table-row">
      <td className="glass-table-td text-[var(--text)] font-medium">{saleChannelLabels[ch.channel] || ch.channel}</td>
      <td className="glass-table-td text-center text-[var(--text)]">{ch.saleCount}</td>
      <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatCents(ch.revenueCents)}</td>
      <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">{formatCents(ch.feesCents)}</td>
      <td className={`glass-table-td text-right font-semibold tabular-nums ${ch.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
        {formatCents(ch.netProfitCents)}
      </td>
      <td className="glass-table-td text-center text-[var(--text-muted)]">{ch.avgDaysToSell.toFixed(1)}</td>
    </tr>
  );
}

interface OverviewTabProps {
  campaignId: string;
  totalSpent: number;
  totalRevenue: number;
  totalProfit: number;
  sellThrough: string;
  purchaseCount: number;
  saleCount: number;
  unsoldCount: number;
  dailySpendCapCents: number;
  expectedFillRate?: number;
}

export default function OverviewTab({
  campaignId, totalSpent, totalRevenue, totalProfit, sellThrough,
  purchaseCount, saleCount, unsoldCount, dailySpendCapCents, expectedFillRate,
}: OverviewTabProps) {
  const { data: pnl, isLoading: pnlLoading } = useCampaignPNL(campaignId);
  const { data: channelPnl = [], isLoading: channelLoading } = useChannelPNL(campaignId);
  const { data: fillRate = [], isLoading: fillLoading } = useFillRate(campaignId, 30);
  const { data: daysToSell = [], isLoading: dtsLoading } = useDaysToSell(campaignId);
  const isMobile = useMediaQuery('(max-width: 768px)');

  const analyticsLoading = pnlLoading || channelLoading || fillLoading || dtsLoading;
  const maxDaysToSellCount = useMemo(
    () => daysToSell.reduce((max, x) => Math.max(max, x.count), 1),
    [daysToSell]
  );

  const hasPurchases = purchaseCount > 0;
  const sellThroughNum = parseFloat(sellThrough);
  const sellThroughColor = !hasPurchases
    ? undefined
    : sellThroughNum >= 50
      ? 'green'
      : sellThroughNum < 10
        ? 'red'
        : undefined;

  return (
    <div className="space-y-6">
      {/* Stat cards */}
      <div id="tabpanel-overview" role="tabpanel" aria-labelledby="overview" className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <StatCard label="Total Spent" value={formatCents(totalSpent)} />
        <StatCard label="Revenue" value={formatCents(totalRevenue)} />
        <StatCard label="Net Profit" value={formatCents(totalProfit)} color={totalProfit >= 0 ? 'green' : 'red'} large />
        <StatCard
          label="ROI"
          value={pnl && hasPurchases ? formatPct(pnl.roi) : '—'}
          color={pnl && hasPurchases ? (pnl.roi >= 0 ? 'green' : 'red') : undefined}
        />
        <StatCard label="Sell-Through" value={`${sellThrough}%`} color={sellThroughColor} />
        <StatCard label="Cards Purchased" value={String(purchaseCount)} />
        <StatCard label="Cards Sold" value={String(saleCount)} />
        <StatCard label="Unsold" value={String(unsoldCount)} />
        <StatCard
          label="Avg Days to Sell"
          value={pnl && saleCount > 0 ? pnl.avgDaysToSell.toFixed(1) : '—'}
        />
        <StatCard label="Daily Cap" value={formatCents(dailySpendCapCents)} />
      </div>

      {/* Analytics section */}
      {analyticsLoading ? (
        <div className="py-8 text-center"><PokeballLoader /></div>
      ) : (
        <>


          {channelPnl.length > 0 && (
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">By Channel</h3>
              {isMobile ? (
                <div className="space-y-3">
                  {channelPnl.map(ch => <ChannelMobileCard key={ch.channel} ch={ch} />)}
                </div>
              ) : (
                <div className="glass-table">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="glass-table-header">
                        <th className="glass-table-th text-left">Channel</th>
                        <th className="glass-table-th text-center">Sales</th>
                        <th className="glass-table-th text-right">Revenue</th>
                        <th className="glass-table-th text-right">Fees</th>
                        <th className="glass-table-th text-right">Net Profit</th>
                        <th className="glass-table-th text-center">Avg Days</th>
                      </tr>
                    </thead>
                    <tbody>
                      {channelPnl.map(ch => <ChannelDesktopRow key={ch.channel} ch={ch} />)}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}

          {daysToSell.length > 0 && (
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Days to Sell Distribution</h3>
              <div className="glass-table p-4">
                <div className="flex gap-2 items-end h-32">
                  {daysToSell.map(b => {
                    const height = (b.count / maxDaysToSellCount) * 100;
                    return (
                      <div key={b.label} className="flex-1 flex flex-col items-center">
                        <div className="text-xs text-[var(--text)] mb-1 font-medium">{b.count}</div>
                        <div
                          className="w-full rounded-t transition-all duration-300"
                          style={{
                            height: `${Math.max(height, 4)}%`,
                            background: 'linear-gradient(180deg, var(--brand-400), var(--brand-600))',
                          }}
                        />
                        <div className="text-[10px] text-[var(--text-muted)] mt-1.5">{b.label}</div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          )}

          {fillRate.length > 0 && expectedFillRate && (() => {
            const avgFillRatePct = fillRate.reduce((sum, d) => sum + (d.fillRatePct ?? 0), 0) / fillRate.length;
            const actualPct = avgFillRatePct * 100;
            const fillColor = actualPct >= expectedFillRate
              ? 'text-[var(--success)]'
              : actualPct >= expectedFillRate * 0.75
              ? 'text-[var(--warning)]'
              : 'text-[var(--danger)]';
            return (
              <div className="p-3 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
                <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">Fill Rate vs Target</h3>
                <div className="flex items-center gap-4 text-sm">
                  <div><span className="text-[var(--text-muted)]">Actual:</span> <span className={`font-medium ${fillColor}`}>{actualPct.toFixed(1)}%</span></div>
                  <div><span className="text-[var(--text-muted)]">Expected:</span> <span className="text-[var(--text)] font-medium">{expectedFillRate}%</span></div>
                </div>
              </div>
            );
          })()}

          {fillRate.length > 0 && (
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">Daily Spend (Last 30 Days)</h3>
              <div className="glass-table max-h-[400px] overflow-y-auto scrollbar-dark">
                <table className="w-full text-sm">
                  <thead className="sticky top-0 z-10">
                    <tr className="glass-table-header">
                      <th className="glass-table-th text-left">Date</th>
                      <th className="glass-table-th text-right">Spend</th>
                      <th className="glass-table-th text-right">Cap</th>
                      <th className="glass-table-th text-right">Fill Rate</th>
                      <th className="glass-table-th text-center">Cards</th>
                    </tr>
                  </thead>
                  <tbody>
                    {fillRate.map(d => (
                      <tr key={d.date} className="glass-table-row">
                        <td className="glass-table-td text-[var(--text)]">{d.date}</td>
                        <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatCents(d.spendCents)}</td>
                        <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">{formatCents(d.capCents)}</td>
                        <td className={`glass-table-td text-right tabular-nums ${d.fillRatePct > 1 ? 'text-[var(--danger)]' : 'text-[var(--text)]'}`}>
                          {formatPct(d.fillRatePct)}
                        </td>
                        <td className="glass-table-td text-center text-[var(--text-muted)]">{d.purchaseCount}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
