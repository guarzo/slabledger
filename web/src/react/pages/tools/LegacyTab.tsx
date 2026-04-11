import { useState } from 'react';
import { Button, SectionErrorBoundary } from '../../ui';
import ShopifySyncPage from '../ShopifySyncPage';
import CardIntakeSection from './CardIntakeSection';
import { LegacyCard } from './CardIntakeSection';
import EbayDescriptionSection from './EbayDescriptionSection';
import SalesImportSection from './SalesImportSection';

type ExpandedCard = 'ebay' | 'sales' | 'priceSync' | null;

function SyncIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <polyline points="23 4 23 10 17 10" />
      <polyline points="1 20 1 14 7 14" />
      <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
    </svg>
  );
}

function TagIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M20.59 13.41l-7.17 7.17a2 2 0 01-2.83 0L2 12V2h10l8.59 8.59a2 2 0 010 2.82z" />
      <line x1="7" y1="7" x2="7.01" y2="7" />
    </svg>
  );
}

function ReceiptIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
      <polyline points="14 2 14 8 20 8" />
      <line x1="16" y1="13" x2="8" y2="13" />
      <line x1="16" y1="17" x2="8" y2="17" />
      <polyline points="10 9 9 9 8 9" />
    </svg>
  );
}

function DownloadIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  );
}

function UploadIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
      <polyline points="17 8 12 3 7 8" />
      <line x1="12" y1="3" x2="12" y2="15" />
    </svg>
  );
}

/* ── ExternalImportCard (self-contained) ─────────────────────────── */

function ExternalImportCard() {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<ExternalImportResult | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const toast = useToast();

  const handleFile = async (file: File) => {
    try {
      setLoading(true);
      setResult(null);
      const res = await api.globalImportExternal(file);
      setResult(res);
      toast.success(`External import: ${res.imported} imported, ${res.updated} updated, ${res.skipped} skipped, ${res.failed} failed`);
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import external data'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <LegacyCard
      icon={<ShopBagIcon />}
      title="External Import"
      description="Import a Shopify product export CSV for external purchases"
    >
      <Button
        size="sm"
        variant="secondary"
        fullWidth
        loading={loading}
        onClick={() => fileRef.current?.click()}
      >
        Upload Shopify CSV
      </Button>
      <input
        ref={fileRef}
        type="file"
        accept=".csv"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) handleFile(file);
          e.target.value = '';
        }}
      />
      {result && (
        <div className="mt-3 p-2 rounded bg-[var(--surface-2)]/50 text-xs text-left">
          <div className="flex flex-wrap gap-2">
            {result.imported > 0 && <span className="text-[var(--success)]">{result.imported} imported</span>}
            {result.updated > 0 && <span className="text-[var(--info)]">{result.updated} updated</span>}
            {result.skipped > 0 && <span className="text-[var(--text-muted)]">{result.skipped} skipped</span>}
            {result.failed > 0 && <span className="text-[var(--danger)]">{result.failed} failed</span>}
          </div>
          {result.errors && result.errors.length > 0 && (
            <div className="mt-1 text-[var(--danger)]">
              {result.errors.map((e, idx) => (
                <div key={idx}>{e.row != null ? `Row ${e.row}: ` : ''}{e.error}</div>
              ))}
            </div>
          )}
        </div>
      )}
    </LegacyCard>
  );
}

/* ── MMExportCard (self-contained) ───────────────────────────────── */

function MMExportCard() {
  const [loading, setLoading] = useState(false);
  const [missingOnly, setMissingOnly] = useState(false);
  const toast = useToast();

  const handleExport = async () => {
    try {
      setLoading(true);
      const blob = await api.globalExportMM(missingOnly);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'market-movers-export.csv';
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      toast.success('Market Movers CSV exported');
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to export'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <LegacyCard
      icon={<DownloadIcon />}
      title="Export for Market Movers"
      description="Download inventory CSV to import into Market Movers collection"
    >
      <div className="flex flex-col gap-2 w-full">
        <label htmlFor="missing-only-mm" className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] cursor-pointer select-none">
          <input
            id="missing-only-mm"
            type="checkbox"
            checked={missingOnly}
            onChange={(e) => setMissingOnly(e.target.checked)}
            className="accent-[var(--brand-500)]"
          />
          Only missing MM data
        </label>
        <Button
          size="sm"
          variant="secondary"
          fullWidth
          loading={loading}
          onClick={handleExport}
        >
          Download CSV
        </Button>
      </div>
    </LegacyCard>
  );
}

