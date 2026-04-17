import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api } from '../../../js/api';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult, MarketSnapshot } from '../../../types/campaigns';
import { formatCents, centsToDollars, dollarsToCents } from '../../utils/formatters';

type CertStatus = 'scanning' | 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed' | 'importing' | 'imported';
type ListingStatus = 'idle' | 'setting-price' | 'listing' | 'listed' | 'list-error';

interface CertRow {
  certNumber: string;
  status: CertStatus;
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  error?: string;
  buyCostCents?: number;
  market?: MarketSnapshot;
  marketLoading?: boolean;
  expanded?: boolean;
  priceInput?: string;
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

  const fetchMarketData = useCallback(async (certNumber: string) => {
    updateCert(certNumber, { marketLoading: true });
    try {
      const result = await api.lookupCert(certNumber);
      updateCert(certNumber, { marketLoading: false, market: result.market });
    } catch {
      updateCert(certNumber, { marketLoading: false });
    }
  }, [updateCert]);

  const resolveInBackground = useCallback(async (certNumber: string) => {
    try {
      const info: ResolveCertResponse = await api.resolveCert(certNumber);
      updateCert(certNumber, {
        status: 'resolved',
        cardName: info.cardName,
      });
    } catch (err) {
      updateCert(certNumber, {
        status: 'failed',
        error: err instanceof Error ? err.message : 'Cert not found',
      });
    }
  }, [updateCert]);

  const handleExpandListing = useCallback((certNumber: string) => {
    setCerts(prev => {
      const next = new Map(prev);
      const row = next.get(certNumber);
      if (!row) return prev;
      const nowExpanded = !row.expanded;
      const priceInput =
        !row.expanded && row.market?.conservativeCents
          ? centsToDollars(row.market.conservativeCents)
          : (row.priceInput ?? '');
      next.set(certNumber, { ...row, expanded: nowExpanded, priceInput, listingError: undefined });
      return next;
    });
  }, []);

  const handleSetPriceAndList = useCallback(async (certNumber: string) => {
    const row = certsRef.current.get(certNumber);
    if (!row?.purchaseId) return;
    const priceCents = dollarsToCents(row.priceInput ?? '');
    if (priceCents <= 0) {
      updateCert(certNumber, { listingError: 'Enter a valid price before listing.' });
      return;
    }
    updateCert(certNumber, { listingStatus: 'setting-price', listingError: undefined });
    try {
      await api.setReviewedPrice(row.purchaseId, priceCents, 'intake');
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
      updateCert(certNumber, { listingStatus: 'listed', expanded: false });
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Listing failed';
      updateCert(certNumber, {
        listingStatus: 'list-error',
        listingError: msg.toLowerCase().includes('stock')
          ? 'DH push pending — check back after sync'
          : msg,
      });
    }
  }, [updateCert]);

