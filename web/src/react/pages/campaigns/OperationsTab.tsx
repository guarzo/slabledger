import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { reportError } from '../../../js/errors';
import type { Campaign, PSAImportResult } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { getErrorMessage } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import ImportResultsDetail from './ImportResultsDetail';
import DHUnmatchedSection from '../tools/DHUnmatchedSection';
import { PendingItemsCard } from './PendingItemsCard';
import { useMarketMoversStatus, useSyncCardLadderCollection, useSyncMarketMoversCollection } from '../../queries/useAdminQueries';

export type OperationState = 'idle' | 'syncing-psa' | 'syncing-mm' | 'syncing-cl';

/* ── OperationCard ────────────────────────────────────────────────── */

function OperationCard({ icon, title, description, action }: {
  icon: ReactNode;
  title: string;
  description: string;
  action: ReactNode;
}) {
  return (
    <CardShell variant="default" padding="lg" role="region" ariaLabel={title}>
      <div className="flex flex-col gap-3">
        <div className="flex items-start gap-3">
          {icon}
          <div className="min-w-0">
            <div className="text-sm font-semibold text-[var(--text)]">{title}</div>
            <div className="text-xs text-[var(--text-muted)] mt-0.5 leading-relaxed">{description}</div>
          </div>
        </div>
        <div className="w-full">{action}</div>
      </div>
    </CardShell>
  );
}

/* ── Inline SVG Icons ─────────────────────────────────────────────── */

function IconCircle({ color, children }: { color: string; children: ReactNode }) {
  return (
    <div className={`w-11 h-11 rounded-xl flex items-center justify-center shrink-0 ${color}`}>
      {children}
    </div>
  );
}


function SyncIcon() {
  return (
    <IconCircle color="bg-[var(--success)]/15 text-[var(--success)]">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        <polyline points="23 4 23 10 17 10" />
        <polyline points="1 20 1 14 7 14" />
        <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
      </svg>
    </IconCircle>
  );
}

function CloudSyncIcon() {
  return (
    <IconCircle color="bg-[var(--success)]/15 text-[var(--success)]">
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        <path d="M12 9.75v6.75m0 0-3-3m3 3 3-3m-8.25 1.5a4.5 4.5 0 0 1-1.41-8.775 5.25 5.25 0 0 1 10.233-2.33 3 3 0 0 1 3.758 3.848A3.752 3.752 0 0 1 18 19.5H6.75Z" />
      </svg>
    </IconCircle>
  );
}

/* ── OperationsTab ────────────────────────────────────────────────── */

