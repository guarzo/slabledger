import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api, isAPIError } from '../../../js/api';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult, MarketSnapshot } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';
import PriceDecisionBar from '../../ui/PriceDecisionBar';
import { buildPriceSources } from '../../ui/priceDecisionHelpers';
import type { PreSelection } from '../../ui/priceDecisionHelpers';

type CertStatus = 'scanning' | 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed' | 'importing' | 'imported';
type ListingStatus = 'setting-price' | 'listing' | 'listed' | 'list-error';

interface CertRow {
  certNumber: string;
  status: CertStatus;
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  error?: string;
  buyCostCents?: number;
  market?: MarketSnapshot;
  listingStatus?: ListingStatus;
  listingError?: string;
}

export default function CardIntakeTab() {
  const [input, setInput] = useState('');
  const [certs, setCerts] = useState<Map<string, CertRow>>(new Map());
  const [importLoading, setImportLoading] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [highlightedCert, setHighlightedCert] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const certsRef = useRef(certs);
  certsRef.current = certs;

  useEffect(() => { inputRef.current?.focus(); }, []);

  const updateCert = useCallback((certNumber: string, updates: Partial<CertRow>) => {
    setCerts(prev => {
      const next = new Map(prev);
      const existing = next.get(certNumber);
      if (existing) {
        next.set(certNumber, { ...existing, ...updates });
      }
      return next;
    });
  }, []);

  const resolveInBackground = useCallback(async (certNumber: string) => {
    try {
      const info: ResolveCertResponse = await api.resolveCert(certNumber);
      updateCert(certNumber, { status: 'resolved', cardName: info.cardName });
    } catch (err) {
      updateCert(certNumber, {
        status: 'failed',
        error: err instanceof Error ? err.message : 'Cert not found',
      });
    }
  }, [updateCert]);

  const handleSetPriceAndList = useCallback(async (certNumber: string, priceCents: number, source: string) => {
    const row = certsRef.current.get(certNumber);
    if (!row?.purchaseId) return;
    if (priceCents <= 0) {
      updateCert(certNumber, { listingError: 'Enter a valid price before listing.' });
      return;
    }
    updateCert(certNumber, { listingStatus: 'setting-price', listingError: undefined });
    try {
      await api.setReviewedPrice(row.purchaseId, priceCents, source);
    } catch (err) {
      updateCert(certNumber, {
        listingStatus: 'list-error',
        listingError: err instanceof Error ? err.message : 'Failed to set price',
      });
      return;
    }
    updateCert(certNumber, { listingStatus: 'listing' });
    try {
      await api.listPurchaseOnDH(row.purchaseId);
      updateCert(certNumber, { listingStatus: 'listed' });
    } catch (err) {
      if (isAPIError(err) && err.status === 409 && err.data?.error === 'Purchase already listed on DH') {
        updateCert(certNumber, { listingStatus: 'listed' });
        return;
      }
      const msg = err instanceof Error ? err.message : 'Listing failed';
      updateCert(certNumber, {
        listingStatus: 'list-error',
        listingError: msg.toLowerCase().includes('stock')
          ? 'DH push pending — check back after sync'
          : msg,
      });
    }
  }, [updateCert]);

  const handleScan = useCallback(async (certNumber: string) => {
    certNumber = certNumber.trim();
    if (!certNumber) return;

    if (certsRef.current.has(certNumber)) {
      setHighlightedCert(certNumber);
      setTimeout(() => setHighlightedCert(prev => prev === certNumber ? null : prev), 1500);
      return;
    }

    setCerts(prev => {
      const next = new Map(prev);
      next.set(certNumber, { certNumber, status: 'scanning' });
      return next;
    });

    try {
      const result: ScanCertResponse = await api.scanCert(certNumber);

      if (result.status === 'existing' || result.status === 'sold') {
        updateCert(certNumber, {
          status: result.status,
          cardName: result.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
          buyCostCents: result.buyCostCents,
          market: result.market,
        });
      } else {
        updateCert(certNumber, { status: 'resolving' });
        resolveInBackground(certNumber);
      }
    } catch (err) {
      updateCert(certNumber, {
        status: 'failed',
        error: err instanceof Error ? err.message : 'Scan failed',
      });
    }
  }, [updateCert, resolveInBackground]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleScan(input);
      setInput('');
    }
  };

  const handleReturnToInventory = async (certNumber: string) => {
    const row = certsRef.current.get(certNumber);
    if (!row?.purchaseId || !row?.campaignId) return;

    updateCert(certNumber, { status: 'scanning' });
    try {
      await api.deleteSale(row.campaignId, row.purchaseId);
      updateCert(certNumber, { status: 'returned', cardName: row.cardName });
    } catch (err) {
      updateCert(certNumber, {
        status: 'sold',
        error: err instanceof Error ? err.message : 'Failed to return',
      });
    }
  };

  const handleDismiss = (certNumber: string) => {
    setCerts(prev => {
      const next = new Map(prev);
      next.delete(certNumber);
      return next;
    });
  };

  const handleImportNew = async () => {
    const resolvedCerts = Array.from(certs.values())
      .filter(c => c.status === 'resolved')
      .map(c => c.certNumber);

    if (resolvedCerts.length === 0) return;

    setImportLoading(true);
    setImportError(null);

    setCerts(prev => {
      const next = new Map(prev);
      for (const cn of resolvedCerts) {
        const row = next.get(cn);
        if (row) next.set(cn, { ...row, status: 'importing' });
      }
      return next;
    });

    try {
      const result: CertImportResult = await api.importCerts(resolvedCerts);

      const failedSet = new Set(result.errors.map(e => e.certNumber));
      setCerts(prev => {
        const next = new Map(prev);
        for (const cn of resolvedCerts) {
          const row = next.get(cn);
          if (!row) continue;
          if (failedSet.has(cn)) {
            const errMsg = result.errors.find(e => e.certNumber === cn)?.error ?? 'Import failed';
            next.set(cn, { ...row, status: 'failed', error: errMsg });
          } else {
            next.set(cn, { ...row, status: 'imported' });
          }
        }
        return next;
      });
    } catch (err) {
      setImportError(err instanceof Error ? err.message : 'Import failed');
      setCerts(prev => {
        const next = new Map(prev);
        for (const cn of resolvedCerts) {
          const row = next.get(cn);
          if (row) next.set(cn, { ...row, status: 'resolved' });
        }
        return next;
      });
    } finally {
      setImportLoading(false);
      inputRef.current?.focus();
    }
  };

  const rows = useMemo(() => Array.from(certs.values()), [certs]);

  const stats = useMemo(() => ({
    existing: rows.filter(r => r.status === 'existing' || r.status === 'returned' || r.status === 'imported').length,
    sold: rows.filter(r => r.status === 'sold').length,
    newCerts: rows.filter(r => r.status === 'resolving' || r.status === 'resolved' || r.status === 'importing').length,
    failed: rows.filter(r => r.status === 'failed').length,
    total: rows.length,
  }), [rows]);

  const resolvedCount = useMemo(() => rows.filter(r => r.status === 'resolved').length, [rows]);
  const displayRows = useMemo(() => [...rows].reverse(), [rows]);

  return (
    <div className="space-y-3">
      {/* Scan input */}
      <div className="flex items-center gap-2">
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Scan or type cert number..."
          className="flex-1 rounded-lg border border-[var(--brand-500)]/60 bg-[var(--surface-1)] px-3 py-2.5 font-mono text-base text-[var(--text)] placeholder:text-[var(--text-muted)] focus:border-[var(--brand-400)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-400)]/40"
          autoFocus
        />
        <span className="text-xs text-[var(--text-muted)] whitespace-nowrap">↵ Enter</span>
      </div>

      {/* Stats bar */}
      {stats.total > 0 && (
        <div className="flex flex-wrap items-center gap-4 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] px-3 py-2 text-xs">
          <StatDot color="var(--success)" label={`${stats.existing} in inventory`} />
          <StatDot color="var(--warning)" label={`${stats.sold} sold`} />
          <StatDot color="var(--brand-400)" label={`${stats.newCerts} new`} />
          <StatDot color="var(--danger)" label={`${stats.failed} failed`} />
          <span className="ml-auto text-[var(--text-muted)]">{stats.total} scanned</span>
        </div>
      )}

      {/* Cert rows */}
      {displayRows.length > 0 && (
        <div className="flex flex-col gap-2">
          {displayRows.map(row => (
            <CertRowItem
              key={row.certNumber}
              row={row}
              highlighted={row.certNumber === highlightedCert}
              onReturn={handleReturnToInventory}
              onDismiss={handleDismiss}
              onList={handleSetPriceAndList}
            />
          ))}
        </div>
      )}

      {/* Import error */}
      {importError && (
        <div className="rounded-lg border border-[var(--danger)]/40 bg-[var(--danger)]/10 p-3 text-sm text-[var(--danger)]">
          {importError}
        </div>
      )}

      {/* Staging area */}
      {resolvedCount > 0 && (
        <div className="rounded-lg border border-dashed border-[var(--brand-500)]/50 bg-[var(--brand-500)]/5 p-3">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--brand-400)]">
              {resolvedCount} new cert{resolvedCount > 1 ? 's' : ''} staged
            </span>
            <button
              onClick={handleImportNew}
              disabled={importLoading}
              className="rounded-lg bg-[var(--brand-500)] px-4 py-1.5 text-xs font-semibold text-white hover:bg-[var(--brand-600)] disabled:opacity-50 transition-colors"
            >
              {importLoading ? 'Importing…' : `Import ${resolvedCount} New`}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function StatDot({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="w-1.5 h-1.5 rounded-full" style={{ background: color }} />
      <span className="text-[var(--text-muted)]">{label}</span>
    </span>
  );
}

const STATUS_STYLE: Record<CertStatus, {
  leftBorder: string;
  pillBg: string;
  pillText: string;
  label: string;
}> = {
  scanning:  { leftBorder: 'var(--surface-3)',  pillBg: 'rgba(255,255,255,0.06)',       pillText: 'var(--text-muted)', label: 'Checking…' },
  existing:  { leftBorder: 'var(--success)',    pillBg: 'rgba(52,211,153,0.12)',        pillText: 'var(--success)',    label: '✓ In inventory' },
  returned:  { leftBorder: 'var(--success)',    pillBg: 'rgba(52,211,153,0.12)',        pillText: 'var(--success)',    label: '✓ Returned' },
  sold:      { leftBorder: 'var(--warning)',    pillBg: 'rgba(251,191,36,0.12)',        pillText: 'var(--warning)',    label: '⚠ Sold' },
  resolving: { leftBorder: 'var(--brand-500)',  pillBg: 'rgba(90,93,232,0.15)',         pillText: 'var(--brand-400)',  label: '⟳ Looking up…' },
  resolved:  { leftBorder: 'var(--brand-500)',  pillBg: 'rgba(90,93,232,0.15)',         pillText: 'var(--brand-400)',  label: '★ New' },
  importing: { leftBorder: 'var(--brand-500)',  pillBg: 'rgba(90,93,232,0.15)',         pillText: 'var(--brand-400)',  label: '⟳ Importing…' },
  imported:  { leftBorder: 'var(--success)',    pillBg: 'rgba(52,211,153,0.12)',        pillText: 'var(--success)',    label: '✓ Imported' },
  failed:    { leftBorder: 'var(--danger)',     pillBg: 'rgba(248,113,113,0.12)',       pillText: 'var(--danger)',     label: '✗ Failed' },
};

function StatusPill({ status }: { status: CertStatus }) {
  const s = STATUS_STYLE[status];
  return (
    <span
      className="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold whitespace-nowrap"
      style={{ background: s.pillBg, color: s.pillText }}
    >
      {s.label}
    </span>
  );
}

function InlinePrice({ market, buyCostCents }: { market?: MarketSnapshot; buyCostCents?: number }) {
  if (!market) return null;
  const price = market.gradePriceCents || market.medianCents || market.lastSoldCents;
  if (!price) return null;
  const delta = buyCostCents && buyCostCents > 0 ? price - buyCostCents : 0;
  const pct = buyCostCents && buyCostCents > 0 ? Math.round((delta / buyCostCents) * 100) : 0;
  const deltaColor = delta > 0 ? 'var(--success)' : delta < 0 ? 'var(--danger)' : 'var(--text-muted)';
  return (
    <span className="inline-flex items-baseline gap-1.5 text-xs tabular-nums">
      <span className="font-semibold text-[var(--text)]">{formatCents(price)}</span>
      {pct !== 0 && (
        <span className="text-[10px]" style={{ color: deltaColor }}>
          {pct > 0 ? '+' : ''}{pct}%
        </span>
      )}
    </span>
  );
}

function CertRowItem({
  row,
  highlighted,
  onReturn,
  onDismiss,
  onList,
}: {
  row: CertRow;
  highlighted?: boolean;
  onReturn: (certNumber: string) => void;
  onDismiss: (certNumber: string) => void;
  onList: (certNumber: string, priceCents: number, source: string) => void;
}) {
  const s = STATUS_STYLE[row.status];
  const canList = (row.status === 'existing' || row.status === 'returned') && !!row.purchaseId;
  const { market, buyCostCents, listingStatus, listingError } = row;
  const busy = listingStatus === 'setting-price' || listingStatus === 'listing';
  const listed = listingStatus === 'listed';

  const sources = canList
    ? buildPriceSources({
        clCents: market?.clValueCents ?? 0,
        dhMidCents: market?.gradePriceCents ?? 0,
        costCents: buyCostCents ?? 0,
        lastSoldCents: market?.lastSoldCents ?? 0,
        mmCents: market?.sourcePrices?.find(p => p.source === 'MarketMovers')?.priceCents ?? 0,
      })
    : [];
  const dhCents = market?.gradePriceCents ?? 0;
  const preSelected: PreSelection = dhCents > 0
    ? { kind: 'source', source: 'dh' }
    : { kind: 'none' };

  return (
    <div
      className={`overflow-hidden rounded-xl border bg-[var(--surface-1)] transition-all ${
        highlighted ? 'border-[var(--warning)] ring-2 ring-[var(--warning)]/30' : 'border-[var(--surface-2)]'
      }`}
      style={{ borderLeft: `3px solid ${s.leftBorder}` }}
    >
      <div className="flex items-center justify-between gap-3 px-4 py-2.5">
        <div className="flex items-center gap-3 min-w-0">
          <span className="font-mono text-sm font-medium text-[var(--text)] tabular-nums">
            {row.certNumber}
          </span>
          <StatusPill status={row.status} />
          {row.cardName && (
            <span className="text-sm text-[var(--text)] truncate">{row.cardName}</span>
          )}
          {row.error && row.status === 'failed' && (
            <span className="text-xs text-[var(--danger)] truncate">{row.error}</span>
          )}
          {(row.status === 'existing' || row.status === 'returned') && (
            <InlinePrice market={row.market} buyCostCents={row.buyCostCents} />
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {row.status === 'sold' && (
            <button
              onClick={() => onReturn(row.certNumber)}
              className="rounded-md bg-[var(--warning)]/15 px-3 py-1.5 text-xs font-semibold text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
            >
              Return
            </button>
          )}
          {row.status === 'failed' && (
            <button
              onClick={() => onDismiss(row.certNumber)}
              aria-label="Dismiss"
              className="rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors"
            >
              ✕
            </button>
          )}
          {(row.status === 'resolved' || row.status === 'imported') && (
            <span className="text-[10px] text-[var(--text-muted)] italic">scan again to list</span>
          )}
        </div>
      </div>

      {canList && (
        <div className="border-t border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.015)] px-4 py-2.5">
          <PriceDecisionBar
            sources={sources}
            preSelected={preSelected}
            recommendedSource="dh"
            costBasisCents={buyCostCents}
            status={listed ? 'accepted' : 'pending'}
            isSubmitting={busy}
            confirmLabel={listingStatus === 'setting-price' ? 'Setting price…' : 'List on DH'}
            onConfirm={(priceCents, source) => onList(row.certNumber, priceCents, source)}
          />
          {listingError && (
            <p className="text-xs text-[var(--danger)] mt-2">{listingError}</p>
          )}
        </div>
      )}
    </div>
  );
}
