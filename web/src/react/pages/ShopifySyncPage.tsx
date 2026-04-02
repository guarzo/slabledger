import { useState, useRef, useCallback, useMemo } from 'react';
import { api } from '../../js/api';
import type { ShopifyPriceSyncMatch, ShopifyPriceSyncResponse } from '../../types/campaigns';
import { formatCents, centsToDollars, dollarsToCents, toTitleCase } from '../utils/formatters';
import { Button, CardShell, PriceDecisionBar, buildPriceSources, preSelectSource } from '../ui';
import { useToast } from '../contexts/ToastContext';

type SyncFilter = 'all' | 'price_drop' | 'price_increase' | 'no_market_data';
type SyncSort = 'delta' | 'value' | 'margin' | 'name';

/* ── Types ────────────────────────────────────────────────────────── */

type CSVFormat = 'shopify' | 'ebay';

interface CSVRow {
  raw: string[];
  certNumber: string;
  grader: string;
  price: string;
  title: string;
}

interface ParsedCSV {
  format: CSVFormat;
  headers: string[];
  prefixLines: string[];  // lines before headers (e.g. eBay info line)
  items: CSVRow[];
  certIdx: number;
  priceIdx: number;
}

type ItemDecision = { action: 'update'; priceCents: number } | { action: 'skip' };

type Phase = 'upload' | 'review' | 'export';

/* ── CSV Parsing ──────────────────────────────────────────────────── */

/** Split a single CSV line respecting quoted fields (RFC 4180). */
function splitCSVLine(line: string): string[] {
  const fields: string[] = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (inQuotes) {
      if (ch === '"') {
        if (i + 1 < line.length && line[i + 1] === '"') {
          current += '"';
          i++; // skip escaped quote
        } else {
          inQuotes = false;
        }
      } else {
        current += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
    } else if (ch === ',') {
      fields.push(current);
      current = '';
    } else {
      current += ch;
    }
  }
  fields.push(current);
  return fields;
}

/** Split CSV text into rows, respecting newlines inside quoted fields (RFC 4180). */
function splitCSVRows(text: string): string[] {
  const rows: string[] = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (inQuotes) {
      if (ch === '"') {
        if (i + 1 < text.length && text[i + 1] === '"') {
          current += '""';
          i++;
        } else {
          inQuotes = false;
          current += ch;
        }
      } else {
        current += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
      current += ch;
    } else if (ch === '\r') {
      if (i + 1 < text.length && text[i + 1] === '\n') {
        i++; // skip \n after \r
      }
      if (current.trim()) rows.push(current);
      current = '';
    } else if (ch === '\n') {
      if (current.trim()) rows.push(current);
      current = '';
    } else {
      current += ch;
    }
  }
  if (current.trim()) rows.push(current);
  return rows;
}