export default function OperationsTab({ campaigns, operationState, setOperationState, psaResult, setPsaResult }: {
  campaigns: Campaign[];
  operationState: OperationState;
  setOperationState: (state: OperationState) => void;
  psaResult: PSAImportResult | null;
  setPsaResult: (result: PSAImportResult | null) => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const busy = operationState !== 'idle';
  const { data: mmStatus } = useMarketMoversStatus();
  const clSync = useSyncCardLadderCollection();
  const mmSync = useSyncMarketMoversCollection();

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
    queryClient.invalidateQueries({ queryKey: queryKeys.admin.dhStatus });
    queryClient.invalidateQueries({ queryKey: queryKeys.admin.dhUnmatched });
    queryClient.invalidateQueries({ queryKey: queryKeys.admin.marketMoversStatus });
    queryClient.invalidateQueries({ queryKey: queryKeys.purchases.psaPendingItems });
    queryClient.invalidateQueries({ queryKey: queryKeys.admin.psaSyncStatus });
  }

  async function handleMMSync() {
    try {
      setOperationState('syncing-mm');
      const result = await mmSync.mutateAsync();
      if (result.failed > 0) {
        toast.warning(`MM sync: ${result.synced} synced, ${result.skipped} skipped, ${result.failed} failed`);
        if (result.errors && result.errors.length > 0) {
          const formatted = result.errors
            .map((e) => `${e.certNumber}: ${e.error}`)
            .join('; ');
          reportError(
            'OperationsTab/mm-sync',
            new Error(`MM sync errors: ${formatted}`),
          );
        }
      } else if (result.synced === 0 && result.skipped === 0) {
        toast.info('All items already synced to Market Movers');
      } else {
        toast.success(`MM sync: ${result.synced} items added to Market Movers collection`);
      }
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to sync to Market Movers'));
    } finally {
      setOperationState('idle');
    }
  }

  async function handleCLSync() {
    try {
      setOperationState('syncing-cl');
      const result = await clSync.mutateAsync();
      if (result.failed > 0) {
        toast.warning(`CL sync: ${result.synced} synced, ${result.skipped} skipped, ${result.failed} failed`);
      } else if (result.synced === 0 && result.skipped === 0) {
        toast.info('All items already synced to Card Ladder');
      } else {
        toast.success(`CL sync: ${result.synced} cards synced to Card Ladder`);
      }
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'CL sync failed'));
    } finally {
      setOperationState('idle');
    }
  }

  async function handlePSASheetsSync() {
    try {
      setOperationState('syncing-psa');
      setPsaResult(null);
      const result = await api.syncPSASheets();
      setPsaResult(result);
      const parts: string[] = [];
      if (result.allocated > 0) parts.push(`${result.allocated} allocated`);
      if (result.updated > 0) parts.push(`${result.updated} updated`);
      if (result.refunded > 0) parts.push(`${result.refunded} refunded`);
      const msg = parts.join(', ');
      if (msg) {
        toast.success(msg);
      } else {
        toast.info('No changes');
      }
      invalidateAll();
    } catch (err) {
      toast.error(getErrorMessage(err, 'PSA Sync Failed'));
    } finally {
      setOperationState('idle');
    }
  }

  return (
    <>
      {/* Section header */}
      <div className="mb-4">
        <h2 className="text-base font-semibold text-[var(--text)]">Daily Operations</h2>
        <p className="text-xs text-[var(--text-muted)] mt-0.5">Daily import, export, and matching workflow</p>
      </div>

      {/* Action card grid — ordered by typical workflow: PSA Sync → Export → Import */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
        <OperationCard
          icon={<CloudSyncIcon />}
          title="PSA Sync"
          description="Sync latest PSA data from Google Sheets"
          action={
            <Button
              size="sm"
              variant="primary"
              fullWidth
              loading={operationState === 'syncing-psa'}
              disabled={busy && operationState !== 'syncing-psa'}
              onClick={handlePSASheetsSync}
            >
              Sync from Sheets
            </Button>
          }
        />

        <OperationCard
          icon={<SyncIcon />}
          title="Sync to Card Ladder"
          description="Push unsold inventory with cert numbers directly to your CL collection via API"
          action={
            <Button
              size="sm"
              variant="secondary"
              fullWidth
              loading={operationState === 'syncing-cl'}
              disabled={busy && operationState !== 'syncing-cl'}
              onClick={handleCLSync}
            >
              Sync Collection
            </Button>
          }
        />

        <OperationCard
          icon={<SyncIcon />}
          title="Sync to Market Movers"
          description="Push mapped unsold inventory directly to your MM collection via API"
          action={
            <div className="flex flex-col gap-2 w-full">
              <Button
                size="sm"
                variant="secondary"
                fullWidth
                loading={operationState === 'syncing-mm'}
                disabled={busy && operationState !== 'syncing-mm'}
                onClick={handleMMSync}
              >
                Sync Collection
              </Button>
              {mmStatus?.priceStats && (
                <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-[10px] text-[var(--text-muted)]">
                  <span>Synced: <span className="text-[var(--text)]">{mmStatus.priceStats.syncedCount}</span></span>
                  <span>Priced: <span className="text-[var(--text)]">{mmStatus.priceStats.withMMPrice}/{mmStatus.priceStats.unsoldTotal}</span></span>
                  {mmStatus.priceStats.staleCount > 0 && (
                    <span>Stale: <span className="text-[var(--warning)]">{mmStatus.priceStats.staleCount}</span></span>
                  )}
                </div>
              )}
            </div>
          }
        />

      </div>



      {/* Results area (full-width, below grid) */}
      {psaResult && (        <div className="mb-4 p-3 rounded-lg bg-[var(--surface-2)]/50 text-sm">
          <div className="flex items-center justify-between mb-1">
            <span className="font-medium text-[var(--text)]">PSA Import Complete</span>
            <div className="flex items-center gap-3">
              <Link to="/inventory" className="text-xs font-medium text-[var(--brand-400)] hover:text-[var(--brand-300)] underline">
                Review prices &rarr;
              </Link>
              <button type="button" onClick={() => setPsaResult(null)} className="text-[var(--text-muted)] hover:text-[var(--text)] text-xs">Dismiss</button>
            </div>
          </div>
          <div className="flex flex-wrap gap-3 text-xs">
            {psaResult.allocated > 0 && <span className="text-[var(--success)]">{psaResult.allocated} allocated</span>}
            {psaResult.updated > 0 && <span className="text-[var(--info)]">{psaResult.updated} updated</span>}
            {psaResult.refunded > 0 && <span className="text-[var(--warning)]">{psaResult.refunded} refunded</span>}
            {psaResult.invoicesCreated != null && psaResult.invoicesCreated > 0 && <span className="text-cyan-400">{psaResult.invoicesCreated} invoices created</span>}
            {psaResult.invoicesUpdated != null && psaResult.invoicesUpdated > 0 && <span className="text-cyan-400">{psaResult.invoicesUpdated} invoices updated</span>}
            {psaResult.unmatched > 0 && <span className="text-[var(--warning)]">{psaResult.unmatched} unmatched</span>}
            {psaResult.ambiguous > 0 && <span className="text-[var(--warning)]">{psaResult.ambiguous} ambiguous</span>}
            {psaResult.skipped > 0 && <span className="text-[var(--text-muted)]">{psaResult.skipped} skipped</span>}
            {psaResult.failed > 0 && <span className="text-[var(--danger)]">{psaResult.failed} failed</span>}
          </div>
          {psaResult.byCampaign && Object.keys(psaResult.byCampaign).length > 0 && (
            <div className="mt-2 text-xs text-[var(--text-muted)]">
              {Object.entries(psaResult.byCampaign).map(([campaignId, s]) => (
                <div key={campaignId}>{s.campaignName}: {s.allocated} allocated</div>
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

      <PendingItemsCard />

      <DHUnmatchedSection />
    </>
  );
}
