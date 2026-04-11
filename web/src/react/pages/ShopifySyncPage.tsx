import { useState, useCallback, useMemo } from 'react';
import { api } from '../../js/api';
import type { ShopifyPriceSyncMatch, ShopifyPriceSyncResponse } from '../../types/campaigns';
import { centsToDollars, dollarsToCents } from '../utils/formatters';
import { Button, CardShell, buildPriceSources, preSelectSource } from '../ui';
import { useToast } from '../contexts/ToastContext';
import type { SyncFilter, SyncSort, ParsedCSV, ItemDecision, Phase } from './shopify/shopifyTypes';
import { detectAndParseCSV, quoteCSVField } from './shopify/shopifyCSVParser';
import { computeFilterCounts, applyFilter, getSortFn } from './shopify/shopifyUtils';
import { SyncReviewPhase } from './shopify/SyncReviewPhase';
import { UploadZone } from './shopify/UploadZone';
import { SectionTable } from './ShopifyPriceReviewFlow';


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
