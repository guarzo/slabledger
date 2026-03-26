import { useState, useRef, useCallback, useMemo } from 'react';
import { api } from '../../js/api';
import type { ShopifyPriceSyncMatch, ShopifyPriceSyncResponse } from '../../types/campaigns';
import { formatCents, centsToDollars, dollarsToCents } from '../utils/formatters';
import { Button, CardShell, TrendArrow } from '../ui';
import { useToast } from '../contexts/ToastContext';

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

type ItemDecision = { action: 'accept'; priceCents: number } | { action: 'edit'; priceCents: number } | { action: 'skip' };

type Phase = 'upload' | 'review' | 'export';

type ExceptionSeverity = 'danger' | 'warning' | 'info';
type FilterMode = 'review' | 'all' | 'danger' | 'warning' | 'no-data' | 'clean';

interface ExceptionTag {
  label: string;
  severity: ExceptionSeverity;
}

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

/* ── Exception Classification ─────────────────────────────────────── */

function classifyExceptions(m: ShopifyPriceSyncMatch): ExceptionTag[] {
  const tags: ExceptionTag[] = [];

  // Danger conditions
  if (m.marketPriceCents > 0 && m.costBasisCents > 0 && m.marketPriceCents < m.costBasisCents) {
    tags.push({ label: 'Underwater', severity: 'danger' });
  }
  if (m.costBasisCents > 0 && m.currentPriceCents > 0 && m.currentPriceCents < m.costBasisCents) {
    tags.push({ label: 'Below Cost', severity: 'danger' });
  }
  if (m.minimumPriceCents > 0 && m.currentPriceCents > 0 && m.currentPriceCents < m.minimumPriceCents) {
    tags.push({ label: 'Below Min', severity: 'danger' });
  }

  // Warning conditions
  if (m.priceDeltaPct < -0.15) {
    tags.push({ label: 'Large Drop', severity: 'warning' });
  }
  if (!m.hasMarketData) {
    tags.push({ label: 'No Data', severity: 'warning' });
  }

  // Info conditions
  const hasDanger = tags.some(t => t.severity === 'danger');
  if (m.clValueCents > 0 && m.marketPriceCents > 0) {
    const maxVal = Math.max(m.clValueCents, m.marketPriceCents);
    const divergence = Math.abs(m.clValueCents - m.marketPriceCents) / maxVal;
    if (divergence > 0.30) {
      tags.push({ label: 'CL/Mkt Gap', severity: 'info' });
    }
  }
  if (!hasDanger && Math.abs(m.priceDeltaPct) > 0.10) {
    tags.push({ label: 'Needs Update', severity: 'info' });
  }

  return tags;
}

function highestSeverity(tags: ExceptionTag[]): ExceptionSeverity | null {
  if (tags.some(t => t.severity === 'danger')) return 'danger';
  if (tags.some(t => t.severity === 'warning')) return 'warning';
  if (tags.some(t => t.severity === 'info')) return 'info';
  return null;
}

const severityRank: Record<ExceptionSeverity, number> = { danger: 0, warning: 1, info: 2 };

/** Returns true when current/market/CL prices are close enough that no action is needed. */
function isAligned(m: ShopifyPriceSyncMatch): boolean {
  const cur = m.currentPriceCents;
  const mkt = m.marketPriceCents;
  const cl = m.clValueCents;
  if (cur <= 0) return false;

  const within = (a: number, b: number, pct: number) =>
    Math.abs(a - b) / Math.max(a, b) <= pct;

  const hasMkt = m.hasMarketData && mkt > 0;
  const hasCL = cl > 0;

  if (hasMkt && hasCL) return within(cur, mkt, 0.05) && within(cur, cl, 0.05);
  if (hasMkt) return within(cur, mkt, 0.05);
  if (hasCL) return within(cur, cl, 0.05);
  return false;
}

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

/* ── Review Table ─────────────────────────────────────────────────── */

function DeltaBadge({ pct }: { pct: number }) {
  const sign = pct >= 0 ? '+' : '';
  const color = pct > 0 ? 'text-[var(--success)]' : pct < 0 ? 'text-[var(--danger)]' : 'text-[var(--text-muted)]';
  return <span className={`text-xs font-medium ${color}`}>{sign}{(pct * 100).toFixed(1)}%</span>;
}

