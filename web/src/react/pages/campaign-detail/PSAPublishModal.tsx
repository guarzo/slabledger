import { Fragment, useState, type ReactNode } from 'react';
import { Dialog } from 'radix-ui';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, isAPIError } from '../../../js/api';
import { getErrorMessage } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { Button, Select, StatusPill, CardShell, SectionEyebrow } from '../../ui';
import type { Campaign, ProposedDiff, CampaignFormData, PSAPushRow } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { classifyPushStatus, syncState, SYNC_LABELS, SYNC_TONES } from '../../utils/psaPush';

/** Tinted status banner (icon + text), matching StatusPill's tone tokens.
    Used for push-lifecycle messages that need more room than a pill. */
function SyncBanner({ tone, children }: { tone: 'info' | 'warning' | 'danger' | 'success'; children: ReactNode }) {
  const toneVar = `var(--${tone})`;
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      className="flex gap-2.5 items-start px-3 py-2.5 rounded-[var(--radius-md)] border text-xs leading-relaxed"
      style={{
        backgroundColor: `color-mix(in oklab, ${toneVar} 10%, transparent)`,
        borderColor: `color-mix(in oklab, ${toneVar} 30%, transparent)`,
        color: 'var(--text)',
      }}
    >
      <span
        className="w-1.5 h-1.5 rounded-full flex-shrink-0 mt-1"
        style={{ backgroundColor: toneVar }}
        aria-hidden="true"
      />
      <span>{children}</span>
    </div>
  );
}

/** Key/value preview grid — replaces stacked "Label: value" sentences with
    a scannable two-column layout. */
function PreviewGrid({ rows }: { rows: Array<[string, ReactNode]> }) {
  return (
    <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-xs">
      {rows.map(([k, v]) => (
        <Fragment key={k}>
          <span className="text-[var(--text-subtle)] whitespace-nowrap">{k}</span>
          <span className="text-[var(--text)] font-medium tabular-nums">{v}</span>
        </Fragment>
      ))}
    </div>
  );
}

export interface PSAPublishModalProps {
  open: boolean;
  onClose: () => void;
  campaign: Campaign;
  /** The campaign's latest push-queue row (from GET /api/psa-pushes), if any. */
  pushRow?: PSAPushRow | null;
}

/**
 * Publish-to-PSA modal: shows link status, a pending before->after diff, and
 * a publish action. Consumes the 4 PSA campaign-sync endpoints (Task 8).
 */
