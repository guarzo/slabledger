import { useState } from 'react';
import { Dialog } from 'radix-ui';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, isAPIError } from '../../../js/api';
import { getErrorMessage } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { Button, Select } from '../../ui';
import type { Campaign, ProposedDiff, CampaignFormData } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';

export interface PSAPublishModalProps {
  open: boolean;
  onClose: () => void;
  campaign: Campaign;
}

/**
 * Publish-to-PSA modal: shows link status, a pending before->after diff, and
 * a publish action. Consumes the 4 PSA campaign-sync endpoints (Task 8).
 */
export default function PSAPublishModal({ open, onClose, campaign }: PSAPublishModalProps) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [selectedPSAId, setSelectedPSAId] = useState('');
  const [diff, setDiff] = useState<ProposedDiff | null>(null);
  const [pushId, setPushId] = useState<string | undefined>(undefined);
  const [publishStatus, setPublishStatus] = useState<string | null>(null);
  const [createPreview, setCreatePreview] = useState<CampaignFormData | null>(null);

  const isLinked = !!campaign.psaCampaignRequestId;

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
    },
    onError: (err) => toast.error(getErrorMessage(err, 'Failed to check for changes')),
  });

  const proposeCreateMutation = useMutation({
    mutationFn: () => api.psaProposeCreate(campaign.id),
    onSuccess: (res) => {
      setCreatePreview(res.formData);
      setPushId(res.pushId);
      setPublishStatus(null);
    },
    onError: (err) => toast.error(getErrorMessage(err, 'Failed to prepare PSA campaign')),
  });

  const publishMutation = useMutation({
    mutationFn: () => {
      if (!pushId) throw new Error('No pending push to publish');
      return api.psaPublish(campaign.id, pushId);
    },
    onSuccess: (res) => {
      setPublishStatus(res.status);
      toast.success('Push approved for publish to PSA');
    },
    onError: (err) => {
      if (isAPIError(err) && err.status === 409) {
        toast.error('This push is no longer pending — check for changes again');
        return;
      }
      toast.error(getErrorMessage(err, 'Failed to publish to PSA'));
    },
  });

  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => { if (!isOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-[var(--surface-overlay)] data-[state=open]:animate-[fadeIn_150ms_ease-out]" />
        <Dialog.Content
          className="fixed right-0 top-0 bottom-0 z-50 w-[min(480px,calc(100%-2rem))] bg-[var(--surface-1)] border-l border-[var(--surface-2)] p-6 shadow-2xl data-[state=open]:animate-[slideInFromRight_200ms_cubic-bezier(0.4,0,0.2,1)] overflow-y-auto"
        >
          <div className="flex items-center justify-between mb-4">
            <Dialog.Title className="text-lg font-semibold text-[var(--text)]">
              Publish to PSA
            </Dialog.Title>
            {isLinked && (
              <span className="text-xs text-[var(--text-muted)]">
                Linked: {campaign.psaCampaignRequestId}
              </span>
            )}
          </div>
          <Dialog.Description className="sr-only">
            Link this campaign to a PSA portal campaign and publish pending changes.
          </Dialog.Description>

          {!isLinked ? (
            <>
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

              <div className="mt-4 pt-4 border-t border-[var(--surface-2)] flex flex-col gap-3">
                <p className="text-xs text-[var(--text-muted)]">
                  Or create a new PSA portal campaign from this campaign&rsquo;s config.
                  It is created <span className="font-medium">paused</span> — add the
                  inclusion list in the portal before activating.
                </p>
                <Button
                  size="sm"
                  variant="secondary"
                  loading={proposeCreateMutation.isPending}
                  onClick={() => proposeCreateMutation.mutate()}
                >
                  Create on PSA
                </Button>

                {createPreview && (
                  <div className="flex flex-col gap-1 text-xs text-[var(--text-muted)]">
                    {Object.entries({
                      Name: createPreview.campaignName,
                      Category: createPreview.category,
                      Status: 'PAUSED',
                      'Bid %': `${createPreview.bidPercentage}%`,
                      'Daily budget': `$${createPreview.dailyBudget}`,
                      'Flat fee': `$${createPreview.flatFee}`,
                      'Daily spec limit': `${createPreview.dailySpecLimit}`,
                      Grades: `${createPreview.gradeMinimum}–${createPreview.gradeMaximum}`,
                      Years: `${createPreview.yearMinimum}–${createPreview.yearMaximum}`,
                      Prices: `$${createPreview.priceMinimum}–$${createPreview.priceMaximum}`,
                      'CL confidence ≥': `${createPreview.cardLadderConfidenceMinimum}`,
                      Subjects: 'none (add in portal before activating)',
                    }).map(([k, v]) => (
                      <div key={k}>
                        <span className="font-medium text-[var(--text)]">{k}</span>: {v}
                      </div>
                    ))}
                  </div>
                )}

                {createPreview && pushId && !publishStatus && (
                  <Button size="sm" loading={publishMutation.isPending} onClick={() => publishMutation.mutate()}>
                    Approve &amp; queue create
                  </Button>
                )}
                {publishStatus && (
                  <span className="text-xs text-[var(--success)]">
                    Queued for harvester (status: {publishStatus}). The campaign links automatically once created.
                  </span>
                )}
              </div>
            </>
          ) : (
            <div className="flex flex-col gap-3">
              <Button
                size="sm"
                variant="secondary"
                loading={proposeMutation.isPending}
                onClick={() => proposeMutation.mutate()}
              >
                Check for changes
              </Button>

              {diff && diff.changes.length > 0 && (
                <div className="flex flex-col gap-1 text-xs text-[var(--text-muted)]">
                  {diff.changes.map((change) => (
                    <div key={change.field}>
                      <span className="font-medium text-[var(--text)]">{change.field}</span>
                      {': '}
                      {change.old} &rarr; {change.new}
                    </div>
                  ))}
                </div>
              )}

              {diff && diff.changes.length > 0 && pushId && (
                <Button
                  size="sm"
                  loading={publishMutation.isPending}
                  onClick={() => publishMutation.mutate()}
                >
                  Publish to PSA
                </Button>
              )}

              {publishStatus && (
                <span className="text-xs text-[var(--success)]">Status: {publishStatus}</span>
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