/* ── MMImportCard (self-contained) ───────────────────────────────── */

function MMImportCard() {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<MMRefreshResult | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const toast = useToast();

  const handleFile = async (file: File) => {
    try {
      setLoading(true);
      setResult(null);
      const res = await api.globalRefreshMM(file);
      setResult(res);
      if (res.failed > 0 || (res.errors && res.errors.length > 0)) {
        toast.warning(`Market Movers import: ${res.failed} failed. ${res.updated} updated, ${res.skipped} skipped, ${res.notFound} not found`);
        if (res.errors && res.errors.length > 0) {
          const formatted = res.errors
            .map((e) => (e.row != null ? `row ${e.row}: ${e.error}` : e.error))
            .join('; ');
          reportError(
            'LegacyTab/mm-import',
            new Error(`Market Movers import errors: ${formatted}`),
          );
        }
      } else {
        toast.success(`Market Movers import: ${res.updated} updated, ${res.skipped} skipped, ${res.notFound} not found`);
      }
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import Market Movers data'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <LegacyCard
      icon={<UploadIcon />}
      title="Import from Market Movers"
      description="Upload a Market Movers export CSV to sync Last Sale Price into mm_value"
    >
      <Button
        size="sm"
        variant="secondary"
        fullWidth
        loading={loading}
        onClick={() => fileRef.current?.click()}
      >
        Upload CSV
      </Button>
      <input
        ref={fileRef}
        type="file"
        accept=".csv"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) handleFile(file);
          e.target.value = '';
        }}
      />
      {result && (
        <div className="mt-3 p-2 rounded bg-[var(--surface-2)]/50 text-xs text-left">
          <div className="flex flex-wrap gap-2">
            {result.updated > 0 && <span className="text-[var(--success)]">{result.updated} updated</span>}
            {result.skipped > 0 && <span className="text-[var(--text-muted)]">{result.skipped} skipped</span>}
            {result.notFound > 0 && <span className="text-[var(--warning)]">{result.notFound} not found</span>}
            {result.failed > 0 && <span className="text-[var(--danger)]">{result.failed} failed</span>}
          </div>
          {result.errors && result.errors.length > 0 && (
            <div className="mt-1 text-[var(--danger)]">
              {result.errors.map((e, idx) => (
                <div key={idx}>{e.row != null ? `Row ${e.row}: ` : ''}{e.error}</div>
              ))}
            </div>
          )}
        </div>
      )}
    </LegacyCard>
  );
}

/* ── CLExportCard (self-contained) ────────────────────────────────── */

function CLExportCard() {
  const [loading, setLoading] = useState(false);
  const [missingOnly, setMissingOnly] = useState(false);
  const toast = useToast();

  const handleExport = async () => {
    try {
      setLoading(true);
      const blob = await api.globalExportCL(missingOnly);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'card_ladder_import.csv';
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 100);
      toast.success('Card Ladder CSV exported');
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to export'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <LegacyCard
      icon={<DownloadIcon />}
      title="CL CSV Export"
      description="Download inventory CSV to import into Card Ladder manually"
    >
      <div className="flex flex-col gap-2 w-full">
        <label htmlFor="missing-only-cl" className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] cursor-pointer select-none">
          <input
            id="missing-only-cl"
            type="checkbox"
            checked={missingOnly}
            onChange={(e) => setMissingOnly(e.target.checked)}
            className="accent-[var(--brand-500)]"
          />
          Only missing CL data
        </label>
        <Button size="sm" variant="secondary" fullWidth loading={loading} onClick={handleExport}>
          Download CSV
        </Button>
      </div>
    </LegacyCard>
  );
}

/* ── CLImportCard (self-contained) ────────────────────────────────── */