export default function PSAPublishModal({ open, onClose, campaign, pushRow = null }: PSAPublishModalProps) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [selectedPSAId, setSelectedPSAId] = useState('');
  const [diff, setDiff] = useState<ProposedDiff | null>(null);
  const [pushId, setPushId] = useState<string | undefined>(undefined);
  const [publishStatus, setPublishStatus] = useState<string | null>(null);
  const [createPreview, setCreatePreview] = useState<CampaignFormData | null>(null);

  const isLinked = !!campaign.psaCampaignRequestId;

  // A queued-but-unresolved row from the shared psa-pushes query. Pending rows
  // are renderable/approvable directly — this is the 409-dead-end fix: the
  // preview and approve button no longer require a fresh propose call.
  const pushState = pushRow ? classifyPushStatus(pushRow.status) : null;
  const pendingRow = pushState === 'pending' ? pushRow : null;
  const inFlightRow = pushState === 'inflight' ? pushRow : null;
  const failedRow = pushState === 'failed' ? pushRow : null;

  // The queued row is authoritative. If the shared query now names a different
  // push than the one local mutation state came from (superseded out-of-band),
  // that local state is stale — drop it so the row's data and pushId win and
  // an old publishStatus can't suppress actions for the new row.
  const rowPushId = pushRow?.pushId;
  const [lastRowPushId, setLastRowPushId] = useState(rowPushId);
  if (rowPushId !== lastRowPushId) {
    setLastRowPushId(rowPushId);
    if (publishStatus) setPublishStatus(null);
    if (pushId && rowPushId && rowPushId !== pushId) {
      setDiff(null);
      setPushId(undefined);
      setCreatePreview(null);
    }
  }

  const effectiveCreatePreview = createPreview ?? ((pendingRow?.operation === 'create' || inFlightRow?.operation === 'create') ? (pendingRow?.formData ?? inFlightRow?.formData ?? null) : null);
  const effectiveDiff = diff ?? ((pendingRow?.operation === 'update' || inFlightRow?.operation === 'update') ? (pendingRow?.diff ?? inFlightRow?.diff ?? null) : null);
  const effectivePushId = pushId ?? pendingRow?.pushId;

  const { data: portalCampaignsData } = useQuery({
    queryKey: queryKeys.psaCampaigns.list,
    queryFn: () => api.listPSACampaigns(),
    enabled: open && !isLinked,
  });

  const linkMutation = useMutation({
    mutationFn: (psaCampaignRequestId: string) => api.psaLink(campaign.id, psaCampaignRequestId),
    onSuccess: () => {
      toast.success('Campaign linked to PSA portal campaign');
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
    },
    onError: (err) => toast.error(getErrorMessage(err, 'Failed to link campaign')),
  });

  const proposeMutation = useMutation({
    mutationFn: () => api.psaPropose(campaign.id),
    onSuccess: (res) => {
      setDiff(res.diff);
      setPushId(res.pushId);
      setPublishStatus(null);
      if (res.diff.changes.length === 0) {
        toast.success('No changes to publish — campaign already matches PSA');
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.psaPushes.list });
    },
    onError: (err) => toast.error(getErrorMessage(err, 'Failed to check for changes')),
  });

  const proposeCreateMutation = useMutation({
    mutationFn: () => api.psaProposeCreate(campaign.id),
    onSuccess: (res) => {
      setCreatePreview(res.formData);
      setPushId(res.pushId);
      setPublishStatus(null);
      queryClient.invalidateQueries({ queryKey: queryKeys.psaPushes.list });
    },
    onError: (err) => {
      if (isAPIError(err) && err.status === 409) {
        // A create is already queued — refresh the shared push list so the
        // queued row (and its approve button) renders instead of a dead end.
        queryClient.invalidateQueries({ queryKey: queryKeys.psaPushes.list });
        toast.error('A PSA create is already queued for this campaign — review it below');
        return;
      }
      toast.error(getErrorMessage(err, 'Failed to prepare PSA campaign'));
    },
  });

  const publishMutation = useMutation({
    mutationFn: () => {
      if (!effectivePushId) throw new Error('No pending push to publish');
      return api.psaPublish(campaign.id, effectivePushId);
    },
    onSuccess: (res) => {
      setPublishStatus(res.status);
      toast.success('Push approved for publish to PSA');
      queryClient.invalidateQueries({ queryKey: queryKeys.psaPushes.list });
    },
    onError: (err) => {
      if (isAPIError(err) && err.status === 409) {
        toast.error('This push is no longer pending — check for changes again');
        queryClient.invalidateQueries({ queryKey: queryKeys.psaPushes.list });
        return;
      }
      toast.error(getErrorMessage(err, 'Failed to publish to PSA'));
    },
  });

  const sync = syncState(isLinked, pushRow?.status);

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed right-0 top-0 bottom-0 z-50 w-[min(480px,calc(100%-2rem))] bg-[var(--surface-1)] border-l border-[var(--surface-2)] p-6 shadow-2xl data-[state=open]:animate-[slideInFromRight_200ms_cubic-bezier(0.4,0,0.2,1)] overflow-y-auto"
        >
          <div className="flex items-center justify-between gap-3 mb-1">
            <Dialog.Title className="text-lg font-semibold text-[var(--text)]">
              Publish to PSA
            </Dialog.Title>
            <StatusPill tone={SYNC_TONES[sync]}>{SYNC_LABELS[sync]}</StatusPill>
          </div>
          {isLinked && (
            <p className="text-xs text-[var(--text-subtle)] mb-4">
              Linked to <span className="font-medium text-[var(--text-muted)]">{campaign.psaCampaignRequestId}</span>
            </p>
          )}
          {!isLinked && <div className="mb-4" />}
          <Dialog.Description className="sr-only">
            Link this campaign to a PSA portal campaign and publish pending changes.
          </Dialog.Description>

          <div className="flex flex-col gap-3 mb-5">
            {inFlightRow && (
              <SyncBanner tone="info">
                Push in flight (status: {inFlightRow.status})
                {inFlightRow.approvedBy ? ` — approved by ${inFlightRow.approvedBy}` : ''}.
                The harvester picks it up on its next run.
              </SyncBanner>
            )}
            {failedRow && (
              <SyncBanner tone="danger">
                Last push failed{failedRow.error ? `: ${failedRow.error}` : ''}. Propose again to retry.
              </SyncBanner>
            )}
            {pendingRow?.operation === 'create' && !isLinked && (
              <SyncBanner tone="warning">
                A create is queued and awaiting approval
                {pendingRow.requestedBy ? ` (requested by ${pendingRow.requestedBy})` : ''}.
              </SyncBanner>
            )}
            {publishStatus && !inFlightRow && (
              <SyncBanner tone="success">
                {isLinked
                  ? `Status: ${publishStatus}`
                  : `Queued for harvester (status: ${publishStatus}). The campaign links automatically once created.`}
              </SyncBanner>
            )}
          </div>

          {!isLinked ? (
            <div className="flex flex-col gap-5">
              <div className="flex flex-col gap-2">
                <SectionEyebrow>Link an existing campaign</SectionEyebrow>
                <div className="flex items-center gap-2">
                  <Select
                    aria-label="PSA portal campaign"
                    value={selectedPSAId}
                    onChange={(e) => setSelectedPSAId(e.target.value)}
                    options={[
                      { value: '', label: 'Select a PSA campaign…' },
                      ...(portalCampaignsData?.campaigns ?? []).map((c) => ({
                        value: c.campaignRequestId,
                        label: c.name,
                      })),
                    ]}
                  />
                  <Button
                    size="sm"
                    disabled={!selectedPSAId}
                    loading={linkMutation.isPending}
                    onClick={() => linkMutation.mutate(selectedPSAId)}
                  >
                    Link
                  </Button>
                </div>
              </div>

              <div className="pt-4 border-t border-[var(--surface-2)] flex flex-col gap-3">
                <div>
                  <SectionEyebrow>Or create a new campaign</SectionEyebrow>
                  <p className="text-xs text-[var(--text-muted)] mt-1.5">
                    Built from this campaign&rsquo;s config. Created <span className="font-medium text-[var(--text)]">paused</span> —
                    add the inclusion list in the portal before activating.
                  </p>
                </div>
                {!pendingRow && !inFlightRow && (
                  <Button
                    size="sm"
                    variant="secondary"
                    loading={proposeCreateMutation.isPending}
                    onClick={() => proposeCreateMutation.mutate()}
                  >
                    Create on PSA
                  </Button>
                )}

                {effectiveCreatePreview && (
                  <CardShell variant="data" padding="sm">
                    <PreviewGrid
                      rows={[
                        ['Name', effectiveCreatePreview.campaignName],
                        ['Category', effectiveCreatePreview.category],
                        ['Status', 'PAUSED'],
                        ['Bid %', `${effectiveCreatePreview.bidPercentage}%`],
                        ['Daily budget', `$${effectiveCreatePreview.dailyBudget}`],
                        ['Flat fee', `$${effectiveCreatePreview.flatFee}`],
                        ['Daily spec limit', `${effectiveCreatePreview.dailySpecLimit}`],
                        ['Grades', `${effectiveCreatePreview.gradeMinimum}–${effectiveCreatePreview.gradeMaximum}`],
                        ['Years', `${effectiveCreatePreview.yearMinimum}–${effectiveCreatePreview.yearMaximum}`],
                        ['Prices', `$${effectiveCreatePreview.priceMinimum}–$${effectiveCreatePreview.priceMaximum}`],
                        ['CL confidence ≥', `${effectiveCreatePreview.cardLadderConfidenceMinimum}`],
                        ['Subjects', 'none (add in portal before activating)'],
                      ]}
                    />
                  </CardShell>
                )}

                {effectiveCreatePreview && effectivePushId && !publishStatus && !inFlightRow && (
                  <Button size="sm" loading={publishMutation.isPending} onClick={() => publishMutation.mutate()}>
                    Approve &amp; queue create
                  </Button>
                )}
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {/* psa-propose has no update dedupe on the backend — proposing
                  while a row is already queued would enqueue a duplicate, so
                  the button hides until the current row resolves or fails. */}
              {!pendingRow && !inFlightRow && (
                <Button
                  size="sm"
                  variant="secondary"
                  loading={proposeMutation.isPending}
                  onClick={() => proposeMutation.mutate()}
                >
                  Check for changes
                </Button>
              )}

              {effectiveDiff && (effectiveDiff.changes?.length ?? 0) === 0 && (
                <p className="text-xs text-[var(--text-subtle)]">Already matches PSA — nothing to publish.</p>
              )}

              {effectiveDiff && (effectiveDiff.changes?.length ?? 0) > 0 && (
                <CardShell variant="data" padding="sm">
                  <div className="flex flex-col gap-2 text-xs">
                    {effectiveDiff.changes.map((change) => (
                      <div key={change.field} className="flex items-baseline justify-between gap-3">
                        <span className="text-[var(--text-subtle)] whitespace-nowrap">{change.field}</span>
                        <span className="tabular-nums text-right">
                          <span className="text-[var(--text-muted)]">{change.old}</span>
                          <span className="text-[var(--text-subtle)] mx-1.5">&rarr;</span>
                          <span className="text-[var(--text)] font-medium">{change.new}</span>
                        </span>
                      </div>
                    ))}
                  </div>
                </CardShell>
              )}

              {effectiveDiff && (effectiveDiff.changes?.length ?? 0) > 0 && effectivePushId && !publishStatus && !inFlightRow && (
                <Button
                  size="sm"
                  loading={publishMutation.isPending}
                  onClick={() => publishMutation.mutate()}
                >
                  Publish to PSA
                </Button>
              )}
            </div>
          )}

          <div className="flex justify-end mt-6">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm">Close</Button>
            </Dialog.Close>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