  const handlePriceChange = useCallback((certNumber: string, value: string) => {
    updateCert(certNumber, { priceInput: value });
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

      if (result.status === 'existing') {
        updateCert(certNumber, {
          status: 'existing',
          cardName: result.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
          buyCostCents: result.buyCostCents,
        });
        fetchMarketData(certNumber);
      } else if (result.status === 'sold') {
        updateCert(certNumber, {
          status: 'sold',
          cardName: result.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
          buyCostCents: result.buyCostCents,
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
  }, [updateCert, resolveInBackground, fetchMarketData]);

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
      {/* Input */}
      <div className="flex items-center gap-2">
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Scan or type cert number..."
          className="flex-1 rounded border-2 border-blue-500 bg-gray-900 px-3 py-2.5 font-mono text-base text-gray-100 placeholder-gray-500 focus:border-blue-400 focus:outline-none"
          autoFocus
        />
        <span className="text-xs text-gray-500 whitespace-nowrap">↵ Enter</span>
      </div>

      {/* Stats bar */}
      {stats.total > 0 && (
        <div className="flex flex-wrap gap-4 rounded bg-gray-800 px-3 py-2 text-xs">
          <span><span className="text-green-400">●</span> <span className="text-gray-400">{stats.existing} in inventory</span></span>
          <span><span className="text-amber-400">●</span> <span className="text-gray-400">{stats.sold} sold</span></span>
          <span><span className="text-blue-400">●</span> <span className="text-gray-400">{stats.newCerts} new</span></span>
          <span><span className="text-red-400">●</span> <span className="text-gray-400">{stats.failed} failed</span></span>
          <span className="ml-auto text-gray-500">{stats.total} scanned</span>
        </div>
      )}

      {/* Cert rows */}
      {displayRows.length > 0 && (
        <div className="flex flex-col gap-1">
          {displayRows.map(row => (
            <CertRowItem
              key={row.certNumber}
              row={row}
              highlighted={row.certNumber === highlightedCert}
              onReturn={handleReturnToInventory}
              onDismiss={handleDismiss}
              onExpand={handleExpandListing}
              onList={handleSetPriceAndList}
              onPriceChange={handlePriceChange}
            />
          ))}
        </div>
      )}

      {/* Import error */}
      {importError && (
        <div className="rounded border border-red-700 bg-red-900/30 p-3 text-sm text-red-300">
          {importError}
        </div>
      )}

      {/* Staging area for new certs */}
      {resolvedCount > 0 && (
        <div className="rounded border border-dashed border-blue-500 bg-blue-900/10 p-3">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-blue-300">
              {resolvedCount} NEW CERT{resolvedCount > 1 ? 'S' : ''} STAGED
            </span>
            <button
              onClick={handleImportNew}
              disabled={importLoading}
              className="rounded bg-blue-600 px-4 py-1.5 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            >
              {importLoading ? 'Importing...' : `Import ${resolvedCount} New Cert${resolvedCount > 1 ? 's' : ''}`}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function PriceChip({ market, loading }: { market?: MarketSnapshot; loading?: boolean }) {
  if (loading) {
    return <span className="ml-2 text-[10px] text-gray-500 animate-pulse">···</span>;
  }
  if (!market) return null;
  const price = market.conservativeCents ?? market.medianCents ?? market.lastSoldCents;
  if (!price) return null;
  const conf = market.confidence ?? 0;
  const cls =
    conf >= 0.7
      ? 'bg-emerald-900/40 text-emerald-300 border-emerald-800'
      : conf >= 0.4
      ? 'bg-amber-900/40 text-amber-300 border-amber-800'
      : 'bg-gray-800 text-gray-400 border-gray-700';
  return (
    <span
      className={`ml-2 inline-flex items-center rounded border px-1.5 py-0.5 text-[10px] font-medium tabular-nums ${cls}`}
      title={`Last sold: ${formatCents(market.lastSoldCents)} · Median: ${formatCents(market.medianCents)}`}
    >
      {formatCents(price)}
    </span>
  );
}

function PanelStat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[10px] uppercase tracking-wider text-gray-500 mb-0.5">{label}</div>
      <div className="text-xs font-semibold tabular-nums text-gray-200">{value}</div>
    </div>
  );
}

function ListingPanel({
  row,
  onPriceChange,
  onList,
}: {
  row: CertRow;
  onPriceChange: (certNumber: string, value: string) => void;
  onList: (certNumber: string) => void;
}) {
  const { market, priceInput, listingStatus, listingError, certNumber, purchaseId } = row;
  const busy = listingStatus === 'setting-price' || listingStatus === 'listing';
  const listed = listingStatus === 'listed';

  return (
    <div className="bg-gray-800/50 border-t border-gray-700 px-3 py-2.5">
      {market ? (
        <div className="grid grid-cols-2 sm:grid-cols-5 gap-x-4 gap-y-1 mb-2.5 text-[11px]">
          <PanelStat label="Cost" value={row.buyCostCents ? formatCents(row.buyCostCents) : '—'} />
          <PanelStat label="DH" value={formatCents(market.gradePriceCents)} />
          <PanelStat label="CL" value={formatCents(market.clValueCents)} />
          <PanelStat label="MM" value={formatCents(market.sourcePrices?.find(s => s.source === 'MarketMovers')?.priceCents)} />
          <PanelStat label="Last Sold" value={formatCents(market.lastSoldCents)} />
        </div>
      ) : (
        <p className="text-[11px] text-gray-500 mb-2">Market data unavailable — enter price manually.</p>
      )}
      <div className="flex items-center gap-2 mb-2">
        <span className="text-gray-400 text-xs">$</span>
        <input
          type="number"
          min="0"
          step="0.01"
          value={priceInput ?? ''}
          onChange={e => onPriceChange(certNumber, e.target.value)}
          disabled={busy || listed}
          placeholder="0.00"
          className="w-28 rounded border border-gray-600 bg-gray-900 px-2 py-1 font-mono text-xs tabular-nums text-gray-100 focus:border-blue-400 focus:outline-none disabled:opacity-50"
        />
        <button
          onClick={() => onList(certNumber)}
          disabled={busy || listed || !purchaseId}
          className="rounded border border-gray-600 px-3 py-1 text-xs text-gray-300 hover:border-blue-500 hover:text-blue-300 disabled:opacity-40 transition-colors"
        >
          {busy
            ? listingStatus === 'setting-price' ? 'Setting price…' : 'Listing…'
            : listed
            ? '✓ Listed'
            : 'List on DH'}
        </button>
      </div>
      {listingError && <p className="text-[11px] text-red-400">{listingError}</p>}
      {!purchaseId && (
        <p className="text-[11px] text-gray-500">Scan again after import to enable listing.</p>
      )}
    </div>
  );
}

function CertRowItem({
  row,
  highlighted,
  onReturn,
  onDismiss,
  onExpand,
  onList,
  onPriceChange,
}: {
  row: CertRow;
  highlighted?: boolean;
  onReturn: (certNumber: string) => void;
  onDismiss: (certNumber: string) => void;
  onExpand: (certNumber: string) => void;
  onList: (certNumber: string) => void;
  onPriceChange: (certNumber: string, value: string) => void;
}) {
  const statusConfig: Record<CertStatus, { bg: string; border: string; certColor: string; label: string }> = {
    scanning:  { bg: 'bg-gray-800',       border: 'border-gray-600',    certColor: 'text-gray-400',    label: 'Checking...' },
    existing:  { bg: 'bg-emerald-950/30', border: 'border-emerald-800', certColor: 'text-emerald-300', label: '✓ In inventory' },
    sold:      { bg: 'bg-amber-950/30',   border: 'border-amber-700',   certColor: 'text-amber-300',   label: '⚠ Sold' },
    returned:  { bg: 'bg-emerald-950/30', border: 'border-emerald-800', certColor: 'text-emerald-300', label: '✓ Returned' },
    resolving: { bg: 'bg-blue-950/30',    border: 'border-blue-800',    certColor: 'text-blue-300',    label: '⟳ Looking up...' },
    resolved:  { bg: 'bg-blue-950/30',    border: 'border-blue-800',    certColor: 'text-blue-300',    label: '★ New' },
    failed:    { bg: 'bg-red-950/30',     border: 'border-red-800',     certColor: 'text-red-300',     label: '✗ Failed' },
    importing: { bg: 'bg-blue-950/30',    border: 'border-blue-800',    certColor: 'text-blue-300',    label: '⟳ Importing...' },
    imported:  { bg: 'bg-emerald-950/30', border: 'border-emerald-800', certColor: 'text-emerald-300', label: '✓ Imported' },
  };

  const cfg = statusConfig[row.status];
  const canList = (row.status === 'existing' || row.status === 'returned') && !!row.purchaseId;
  const showListedChip = row.listingStatus === 'listed';

  return (
    <div
      className={`rounded border transition-all ${cfg.bg} ${cfg.border}${highlighted ? ' ring-2 ring-yellow-400' : ''}`}
    >
      {/* Row header */}
      <div className="flex items-center justify-between px-3 py-2 text-sm">
        <div className="flex items-center gap-2 min-w-0">
          <span className={`font-mono ${cfg.certColor} min-w-[80px]`}>{row.certNumber}</span>
          <span className={`${cfg.certColor} min-w-[110px] text-xs`}>{cfg.label}</span>
          {row.cardName && <span className="text-gray-400 text-xs truncate">{row.cardName}</span>}
          {row.error && row.status === 'failed' && (
            <span className="text-red-400 text-xs truncate">{row.error}</span>
          )}
          {(row.status === 'existing' || row.status === 'returned') && (
            <PriceChip market={row.market} loading={row.marketLoading} />
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {showListedChip && (
            <span className="inline-flex items-center rounded border border-purple-700 bg-purple-900/40 px-2 py-0.5 text-[10px] text-purple-300">
              ✓ Listed
            </span>
          )}
          {canList && !showListedChip && (
            <button
              onClick={() => onExpand(row.certNumber)}
              className={`rounded border px-2 py-1 text-xs transition-colors ${
                row.expanded
                  ? 'border-blue-500 text-blue-300'
                  : 'border-gray-600 text-gray-300 hover:border-blue-500 hover:text-blue-300'
              }`}
            >
              $ List
            </button>
          )}
          {row.status === 'sold' && (
            <button
              onClick={() => onReturn(row.certNumber)}
              className="rounded bg-amber-600 px-3 py-1 text-xs font-medium text-white hover:bg-amber-500"
            >
              Return to Inventory
            </button>
          )}
          {row.status === 'failed' && (
            <button
              onClick={() => onDismiss(row.certNumber)}
              className="rounded border border-gray-600 px-2 py-1 text-xs text-gray-400 hover:bg-gray-700"
            >
              ✕
            </button>
          )}
          {(row.status === 'resolved' || row.status === 'imported') && (
            <span className="text-[10px] text-gray-600 italic">scan again to list</span>
          )}
        </div>
      </div>

      {/* Listing panel */}
      {row.expanded && (
        <ListingPanel row={row} onPriceChange={onPriceChange} onList={onList} />
      )}
    </div>
  );
}