/** Quote a field for CSV output if it contains commas, quotes, or newlines. */
function quoteCSVField(field: string): string {
  if (field.includes(',') || field.includes('"') || field.includes('\n') || field.includes('\r')) {
    return '"' + field.replace(/"/g, '""') + '"';
  }
  return field;
}

/** Find a column index matching any of the candidate names (case-insensitive, trimmed). */
function findColumn(headers: string[], ...candidates: string[]): number {
  const lower = headers.map(h => h.trim().toLowerCase());
  for (const c of candidates) {
    const idx = lower.findIndex(h => h === c || h.includes(c));
    if (idx >= 0) return idx;
  }
  return -1;
}

function detectAndParseCSV(text: string): ParsedCSV {
  // Strip BOM
  const clean = text.replace(/^\uFEFF/, '');
  const rawLines = splitCSVRows(clean);
  if (rawLines.length < 2) {
    return { format: 'shopify', headers: [], prefixLines: [], items: [], certIdx: -1, priceIdx: -1 };
  }

  // Detect eBay format: first line starts with info/version marker
  const isEbay = rawLines[0].startsWith('Info,');
  const prefixLines = isEbay ? [rawLines[0]] : [];
  const headerLine = isEbay ? rawLines[1] : rawLines[0];
  const dataLines = isEbay ? rawLines.slice(2) : rawLines.slice(1);
  const format: CSVFormat = isEbay ? 'ebay' : 'shopify';

  const headers = splitCSVLine(headerLine);
  const rows = dataLines.map(line => splitCSVLine(line));

  // Find cert column: eBay uses "certification number", Shopify uses "cert number" or "cert"
  const certIdx = findColumn(headers, 'certification number', 'cert number', 'cert');
  // Find price column: eBay uses "*startprice", Shopify uses "price"
  const priceIdx = findColumn(headers, '*startprice', 'price');
  // Find title column
  const titleIdx = findColumn(headers, '*title', 'title');
  // Find SKU column
  const skuIdx = findColumn(headers, 'customlabel', 'sku');

  const items: CSVRow[] = [];
  for (const row of rows) {
    let certNumber = certIdx >= 0 ? row[certIdx]?.trim() : '';
    let grader = '';

    if (certNumber) {
      grader = 'PSA';
    } else if (skuIdx >= 0) {
      const sku = row[skuIdx]?.trim() || '';
      const psaMatch = sku.match(/^PSA-(\d+)$/i);
      if (psaMatch) {
        certNumber = psaMatch[1];
        grader = 'PSA';
      }
    }

    items.push({
      raw: row,
      certNumber,
      grader,
      price: priceIdx >= 0 ? row[priceIdx]?.trim() || '' : '',
      title: titleIdx >= 0 ? row[titleIdx]?.trim() || '' : '',
    });
  }

  return { format, headers, prefixLines, items, certIdx, priceIdx };
}

/* ── Helpers ──────────────────────────────────────────────────────── */


/* ── Upload Phase ─────────────────────────────────────────────────── */

function UploadZone({ onFile }: { onFile: (file: File) => void }) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [dragOver, setDragOver] = useState(false);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) onFile(file);
  }, [onFile]);

  return (
    <div
      className={`border-2 border-dashed rounded-xl p-12 text-center cursor-pointer transition-colors ${
        dragOver ? 'border-[var(--brand-500)] bg-[var(--brand-500)]/5' : 'border-[var(--surface-2)] hover:border-[var(--brand-500)]/50'
      }`}
      onDragOver={e => { e.preventDefault(); setDragOver(true); }}
      onDragLeave={() => setDragOver(false)}
      onDrop={handleDrop}
      onClick={() => fileRef.current?.click()}
    >
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="mx-auto mb-4 text-[var(--text-muted)]">
        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
        <polyline points="17 8 12 3 7 8" />
        <line x1="12" y1="3" x2="12" y2="15" />
      </svg>
      <div className="text-sm font-medium text-[var(--text)]">Drop a CSV here or click to browse</div>
      <div className="text-xs text-[var(--text-muted)] mt-1">Supports Shopify product CSV or eBay graded batch export</div>
      <input ref={fileRef} type="file" accept=".csv" className="hidden" onChange={e => {
        const file = e.target.files?.[0];
        if (file) onFile(file);
        e.target.value = '';
      }} />
    </div>
  );
}

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

  const status: 'pending' | 'accepted' | 'skipped' =
    decision?.action === 'update' ? 'accepted' :
    decision?.action === 'skip' ? 'skipped' : 'pending';

  // Delta between recommended and current price
  const deltaCents = match.recommendedPriceCents - match.currentPriceCents;
  const deltaPct = match.currentPriceCents > 0
    ? ((deltaCents / match.currentPriceCents) * 100) : 0;
  const isIncrease = deltaCents > 0;

  return (
    <tr className={`border-b border-[var(--surface-2)]/50 ${
      status === 'accepted' ? 'bg-[var(--success)]/[0.04]' :
      status === 'skipped' ? 'bg-[var(--surface-2)]/30 opacity-50' : ''
    }`}>
      <td className="py-2 px-2">
        <div className="text-sm font-medium text-[var(--text)]">{toTitleCase(match.cardName)}</div>
        {match.setName && (
          <div className="text-[10px] text-[var(--text-muted)] uppercase tracking-wide">
            {match.setName}{match.cardNumber ? ` #${match.cardNumber}` : ''}
          </div>
        )}
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

    // Compute filter counts before filtering
    const counts = {
      all: mismatches.length,
      price_drop: mismatches.filter(m => m.recommendedPriceCents < m.currentPriceCents).length,
      price_increase: mismatches.filter(m => m.recommendedPriceCents > m.currentPriceCents).length,
      no_market_data: mismatches.filter(m => !m.hasMarketData).length,
    };

    // Apply filter
    let filtered = mismatches;
    if (filter === 'price_drop') filtered = mismatches.filter(m => m.recommendedPriceCents < m.currentPriceCents);
    else if (filter === 'price_increase') filtered = mismatches.filter(m => m.recommendedPriceCents > m.currentPriceCents);
    else if (filter === 'no_market_data') filtered = mismatches.filter(m => !m.hasMarketData);

    // Apply sort
    const sortFn = (a: ShopifyPriceSyncMatch, b: ShopifyPriceSyncMatch) => {
      switch (sort) {
        case 'value': return Math.max(b.currentPriceCents, b.recommendedPriceCents) - Math.max(a.currentPriceCents, a.recommendedPriceCents);
        case 'margin': return (b.recommendedPriceCents - b.costBasisCents) - (a.recommendedPriceCents - a.costBasisCents);
        case 'name': return a.cardName.localeCompare(b.cardName);
        default: return Math.abs(b.recommendedPriceCents - b.currentPriceCents) - Math.abs(a.recommendedPriceCents - a.currentPriceCents);
      }
    };

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
        <>
          {/* Summary bar */}
          <div className="flex flex-wrap items-center gap-4 mb-3 p-3 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
            <div className="text-sm">
              <span className="text-[var(--success)] font-medium">{matched.length}</span>
              <span className="text-[var(--text-muted)]"> matched</span>
            </div>
            {unmatched.length > 0 && (
              <div className="text-sm">
                <span className="text-[var(--warning)] font-medium">{unmatched.length}</span>
                <span className="text-[var(--text-muted)]"> unmatched certs</span>
              </div>
            )}
            {noCertCount > 0 && (
              <div className="text-sm">
                <span className="text-[var(--text-muted)]">{noCertCount} without certs</span>
              </div>
            )}
            <div className="ml-auto flex items-center gap-4 text-sm">
              <span className="text-[var(--text-muted)]">
                {updatedCount} of {filterCounts.all} marked
              </span>
              {updatedCount > 0 && (
                <span className={`font-semibold border-l border-[var(--surface-2)] pl-4 ${
                  totalImpactCents >= 0 ? 'text-[var(--success)]' : 'text-red-400'
                }`}>
                  Impact: {totalImpactCents >= 0 ? '+' : ''}{formatCents(totalImpactCents)}
                </span>
              )}
            </div>
          </div>

          {/* Filter / sort toolbar */}
          <div className="flex flex-wrap items-center gap-2 mb-4">
            <span className="text-[10px] font-semibold text-[var(--text-muted)] uppercase tracking-wide mr-1">Show:</span>
            {([
              ['all', `All (${filterCounts.all})`],
              ['price_drop', `Drops (${filterCounts.price_drop})`],
              ['price_increase', `Increases (${filterCounts.price_increase})`],
              ['no_market_data', `No Market (${filterCounts.no_market_data})`],
            ] as [SyncFilter, string][]).map(([key, label]) => (
              <button
                key={key}
                onClick={() => setFilter(key)}
                className={`text-xs px-3 py-1 rounded-md border transition-colors ${
                  filter === key
                    ? 'border-[var(--accent)] bg-[var(--accent)]/10 text-[var(--accent)]'
                    : 'border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--text-muted)]'
                }`}
              >
                {label}
              </button>
            ))}

            <select
              value={sort}
              onChange={e => setSort(e.target.value as SyncSort)}
              className="ml-auto text-xs px-2 py-1 rounded-md border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text-muted)] cursor-pointer"
            >
              <option value="delta">Sort: Largest Delta</option>
              <option value="value">Sort: Highest Value</option>
              <option value="margin">Sort: Most Margin</option>
              <option value="name">Sort: Card Name</option>
            </select>

            <Button size="sm" variant="success" onClick={updateAll}>Update All</Button>
            <Button
              size="sm"
              variant="primary"
              disabled={updatedCount === 0}
              onClick={() => { setPhase('export'); handleExport(); }}
            >
              Export ({updatedCount})
            </Button>
          </div>

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

          {/* Aligned footer */}
          {alignedCount > 0 && (
            <div className="text-center text-sm text-[var(--text-muted)] py-3 mt-2 border-t border-[var(--surface-2)]">
              {alignedCount} card{alignedCount !== 1 ? 's' : ''} already aligned — not shown
            </div>
          )}

          {/* Unmatched section */}
          {unmatched.length > 0 && (
            <details className="mt-4">
              <summary className="text-sm text-[var(--text-muted)] cursor-pointer hover:text-[var(--text)]">
                {unmatched.length} unmatched cert numbers (not found in inventory)
              </summary>
              <div className="mt-2 p-3 bg-[var(--surface-1)] rounded-lg text-xs text-[var(--text-muted)]">
                {unmatched.join(', ')}
              </div>
            </details>
          )}
        </>
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