function ExceptionBadge({ label, severity }: ExceptionTag) {
  const styles: Record<ExceptionSeverity, string> = {
    danger: 'bg-[var(--danger-bg)] text-[var(--danger)] border-[var(--danger-border)]',
    warning: 'bg-[var(--warning-bg)] text-[var(--warning)] border-[var(--warning-border)]',
    info: 'bg-[var(--info-bg)] text-[var(--info)] border-[var(--info-border)]',
  };
  return (
    <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full border ${styles[severity]}`}>
      {label}
    </span>
  );
}

function costColor(m: ShopifyPriceSyncMatch): string {
  if (m.costBasisCents > 0 && m.currentPriceCents > 0 && m.costBasisCents > m.currentPriceCents) return 'text-[var(--danger)]';
  return 'text-[var(--text-muted)]';
}

/** Current price vs market: orange if overpriced, blue if underpriced. */
function currentColor(m: ShopifyPriceSyncMatch): string {
  const mkt = m.marketPriceCents;
  const cur = m.currentPriceCents;
  if (!m.hasMarketData || mkt <= 0 || cur <= 0) return 'text-[var(--text)]';
  if (cur > mkt * 1.10) return 'text-[var(--warning)]';   // priced above market
  if (cur < mkt * 0.90) return 'text-[var(--info)]';      // priced below market
  return 'text-[var(--text)]';
}

/** Market vs current: green if market above you, red if market below you. */
function marketColor(m: ShopifyPriceSyncMatch): string {
  const mkt = m.marketPriceCents;
  const cur = m.currentPriceCents;
  if (!m.hasMarketData || mkt <= 0 || cur <= 0) return 'text-[var(--text-muted)]';
  if (mkt > cur * 1.05) return 'text-[var(--success)]';   // market above → room to raise
  if (mkt < cur * 0.95) return 'text-[var(--danger)]';    // market below → may need to lower
  return 'text-[var(--text-muted)]';
}

/** CL vs market (or current if no market): blue if CL above, orange if CL below. */
function clColor(m: ShopifyPriceSyncMatch): string {
  const cl = m.clValueCents;
  if (cl <= 0) return 'text-[var(--text-muted)]';
  const ref = (m.hasMarketData && m.marketPriceCents > 0) ? m.marketPriceCents : m.currentPriceCents;
  if (ref <= 0) return 'text-[var(--text-muted)]';
  if (cl > ref * 1.10) return 'text-[var(--info)]';       // CL above market
  if (cl < ref * 0.90) return 'text-[var(--warning)]';    // CL below market
  return 'text-[var(--text-muted)]';
}

function ReviewRow({ match, exceptions, decision, onDecide }: {
  match: ShopifyPriceSyncMatch;
  exceptions: ExceptionTag[];
  decision: ItemDecision | undefined;
  onDecide: (d: ItemDecision) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [editValue, setEditValue] = useState('');

  const effectiveAction = decision?.action || 'pending';
  const effectivePrice = decision && decision.action !== 'skip' ? decision.priceCents : match.suggestedPriceCents;

  const severity = highestSeverity(exceptions);
  const borderColor = severity === 'danger' ? 'var(--danger)'
    : severity === 'warning' ? 'var(--warning)'
    : severity === 'info' ? 'var(--info)'
    : 'transparent';

  // Action state background takes precedence over exception tint
  let rowBg = '';
  if (effectiveAction === 'accept' || effectiveAction === 'edit') {
    rowBg = 'bg-[var(--success-bg)]/30';
  } else if (effectiveAction === 'skip') {
    rowBg = 'bg-[var(--surface-2)]/30';
  }

  const exceptionBg = effectiveAction === 'pending'
    ? severity === 'danger' ? 'var(--danger-bg)' : severity === 'warning' ? 'var(--warning-bg)' : undefined
    : undefined;

  const trendDir = match.recommendation === 'rising' ? 'up' as const
    : match.recommendation === 'falling' ? 'down' as const
    : 'stable' as const;

  const suggestedDimmed = !match.hasMarketData;

  return (
    <tr
      className={`border-b border-[var(--surface-2)]/50 ${rowBg}`}
      style={{ borderLeft: `3px solid ${borderColor}`, background: exceptionBg }}
    >
      <td className="py-2 px-2">
        <div className="text-sm font-medium text-[var(--text)]">{match.cardName}</div>
        {match.setName && <div className="text-[10px] text-[var(--text-muted)]">{match.setName}{match.cardNumber ? ` #${match.cardNumber}` : ''}</div>}
        {exceptions.length > 0 && (
          <div className="flex gap-1 flex-wrap mt-0.5">
            {exceptions.map(e => <ExceptionBadge key={e.label} label={e.label} severity={e.severity} />)}
          </div>
        )}
      </td>
      <td className="py-2 px-2 text-xs text-[var(--text-muted)]">{match.certNumber}</td>
      <td className="py-2 px-2 text-xs text-center text-[var(--text)]">{match.grader} {match.grade}</td>
      <td className={`py-2 px-2 text-right text-sm ${currentColor(match)}`}>{formatCents(match.currentPriceCents)}</td>
      <td className={`py-2 px-2 text-right text-xs ${costColor(match)}`}>{match.costBasisCents > 0 ? formatCents(match.costBasisCents) : '—'}</td>
      <td className={`py-2 px-2 text-right text-xs ${marketColor(match)}`}>{match.hasMarketData ? formatCents(match.marketPriceCents) : '—'}</td>
      <td className={`py-2 px-2 text-right text-xs ${clColor(match)}`}>{match.clValueCents > 0 ? formatCents(match.clValueCents) : '—'}</td>
      <td className="py-2 px-2 text-right">
        {editing ? (
          <div className="flex items-center justify-end gap-1">
            <span className="text-xs text-[var(--text-muted)]">$</span>
            <input
              type="number"
              step="0.01"
              className="w-20 px-1.5 py-0.5 text-sm text-right bg-[var(--surface-1)] border border-[var(--surface-2)] rounded"
              value={editValue}
              onChange={e => setEditValue(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') {
                  const cents = dollarsToCents(editValue);
                  if (cents > 0) {
                    onDecide({ action: 'edit', priceCents: cents });
                    setEditing(false);
                  }
                } else if (e.key === 'Escape') {
                  setEditing(false);
                }
              }}
              autoFocus
            />
            <button
              className="text-xs text-[var(--success)] hover:underline"
              onClick={() => {
                const cents = dollarsToCents(editValue);
                if (cents > 0) {
                  onDecide({ action: 'edit', priceCents: cents });
                  setEditing(false);
                }
              }}
            >OK</button>
          </div>
        ) : (
          <div
            className="flex items-center justify-end gap-1"
            title={suggestedDimmed ? 'No market data — suggested price may be inaccurate' : undefined}
          >
            <span className={`text-sm font-medium ${suggestedDimmed ? 'text-[var(--text-muted)]' : 'text-[var(--text)]'}`}>
              {formatCents(effectivePrice)}
            </span>
            <DeltaBadge pct={match.priceDeltaPct} />
            <TrendArrow trend={trendDir} size="sm" />
          </div>
        )}
      </td>
      <td className="py-2 px-2 text-right">
        <div className="flex items-center justify-end gap-1">
          <button
            className={`px-2 py-0.5 text-xs rounded ${effectiveAction === 'accept' ? 'bg-[var(--success)] text-white' : 'bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--success)]/20'}`}
            onClick={() => onDecide({ action: 'accept', priceCents: match.suggestedPriceCents })}
          >Accept</button>
          <button
            className={`px-2 py-0.5 text-xs rounded ${effectiveAction === 'edit' ? 'bg-[var(--brand-500)] text-white' : 'bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--brand-500)]/20'}`}
            onClick={() => { setEditValue(centsToDollars(match.suggestedPriceCents)); setEditing(true); }}
          >Edit</button>
          <button
            className={`px-2 py-0.5 text-xs rounded ${effectiveAction === 'skip' ? 'bg-[var(--text-muted)] text-white' : 'bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--text-muted)]/20'}`}
            onClick={() => onDecide({ action: 'skip' })}
          >Skip</button>
        </div>
      </td>
    </tr>
  );
}

