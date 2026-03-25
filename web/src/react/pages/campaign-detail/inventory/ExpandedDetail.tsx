import type { AgingItem, ExpectedValue } from '../../../../types/campaigns';
import { formatCents, formatPct } from '../../../utils/formatters';
import { ConfidenceIndicator } from '../../../ui';
import MarketplaceLinks from './MarketplaceLinks';
import { unrealizedPL, fmtDateShort, plColor, formatPL } from './utils';

interface ExpandedDetailProps {
  item: AgingItem;
  ev?: ExpectedValue;
  showCampaignColumn?: boolean;
  deltaPct?: number | null;
}

export default function ExpandedDetail({ item, ev, showCampaignColumn }: ExpandedDetailProps) {
  const snap = item.currentMarket;
  const costBasis = item.purchase.buyCostCents + item.purchase.psaSourcingFeeCents;
  return (
    <div className="glass-vrow-expanded px-6 py-4 border-t border-[rgba(255,255,255,0.05)]">
      {/* Card identity + marketplace links */}
      <div className="flex items-baseline justify-between mb-3">
        <div className="min-w-0">
          <div className="text-sm font-medium text-[var(--text)]">
            {item.purchase.cardName}
            {item.priceAnomaly && (
              <span className="ml-1.5 text-xs text-amber-500" title={item.anomalyReason || 'Pricing may be inaccurate'}>&#9888;</span>
            )}
          </div>
          <div className="text-xs text-[var(--text-muted)]">
            {showCampaignColumn && item.campaignName && <>{item.campaignName} &middot; </>}
            {item.purchase.setName && <>{item.purchase.setName} &middot; </>}
            Cert #{item.purchase.certNumber} &middot; {item.purchase.grader && item.purchase.grader !== 'PSA' ? `${item.purchase.grader} ${item.purchase.gradeValue}` : `PSA ${item.purchase.gradeValue}`}
          </div>
        </div>
        {item.purchase.cardName && item.purchase.setName && (
          <MarketplaceLinks
            cardName={item.purchase.cardName}
            setName={item.purchase.setName}
            cardNumber={item.purchase.cardNumber ?? ''}
            gradeValue={item.purchase.gradeValue}
            variant="expanded"
          />
        )}
      </div>
      {!snap && <div className="text-xs text-[var(--text-muted)] mb-2">No market data available.</div>}
      {snap && <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-xs">
        {/* Left column */}
        <div className="space-y-1.5">
          {/* Per-source prices */}
          {snap.sourcePrices && snap.sourcePrices.length > 0 && (
            <div>
              <div className="font-medium text-[var(--text-muted)] mb-1">Source Prices</div>
              {snap.sourcePrices.map(sp => (
                <div key={sp.source} className="flex items-center gap-2 py-0.5">
                  <span className="text-[var(--text-muted)] w-28">{sp.source}</span>
                  <span className="text-[var(--text)] font-medium">{formatCents(sp.priceCents)}</span>
                  {sp.trend === 'up' && <span className="text-[var(--success)]">&uarr;</span>}
                  {sp.trend === 'down' && <span className="text-[var(--danger)]">&darr;</span>}
                  {sp.confidence && (
                    <ConfidenceIndicator confidence={sp.confidence as 'high' | 'medium' | 'low'} size="sm" />
                  )}
                  {sp.minCents && sp.maxCents && (
                    <span className="text-[var(--text-muted)]">({formatCents(sp.minCents)}-{formatCents(sp.maxCents)})</span>
                  )}
                </div>
              ))}
            </div>
          )}
          {/* 7-day averages */}
          {snap.sourcePrices?.some(sp => sp.avg7DayCents && sp.avg7DayCents > 0) && (
            <div>
              <div className="font-medium text-[var(--text-muted)] mb-1">7-Day Averages</div>
              {snap.sourcePrices.filter(sp => sp.avg7DayCents && sp.avg7DayCents > 0).map(sp => (
                <div key={sp.source} className="flex items-center gap-2 py-0.5">
                  <span className="text-[var(--text-muted)] w-28">{sp.source}</span>
                  <span className="text-[var(--text)]">{formatCents(sp.avg7DayCents!)}</span>
                </div>
              ))}
            </div>
          )}
          {/* Last sold */}
          {snap.lastSoldCents > 0 && (
            <div>
              <span className="text-[var(--text-muted)]">Last sold: </span>
              <span className="text-[var(--text)] font-medium">{formatCents(snap.lastSoldCents)}</span>
              {snap.lastSoldDate && <span className="text-[var(--text-muted)] ml-1">({snap.lastSoldDate})</span>}
            </div>
          )}
          {/* Lowest listing */}
          {snap.lowestListCents ? (
            <div>
              <span className="text-[var(--text-muted)]">Lowest listing: </span>
              <span className="text-[var(--text)] font-medium">{formatCents(snap.lowestListCents)}</span>
              {snap.activeListings ? <span className="text-[var(--text-muted)] ml-1">({snap.activeListings} active)</span> : null}
            </div>
          ) : null}
          {/* Price range (conservative - optimistic) */}
          {snap.conservativeCents != null && snap.conservativeCents > 0 && snap.optimisticCents != null && snap.optimisticCents > 0 && (
            <div>
              <span className="text-[var(--text-muted)]">Price range: </span>
              <span className="text-[var(--text)]">{formatCents(snap.conservativeCents)} - {formatCents(snap.optimisticCents)}</span>
            </div>
          )}
          {/* Card Ladder value */}
          {item.purchase.clValueCents > 0 && (
            <div>
              <span className="text-[var(--text-muted)]">Card Ladder: </span>
              <span className="text-[var(--text)] font-medium">{formatCents(item.purchase.clValueCents)}</span>
            </div>
          )}
        </div>
        {/* Right column */}
        <div className="space-y-1.5">
          {/* Market drift since purchase */}
          {(() => {
            const purchaseMedian = item.purchase.medianCents;
            if (purchaseMedian && purchaseMedian > 0 && snap.medianCents && snap.medianCents > 0) {
              const drift = (snap.medianCents - purchaseMedian) / purchaseMedian;
              const driftDate = item.purchase.snapshotDate ? fmtDateShort(item.purchase.snapshotDate) : item.purchase.purchaseDate ? fmtDateShort(item.purchase.purchaseDate) : null;
              return (
                <div>
                  <span className="text-[var(--text-muted)]">Market drift: </span>
                  <span className={drift > 0 ? 'text-[var(--success)]' : drift < 0 ? 'text-[var(--danger)]' : 'text-[var(--text)]'}>
                    {drift > 0 ? '+' : ''}{(drift * 100).toFixed(0)}% since purchase
                  </span>
                  {driftDate && <span className="text-[var(--text-muted)]"> ({driftDate})</span>}
                </div>
              );
            }
            return null;
          })()}
          {/* Population */}
          {item.purchase.population != null && item.purchase.population > 0 && (
            <div>
              <span className="text-[var(--text-muted)]">Population: </span>
              <span className="text-[var(--text)]">{item.purchase.population.toLocaleString()}</span>
            </div>
          )}
          {/* P10-P90 */}
          {snap.p10Cents && snap.p90Cents && (
            <div>
              <span className="text-[var(--text-muted)]">P10-P90: </span>
              <span className="text-[var(--text)]">{formatCents(snap.p10Cents)} - {formatCents(snap.p90Cents)}</span>
            </div>
          )}
          {/* Volatility */}
          {snap.volatility != null && (
            <div>
              <span className="text-[var(--text-muted)]">Volatility: </span>
              <span className={snap.volatility > 0.3 ? 'text-[var(--danger)]' : snap.volatility > 0.15 ? 'text-[var(--warning)]' : 'text-[var(--success)]'}>
                {(snap.volatility * 100).toFixed(0)}%
              </span>
            </div>
          )}
          {/* 90-day trend */}
          {snap.trend90d != null && snap.trend90d !== 0 && (
            <div>
              <span className="text-[var(--text-muted)]">90d trend: </span>
              <span className={snap.trend90d > 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>
                {snap.trend90d > 0 ? '+' : ''}{(snap.trend90d * 100).toFixed(0)}%
              </span>
            </div>
          )}
          {/* 30-day sales */}
          {snap.salesLast30d != null && snap.salesLast30d > 0 && (
            <div>
              <span className="text-[var(--text-muted)]">30d sales: </span>
              <span className="text-[var(--text)]">{snap.salesLast30d}</span>
              {snap.activeListings ? <span className="text-[var(--text-muted)]"> ({snap.activeListings} listed)</span> : null}
            </div>
          )}
          {/* 90-day sales */}
          {snap.salesLast90d != null && snap.salesLast90d > 0 && (
            <div>
              <span className="text-[var(--text-muted)]">90d sales: </span>
              <span className="text-[var(--text)]">{snap.salesLast90d}</span>
            </div>
          )}
          {/* Unrealized P/L */}
          {(() => {
            const pl = unrealizedPL(costBasis, snap);
            return pl != null ? (
              <div>
                <span className="text-[var(--text-muted)]">Est. P/L: </span>
                <span className={plColor(pl)}>{formatPL(pl)}</span>
              </div>
            ) : null;
          })()}
        </div>
      </div>}
      {/* Price adjustments */}
      {(item.purchase.overridePriceCents || item.purchase.aiSuggestedPriceCents) && (
        <div className="mt-2 space-y-1 text-xs">
          {item.purchase.overridePriceCents && (
            <div>
              <span className="text-[var(--brand-400)]">Override: </span>
              <span className="text-[var(--text)] font-medium">{formatCents(item.purchase.overridePriceCents)}</span>
              {item.purchase.overrideSource && <span className="text-[var(--text-muted)] ml-1">({item.purchase.overrideSource})</span>}
            </div>
          )}
          {item.purchase.aiSuggestedPriceCents && (
            <div>
              <span className="text-purple-400">AI Suggested: </span>
              <span className="text-[var(--text)] font-medium">{formatCents(item.purchase.aiSuggestedPriceCents)}</span>
            </div>
          )}
        </div>
      )}
      {/* Expected Value Breakdown */}
      {ev && (
        <div className="mt-3 pt-3 border-t border-[var(--surface-2)]/50">
          <div className="font-medium text-[var(--text-muted)] mb-2">Expected Value Breakdown</div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-1 text-xs">
            <div>
              <span className="text-[var(--text-muted)]">Sell probability: </span>
              <span className="text-[var(--text)]">{formatPct(ev.sellProbability)}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)]">Expected sale price: </span>
              <span className="text-[var(--text)]">{formatCents(ev.expectedSalePriceCents)}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)]">Expected fees: </span>
              <span className="text-[var(--text)]">{formatCents(ev.expectedFeesCents)}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)]">Expected profit: </span>
              <span className={ev.expectedProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>{formatPL(ev.expectedProfitCents)}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)]">Carrying cost: </span>
              <span className="text-[var(--text)]">{formatCents(ev.carryingCostCents)}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)]">Net EV: </span>
              <span className={ev.evCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}>{formatPL(ev.evCents)}</span>
            </div>
            <div>
              <span className="text-[var(--text-muted)]">Confidence: </span>
              <ConfidenceIndicator confidence={ev.confidence} size="sm" />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