function CLImportCard() {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<GlobalImportResult | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const toast = useToast();

  const handleFile = async (file: File) => {
    try {
      setLoading(true);
      setResult(null);
      const res = await api.globalImportCL(file);
      setResult(res);
      if (res.unmatched > 0) {
        toast.warning(`CL import: ${res.allocated} allocated, ${res.refreshed} refreshed, ${res.unmatched} unmatched`);
      } else {
        toast.success(`CL import: ${res.allocated} allocated, ${res.refreshed} refreshed`);
      }
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <LegacyCard
      icon={<UploadIcon />}
      title="CL CSV Import"
      description="Upload a Card Ladder CSV to allocate and refresh purchases"
    >
      <Button size="sm" variant="secondary" fullWidth loading={loading} onClick={() => fileRef.current?.click()}>
        Upload CSV
      </Button>
      <input
        ref={fileRef}
        type="file"
        accept=".csv"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) handleFile(file);
          e.target.value = '';
        }}
      />
      {result && (
        <div className="mt-2 text-xs text-[var(--text-muted)]">
          {result.allocated > 0 && <span className="text-[var(--success)] mr-2">{result.allocated} allocated</span>}
          {result.refreshed > 0 && <span className="text-[var(--info)] mr-2">{result.refreshed} refreshed</span>}
          {result.unmatched > 0 && <span className="text-[var(--warning)]">{result.unmatched} unmatched</span>}
        </div>
      )}
    </LegacyCard>
  );
}

/* ── LegacyTab ───────────────────────────────────────────────────── */

export default function LegacyTab() {
  const [expandedCard, setExpandedCard] = useState<ExpandedCard>(null);

  const toggle = (card: ExpandedCard) => {
    setExpandedCard((prev) => (prev === card ? null : card));
  };

  return (
    <>
      {/* Section header */}
      <div className="mb-4">
        <h2 className="text-base font-semibold text-[var(--text)]">Legacy Tools</h2>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">
          Transitional tools — will be removed after full DH migration
        </p>
      </div>

      {/* Card intake grid */}
      <div className="mb-6">
        <CardIntakeSection />
      </div>

      {/* Expandable tool cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6">
        <LegacyCard
          icon={<SyncIcon />}
          title="Price Sync"
          description="Sync prices with Shopify store"
        >
          <Button
            size="sm"
            variant={expandedCard === 'priceSync' ? 'primary' : 'secondary'}
            fullWidth
            onClick={() => toggle('priceSync')}
            aria-expanded={expandedCard === 'priceSync'}
            aria-controls="priceSync-panel"
          >
            {expandedCard === 'priceSync' ? 'Collapse' : 'Open Price Sync'}
          </Button>
        </LegacyCard>

        <LegacyCard
          icon={<TagIcon />}
          title="eBay Export"
          description="Generate eBay bulk listing CSV"
        >
          <Button
            size="sm"
            variant={expandedCard === 'ebay' ? 'primary' : 'secondary'}
            fullWidth
            onClick={() => toggle('ebay')}
            aria-expanded={expandedCard === 'ebay'}
            aria-controls="ebay-panel"
          >
            {expandedCard === 'ebay' ? 'Collapse' : 'Open eBay Export'}
          </Button>
        </LegacyCard>

        <LegacyCard
          icon={<ReceiptIcon />}
          title="Import Sales"
          description="Import sales from order CSVs"
        >
          <Button
            size="sm"
            variant={expandedCard === 'sales' ? 'primary' : 'secondary'}
            fullWidth
            onClick={() => toggle('sales')}
            aria-expanded={expandedCard === 'sales'}
            aria-controls="sales-panel"
          >
            {expandedCard === 'sales' ? 'Collapse' : 'Open Import Sales'}
          </Button>
        </LegacyCard>
      </div>

      {/* Expanded content — full-width below grid */}
      {expandedCard === 'priceSync' && (
        <div id="priceSync-panel" role="region" aria-label="Price Sync">
          <SectionErrorBoundary sectionName="Price Sync">
            <ShopifySyncPage embedded />
          </SectionErrorBoundary>
        </div>
      )}

      {expandedCard === 'ebay' && <EbayDescriptionSection />}

      {expandedCard === 'sales' && <SalesImportSection />}
    </>
  );
}