/* ── Filter Toggle ────────────────────────────────────────────────── */

function FilterToggle({ label, count, active, onClick }: { label: string; count: number; active: boolean; onClick: () => void }) {
  return (
    <button
      className={`px-2.5 py-1 text-xs rounded-full transition-colors ${
        active ? 'bg-[var(--brand-500)] text-white' : 'bg-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]'
      }`}
      onClick={onClick}
    >
      {label} ({count})
    </button>
  );
}

/* ── Main Page ────────────────────────────────────────────────────── */

export default function ShopifySyncPage({ embedded = false }: { embedded?: boolean } = {}) {
  const toast = useToast();
  const [phase, setPhase] = useState<Phase>('upload');
  const [loading, setLoading] = useState(false);
  const [filterMode, setFilterMode] = useState<FilterMode>('review');

  // CSV state
  const [parsedCSV, setParsedCSV] = useState<ParsedCSV | null>(null);

  // Match state
  const [matched, setMatched] = useState<ShopifyPriceSyncMatch[]>([]);
  const [unmatched, setUnmatched] = useState<string[]>([]);
  const [noCertCount, setNoCertCount] = useState(0);
  const [decisions, setDecisions] = useState<Map<string, ItemDecision>>(new Map());

  // Classify, count, and sort matches in two passes (classify+count, then sort)
  const { sortedMatches, dangerCount, warningCount, noDataCount, cleanCount, alignedCount } = useMemo(() => {
    let danger = 0, warning = 0, noData = 0, clean = 0, aligned = 0;
    const classified = matched.map(m => {
      const exceptions = classifyExceptions(m);
      const itemAligned = exceptions.length === 0 && isAligned(m);
      if (exceptions.some(e => e.severity === 'danger')) danger++;
      if (exceptions.some(e => e.severity === 'warning')) warning++;
      if (exceptions.some(e => e.label === 'No Data')) noData++;
      if (exceptions.length === 0) clean++;
      if (itemAligned) aligned++;
      return { match: m, exceptions, aligned: itemAligned };
    });

    const sorted = classified.sort((a, b) => {
      const aSev = highestSeverity(a.exceptions);
      const bSev = highestSeverity(b.exceptions);
      const aRank = aSev != null ? severityRank[aSev] : 3;
      const bRank = bSev != null ? severityRank[bSev] : 3;
      if (aRank !== bRank) return aRank - bRank;
      return Math.abs(b.match.priceDeltaPct) - Math.abs(a.match.priceDeltaPct);
    });

    return { sortedMatches: sorted, dangerCount: danger, warningCount: warning, noDataCount: noData, cleanCount: clean, alignedCount: aligned };
  }, [matched]);

  // Reviewable = has exceptions OR not aligned (i.e., prices diverge enough to warrant a look)
  const reviewCount = matched.length - alignedCount;

  // Filter
  const filteredMatches = useMemo(() => {
    switch (filterMode) {
      case 'review': return sortedMatches.filter(c => c.exceptions.length > 0 || !c.aligned);
      case 'danger': return sortedMatches.filter(c => c.exceptions.some(e => e.severity === 'danger'));
      case 'warning': return sortedMatches.filter(c => c.exceptions.some(e => e.severity === 'warning'));
      case 'no-data': return sortedMatches.filter(c => c.exceptions.some(e => e.label === 'No Data'));
      case 'clean': return sortedMatches.filter(c => c.exceptions.length === 0);
      default: return sortedMatches;
    }
  }, [sortedMatches, filterMode]);

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
      setFilterMode('review');
      const formatLabel = csv.format === 'ebay' ? 'eBay' : 'Shopify';
      toast.success(`${formatLabel} CSV: matched ${resp.matched.length} items, ${resp.unmatched.length} unmatched`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to process CSV');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  const setDecision = useCallback((certNumber: string, decision: ItemDecision) => {
    setDecisions(prev => {
      const next = new Map(prev);
      next.set(certNumber, decision);
      return next;
    });
  }, []);

  // Bulk actions always operate on full matched array, not filtered view
  const acceptAll = useCallback(() => {
    const next = new Map(decisions);
    for (const m of matched) {
      next.set(m.certNumber, { action: 'accept', priceCents: m.suggestedPriceCents });
    }
    setDecisions(next);
  }, [matched, decisions]);

  const skipAll = useCallback(() => {
    const next = new Map(decisions);
    for (const m of matched) {
      next.set(m.certNumber, { action: 'skip' });
    }
    setDecisions(next);
  }, [matched, decisions]);

  const reviewedCount = decisions.size;
  const acceptedCount = Array.from(decisions.values()).filter(d => d.action !== 'skip').length;

  const handleExport = useCallback(() => {
    if (!parsedCSV) return;
    const { format, headers, prefixLines, items, priceIdx } = parsedCSV;

    // Build lookup of certNumber -> new price in dollars
    const priceUpdates = new Map<string, string>();
    for (const m of matched) {
      const d = decisions.get(m.certNumber);
      if (d && d.action !== 'skip') {
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
    setFilterMode('review');
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
          <div className="flex flex-wrap items-center gap-4 mb-4 p-3 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
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
                <span className="text-[var(--text-muted)]">{noCertCount} items without certs (pass-through)</span>
              </div>
            )}
            {dangerCount > 0 && (
              <div className="text-sm">
                <span className="text-[var(--danger)] font-medium">{dangerCount}</span>
                <span className="text-[var(--text-muted)]"> need attention</span>
              </div>
            )}
            {warningCount > 0 && (
              <div className="text-sm">
                <span className="text-[var(--warning)] font-medium">{warningCount}</span>
                <span className="text-[var(--text-muted)]"> warnings</span>
              </div>
            )}
            {alignedCount > 0 && (
              <div className="text-sm">
                <span className="text-[var(--text-muted)]">{alignedCount} aligned (hidden)</span>
              </div>
            )}
            <div className="ml-auto text-sm text-[var(--text-muted)]">
              {reviewedCount} of {matched.length} reviewed
            </div>
          </div>

          {/* Filter toggles */}
          <div className="flex items-center gap-1.5 mb-3">
            <FilterToggle label="Review" count={reviewCount} active={filterMode === 'review'} onClick={() => setFilterMode('review')} />
            {dangerCount > 0 && <FilterToggle label="Attention" count={dangerCount} active={filterMode === 'danger'} onClick={() => setFilterMode('danger')} />}
            {warningCount > 0 && <FilterToggle label="Warnings" count={warningCount} active={filterMode === 'warning'} onClick={() => setFilterMode('warning')} />}
            {noDataCount > 0 && <FilterToggle label="No Data" count={noDataCount} active={filterMode === 'no-data'} onClick={() => setFilterMode('no-data')} />}
            <FilterToggle label="Clean" count={cleanCount} active={filterMode === 'clean'} onClick={() => setFilterMode('clean')} />
            <FilterToggle label="All" count={matched.length} active={filterMode === 'all'} onClick={() => setFilterMode('all')} />
          </div>

          {/* Bulk actions */}
          <div className="flex items-center gap-2 mb-3">
            <Button size="sm" variant="success" onClick={acceptAll}>Accept All</Button>
            <Button size="sm" variant="ghost" onClick={skipAll}>Skip All</Button>
            <div className="ml-auto">
              <Button
                size="sm"
                variant="primary"
                disabled={acceptedCount === 0}
                onClick={() => { setPhase('export'); handleExport(); }}
              >
                Export Updated CSV ({acceptedCount} changes)
              </Button>
            </div>
          </div>

          {/* Review table */}
          <CardShell variant="default" padding="none">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b-2 border-[var(--surface-2)]">
                    <th className="text-left py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Card</th>
                    <th className="text-left py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Cert</th>
                    <th className="text-center py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Grade</th>
                    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Current</th>
                    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Cost</th>
                    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Market</th>
                    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">CL</th>
                    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Suggested</th>
                    <th className="text-right py-2 px-2 text-[var(--text-muted)] font-medium text-xs">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredMatches.map(({ match: m, exceptions }) => (
                    <ReviewRow
                      key={m.certNumber}
                      match={m}
                      exceptions={exceptions}
                      decision={decisions.get(m.certNumber)}
                      onDecide={d => setDecision(m.certNumber, d)}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          </CardShell>

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
            <div className="text-sm text-[var(--text-muted)] mb-4">{acceptedCount} prices updated in the exported CSV</div>
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
