import { useRef, useState, ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import type { Campaign, GlobalImportResult, PSAImportResult, ExternalImportResult } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { formatCents, getErrorMessage } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { useInvoices, useUpdateInvoice } from '../../queries/useCampaignQueries';
import { Button, CardShell } from '../../ui';
import ImportResultsDetail from './ImportResultsDetail';

export type OperationState = 'idle' | 'importing' | 'exporting' | 'importing-psa' | 'importing-external';

/* ── FileUploadButton ─────────────────────────────────────────────── */

function FileUploadButton({ label, loading, accept, onFile, busy = false, variant = 'secondary', fullWidth = true, size = 'sm' }: {
  label: string;
  loading: boolean;
  accept: string;
  onFile: (file: File) => void;
  busy?: boolean;
  variant?: 'primary' | 'secondary' | 'success' | 'danger' | 'warning' | 'ghost' | 'link';
  fullWidth?: boolean;
  size?: 'sm' | 'md' | 'lg';
}) {
  const fileRef = useRef<HTMLInputElement>(null);

  return (
    <>
      <Button
        size={size}
        variant={variant}
        fullWidth={fullWidth}
        loading={loading}
        disabled={busy && !loading}
        onClick={() => { if (!busy) fileRef.current?.click(); }}
      >
        {label}
      </Button>
      <input
        ref={fileRef}
        type="file"
        accept={accept}
        className="hidden"
        onChange={(e) => {
          if (busy) return;
          const file = e.target.files?.[0];
          if (file) onFile(file);
          e.target.value = '';
        }}
      />
    </>
  );
}

/* ── OperationCard ────────────────────────────────────────────────── */

function OperationCard({ icon, title, description, action }: {
  icon: ReactNode;
  title: string;
  description: string;
  action: ReactNode;
}) {
  return (
    <CardShell variant="default" padding="lg" role="region" ariaLabel={title}>
      <div className="flex flex-col items-center text-center gap-3">
        {icon}
        <div>
          <div className="text-sm font-semibold text-[var(--text)]">{title}</div>
          <div className="text-xs text-[var(--text-muted)] mt-0.5">{description}</div>
        </div>
        <div className="w-full">{action}</div>
      </div>
    </CardShell>
  );
}

/* ── Inline SVG Icons ─────────────────────────────────────────────── */

function IconCircle({ color, children }: { color: string; children: ReactNode }) {
  return (
    <div className={`w-10 h-10 rounded-full flex items-center justify-center ${color}`}>
      {children}
    </div>
  );
}

function UploadIcon() {
  return (
    <IconCircle color="bg-[var(--brand-500)]/15 text-[var(--brand-500)]">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
        <polyline points="17 8 12 3 7 8" />
        <line x1="12" y1="3" x2="12" y2="15" />
      </svg>
    </IconCircle>
  );
}

function DownloadIcon() {
  return (
    <IconCircle color="bg-cyan-500/15 text-cyan-400">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
        <polyline points="7 10 12 15 17 10" />
        <line x1="12" y1="15" x2="12" y2="3" />
      </svg>
    </IconCircle>
  );
}

function FileTextIcon() {
  return (
    <IconCircle color="bg-[var(--warning-bg)] text-[var(--warning)]">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
        <polyline points="14 2 14 8 20 8" />
        <line x1="16" y1="13" x2="8" y2="13" />
        <line x1="16" y1="17" x2="8" y2="17" />
        <polyline points="10 9 9 9 8 9" />
      </svg>
    </IconCircle>
  );
}

function ShopBagIcon() {
  return (
    <IconCircle color="bg-[var(--success-bg)] text-[var(--success)]">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        <path d="M6 2L3 6v14a2 2 0 002 2h14a2 2 0 002-2V6l-3-4z" />
        <line x1="3" y1="6" x2="21" y2="6" />
        <path d="M16 10a4 4 0 01-8 0" />
      </svg>
    </IconCircle>
  );
}

function ReceiptIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--text-muted)]" aria-hidden="true" focusable="false">
      <path d="M4 2v20l2-1 2 1 2-1 2 1 2-1 2 1 2-1 2 1V2l-2 1-2-1-2 1-2-1-2 1-2-1-2 1-2-1z" />
      <line x1="8" y1="10" x2="16" y2="10" />
      <line x1="8" y1="14" x2="16" y2="14" />
    </svg>
  );
}

