import { useState, useCallback, useMemo } from 'react';
import { api } from '../../js/api';
import type { ShopifyPriceSyncMatch, ShopifyPriceSyncResponse } from '../../types/campaigns';
import { formatCents, centsToDollars, dollarsToCents, toTitleCase } from '../utils/formatters';
import { Button, CardShell, PriceDecisionBar, buildPriceSources, preSelectSource } from '../ui';
import { useToast } from '../contexts/ToastContext';
import type { SyncFilter, SyncSort, ParsedCSV, ItemDecision, Phase } from './shopify/shopifyTypes';
import { detectAndParseCSV, quoteCSVField } from './shopify/shopifyCSVParser';
import { computeFilterCounts, applyFilter, getSortFn } from './shopify/shopifyUtils';
import { SyncReviewPhase } from './shopify/SyncReviewPhase';
import { IntelDetail } from './shopify/IntelDetail';
import { UploadZone } from './shopify/UploadZone';


/* ── Review Row ───────────────────────────────────────────────────── */

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

/* ── Section Table ────────────────────────────────────────────────── */

function SectionTable({ title, titleColor, items, decisions, onDecide }: {
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

/* ── Main Page ────────────────────────────────────────────────────── */

export default function ShopifySyncPage({ embedded = false }: { embedded?: boolean } = {}) {
  const toast = useToast();
  const [phase, setPhase] = useState<Phase>('upload');
  const [loading, setLoading] = useState(false);

  // CSV state
  const [parsedCSV, setParsedCSV] = useState<ParsedCSV | null>(null);

  // Match state
  const [matched, setMatched] = useState<ShopifyPriceSyncMatch[]>([]);
  const [unmatched, setUnmatched] = useState<string[]>([]);
  const [noCertCount, setNoCertCount] = useState(0);
  const [decisions, setDecisions] = useState<Map<string, ItemDecision>>(new Map());

  // Sort & filter controls
  const [filter, setFilter] = useState<SyncFilter>('all');
  const [sort, setSort] = useState<SyncSort>('delta');

  // Filter to mismatches, apply user sort/filter, split into two sections
  const { userReviewed, clDerived, alignedCount, filterCounts } = useMemo(() => {
    const mismatches = matched.filter(
      (m) => m.recommendedPriceCents > 0 && m.currentPriceCents !== m.recommendedPriceCents
    );
    const aligned = matched.length - mismatches.length;

    const counts = computeFilterCounts(mismatches);
    const filtered = applyFilter(mismatches, filter);
    const sortFn = getSortFn(sort);

    const reviewed = filtered.filter((m) => m.recommendedSource === 'user_reviewed');
    const cl = filtered.filter((m) => m.recommendedSource !== 'user_reviewed');

    reviewed.sort(sortFn);
    cl.sort(sortFn);

    return { userReviewed: reviewed, clDerived: cl, alignedCount: aligned, filterCounts: counts };
  }, [matched, filter, sort]);

  const allMismatches = useMemo(() => [...userReviewed, ...clDerived], [userReviewed, clDerived]);

  const handleFile = useCallback(async (file: File) => {
    try {
      setLoading(true);
      const text = await file.text();
      const csv = detectAndParseCSV(text);
      if (csv.items.length === 0) {
        toast.error('CSV appears empty');
        setLoading(false);
        return;
      }

      setParsedCSV(csv);

      // Split into cert-bearing and non-cert items
      const withCerts = csv.items.filter(i => i.certNumber);
      const noCerts = csv.items.filter(i => !i.certNumber);
      setNoCertCount(noCerts.length);

      if (withCerts.length === 0) {
        toast.error('No items with cert numbers found in CSV');
        setLoading(false);
        return;
      }

      // Call API
      const apiItems = withCerts.map(i => ({
        certNumber: i.certNumber,
        currentPriceCents: dollarsToCents(i.price),
        grader: i.grader,
      }));

      const resp: ShopifyPriceSyncResponse = await api.shopifyPriceSync(apiItems);
      setMatched(resp.matched);
      setUnmatched(resp.unmatched);
      setPhase('review');
      const formatLabel = csv.format === 'ebay' ? 'eBay' : 'Shopify';
      toast.success(`${formatLabel} CSV: matched ${resp.matched.length} items, ${resp.unmatched.length} unmatched`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to process CSV');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  const setDecisionFor = useCallback((certNumber: string, decision: ItemDecision | undefined) => {
    setDecisions(prev => {
      const next = new Map(prev);
      if (!decision) {
        next.delete(certNumber);
      } else {
        next.set(certNumber, decision);
      }
      return next;
    });
  }, []);

  const updateAll = useCallback(() => {
    const next = new Map(decisions);
    for (const m of allMismatches) {
      const existing = next.get(m.certNumber);
      if (existing?.action === 'skip') continue;
      const sources = buildPriceSources({
        clCents: m.clValueCents,
        marketCents: m.marketPriceCents,
        costCents: m.costBasisCents,
        lastSoldCents: m.lastSoldCents,
      });
      const reviewedCents = m.recommendedSource === 'user_reviewed' ? m.recommendedPriceCents : undefined;
      const pre = preSelectSource(sources, reviewedCents);
      if (pre.kind === 'source') {
        const source = sources.find(s => s.source === pre.source && s.priceCents > 0);
        if (source) {
          next.set(m.certNumber, { action: 'update', priceCents: source.priceCents });
        }
      } else if (pre.kind === 'manual') {
        next.set(m.certNumber, { action: 'update', priceCents: pre.priceCents });
      }
    }
    setDecisions(next);
  }, [allMismatches, decisions]);

  const updatedCount = Array.from(decisions.values()).filter(d => d?.action === 'update').length;

  // Total dollar impact of marked updates
  const totalImpactCents = useMemo(() => {
    let total = 0;
    for (const m of matched) {
      const d = decisions.get(m.certNumber);
      if (d?.action === 'update') {
        total += d.priceCents - m.currentPriceCents;
      }
    }
    return total;
  }, [matched, decisions]);

  const handleExport = useCallback(() => {
    if (!parsedCSV) return;
    const { format, headers, prefixLines, items, priceIdx } = parsedCSV;

    // Build lookup of certNumber -> new price in dollars
    const priceUpdates = new Map<string, string>();
    for (const m of matched) {
      const d = decisions.get(m.certNumber);
      if (d && d.action === 'update') {
        priceUpdates.set(m.certNumber, centsToDollars(d.priceCents));
      }
    }

    const isEbay = format === 'ebay';

    // Build output lines, preserving original format
    const outputLines: string[] = [];

    // Re-add prefix lines (e.g. eBay info line)
    for (const line of prefixLines) {
      outputLines.push(line);
    }

    // Header line
    outputLines.push(headers.map(quoteCSVField).join(','));

    // Data rows with updated prices
    for (const row of items) {
      const outputRow = [...row.raw];

      if (row.certNumber && priceIdx >= 0 && priceUpdates.has(row.certNumber)) {
        outputRow[priceIdx] = priceUpdates.get(row.certNumber)!;
      }

      outputLines.push(outputRow.map(quoteCSVField).join(','));
    }

    const filename = isEbay ? 'ebay_updated_prices.csv' : 'shopify_updated_prices.csv';
    const blob = new Blob([outputLines.join('\n')], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 100);
    toast.success(`Exported CSV with ${priceUpdates.size} updated prices`);
  }, [parsedCSV, matched, decisions, toast]);

  const reset = useCallback(() => {
    setPhase('upload');
    setParsedCSV(null);
    setMatched([]);
    setUnmatched([]);
    setNoCertCount(0);
    setDecisions(new Map());
    setFilter('all');
    setSort('delta');
  }, []);

  const content = (
    <>
      {!embedded && (
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-xl font-bold text-[var(--text)]">Price Sync</h1>
            <p className="text-sm text-[var(--text-muted)]">Update listing prices using market data</p>
          </div>
          {phase !== 'upload' && (
            <Button size="sm" variant="ghost" onClick={reset}>Start Over</Button>
          )}
        </div>
      )}
      {embedded && phase !== 'upload' && (
        <div className="flex justify-end mb-4">
          <Button size="sm" variant="ghost" onClick={reset}>Start Over</Button>
        </div>
      )}

      {/* Upload phase */}
      {phase === 'upload' && (
        <CardShell variant="default" padding="lg">
          {loading ? (
            <div className="flex flex-col items-center py-12">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-[var(--brand-500)] mb-3" />
              <div className="text-sm text-[var(--text-muted)]">Processing CSV and matching inventory...</div>
            </div>
          ) : (
            <UploadZone onFile={handleFile} />
          )}
        </CardShell>
      )}

      {/* Review phase */}
      {phase === 'review' && (
        <SyncReviewPhase
          matchedCount={matched.length}
          unmatchedCount={unmatched.length}
          noCertCount={noCertCount}
          updatedCount={updatedCount}
          totalImpactCents={totalImpactCents}
          filterCounts={filterCounts}
          filter={filter}
          sort={sort}
          onFilterChange={setFilter}
          onSortChange={setSort}
          onUpdateAll={updateAll}
          onExport={() => { setPhase('export'); handleExport(); }}
          alignedCount={alignedCount}
          unmatched={unmatched}
        >
          {/* User-reviewed section */}
          <SectionTable
            title="User-Reviewed Prices"
            titleColor="text-[var(--success)]"
            items={userReviewed}
            decisions={decisions}
            onDecide={setDecisionFor}
          />

          {/* CL-derived section */}
          <SectionTable
            title="Card Ladder Prices — Not Yet Reviewed"
            titleColor="text-[var(--warning)]"
            items={clDerived}
            decisions={decisions}
            onDecide={setDecisionFor}
          />
        </SyncReviewPhase>
      )}

      {/* Export complete */}
      {phase === 'export' && (
        <CardShell variant="default" padding="lg">
          <div className="text-center py-8">
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="mx-auto mb-4 text-[var(--success)]">
              <path d="M22 11.08V12a10 10 0 11-5.93-9.14" />
              <polyline points="22 4 12 14.01 9 11.01" />
            </svg>
            <div className="text-lg font-medium text-[var(--text)] mb-1">Export Complete</div>
            <div className="text-sm text-[var(--text-muted)] mb-4">{updatedCount} prices updated in the exported CSV</div>
            <div className="flex items-center justify-center gap-3">
              <Button size="sm" variant="primary" onClick={handleExport}>Download Again</Button>
              <Button size="sm" variant="ghost" onClick={reset}>Start Over</Button>
            </div>
          </div>
        </CardShell>
      )}
    </>
  );

  if (embedded) return content;

  return (
    <div className="max-w-6xl mx-auto px-4">
      {content}
    </div>
  );
}
