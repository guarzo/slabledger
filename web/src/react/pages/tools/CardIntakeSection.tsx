import { useState, useRef } from 'react';
import { useMutation } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { reportError } from '../../../js/errors';
import type { ExternalImportResult, MMRefreshResult } from '../../../types/campaigns';
import { getErrorMessage } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import type { ReactNode } from 'react';

/* ── TransitionalBadge ───────────────────────────────────────────── */

function TransitionalBadge() {
  return (
    <span className="inline-flex items-center px-1.5 py-0.5 rounded-full text-[10px] font-medium bg-[var(--warning)]/15 text-[var(--warning)]">
      Transitional
    </span>
  );
}

/* ── LegacyCard ──────────────────────────────────────────────────── */

export function LegacyCard({ icon, title, description, children }: {
  icon: ReactNode;
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <CardShell variant="default" padding="lg" role="region" ariaLabel={title}>
      <div className="flex flex-col items-center text-center gap-3">
        <div className="w-10 h-10 rounded-full flex items-center justify-center bg-[var(--warning)]/10 text-[var(--warning)]">
          {icon}
        </div>
        <div className="flex flex-col items-center gap-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-[var(--text)]">{title}</span>
            <TransitionalBadge />
          </div>
          <div className="text-xs text-[var(--text-muted)] mt-0.5">{description}</div>
        </div>
        <div className="w-full">{children}</div>
      </div>
    </CardShell>
  );
}

/* ── downloadBlob ─────────────────────────────────────────────────── */

function downloadBlob(blob: Blob, filename: string, revokeDelayMs = 100): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), revokeDelayMs);
}

/* ── Icons ────────────────────────────────────────────────────────── */

function ShopBagIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d="M6 2L3 6v14a2 2 0 002 2h14a2 2 0 002-2V6l-3-4z" />
      <line x1="3" y1="6" x2="21" y2="6" />
      <path d="M16 10a4 4 0 01-8 0" />
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

/* ── ExternalImportCard ──────────────────────────────────────────── */

function ExternalImportCard() {
  const fileRef = useRef<HTMLInputElement>(null);
  const toast = useToast();

  const importMutation = useMutation<ExternalImportResult, Error, File>({
    mutationFn: (file: File) => api.globalImportExternal(file),
    onSuccess: (res) => {
      toast.success(`External import: ${res.imported} imported, ${res.updated} updated, ${res.skipped} skipped, ${res.failed} failed`);
    },
    onError: (err) => {
      toast.error(getErrorMessage(err, 'Failed to import external data'));
    },
  });

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
        loading={importMutation.isPending}
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
          if (file) importMutation.mutate(file);
          e.target.value = '';
        }}
      />
      {importMutation.data && (
        <div className="mt-3 p-2 rounded bg-[var(--surface-2)]/50 text-xs text-left">
          <div className="flex flex-wrap gap-2">
            {importMutation.data.imported > 0 && <span className="text-[var(--success)]">{importMutation.data.imported} imported</span>}
            {importMutation.data.updated > 0 && <span className="text-[var(--info)]">{importMutation.data.updated} updated</span>}
            {importMutation.data.skipped > 0 && <span className="text-[var(--text-muted)]">{importMutation.data.skipped} skipped</span>}
            {importMutation.data.failed > 0 && <span className="text-[var(--danger)]">{importMutation.data.failed} failed</span>}
          </div>
          {importMutation.data.errors && importMutation.data.errors.length > 0 && (
            <div className="mt-1 text-[var(--danger)]">
              {importMutation.data.errors.map((e, idx) => (
                <div key={idx}>{e.row != null ? `Row ${e.row}: ` : ''}{e.error}</div>
              ))}
            </div>
          )}
        </div>
      )}
    </LegacyCard>
  );
}

/* ── MMExportCard ────────────────────────────────────────────────── */

function MMExportCard() {
  const [missingOnly, setMissingOnly] = useState(false);
  const toast = useToast();

  const exportMutation = useMutation<Blob, Error, boolean>({
    mutationFn: (missing: boolean) => api.globalExportMM(missing),
    onSuccess: (blob) => {
      downloadBlob(blob, 'market-movers-export.csv');
      toast.success('Market Movers CSV exported');
    },
    onError: (err) => {
      toast.error(getErrorMessage(err, 'Failed to export'));
    },
  });

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
          loading={exportMutation.isPending}
          onClick={() => exportMutation.mutate(missingOnly)}
        >
          Download CSV
        </Button>
      </div>
    </LegacyCard>
  );
}

/* ── MMImportCard ────────────────────────────────────────────────── */

function MMImportCard() {
  const fileRef = useRef<HTMLInputElement>(null);
  const toast = useToast();

  const importMutation = useMutation<MMRefreshResult, Error, File>({
    mutationFn: (file: File) => api.globalRefreshMM(file),
    onSuccess: (res) => {
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
    },
    onError: (err) => {
      toast.error(getErrorMessage(err, 'Failed to import Market Movers data'));
    },
  });

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
        loading={importMutation.isPending}
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
          if (file) importMutation.mutate(file);
          e.target.value = '';
        }}
      />
      {importMutation.data && (
        <div className="mt-3 p-2 rounded bg-[var(--surface-2)]/50 text-xs text-left">
          <div className="flex flex-wrap gap-2">
            {importMutation.data.updated > 0 && <span className="text-[var(--success)]">{importMutation.data.updated} updated</span>}
            {importMutation.data.skipped > 0 && <span className="text-[var(--text-muted)]">{importMutation.data.skipped} skipped</span>}
            {importMutation.data.notFound > 0 && <span className="text-[var(--warning)]">{importMutation.data.notFound} not found</span>}
            {importMutation.data.failed > 0 && <span className="text-[var(--danger)]">{importMutation.data.failed} failed</span>}
          </div>
          {importMutation.data.errors && importMutation.data.errors.length > 0 && (
            <div className="mt-1 text-[var(--danger)]">
              {importMutation.data.errors.map((e, idx) => (
                <div key={idx}>{e.row != null ? `Row ${e.row}: ` : ''}{e.error}</div>
              ))}
            </div>
          )}
        </div>
      )}
    </LegacyCard>
  );
}

/* ── CardIntakeSection ───────────────────────────────────────────── */

export default function CardIntakeSection() {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
      <ExternalImportCard />
      <MMExportCard />
      <MMImportCard />
    </div>
  );
}