/* ── InvoiceSection ───────────────────────────────────────────────── */

function InvoiceSection() {
  const { data: invoices = [] } = useInvoices();
  const updateInvoice = useUpdateInvoice();
  const toast = useToast();

  if (invoices.length === 0) return null;

  function handleMarkPaid(id: string) {
    const inv = invoices.find(i => i.id === id);
    if (!inv) return;
    const now = new Date();
    const paidDate = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
    updateInvoice.mutate(
      { id, data: { ...inv, status: 'paid', paidDate, paidCents: inv.totalCents } },
      {
        onSuccess: () => toast.success('Invoice marked as paid'),
        onError: (err) => toast.error(getErrorMessage(err, 'Failed to update invoice')),
      },
    );
  }

  const statusBadge = (status: string) => {
    if (status === 'paid') return 'bg-[var(--success-bg)] text-[var(--success)]';
    if (status === 'partial') return 'bg-[var(--warning-bg)] text-[var(--warning)]';
    return 'bg-[var(--danger-bg)] text-[var(--danger)]';
  };

  return (
    <div className="mb-6 p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text)] mb-3 flex items-center gap-2">
        <ReceiptIcon />
        Invoices
      </h3>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--surface-2)]">
              <th className="text-left py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Date</th>
              <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Total</th>
              <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Paid</th>
              <th className="text-center py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Status</th>
              <th className="text-right py-2 px-3 text-[var(--text-muted)] font-medium text-xs">Due</th>
              <th className="py-2 px-3"></th>
            </tr>
          </thead>
          <tbody>
            {invoices.map(inv => (
              <tr key={inv.id} className="border-b border-[var(--surface-2)]/50">
                <td className="py-2 px-3 text-xs text-[var(--text)]">{inv.invoiceDate}</td>
                <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(inv.totalCents)}</td>
                <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(inv.paidCents)}</td>
                <td className="py-2 px-3 text-xs text-center">
                  <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusBadge(inv.status)}`}>
                    {inv.status}
                  </span>
                </td>
                <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{inv.dueDate || '-'}</td>
                <td className="py-2 px-3 text-right">
                  {inv.status !== 'paid' && (
                    <Button
                      size="sm"
                      variant="success"
                      loading={updateInvoice.isPending}
                      onClick={() => handleMarkPaid(inv.id)}
                    >
                      Mark Paid
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/* ── OperationsTab ────────────────────────────────────────────────── */

export default function OperationsTab({ campaigns, operationState, setOperationState, importResult, setImportResult, psaResult, setPsaResult, externalResult, setExternalResult }: {
  campaigns: Campaign[];
  operationState: OperationState;
  setOperationState: (state: OperationState) => void;
  importResult: GlobalImportResult | null;
  setImportResult: (result: GlobalImportResult | null) => void;
  psaResult: PSAImportResult | null;
  setPsaResult: (result: PSAImportResult | null) => void;
  externalResult: ExternalImportResult | null;
  setExternalResult: (result: ExternalImportResult | null) => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const busy = operationState !== 'idle';
  const [exportMissingOnly, setExportMissingOnly] = useState(false);

  function invalidateAll() {
    queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
    queryClient.invalidateQueries({ queryKey: queryKeys.credit.summary });
    queryClient.invalidateQueries({ queryKey: queryKeys.credit.invoices });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.capitalTimeline });
    queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });
  }

  async function handleGlobalImport(file: File) {
    try {
      setOperationState('importing');
      setImportResult(null);
      const result = await api.globalImportCL(file);
      setImportResult(result);
      toast.success(`Allocated ${result.allocated}, refreshed ${result.refreshed}, unmatched ${result.unmatched}`);
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import'));
    } finally {
      setOperationState('idle');
    }
  }

  async function handleGlobalExport() {
    try {
      setOperationState('exporting');
      const blob = await api.globalExportCL(exportMissingOnly);
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
      setOperationState('idle');
    }
  }

  async function handlePSAImport(file: File) {
    try {
      setOperationState('importing-psa');
      setPsaResult(null);
      const result = await api.globalImportPSA(file);
      setPsaResult(result);
      toast.success(`PSA import: ${result.allocated} allocated, ${result.updated} updated, ${result.refunded} refunded${result.invoicesCreated ? `, ${result.invoicesCreated} invoices created` : ''}. Market pricing will update in the background.`);
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import PSA data'));
    } finally {
      setOperationState('idle');
    }
  }

  async function handleExternalImport(file: File) {
    try {
      setOperationState('importing-external');
      setExternalResult(null);
      const result = await api.globalImportExternal(file);
      setExternalResult(result);
      toast.success(`External import: ${result.imported} imported, ${result.updated} updated, ${result.skipped} skipped, ${result.failed} failed`);
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to import external data'));
    } finally {
      setOperationState('idle');
    }
  }

  return (
    <>
      {/* Section header */}
      <div className="mb-4">
        <h2 className="text-base font-semibold text-[var(--text)]">Data Operations</h2>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">Import and export campaign data</p>
      </div>

      {/* Action card grid — ordered by typical workflow: PSA → Export → Import → External */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <OperationCard
          icon={<FileTextIcon />}
          title="PSA Import"
          description="Import PSA data and create invoices"
          action={
            <FileUploadButton
              label="Upload PSA CSV"
              loading={operationState === 'importing-psa'}
              accept=".csv"
              onFile={handlePSAImport}
              busy={busy}
            />
          }
        />

        <OperationCard
          icon={<DownloadIcon />}
          title="Export for Card Ladder"
          description="Download inventory CSV to import into Card Ladder"
          action={
            <div className="flex flex-col gap-2 w-full">
              <label className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={exportMissingOnly}
                  onChange={(e) => setExportMissingOnly(e.target.checked)}
                  className="accent-[var(--brand-500)]"
                />
                Only missing CL data
              </label>
              <Button
                size="sm"
                variant="secondary"
                fullWidth
                loading={operationState === 'exporting'}
                disabled={busy && operationState !== 'exporting'}
                onClick={handleGlobalExport}
              >
                Download CSV
              </Button>
            </div>
          }
        />

        <OperationCard
          icon={<UploadIcon />}
          title="Import from Card Ladder"
          description="Upload a Card Ladder CSV to allocate new and refresh existing purchases"
          action={
            <FileUploadButton
              label="Upload CSV"
              loading={operationState === 'importing'}
              accept=".csv"
              onFile={handleGlobalImport}
              busy={busy}
            />
          }
        />

        <OperationCard
          icon={<ShopBagIcon />}
          title="External Import"
          description="Import a Shopify product export CSV for external purchases"
          action={
            <FileUploadButton
              label="Upload Shopify CSV"
              loading={operationState === 'importing-external'}
              accept=".csv"
              onFile={handleExternalImport}
              busy={busy}
            />
          }
        />

      </div>

      {/* Results area (full-width, below grid) */}
      {importResult && (
        <div className="mb-4 p-3 rounded-lg bg-[var(--surface-2)]/50 text-sm">
          <div className="flex items-center justify-between mb-1">
            <span className="font-medium text-[var(--text)]">Import Complete</span>
            <button type="button" onClick={() => setImportResult(null)} className="text-[var(--text-muted)] hover:text-[var(--text)] text-xs">Dismiss</button>
          </div>
          <div className="flex flex-wrap gap-3 text-xs">
            {importResult.allocated > 0 && <span className="text-[var(--success)]">{importResult.allocated} allocated</span>}
            {importResult.refreshed > 0 && <span className="text-[var(--info)]">{importResult.refreshed} refreshed</span>}
            {importResult.unmatched > 0 && <span className="text-[var(--warning)]">{importResult.unmatched} unmatched</span>}
            {importResult.ambiguous > 0 && <span className="text-orange-400">{importResult.ambiguous} ambiguous</span>}
            {importResult.skipped > 0 && <span className="text-[var(--text-muted)]">{importResult.skipped} skipped</span>}
            {importResult.failed > 0 && <span className="text-[var(--danger)]">{importResult.failed} failed</span>}
          </div>
          {importResult.byCampaign && Object.keys(importResult.byCampaign).length > 0 && (
            <div className="mt-2 text-xs text-[var(--text-muted)]">
              {Object.entries(importResult.byCampaign).map(([campaignId, s]) => (
                <div key={campaignId}>{s.campaignName}: {s.allocated} new, {s.refreshed} refreshed</div>
              ))}
            </div>
          )}
          {importResult.results?.some(r => r.status === 'unmatched' || r.status === 'ambiguous') && (
            <ImportResultsDetail
              results={importResult.results}
              campaigns={campaigns}
              onItemResolved={() => queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all })}
            />
          )}
        </div>
      )}

      {psaResult && (
        <div className="mb-4 p-3 rounded-lg bg-[var(--surface-2)]/50 text-sm">
          <div className="flex items-center justify-between mb-1">
            <span className="font-medium text-[var(--text)]">PSA Import Complete</span>
            <button type="button" onClick={() => setPsaResult(null)} className="text-[var(--text-muted)] hover:text-[var(--text)] text-xs">Dismiss</button>
          </div>
          <div className="flex flex-wrap gap-3 text-xs">
            {psaResult.allocated > 0 && <span className="text-[var(--success)]">{psaResult.allocated} allocated</span>}
            {psaResult.updated > 0 && <span className="text-[var(--info)]">{psaResult.updated} updated</span>}
            {psaResult.refunded > 0 && <span className="text-orange-400">{psaResult.refunded} refunded</span>}
            {psaResult.invoicesCreated != null && psaResult.invoicesCreated > 0 && <span className="text-cyan-400">{psaResult.invoicesCreated} invoices created</span>}
            {psaResult.unmatched > 0 && <span className="text-[var(--warning)]">{psaResult.unmatched} unmatched</span>}
            {psaResult.ambiguous > 0 && <span className="text-orange-400">{psaResult.ambiguous} ambiguous</span>}
            {psaResult.skipped > 0 && <span className="text-[var(--text-muted)]">{psaResult.skipped} skipped</span>}
            {psaResult.failed > 0 && <span className="text-[var(--danger)]">{psaResult.failed} failed</span>}
          </div>
          {psaResult.byCampaign && Object.keys(psaResult.byCampaign).length > 0 && (
            <div className="mt-2 text-xs text-[var(--text-muted)]">
              {Object.entries(psaResult.byCampaign).map(([campaignId, s]) => (
                <div key={campaignId}>{s.campaignName}: {s.allocated} new, {s.refreshed} refreshed</div>
              ))}
            </div>
          )}
          {psaResult.results?.some(r => r.status === 'unmatched' || r.status === 'ambiguous') && (
            <ImportResultsDetail
              results={psaResult.results}
              campaigns={campaigns}
              onItemResolved={() => queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all })}
            />
          )}
        </div>
      )}

      {externalResult && (
        <div className="mb-4 p-3 rounded-lg bg-[var(--surface-2)]/50 text-sm">
          <div className="flex items-center justify-between mb-1">
            <span className="font-medium text-[var(--text)]">External Import Complete</span>
            <button type="button" onClick={() => setExternalResult(null)} className="text-[var(--text-muted)] hover:text-[var(--text)] text-xs">Dismiss</button>
          </div>
          <div className="flex flex-wrap gap-3 text-xs">
            {externalResult.imported > 0 && <span className="text-[var(--success)]">{externalResult.imported} imported</span>}
            {externalResult.updated > 0 && <span className="text-[var(--info)]">{externalResult.updated} updated</span>}
            {externalResult.skipped > 0 && <span className="text-[var(--text-muted)]">{externalResult.skipped} skipped</span>}
            {externalResult.failed > 0 && <span className="text-[var(--danger)]">{externalResult.failed} failed</span>}
          </div>
          {externalResult.errors && externalResult.errors.length > 0 && (
            <div className="mt-2 text-xs text-[var(--danger)]">
              {externalResult.errors.map((e, idx) => (
                <div key={idx}>{e.row != null ? `Row ${e.row}: ` : ''}{e.error}</div>
              ))}
            </div>
          )}
        </div>
      )}

      <InvoiceSection />
    </>
  );
}
