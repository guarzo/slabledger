import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api, isAPIError } from '../../../js/api';
import type { ScannerMode, SaleRowData, SaleSummary } from './sale-types';
import { SALE_COST_VISIBLE_KEY, SALE_DEFAULT_DISCOUNT_KEY } from './sale-types';
import { reportError } from '../../../js/errors';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult } from '../../../types/campaigns';
import FixDHMatchDialog from '../campaign-detail/inventory/FixDHMatchDialog';
import { SaleToolbar } from './SaleToolbar';
import { SaleRow } from './SaleRow';
import { SaleSummaryBar } from './SaleSummaryBar';
import { RecordSalesModal } from './RecordSalesModal';
import type { CertRow } from './cardIntakeTypes';
import { rowIsListable, rowAwaitingSync, dhPushStuck, scanFieldsFromResult } from './cardIntakeTypes';
import { loadQueue, saveQueue } from './cardIntakeStorage';
import { CertRowItem, StatDot } from './CardIntakeRow';
import { useCardIntakePolling } from './useCardIntakePolling';

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

  // Sale mode state
  const [saleRows, setSaleRows] = useState<SaleRowData[]>([]);
  const saleCertsRef = useRef<Set<string>>(new Set());
  const [defaultDiscountPct, setDefaultDiscountPct] = useState<number>(() => {
    try {
      const saved = localStorage.getItem(SALE_DEFAULT_DISCOUNT_KEY);
      if (saved !== null) {
        const n = Number(saved);
        if (Number.isFinite(n)) return n;
      }
    } catch { /* blocked storage */ }
    return 80;
  });
  const [costVisible, setCostVisible] = useState<boolean>(() => {
    try {
      return localStorage.getItem(SALE_COST_VISIBLE_KEY) === 'true';
    } catch { return false; }
  });
  const [showRecordModal, setShowRecordModal] = useState(false);
  const [recordLoading, setRecordLoading] = useState(false);
  const [recordError, setRecordError] = useState<string | null>(null);

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

  const handleModeSwitch = useCallback((newMode: ScannerMode) => {
    if (newMode === mode) return;
    const count = newMode === 'sale' ? certs.size : saleRows.length;
    if (count > 0) {
      if (!window.confirm(`Switch to ${newMode} mode? This will clear ${count} scanned card(s).`)) return;
      if (newMode === 'sale') { setCerts(new Map()); saveQueue(new Map()); }
      else { setSaleRows([]); saleCertsRef.current.clear(); }
    }
    setMode(newMode);
  }, [mode, certs.size, saleRows.length]);

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

  const handleSaleScan = useCallback(async (certNumber: string) => {
    if (saleCertsRef.current.has(certNumber)) return;
    saleCertsRef.current.add(certNumber);

    setSaleRows(prev => [...prev, {
      certNumber, status: 'scanning', compValueCents: 0,
      compManuallySet: false, salePriceCents: 0, salePriceManuallySet: false,
    }]);

    try {
      const result = await api.scanCert(certNumber);

      if (result.status === 'new') {
        setSaleRows(prev => prev.map(r => r.certNumber === certNumber
          ? { ...r, status: 'error' as const, error: 'Not in inventory' } : r));
        return;
      }
      if (!result.receivedAt) {
        setSaleRows(prev => prev.map(r => r.certNumber === certNumber
          ? { ...r, status: 'error' as const, error: 'Not in-hand' } : r));
        return;
      }
      if (result.status === 'sold') {
        setSaleRows(prev => prev.map(r => r.certNumber === certNumber
          ? { ...r, status: 'error' as const, error: 'Already sold' } : r));
        return;
      }

      const clValue = result.market?.clValueCents ?? 0;
      const compValue = clValue;
      const salePrice = Math.round(compValue * defaultDiscountPct / 100);

      setSaleRows(prev => prev.map(r => r.certNumber === certNumber ? {
        ...r, status: 'resolved' as const,
        cardName: result.cardName, purchaseId: result.purchaseId,
        campaignId: result.campaignId, setName: result.setName,
        cardNumber: result.cardNumber, cardYear: result.cardYear,
        gradeValue: result.gradeValue, frontImageUrl: result.frontImageUrl,
        buyCostCents: result.buyCostCents, clValueCents: clValue,
        dhListingPriceCents: result.dhListingPriceCents,
        lastSoldCents: result.market?.lastSoldCents,
        compValueCents: compValue, compManuallySet: false,
        salePriceCents: salePrice, salePriceManuallySet: false,
      } : r));
    } catch (err) {
      reportError('handleSaleScan', err instanceof Error ? err : new Error(String(err)));
      setSaleRows(prev => prev.map(r => r.certNumber === certNumber
        ? { ...r, status: 'error' as const, error: err instanceof Error ? err.message : 'Scan failed' } : r));
    }
  }, [defaultDiscountPct]);

  const handleScan = useCallback(async (certNumber: string) => {
    certNumber = certNumber.trim();
    if (!certNumber) return;

    if (mode === 'sale') {
      await handleSaleScan(certNumber);
      return;
    }

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
  }, [mode, handleSaleScan, applyScanResult, updateCert, resolveInBackground]);

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

  const handleCompValueChange = useCallback((certNumber: string, cents: number) => {
    setSaleRows(prev => prev.map(r => {
      if (r.certNumber !== certNumber) return r;
      const newSalePrice = r.salePriceManuallySet ? r.salePriceCents : Math.round(cents * defaultDiscountPct / 100);
      return { ...r, compValueCents: cents, compManuallySet: true, salePriceCents: newSalePrice };
    }));
  }, [defaultDiscountPct]);

  const handleSalePriceChange = useCallback((certNumber: string, cents: number) => {
    setSaleRows(prev => prev.map(r =>
      r.certNumber === certNumber ? { ...r, salePriceCents: cents, salePriceManuallySet: true } : r));
  }, []);

  const handleDismissSaleRow = useCallback((certNumber: string) => {
    setSaleRows(prev => prev.filter(r => r.certNumber !== certNumber));
    saleCertsRef.current.delete(certNumber);
  }, []);

  const handleDiscountChange = useCallback((pct: number) => {
    setDefaultDiscountPct(pct);
    setSaleRows(prev => prev.map(r => {
      if (r.salePriceManuallySet) return r;
      return { ...r, salePriceCents: Math.round(r.compValueCents * pct / 100) };
    }));
  }, []);

  const handleClearAllSaleRows = useCallback(() => {
    if (saleRows.length > 0 && !window.confirm(`Clear ${saleRows.length} scanned card(s)?`)) return;
    setSaleRows([]); saleCertsRef.current.clear();
  }, [saleRows.length]);

  const saleSummary = useMemo<SaleSummary>(() => {
    const resolved = saleRows.filter(r => r.status === 'resolved');
    const compTotal = resolved.reduce((s, r) => s + r.compValueCents, 0);
    const saleTotal = resolved.reduce((s, r) => s + r.salePriceCents, 0);
    const costTotal = resolved.reduce((s, r) => s + (r.buyCostCents ?? 0), 0);
    return {
      cardCount: resolved.length,
      compTotalCents: compTotal,
      saleTotalCents: saleTotal,
      costTotalCents: costTotal,
      profitCents: saleTotal - costTotal,
      avgDiscountPct: compTotal > 0 ? Math.round(saleTotal / compTotal * 100) : 0,
    };
  }, [saleRows]);

  const handleRecordSales = useCallback(async (saleDate: string, channel: string) => {
    const resolved = saleRows.filter(r => r.status === 'resolved' && r.purchaseId && r.campaignId);
    if (resolved.length === 0) return;

    setRecordLoading(true);
    setRecordError(null);
    const byCampaign = new Map<string, { purchaseId: string; salePriceCents: number }[]>();
    for (const r of resolved) {
      const items = byCampaign.get(r.campaignId!) ?? [];
      items.push({ purchaseId: r.purchaseId!, salePriceCents: r.salePriceCents });
      byCampaign.set(r.campaignId!, items);
    }

    const succeededCerts = new Set<string>();
    const errors: string[] = [];
    for (const [campaignId, items] of byCampaign) {
      try {
        const result = await api.createBulkSales(campaignId, channel, saleDate, items);
        const failedPurchaseIds = new Set(result.errors?.map(e => e.purchaseId) ?? []);
        for (const item of items) {
          if (failedPurchaseIds.has(item.purchaseId)) continue;
          const row = resolved.find(r => r.purchaseId === item.purchaseId);
          if (row) succeededCerts.add(row.certNumber);
        }
        if (result.errors?.length) {
          errors.push(...result.errors.map(e => e.error));
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        errors.push(msg);
        reportError('handleRecordSales', err instanceof Error ? err : new Error(msg));
      }
    }

    if (succeededCerts.size > 0) {
      setSaleRows(prev => prev.filter(r => !succeededCerts.has(r.certNumber)));
      for (const cert of succeededCerts) saleCertsRef.current.delete(cert);
    }
    if (errors.length > 0) {
      setRecordError(`Failed for ${errors.length} campaign(s): ${errors.join('; ')}`);
    } else {
      setShowRecordModal(false);
    }
    setRecordLoading(false);
  }, [saleRows]);

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
        <div className="mt-4 space-y-3">
          <SaleToolbar
            discountPct={defaultDiscountPct}
            onDiscountChange={handleDiscountChange}
            costVisible={costVisible}
            onCostVisibleChange={setCostVisible}
          />

          {saleRows.length > 0 && (
            <div className="rounded-md border border-zinc-800 overflow-hidden">
              <div className="grid items-center gap-1 px-3 py-1.5 text-[10px] uppercase tracking-wider text-zinc-600"
                style={{ gridTemplateColumns: costVisible
                  ? '24px 1fr 64px 64px 64px 64px 80px 80px 36px'
                  : '24px 1fr 64px 64px 64px 80px 80px 36px'
                }}>
                <span />
                <span>Card</span>
                {costVisible && <span className="text-right">Cost</span>}
                <span className="text-right">CL</span>
                <span className="text-right">DH List</span>
                <span className="text-right">Last Sold</span>
                <span className="text-right">Comp Value</span>
                <span className="text-right">Sale Price</span>
                <span />
              </div>
              {saleRows.map(row => (
                <SaleRow
                  key={row.certNumber}
                  row={row}
                  costVisible={costVisible}
                  onCompValueChange={handleCompValueChange}
                  onSalePriceChange={handleSalePriceChange}
                  onDismiss={handleDismissSaleRow}
                />
              ))}
            </div>
          )}

          <SaleSummaryBar
            summary={saleSummary}
            costVisible={costVisible}
            onClearAll={handleClearAllSaleRows}
            onRecordSales={() => setShowRecordModal(true)}
          />

          {showRecordModal && (
            <RecordSalesModal
              rows={saleRows}
              summary={saleSummary}
              onConfirm={handleRecordSales}
              onCancel={() => setShowRecordModal(false)}
              loading={recordLoading}
              error={recordError}
            />
          )}
        </div>
      )}
    </div>
  );
}
