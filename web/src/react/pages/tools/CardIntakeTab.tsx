import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api, isAPIError } from '../../../js/api';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult, MarketSnapshot } from '../../../types/campaigns';
import { formatCents } from '../../utils/formatters';
import PriceDecisionBar from '../../ui/PriceDecisionBar';
import { buildPriceSources } from '../../ui/priceDecisionHelpers';
import type { PreSelection } from '../../ui/priceDecisionHelpers';
import FixDHMatchDialog from '../campaign-detail/inventory/FixDHMatchDialog';

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
  /** dhCardId is known once DH has matched the cert. */
  dhCardId?: number;
  /** Unix ms of when the row was first added — used to display "syncing…Xs" elapsed. */
  firstScanAt?: number;
}

const STORAGE_KEY_PREFIX = 'intake:queue:';
function storageKey(): string {
  // Use the operator's local date so the queue doesn't roll over at UTC midnight
  // (e.g., mid-afternoon in Pacific time).
  const d = new Date();
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${STORAGE_KEY_PREFIX}${yyyy}-${mm}-${dd}`;
}

function loadQueue(): Map<string, CertRow> {
  try {
    const raw = localStorage.getItem(storageKey());
    if (!raw) return new Map();
    const entries: [string, CertRow][] = JSON.parse(raw);
    const cleaned = entries.map(([k, v]): [string, CertRow] => [
      k,
      // Drop transient mid-flight statuses on reload so they get re-driven by polling.
      v.status === 'scanning' || v.status === 'importing'
        ? { ...v, status: v.purchaseId ? 'existing' : 'resolving' }
        : v,
    ]);
    return new Map(cleaned);
  } catch {
    return new Map();
  }
}

function saveQueue(certs: Map<string, CertRow>) {
  try {
    if (certs.size === 0) {
      localStorage.removeItem(storageKey());
      return;
    }
    localStorage.setItem(storageKey(), JSON.stringify(Array.from(certs.entries())));
  } catch {
    // quota exceeded or disabled — non-fatal
  }
}

function hasDHMatch(row: CertRow): boolean {
  return (row.dhCardId ?? 0) > 0 || (row.market?.gradePriceCents ?? 0) > 0;
}

function hasCLPrice(row: CertRow): boolean {
  return (row.market?.clValueCents ?? 0) > 0;
}

function rowIsListable(row: CertRow): boolean {
  return !!row.purchaseId && hasDHMatch(row) && hasCLPrice(row);
}

function rowAwaitingSync(row: CertRow): boolean {
  if (row.listingStatus === 'listed') return false;
  if (row.status === 'failed' || row.status === 'sold') return false;
  if (row.status === 'resolving') return true;
  if ((row.status === 'existing' || row.status === 'returned' || row.status === 'imported') && !rowIsListable(row)) {
    return true;
  }
  return false;
}

export default function CardIntakeTab() {
  const [input, setInput] = useState('');
  const [certs, setCerts] = useState<Map<string, CertRow>>(() => loadQueue());
  const [importLoading, setImportLoading] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [highlightedCert, setHighlightedCert] = useState<string | null>(null);
  const [fixMatchTarget, setFixMatchTarget] = useState<CertRow | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const certsRef = useRef(certs);
  certsRef.current = certs;
  // Tracks certs with an in-flight scanCert poll so we don't stack concurrent calls.
  const inflightPollsRef = useRef<Set<string>>(new Set());

  useEffect(() => { inputRef.current?.focus(); }, []);

  // Persist queue to localStorage whenever it changes.
  useEffect(() => { saveQueue(certs); }, [certs]);

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

  const applyScanResult = useCallback((certNumber: string, result: ScanCertResponse) => {
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
    }
  }, [updateCert]);

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

  const pollCert = useCallback(async (certNumber: string) => {
    if (inflightPollsRef.current.has(certNumber)) return;
    inflightPollsRef.current.add(certNumber);
    try {
      const result: ScanCertResponse = await api.scanCert(certNumber);
      if (result.status === 'existing' || result.status === 'sold') {
        // Don't regress from imported → existing; just refresh the market snapshot.
        const current = certsRef.current.get(certNumber);
        const preserveStatus = current?.status === 'imported' ? 'imported' : result.status;
        updateCert(certNumber, {
          status: preserveStatus,
          cardName: result.cardName ?? current?.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
          buyCostCents: result.buyCostCents,
          market: result.market,
        });
      }
      // For 'new', backend is still working — nothing to update.
    } catch {
      // Transient poll failure — next tick will retry.
    } finally {
      inflightPollsRef.current.delete(certNumber);
    }
  }, [updateCert]);

  // Rehydrate: on mount, kick polling for any rows that need it, and fire resolve for stale 'resolving'.
  useEffect(() => {
    const current = certsRef.current;
    for (const row of current.values()) {
      if (row.status === 'resolving') {
        void resolveInBackground(row.certNumber);
      } else if (rowAwaitingSync(row)) {
        void pollCert(row.certNumber);
      }
    }
    // mount-only
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Single polling loop: every 4s, refresh rows awaiting sync.
  useEffect(() => {
    if (certs.size === 0) return;
    const interval = window.setInterval(() => {
      for (const row of certsRef.current.values()) {
        if (rowAwaitingSync(row)) {
          void pollCert(row.certNumber);
        }
      }
    }, 4000);
    return () => window.clearInterval(interval);
  }, [certs.size, pollCert]);

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
      next.set(certNumber, { certNumber, status: 'scanning', firstScanAt: Date.now() });
      return next;
    });

    try {
      const result: ScanCertResponse = await api.scanCert(certNumber);
      applyScanResult(certNumber, result);
      if (result.status === 'new') {
        void resolveInBackground(certNumber);
      }
    } catch (err) {
      updateCert(certNumber, {
        status: 'failed',
        error: err instanceof Error ? err.message : 'Scan failed',
      });
    }
  }, [applyScanResult, updateCert, resolveInBackground]);

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

  const handleClearCompleted = () => {
    // Keep 'sold' rows: they still have a "Return to inventory" recovery action.
    setCerts(prev => {
      const next = new Map(prev);
      for (const [k, row] of next) {
        if (row.listingStatus === 'listed' || row.status === 'failed') {
          next.delete(k);
        }
      }
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
      // Nudge polling to fetch fresh data for newly imported rows.
      for (const cn of resolvedCerts) {
        if (!failedSet.has(cn)) void pollCert(cn);
      }
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

  const batchStats = useMemo(() => {
    let ready = 0;
    let syncing = 0;
    let listed = 0;
    let failed = 0;
    for (const r of rows) {
      if (r.listingStatus === 'listed') listed++;
      else if (r.status === 'failed' || r.status === 'sold') failed++;
      else if (rowIsListable(r)) ready++;
      else if (rowAwaitingSync(r)) syncing++;
    }
    return { ready, syncing, listed, failed, total: rows.length };
  }, [rows]);

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

      {/* Batch status bar */}
      {batchStats.total > 0 && (
        <div className="flex flex-wrap items-center gap-4 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] px-3 py-2 text-xs">
          <StatDot color="var(--success)" label={`${batchStats.ready} ready to list`} />
          {batchStats.syncing > 0 && <StatDot color="var(--brand-400)" label={`${batchStats.syncing} syncing`} pulse />}
          {batchStats.listed > 0 && <StatDot color="var(--success)" label={`${batchStats.listed} listed`} icon="check" />}
          {batchStats.failed > 0 && <StatDot color="var(--danger)" label={`${batchStats.failed} failed/sold`} />}
          <span className="ml-auto text-[var(--text-muted)]">{batchStats.total} scanned</span>
          {(batchStats.listed > 0 || batchStats.failed > 0) && (
            <button
              onClick={handleClearCompleted}
              className="text-[var(--text-muted)] hover:text-[var(--text)] underline underline-offset-2 text-[11px]"
              title="Remove listed + failed rows (sold rows are kept so you can Return them)"
            >
              Clear completed
            </button>
          )}
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
              onFixDHMatch={() => setFixMatchTarget(row)}
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

      {fixMatchTarget && fixMatchTarget.purchaseId && (
        <FixDHMatchDialog
          purchaseId={fixMatchTarget.purchaseId}
          cardName={fixMatchTarget.cardName ?? ''}
          certNumber={fixMatchTarget.certNumber}
          currentDHCardId={fixMatchTarget.dhCardId}
          onClose={() => setFixMatchTarget(null)}
          onSaved={() => {
            void pollCert(fixMatchTarget.certNumber);
            setFixMatchTarget(null);
          }}
        />
      )}
    </div>
  );
}

function StatDot({ color, label, icon, pulse }: { color: string; label: string; icon?: 'check'; pulse?: boolean }) {
  return (
    <span className="flex items-center gap-1.5">
      {icon === 'check' ? (
        <span className="inline-flex items-center justify-center w-3 h-3 text-[10px] font-bold leading-none" style={{ color }} aria-hidden="true">
          &#10003;
        </span>
      ) : (
        <span className={`w-1.5 h-1.5 rounded-full ${pulse ? 'animate-pulse' : ''}`} style={{ background: color }} />
      )}
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

const STALL_THRESHOLD_SEC = 60;

function syncBlockerLabel(row: CertRow): string {
  const dh = hasDHMatch(row);
  const cl = hasCLPrice(row);
  if (!dh && !cl) return 'Waiting for DH match + CL price';
  if (!dh) return 'Waiting for DH match';
  if (!cl) return 'Waiting for CL price';
  return 'Syncing';
}

function SyncingIndicator({ row }: { row: CertRow }) {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick(t => t + 1), 1000);
    return () => window.clearInterval(id);
  }, []);
  const elapsedSec = row.firstScanAt ? Math.max(0, Math.floor((Date.now() - row.firstScanAt) / 1000)) : 0;
  const stalled = elapsedSec >= STALL_THRESHOLD_SEC;
  const label = syncBlockerLabel(row);
  const color = stalled ? 'var(--warning)' : 'var(--brand-400)';
  const textColor = stalled ? 'text-[var(--warning)]' : 'text-[var(--text-muted)]';
  return (
    <span
      className={`inline-flex items-center gap-1.5 text-[10px] ${textColor}`}
      data-tick={tick}
      title={stalled ? `Stalled ${elapsedSec}s — try Fix DH Match or check pricing sync` : undefined}
    >
      <span className="w-1.5 h-1.5 rounded-full animate-pulse" style={{ background: color }} />
      {label}{stalled ? ' — stalled' : '…'} {elapsedSec > 0 ? `${elapsedSec}s` : ''}
    </span>
  );
}

function CertRowItem({
  row,
  highlighted,
  onReturn,
  onDismiss,
  onList,
  onFixDHMatch,
}: {
  row: CertRow;
  highlighted?: boolean;
  onReturn: (certNumber: string) => void;
  onDismiss: (certNumber: string) => void;
  onList: (certNumber: string, priceCents: number, source: string) => void;
  onFixDHMatch: () => void;
}) {
  const s = STATUS_STYLE[row.status];
  const canList = rowIsListable(row);
  const awaitingSync = rowAwaitingSync(row);
  const { market, buyCostCents, listingStatus, listingError } = row;
  const busy = listingStatus === 'setting-price' || listingStatus === 'listing';
  const listed = listingStatus === 'listed';
  const showFixDH = !!row.purchaseId && !hasDHMatch(row) && (row.status === 'existing' || row.status === 'returned' || row.status === 'imported');

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
    ? { kind: 'source', source: 'market' }
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
          {awaitingSync && !canList && row.status !== 'resolving' && (
            <SyncingIndicator row={row} />
          )}
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {showFixDH && (
            <button
              onClick={onFixDHMatch}
              className="rounded-md bg-[var(--brand-500)]/15 px-2.5 py-1 text-[11px] font-semibold text-[var(--brand-400)] hover:bg-[var(--brand-500)]/30 transition-colors"
              title="Paste the correct DoubleHolo URL to fix the match"
            >
              Fix DH Match
            </button>
          )}
          {row.status === 'sold' && (
            <button
              onClick={() => onReturn(row.certNumber)}
              className="rounded-md bg-[var(--warning)]/15 px-3 py-1.5 text-xs font-semibold text-[var(--warning)] hover:bg-[var(--warning)]/30 transition-colors"
            >
              Return
            </button>
          )}
          {(row.status === 'failed' || listed) && (
            <button
              onClick={() => onDismiss(row.certNumber)}
              aria-label="Dismiss"
              className="rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors"
            >
              ✕
            </button>
          )}
        </div>
      </div>

      {canList && (
        <div className="border-t border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.015)] px-4 py-2.5">
          <PriceDecisionBar
            sources={sources}
            preSelected={preSelected}
            recommendedSource="market"
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
