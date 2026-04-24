import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api, isAPIError } from '../../../js/api';
import type { ScannerMode } from './sale-types';
import { reportError } from '../../../js/errors';
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
  /** dhCardId is the DH catalog match (from cert → card_id resolution). */
  dhCardId?: number;
  /** dhInventoryId is the DH inventory line (set after a successful push). */
  dhInventoryId?: number;
  /** dhPushStatus tracks the push pipeline state: pending / matched / unmatched / held / dismissed. */
  dhPushStatus?: string;
  /** dhStatus is the DH-reported inventory status: in_stock / listed / sold. */
  dhStatus?: string;
  /** Unix ms of when the row was first added — used to display "syncing…Xs" elapsed. */
  firstScanAt?: number;
  // Search-helper metadata — populated on existing/sold scan results.
  frontImageUrl?: string;
  setName?: string;
  cardNumber?: string;
  cardYear?: string;
  gradeValue?: number;
  population?: number;
  dhSearchQuery?: string;
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

// hasDHMatch: cert has been matched to a DH catalog card. Proxied through
// gradePriceCents for backwards compatibility with scan results from older
// servers that don't return dhCardId.
function hasDHMatch(row: CertRow): boolean {
  return (row.dhCardId ?? 0) > 0 || (row.market?.gradePriceCents ?? 0) > 0;
}

// hasDHInventory: cert has been pushed to DH's inventory — this is the real
// "ready to list" signal that mirrors the inventory tab's needsPriceReview().
// Snapshot data can lag behind the match/push pipeline, so gating on inventory
// push is what keeps intake unblocked for cards that are otherwise complete.
function hasDHInventory(row: CertRow): boolean {
  return (row.dhInventoryId ?? 0) > 0;
}

function hasCLPrice(row: CertRow): boolean {
  return (row.market?.clValueCents ?? 0) > 0;
}

// dhPushStuck: the push pipeline landed in an actionable-but-blocked state
// (unmatched / held / dismissed). These won't resolve via polling — the user
// needs to Fix DH Match, approve a hold, or dismiss the row manually.
function dhPushStuck(row: CertRow): boolean {
  const s = row.dhPushStatus;
  return s === 'unmatched' || s === 'held' || s === 'dismissed';
}

function rowIsListable(row: CertRow): boolean {
  return !!row.purchaseId && hasDHInventory(row) && hasCLPrice(row);
}

function rowAwaitingSync(row: CertRow): boolean {
  if (row.listingStatus === 'listed') return false;
  if (row.status === 'failed' || row.status === 'sold') return false;
  if (row.status === 'resolving') return true;
  if ((row.status === 'existing' || row.status === 'returned' || row.status === 'imported') && !rowIsListable(row)) {
    // Don't keep polling a row whose push pipeline is stuck in a state the
    // server can't recover on its own — the user has to intervene.
    if (dhPushStuck(row)) return false;
    return true;
  }
  return false;
}

/** Maps an existing/sold ScanCertResponse onto the CertRow fields it drives. */
function scanFieldsFromResult(result: ScanCertResponse): Partial<CertRow> {
  return {
    cardName: result.cardName,
    purchaseId: result.purchaseId,
    campaignId: result.campaignId,
    buyCostCents: result.buyCostCents,
    market: result.market,
    frontImageUrl: result.frontImageUrl,
    setName: result.setName,
    cardNumber: result.cardNumber,
    cardYear: result.cardYear,
    gradeValue: result.gradeValue,
    population: result.population,
    dhSearchQuery: result.dhSearchQuery,
    dhCardId: result.dhCardId,
    dhInventoryId: result.dhInventoryId,
    dhPushStatus: result.dhPushStatus,
    dhStatus: result.dhStatus,
  };
}

const DH_SEARCH_BASE = 'https://doubleholo.com/marketplace';

/**
 * Builds a DH marketplace search URL. Prefers the backend-normalized
 * `dhSearchQuery` (set + simplified name + number via cardutil) because that's
 * the same pipeline DH's own matcher uses; falls back to the raw card name
 * when the normalized query isn't available (e.g. sold rows from older scans).
 * Spaces become `+` per DH's URL convention.
 */
function buildDHSearchURL(row: Pick<CertRow, 'cardName' | 'dhSearchQuery'>): string {
  const query = (row.dhSearchQuery ?? row.cardName ?? '').trim();
  if (!query) return DH_SEARCH_BASE;
  const q = query.split(/\s+/).map(encodeURIComponent).join('+');
  return `${DH_SEARCH_BASE}?q=${q}`;
}

