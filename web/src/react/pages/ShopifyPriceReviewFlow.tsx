import { useState, useMemo } from 'react';
import type { ShopifyPriceSyncMatch } from '../../types/campaigns';
import { formatCents } from '../utils/formatters';
import { CardShell, PriceDecisionBar, buildPriceSources, preSelectSource } from '../ui';
import { toTitleCase } from '../utils/formatters';
import { IntelDetail } from './shopify/IntelDetail';
import type { ItemDecision } from './shopify/shopifyTypes';

/* ── ReviewRow ────────────────────────────────────────────────────── */

function ReviewRow({ match, decision, onDecide }: {
  match: ShopifyPriceSyncMatch;
  decision: ItemDecision | undefined;
  onDecide: (d: ItemDecision | undefined) => void;
}) {
  const sources = useMemo(
    () => buildPriceSources({
      clCents: match.clValueCents,
      marketCents: match.marketPriceCents,
      costCents: match.costBasisCents,
      lastSoldCents: match.lastSoldCents,
    }),
    [match.clValueCents, match.marketPriceCents, match.costBasisCents, match.lastSoldCents],
  );

  const reviewedCents = match.recommendedSource === 'user_reviewed' ? match.recommendedPriceCents : undefined;
  const preSelected = useMemo(
    () => preSelectSource(sources, reviewedCents),
    [sources, reviewedCents],
  );

  const [expanded, setExpanded] = useState(false);
  const hasIntel = !!match.intel;

  const status: 'pending' | 'accepted' | 'skipped' =
    decision?.action === 'update' ? 'accepted' :
    decision?.action === 'skip' ? 'skipped' : 'pending';

  // Delta between recommended and current price
  const deltaCents = match.recommendedPriceCents - match.currentPriceCents;
  const deltaPct = match.currentPriceCents > 0
    ? ((deltaCents / match.currentPriceCents) * 100) : 0;
  const isIncrease = deltaCents > 0;

  return (
    <>
      <tr className={`border-b border-[var(--surface-2)]/50 ${
        status === 'accepted' ? 'bg-[var(--success)]/[0.04]' :
        status === 'skipped' ? 'bg-[var(--surface-2)]/30 opacity-50' : ''
      }`}>
        <td className="py-2 px-2">
          <div className="flex items-start gap-1.5">
            {hasIntel && (
              <button
                type="button"
                onClick={() => setExpanded(e => !e)}
                className="mt-0.5 text-[var(--text-muted)] hover:text-[var(--text)] transition-transform"
                style={{ transform: expanded ? 'rotate(90deg)' : 'none' }}
                aria-label={expanded ? 'Collapse details' : 'Expand details'}
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="9 18 15 12 9 6" />
                </svg>
              </button>
            )}
            <div>
              <div className="text-sm font-medium text-[var(--text)]">{toTitleCase(match.cardName)}</div>
              {match.setName && (
                <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wide">
                  {match.setName}{match.cardNumber ? ` #${match.cardNumber}` : ''}
                </div>
              )}
              {match.intel && (
                <div className="flex items-center gap-1.5 mt-1">
                  {match.intel.sentimentTrend && (
                    <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
                      match.intel.sentimentTrend === 'rising'
                        ? 'text-[var(--success)] bg-[var(--success)]/10'
                        : match.intel.sentimentTrend === 'falling'
                          ? 'text-red-400 bg-red-400/10'
                          : 'text-[var(--text-muted)] bg-[var(--surface-2)]'
                    }`}>
                      {match.intel.sentimentTrend === 'rising' ? '\u25B2' : match.intel.sentimentTrend === 'falling' ? '\u25BC' : '\u25CF'}{' '}
                      {match.intel.sentimentTrend.charAt(0).toUpperCase() + match.intel.sentimentTrend.slice(1)}
                      {match.intel.sentimentMentions > 0 && ` (${match.intel.sentimentMentions})`}
                    </span>
                  )}
                  {match.intel.forecastCents > 0 && (
                    <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
                      match.intel.forecastCents > match.currentPriceCents
                        ? 'text-[var(--success)] bg-[var(--success)]/10'
                        : 'text-red-400 bg-red-400/10'
                    }`} title={`Confidence: ${(match.intel.forecastConfidence * 100).toFixed(0)}%`}>
                      {match.intel.forecastCents > match.currentPriceCents ? '\u25B2' : '\u25BC'}{' '}
                      {formatCents(match.intel.forecastCents)}
                    </span>
                  )}
                  {match.intel.recentSalesCount >= 1 && (
                    <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
                      match.intel.recentSalesCount >= 3
                        ? 'text-[var(--success)] bg-[var(--success)]/10'
                        : 'text-yellow-400 bg-yellow-400/10'
                    }`}>
                      {match.intel.recentSalesCount >= 3 ? 'Liquid' : 'Thin'}
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>
        </td>
        <td className="py-2 px-2 text-xs text-center text-[var(--text)]">
          {match.grader ? `${match.grader} ` : ''}{match.grade}
        </td>
        <td className="py-2 px-2 text-right">
          <div className="text-sm font-semibold text-[var(--text)]">{formatCents(match.currentPriceCents)}</div>
          {deltaCents !== 0 && (
            <div className={`text-[11px] font-semibold flex items-center justify-end gap-0.5 ${
              isIncrease ? 'text-[var(--success)]' : 'text-red-400'
            }`}>
              <span className="text-[9px]">{isIncrease ? '\u25B2' : '\u25BC'}</span>
              {isIncrease ? '+' : ''}{formatCents(deltaCents)} ({deltaPct > 0 ? '+' : ''}{deltaPct.toFixed(1)}%)
            </div>
          )}
        </td>
        <td className="py-2 px-2" colSpan={4}>
          <PriceDecisionBar
            sources={sources}
            preSelected={preSelected}
            status={status}
            confirmLabel="Update"
            recommendedSource={match.recommendedSource === 'user_reviewed' ? undefined : match.recommendedSource}
            costBasisCents={match.costBasisCents}
            onConfirm={(priceCents) => onDecide({ action: 'update', priceCents })}
            onSkip={() => onDecide({ action: 'skip' })}
            onReset={() => onDecide(undefined)}
          />
        </td>
      </tr>
      {expanded && match.intel && (
        <tr className="border-b border-[var(--surface-2)]/50">
          <td colSpan={7} className="px-4 py-3 bg-[var(--surface-1)]/50">
            <IntelDetail intel={match.intel} />
          </td>
        </tr>
      )}
    </>
  );
}

/* ── SectionTable ─────────────────────────────────────────────────── */

export function SectionTable({ title, titleColor, items, decisions, onDecide }: {
  title: string;
  titleColor: string;
  items: ShopifyPriceSyncMatch[];
  decisions: Map<string, ItemDecision>;
  onDecide: (certNumber: string, d: ItemDecision | undefined) => void;
}) {
  if (items.length === 0) return null;
  return (
    <div className="mb-6">
      <div className={`text-sm font-semibold mb-2 ${titleColor}`}>
        {title} — {items.length} update{items.length !== 1 ? 's' : ''}
      </div>
      <CardShell variant="default" padding="none">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b-2 border-[var(--surface-2)]">
                <th className="text-left py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Card</th>
                <th className="text-center py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Grade</th>
                <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Store Price</th>
                <th className="text-left py-2 px-2 text-[var(--text-muted)] font-medium text-xs" colSpan={4}>Price Decision</th>
              </tr>
            </thead>
            <tbody>
              {items.map(m => (
                <ReviewRow
                  key={m.certNumber}
                  match={m}
                  decision={decisions.get(m.certNumber)}
                  onDecide={d => onDecide(m.certNumber, d)}
                />
              ))}
            </tbody>
          </table>
        </div>
      </CardShell>
    </div>
  );
}
