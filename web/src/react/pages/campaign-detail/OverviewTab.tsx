import { useEffect, useMemo, useState } from 'react';
import PokeballLoader from '../../PokeballLoader';
import { formatCents, formatDollarsWhole, formatPct } from '../../utils/formatters';
import { useMediaQuery } from '../../hooks/useMediaQuery';
import CardShell from '../../ui/CardShell';
import { saleChannelLabels } from '../../utils/campaignConstants';
import { useCampaignPNL, useChannelPNL, useFillRate, useDaysToSell } from '../../queries/useCampaignQueries';
import type { ChannelPNL } from '../../../types/campaigns';
import CampaignHeroStats from './CampaignHeroStats';

function ChannelMobileCard({ ch }: { ch: ChannelPNL }) {
  return (
    <CardShell variant="data" padding="sm">
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm font-medium text-[var(--text)]">{saleChannelLabels[ch.channel] || ch.channel}</span>
        <span className={`text-sm font-semibold tabular-nums ${ch.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {formatCents(ch.netProfitCents)}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        <div><span className="text-[var(--text-muted)]">Sales:</span> <span className="text-[var(--text)] tabular-nums">{ch.saleCount}</span></div>
        <div><span className="text-[var(--text-muted)]">Revenue:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(ch.revenueCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Fees:</span> <span className="text-[var(--text)] tabular-nums">{formatCents(ch.feesCents)}</span></div>
        <div><span className="text-[var(--text-muted)]">Avg Days:</span> <span className="text-[var(--text)] tabular-nums">{ch.avgDaysToSell.toFixed(1)}</span></div>
      </div>
    </CardShell>
  );
}

function ChannelDesktopRow({ ch }: { ch: ChannelPNL }) {
  return (
    <tr className="glass-table-row">
      <td className="glass-table-td text-[var(--text)] font-medium">{saleChannelLabels[ch.channel] || ch.channel}</td>
      <td className="glass-table-td text-center text-[var(--text)] tabular-nums">{ch.saleCount}</td>
      <td className="glass-table-td text-right text-[var(--text)] tabular-nums">{formatCents(ch.revenueCents)}</td>
      <td className="glass-table-td text-right text-[var(--text-muted)] tabular-nums">{formatCents(ch.feesCents)}</td>
      <td className={`glass-table-td text-right font-semibold tabular-nums ${ch.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
        {formatCents(ch.netProfitCents)}
      </td>
      <td className="glass-table-td text-center text-[var(--text-muted)] tabular-nums">{ch.avgDaysToSell.toFixed(1)}</td>
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
  const [showAnalytics, setShowAnalytics] = useState(false);

  // Collapse the analytics disclosure when navigating to a different
  // campaign — without this, useState retains the previous campaign's
  // open/closed state.
  useEffect(() => {
    setShowAnalytics(false);
  }, [campaignId]);

  // Visible loading gates the channel section (always shown). Deep loading
  // gates the histogram / fill-rate / daily-spend sections, which only
  // render when the disclosure is expanded — splitting these means the
  // channel breakdown surfaces immediately when its query resolves rather
  // than waiting on the slower aggregations.
  const visibleLoading = pnlLoading || channelLoading;
  const deepLoading = fillLoading || dtsLoading;
  const maxDaysToSellCount = useMemo(
    () => daysToSell.reduce((max, x) => Math.max(max, x.count), 1),
    [daysToSell]
  );

  const hasDeepAnalytics = daysToSell.length > 0 || fillRate.length > 0;

  return (
    <div
      id="tabpanel-overview"
      role="tabpanel"
      aria-labelledby="overview"
      className="space-y-6"
    >
      <CampaignHeroStats
        totalSpentCents={totalSpent}
        totalProfitCents={totalProfit}
        totalRevenueCents={totalRevenue}
        roi={pnl?.roi ?? null}
        purchaseCount={purchaseCount}
        saleCount={saleCount}
        sellThroughPct={sellThrough}
        avgDaysToSell={pnl?.avgDaysToSell ?? null}
      />

      <div className="flex flex-wrap gap-x-6 gap-y-2 px-1 text-sm text-[var(--text-muted)]">
        <span><span className="text-[var(--text)] font-medium tabular-nums">{saleCount}</span> sold</span>
        <span><span className="text-[var(--text)] font-medium tabular-nums">{unsoldCount}</span> unsold</span>
        <span>Daily cap <span className="text-[var(--text)] font-medium tabular-nums">{formatDollarsWhole(dailySpendCapCents)}</span></span>
      </div>

      {/* Analytics section */}
      {visibleLoading ? (
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

          {(hasDeepAnalytics || deepLoading) && (
            <div>
              <button
                type="button"
                onClick={() => setShowAnalytics(s => !s)}
                aria-expanded={showAnalytics}
                aria-controls="deep-analytics"
                disabled={!hasDeepAnalytics && deepLoading}
                className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] hover:text-[var(--text)] focus:outline-none focus-visible:text-[var(--text)] inline-flex items-center gap-1.5 py-1 disabled:opacity-60 disabled:cursor-wait"
              >
                <span aria-hidden className="inline-block transition-transform" style={{ transform: showAnalytics ? 'rotate(90deg)' : 'rotate(0deg)' }}>›</span>
                {showAnalytics ? 'Hide analytics' : deepLoading && !hasDeepAnalytics ? 'Loading analytics…' : 'View analytics'}
              </button>
            </div>
          )}

          {showAnalytics && (
          <div id="deep-analytics" className="space-y-6">
          {deepLoading && !hasDeepAnalytics && (
            <div className="py-6 text-center"><PokeballLoader /></div>
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
                          className="w-full rounded-t"
                          style={{
                            height: `${Math.max(height, 4)}%`,
                            background: 'var(--brand-500)',
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

          {fillRate.length > 0 && expectedFillRate != null && (() => {
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
          </div>
          )}
        </>
      )}
    </div>
  );
}