export default function CardIntakeTab() {
  const [mode, setMode] = useState<ScannerMode>('intake');
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

  const handleModeSwitch = useCallback((newMode: ScannerMode) => {
    if (newMode === mode) return;
    if (certs.size > 0) {
      if (!window.confirm(`Switch to ${newMode} mode? This will clear ${certs.size} scanned card(s).`)) {
        return;
      }
      setCerts(new Map());
      saveQueue(new Map());
    }
    setMode(newMode);
  }, [mode, certs.size]);

  const applyScanResult = useCallback((certNumber: string, result: ScanCertResponse) => {
    if (result.status === 'existing' || result.status === 'sold') {
      updateCert(certNumber, {
        status: result.status,
        ...scanFieldsFromResult(result),
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

  const applyPollResult = useCallback((certNumber: string, result: ScanCertResponse) => {
    if (result.status !== 'existing' && result.status !== 'sold') {
      // For 'new', backend is still working — nothing to update.
      return;
    }
    // Don't regress from imported → existing; just refresh the market snapshot.
    const current = certsRef.current.get(certNumber);
    const preserveStatus = current?.status === 'imported' ? 'imported' : result.status;
    const fields = scanFieldsFromResult(result);
    updateCert(certNumber, {
      status: preserveStatus,
      ...fields,
      cardName: fields.cardName ?? current?.cardName,
    });
  }, [updateCert]);

  const pollCert = useCallback(async (certNumber: string) => {
    if (inflightPollsRef.current.has(certNumber)) return;
    inflightPollsRef.current.add(certNumber);
    try {
      const result = await api.scanCert(certNumber);
      applyPollResult(certNumber, result);
    } catch {
      // Transient poll failure — next tick will retry.
    } finally {
      inflightPollsRef.current.delete(certNumber);
    }
  }, [applyPollResult]);

  // Batch-poll every row awaiting sync in a single request. Replaces a
  // per-row fan-out that was tripping the server's rate limiter (300 req/min)
  // once the operator had ~20+ certs in flight.
  const pollAwaitingCerts = useCallback(async () => {
    const awaiting: string[] = [];
    for (const row of certsRef.current.values()) {
      if (rowAwaitingSync(row) && !inflightPollsRef.current.has(row.certNumber)) {
        awaiting.push(row.certNumber);
      }
    }
    if (awaiting.length === 0) return;
    awaiting.forEach(c => inflightPollsRef.current.add(c));
    try {
      const batch = await api.scanCerts(awaiting);
      for (const cert of awaiting) {
        const result = batch.results?.[cert];
        if (result) applyPollResult(cert, result);
      }
      // Per-cert failures come back in batch.errors. Report them so they're
      // visible via the central telemetry funnel; the inflight clear below
      // lets the next tick retry (matching pre-batch single-poll semantics
      // for transient errors).
      if (batch.errors && batch.errors.length > 0) {
        reportError('scan-certs batch', new Error(
          `per-cert errors: ${batch.errors.map(e => `${e.certNumber}: ${e.error}`).join('; ')}`
        ));
      }
    } catch {
      // Transient batch failure — next tick will retry.
    } finally {
      awaiting.forEach(c => inflightPollsRef.current.delete(c));
    }
  }, [applyPollResult]);

  // Rehydrate: on mount, fire resolve for stale 'resolving' rows, then a
  // single batch poll picks up everything else needing sync.
  useEffect(() => {
    const current = certsRef.current;
    for (const row of current.values()) {
      if (row.status === 'resolving') {
        void resolveInBackground(row.certNumber);
      }
    }
    void pollAwaitingCerts();
    // mount-only
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Single polling loop: every 4s, refresh all rows awaiting sync in one call.
  useEffect(() => {
    if (certs.size === 0) return;
    const interval = window.setInterval(() => { void pollAwaitingCerts(); }, 4000);
    return () => window.clearInterval(interval);
  }, [certs.size, pollAwaitingCerts]);

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
    let stuck = 0;
    let listed = 0;
    let failed = 0;
    for (const r of rows) {
      if (r.listingStatus === 'listed') listed++;
      else if (r.status === 'failed' || r.status === 'sold') failed++;
      else if (rowIsListable(r)) ready++;
      else if (dhPushStuck(r)) stuck++;
      else if (rowAwaitingSync(r)) syncing++;
    }
    return { ready, syncing, stuck, listed, failed, total: rows.length };
  }, [rows]);

  const resolvedCount = useMemo(() => rows.filter(r => r.status === 'resolved').length, [rows]);
  const displayRows = useMemo(() => [...rows].reverse(), [rows]);

  return (
    <div className="space-y-3">
      {/* Scan input + mode toggle */}
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
        <div className="flex overflow-hidden rounded-md border border-zinc-700">
          <button
            className={`px-3 py-1.5 text-xs font-medium transition-colors ${
              mode === 'intake'
                ? 'bg-indigo-600 text-white'
                : 'text-zinc-400 hover:text-zinc-200'
            }`}
            onClick={() => handleModeSwitch('intake')}
          >
            Intake
          </button>
          <button
            className={`px-3 py-1.5 text-xs font-medium transition-colors ${
              mode === 'sale'
                ? 'bg-indigo-600 text-white'
                : 'text-zinc-400 hover:text-zinc-200'
            }`}
            onClick={() => handleModeSwitch('sale')}
          >
            Sale
          </button>
        </div>
      </div>

      {mode === 'intake' ? (
        <>
      {/* Batch status bar */}
      {batchStats.total > 0 && (
        <div className="flex flex-wrap items-center gap-4 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] px-3 py-2 text-xs">
          <StatDot color="var(--success)" label={`${batchStats.ready} ready to list`} />
          {batchStats.syncing > 0 && <StatDot color="var(--brand-400)" label={`${batchStats.syncing} syncing`} pulse />}
          {batchStats.stuck > 0 && <StatDot color="var(--warning)" label={`${batchStats.stuck} needs attention`} />}
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
        </>
      ) : (
        <div className="mt-4 text-sm text-zinc-500">Sale mode — UI coming soon</div>
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
  if (row.dhPushStatus === 'unmatched') return 'DH match failed — Fix DH Match';
  if (row.dhPushStatus === 'held') return 'DH push held for review';
  if (row.dhPushStatus === 'dismissed') return 'DH push dismissed';
  const inv = hasDHInventory(row);
  const cl = hasCLPrice(row);
  if (!inv && !cl) return 'Waiting for DH push + CL price';
  if (!inv) return 'Waiting for DH push';
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
  const pushStuck = dhPushStuck(row);
  const stalled = pushStuck || elapsedSec >= STALL_THRESHOLD_SEC;
  const label = syncBlockerLabel(row);
  const color = stalled ? 'var(--warning)' : 'var(--brand-400)';
  const textColor = stalled ? 'text-[var(--warning)]' : 'text-[var(--text-muted)]';
  const title = pushStuck
    ? 'Push pipeline is blocked — use Fix DH Match or dismiss this row.'
    : stalled
      ? `Stalled ${elapsedSec}s — try Fix DH Match or dismiss and retry from the Inventory tab.`
      : undefined;
  const trailing = pushStuck ? '' : stalled ? ' — stalled' : '…';
  return (
    <span
      className={`inline-flex items-center gap-1.5 text-[10px] ${textColor}`}
      data-tick={tick}
      title={title}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${pushStuck ? '' : 'animate-pulse'}`} style={{ background: color }} />
      {label}{trailing} {!pushStuck && elapsedSec > 0 ? `${elapsedSec}s` : ''}
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
  const inPlaceableStatus = row.status === 'existing' || row.status === 'returned' || row.status === 'imported';
  // Show Fix DH Match when the push pipeline explicitly failed the match,
  // or (for older servers that don't return dhPushStatus) when we have no
  // other signal that DH has matched the cert.
  const showFixDH = !!row.purchaseId && inPlaceableStatus && (
    row.dhPushStatus === 'unmatched' ||
    (row.dhPushStatus === undefined && !hasDHMatch(row))
  );
  // A row is "stuck" when intake can't make progress without operator action
  // (push pipeline dead-end or nothing syncing). Offer a dismiss path so the
  // user doesn't have to wipe localStorage to unstick the queue.
  const showDismiss = row.status === 'failed' || listed ||
    (inPlaceableStatus && !canList && !busy && (dhPushStuck(row) || !awaitingSync));

  // Row detail expander — available for rows backed by a stored purchase,
  // so the operator can ID the card (image, set, #) and jump to a DH search.
  const canExpand = row.status === 'existing' || row.status === 'sold'
    || row.status === 'returned' || row.status === 'imported';
  const [expanded, setExpanded] = useState(false);

  const clCents = market?.clValueCents ?? 0;
  const dhCents = market?.gradePriceCents ?? 0;
  const lastSoldCents = market?.lastSoldCents ?? 0;
  const mmCents = market?.sourcePrices?.find(p => p.source === 'MarketMovers')?.priceCents ?? 0;
  const costCents = buyCostCents ?? 0;

  // Memoize sources and preSelected so PriceDecisionBar's effect doesn't
  // re-fire on every parent re-render (the 4s polling loop re-renders this
  // tab on every tick, which would otherwise reset the user's pill choice).
  const sources = useMemo(
    () => canList
      ? buildPriceSources({ clCents, dhMidCents: dhCents, costCents, lastSoldCents, mmCents })
      : [],
    [canList, clCents, dhCents, costCents, lastSoldCents, mmCents],
  );
  const preSelected = useMemo<PreSelection>(
    () => dhCents > 0 ? { kind: 'source', source: 'market' } : { kind: 'none' },
    [dhCents],
  );

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
          {!canList && row.status !== 'resolving' && inPlaceableStatus && (awaitingSync || dhPushStuck(row)) && (
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
          {showDismiss && (
            <button
              onClick={() => onDismiss(row.certNumber)}
              aria-label="Dismiss"
              title="Remove this row from the intake queue (doesn't affect the purchase)"
              className="rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors"
            >
              ✕
            </button>
          )}
          {canExpand && (
            <button
              onClick={() => setExpanded(v => !v)}
              aria-label={expanded ? 'Hide card details' : 'Show card details'}
              aria-expanded={expanded}
              title={expanded ? 'Hide card details' : 'Show card details (image, set, DH search)'}
              className="rounded-md p-1 text-[var(--text-muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors"
            >
              <span aria-hidden="true" className="inline-block w-3 text-center text-xs leading-none">
                {expanded ? '▾' : '▸'}
              </span>
            </button>
          )}
        </div>
      </div>

      {canExpand && expanded && (
        <CertRowDetail row={row} />
      )}

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

function CertRowDetail({ row }: { row: CertRow }) {
  const [copied, setCopied] = useState(false);
  const handleCopy = async () => {
    if (!row.cardName) return;
    try {
      await navigator.clipboard.writeText(row.cardName);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard API unavailable (e.g. non-secure origin) — fail silently.
    }
  };

  const setLine = [row.setName, row.cardNumber ? `#${row.cardNumber}` : null]
    .filter(Boolean).join(' · ');
  const gradeLine = [
    row.cardYear,
    row.gradeValue ? `PSA ${row.gradeValue}` : null,
    row.population ? `pop ${row.population.toLocaleString()}` : null,
  ].filter(Boolean).join(' · ');

  return (
    <div className="border-t border-[rgba(255,255,255,0.06)] bg-[rgba(255,255,255,0.015)] px-4 py-3">
      <div className="flex gap-4 items-start">
        {row.frontImageUrl ? (
          <a
            href={row.frontImageUrl}
            target="_blank"
            rel="noreferrer noopener"
            className="shrink-0"
            title="Open full-size image in new tab"
          >
            <img
              src={row.frontImageUrl}
              alt={row.cardName ? `Slab photo: ${row.cardName}` : 'Slab photo'}
              className="h-32 w-auto rounded border border-[var(--surface-2)] bg-black/20 object-contain"
              loading="lazy"
            />
          </a>
        ) : (
          <div className="shrink-0 flex h-32 w-24 items-center justify-center rounded border border-dashed border-[var(--surface-2)] text-[10px] text-[var(--text-muted)] text-center px-2">
            No image yet
          </div>
        )}

        <div className="flex flex-col gap-1 min-w-0 flex-1">
          {row.cardName && (
            <div className="text-sm font-semibold text-[var(--text)] truncate">{row.cardName}</div>
          )}
          {setLine && (
            <div className="text-xs text-[var(--text-muted)]">{setLine}</div>
          )}
          {gradeLine && (
            <div className="text-xs text-[var(--text-muted)]">{gradeLine}</div>
          )}

          <div className="flex flex-wrap items-center gap-2 mt-2">
            {(row.dhSearchQuery || row.cardName) && (
              <a
                href={buildDHSearchURL(row)}
                target="_blank"
                rel="noreferrer noopener"
                className="rounded-md bg-[var(--brand-500)]/15 px-2.5 py-1 text-[11px] font-semibold text-[var(--brand-400)] hover:bg-[var(--brand-500)]/30 transition-colors"
              >
                Search on DH ↗
              </a>
            )}
            {row.cardName && (
              <button
                onClick={handleCopy}
                className="rounded-md bg-[var(--surface-2)] px-2.5 py-1 text-[11px] font-semibold text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
              >
                {copied ? 'Copied ✓' : 'Copy name'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
