import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api, isAPIError } from '../../../js/api';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult } from '../../../types/campaigns';
import FixDHMatchDialog from '../campaign-detail/inventory/FixDHMatchDialog';
import type { CertRow } from './cardIntakeTypes';
import { rowIsListable, rowAwaitingSync, dhPushStuck, scanFieldsFromResult, importErrorStatus } from './cardIntakeTypes';
import { loadQueue, saveQueue } from './cardIntakeStorage';
import { CertRowItem, StatDot } from './CardIntakeRow';
import { useCardIntakePolling } from './useCardIntakePolling';

export default function CardIntakeTab() {
  const [input, setInput] = useState('');
  const [certs, setCerts] = useState<Map<string, CertRow>>(() => loadQueue());
  const [importLoading, setImportLoading] = useState(false);
  // Banner severity is pinned to the message at the time it is set, not derived
  // from live state — otherwise dismissing retry rows would flip an amber
  // "staged to retry" notice to red after the fact.
  const [importNotice, setImportNotice] = useState<{ text: string; kind: 'warn' | 'error' } | null>(null);
  const [highlightedCert, setHighlightedCert] = useState<string | null>(null);
  const [fixMatchTarget, setFixMatchTarget] = useState<CertRow | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const certsRef = useRef(certs);
  certsRef.current = certs;

  useEffect(() => { inputRef.current?.focus(); }, []);

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
        // Sold supersedes any prior listing state: clear listingStatus so
        // a previously-listed row isn't misclassified by batchStats.
        ...(result.status === 'sold' ? { listingStatus: undefined } : {}),
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

  const { pollCert } = useCardIntakePolling(certsRef, certs.size, updateCert, resolveInBackground);

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
    setCerts(prev => {
      const next = new Map(prev);
      for (const [k, row] of next) {
        if (row.status === 'sold') continue; // sold rows stay for Return action
        if (row.listingStatus === 'listed' || row.status === 'failed') {
          next.delete(k);
        }
      }
      return next;
    });
  };

  const handleImportNew = async () => {
    // Re-collect both freshly-resolved rows and rows staged for retry after a
    // previous transient failure.
    const staged = Array.from(certs.values())
      .filter(c => c.status === 'resolved' || c.status === 'retry');
    const resolvedCerts = staged.map(c => c.certNumber);

    if (resolvedCerts.length === 0) return;

    // Remember each cert's pre-import status so a whole-batch failure can
    // restore it exactly — a 'retry' row must come back as 'retry', not be
    // downgraded to 'resolved' and lose its amber signal.
    const priorStatus = new Map(staged.map(c => [c.certNumber, c.status]));

    setImportLoading(true);
    setImportNotice(null);

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

      // Partition per-cert errors: transient failures (provider down / quota /
      // a retryable DB write) are staged as 'retry' so the operator can
      // re-import them with one click; permanent failures (cert not found)
      // become terminal 'failed'. Each row keeps its own error message so the
      // actual reason is visible on the row — see CertRowItem.
      const errorByCert = new Map(result.errors.map(e => [e.certNumber, e]));
      // Count retryable certs OUTSIDE the state updater — React 18 Strict Mode
      // invokes updaters twice in dev, which would double a counter mutated
      // inside one.
      const retryCount = result.errors.filter(e => importErrorStatus(e) === 'retry').length;
      setCerts(prev => {
        const next = new Map(prev);
        for (const cn of resolvedCerts) {
          const row = next.get(cn);
          if (!row) continue;
          const err = errorByCert.get(cn);
          if (err) {
            next.set(cn, { ...row, status: importErrorStatus(err), error: err.error || 'Import failed' });
          } else {
            next.set(cn, { ...row, status: 'imported' });
          }
        }
        return next;
      });
      if (retryCount > 0) {
        setImportNotice({
          kind: 'warn',
          text: `${retryCount} cert${retryCount > 1 ? 's' : ''} couldn't be imported yet and ${retryCount > 1 ? 'are' : 'is'} staged to retry — ` +
            `see each row for the reason, then click "Import" again. Nothing was lost.`,
        });
      }
      for (const cn of resolvedCerts) {
        if (!errorByCert.has(cn)) void pollCert(cn);
      }
    } catch (err) {
      // Whole-batch throw (5xx, or a network error). Nothing was durably
      // committed on this path — ImportCerts' only error returns precede the
      // per-cert write loop — and re-import is idempotent regardless, so
      // restore each row to its pre-import status (NOT a blanket 'resolved',
      // which would erase the retry signal on rows that were already 'retry').
      setImportNotice({ kind: 'error', text: err instanceof Error ? err.message : 'Import failed' });
      setCerts(prev => {
        const next = new Map(prev);
        for (const cn of resolvedCerts) {
          const row = next.get(cn);
          if (row) next.set(cn, { ...row, status: priorStatus.get(cn) ?? 'resolved' });
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
    let retry = 0;
    let sold = 0;
    for (const r of rows) {
      // Check sold first so a sold row that still carries a stale
      // listingStatus is not misclassified as listed.
      if (r.status === 'sold') sold++;
      else if (r.listingStatus === 'listed') listed++;
      else if (r.status === 'failed') failed++;
      else if (r.status === 'retry') retry++;
      else if (rowIsListable(r)) ready++;
      else if (dhPushStuck(r)) stuck++;
      else if (rowAwaitingSync(r)) syncing++;
    }
    // handleClearCompleted only removes listed + failed; sold rows stay so the
    // operator can Return them, and retry rows stay so they can be re-imported.
    // Surface clearable separately so the Clear button hides when only
    // sold/retry rows are left.
    const clearable = listed + failed;
    return { ready, syncing, stuck, listed, failed, retry, sold, clearable, total: rows.length };
  }, [rows]);

  const resolvedCount = useMemo(
    () => rows.filter(r => r.status === 'resolved' || r.status === 'retry').length,
    [rows],
  );
  const displayRows = useMemo(() => [...rows].reverse(), [rows]);

  return (
    <div className="space-y-3">
      {/* Scan input */}
      <div className="space-y-1">
        <input
          ref={inputRef}
          type="text"
          inputMode="numeric"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Scan or type cert number…"
          className="w-full rounded-lg border border-[var(--brand-500)]/60 bg-[var(--surface-0)] px-4 py-3 font-mono text-lg tracking-[0.15em] text-[var(--text)] placeholder:text-[var(--text-subtle)] placeholder:tracking-normal focus:border-[var(--brand-400)] focus:bg-[var(--surface-1)] focus:outline-none focus:ring-2 focus:ring-[var(--brand-400)]/30 transition-colors"
          autoFocus
          autoComplete="off"
          spellCheck={false}
        />
        <p className="text-xs text-[var(--text-muted)] pl-1">↵ Enter to submit</p>
      </div>

      {/* Batch status bar */}
      {batchStats.total > 0 && (
        <div className="flex flex-wrap items-center gap-4 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)] px-3 py-2 text-xs">
          <StatDot color="var(--success)" label={`${batchStats.ready} ready to list`} />
          {batchStats.syncing > 0 && <StatDot color="var(--brand-400)" label={`${batchStats.syncing} syncing`} pulse />}
          {batchStats.stuck > 0 && <StatDot color="var(--warning)" label={`${batchStats.stuck} needs attention`} />}
          {batchStats.listed > 0 && <StatDot color="var(--success)" label={`${batchStats.listed} listed`} icon="check" />}
          {batchStats.failed > 0 && <StatDot color="var(--danger)" label={`${batchStats.failed} failed`} />}
          {batchStats.retry > 0 && <StatDot color="var(--warning)" label={`${batchStats.retry} will retry`} pulse />}
          {batchStats.sold > 0 && <StatDot color="var(--text-muted)" label={`${batchStats.sold} sold`} />}
          <span className="ml-auto text-[var(--text-muted)]">{batchStats.total} scanned</span>
          {batchStats.clearable > 0 && (
            <button
              onClick={handleClearCompleted}
              className="text-[var(--text-muted)] hover:text-[var(--text)] underline underline-offset-2 text-[11px]"
              title="Remove listed + failed rows (sold and retry rows are kept)"
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

      {/* Import status — severity is pinned to the message (amber for a
          recoverable "staged to retry" notice, red for a whole-batch failure),
          so it doesn't flip if retry rows are dismissed afterward. */}
      {importNotice && (
        importNotice.kind === 'warn' ? (
          <div className="rounded-lg border border-[var(--warning)]/40 bg-[var(--warning)]/10 p-3 text-sm text-[var(--warning)]">
            {importNotice.text}
          </div>
        ) : (
          <div className="rounded-lg border border-[var(--danger)]/40 bg-[var(--danger)]/10 p-3 text-sm text-[var(--danger)]">
            {importNotice.text}
          </div>
        )
      )}

      {/* Staging area */}
      {resolvedCount > 0 && (
        <div className="rounded-lg border border-dashed border-[var(--brand-500)]/50 bg-[var(--brand-500)]/5 p-3">
          <div className="flex items-center justify-between">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-[var(--brand-400)]">
              {batchStats.retry > 0
                ? `${resolvedCount} cert${resolvedCount > 1 ? 's' : ''} staged (${batchStats.retry} to retry)`
                : `${resolvedCount} new cert${resolvedCount > 1 ? 's' : ''} staged`}
            </span>
            <button
              onClick={handleImportNew}
              disabled={importLoading}
              className="rounded-lg bg-[var(--brand-500)] px-4 py-1.5 text-xs font-semibold text-white hover:bg-[var(--brand-600)] disabled:opacity-50 transition-colors"
            >
              {importLoading ? 'Importing…' : batchStats.retry > 0 ? `Import ${resolvedCount}` : `Import ${resolvedCount} New`}
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
