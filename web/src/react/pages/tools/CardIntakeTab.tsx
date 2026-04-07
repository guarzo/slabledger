import { useState, useRef, useCallback, useEffect, useMemo } from 'react';
import { api } from '../../../js/api';
import type { ScanCertResponse, ResolveCertResponse, CertImportResult } from '../../../types/campaigns';

type CertStatus = 'scanning' | 'existing' | 'sold' | 'returned' | 'resolving' | 'resolved' | 'failed' | 'importing' | 'imported';

interface CertRow {
  certNumber: string;
  status: CertStatus;
  cardName?: string;
  purchaseId?: string;
  campaignId?: string;
  error?: string;
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

  const handleScan = useCallback(async (certNumber: string) => {
    certNumber = certNumber.trim();
    if (!certNumber) return;

    // Duplicate check via ref to avoid stale closure
    if (certsRef.current.has(certNumber)) {
      setHighlightedCert(certNumber);
      setTimeout(() => setHighlightedCert(prev => prev === certNumber ? null : prev), 1500);
      return;
    }

    // Add row in scanning state
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
        });
      } else if (result.status === 'sold') {
        updateCert(certNumber, {
          status: 'sold',
          cardName: result.cardName,
          purchaseId: result.purchaseId,
          campaignId: result.campaignId,
        });
      } else {
        // New cert — trigger background resolve
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

    // Batch status update to 'importing'
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

function CertRowItem({ row, highlighted, onReturn, onDismiss }: {
  row: CertRow;
  highlighted?: boolean;
  onReturn: (certNumber: string) => void;
  onDismiss: (certNumber: string) => void;
}) {
  const base = 'flex items-center justify-between rounded border px-3 py-2 text-sm transition-all';

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

  return (
    <div className={`${base} ${cfg.bg} ${cfg.border}${highlighted ? ' ring-2 ring-yellow-400' : ''}`}>
      <div className="flex items-center gap-2 min-w-0">
        <span className={`font-mono ${cfg.certColor} min-w-[80px]`}>{row.certNumber}</span>
        <span className={`${cfg.certColor} min-w-[110px] text-xs`}>{cfg.label}</span>
        {row.cardName && <span className="text-gray-400 text-xs truncate">{row.cardName}</span>}
        {row.error && row.status === 'failed' && (
          <span className="text-red-400 text-xs truncate">{row.error}</span>
        )}
      </div>
      <div className="flex items-center gap-2 shrink-0">
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
      </div>
    </div>
  );
}
