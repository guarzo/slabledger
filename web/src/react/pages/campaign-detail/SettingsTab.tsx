import { useState, useRef, useEffect } from 'react';
import { api } from '../../../js/api';
import type { Campaign } from '../../../types/campaigns';
import { formatCents, formatPct } from '../../utils/formatters';
import { useToast } from '../../contexts/ToastContext';
import { Input, Button, ConfirmDialog } from '../../ui';
import CampaignFormFields from '../../ui/CampaignFormFields';
import { useActivationChecklist } from '../../queries/useCampaignQueries';

export default function SettingsTab({ campaign, onUpdate, onDelete }: {
  campaign: Campaign;
  onUpdate: ((c: Campaign) => void) | (() => void);
  onDelete: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState(campaign);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const toast = useToast();
  const mountedRef = useRef(true);
  useEffect(() => { mountedRef.current = true; return () => { mountedRef.current = false; }; }, []);

  const isClosed = campaign.phase === 'closed';
  const showChecklist = campaign.phase !== 'active' && campaign.phase !== 'closed';
  const { data: checklist, isLoading: checklistLoading } = useActivationChecklist(showChecklist ? campaign.id : '');

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    try {
      setSaving(true);
      const updated = await api.updateCampaign(campaign.id, form);
      if (mountedRef.current) {
        onUpdate(updated);
        setEditing(false);
        toast.success('Settings saved');
      }
    } catch {
      if (mountedRef.current) toast.error('Failed to save settings');
    } finally {
      if (mountedRef.current) setSaving(false);
    }
  }

  async function handleDelete() {
    try {
      setDeleting(true);
      await api.deleteCampaign(campaign.id);
      toast.success('Campaign deleted');
      onDelete();
    } catch {
      toast.error('Failed to delete campaign');
    } finally {
      if (mountedRef.current) {
        setDeleting(false);
        setShowDeleteConfirm(false);
        setDeleteConfirmText('');
      }
    }
  }

  async function handleReopen() {
    try {
      setSaving(true);
      const updated = await api.updateCampaign(campaign.id, { ...campaign, phase: 'pending' as Campaign['phase'] });
      onUpdate(updated);
      toast.success('Campaign reopened');
    } catch {
      toast.error('Failed to reopen campaign');
    } finally {
      if (mountedRef.current) setSaving(false);
    }
  }

  if (!editing) {
    return (
      <div className="space-y-6">
        <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider">Campaign Settings</h3>
            {isClosed ? (
              <Button variant="secondary" size="sm" loading={saving} onClick={handleReopen}>Reopen</Button>
            ) : (
              <Button variant="link" size="sm" onClick={() => setEditing(true)}>Edit</Button>
            )}
          </div>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-4 text-sm">
            <div><span className="text-[var(--text-muted)]">Phase:</span> <span className="text-[var(--text)] ml-1">{campaign.phase}</span></div>
            <div><span className="text-[var(--text-muted)]">Buy Terms:</span> <span className="text-[var(--text)] ml-1">{formatPct(campaign.buyTermsCLPct)} CL</span></div>
            <div><span className="text-[var(--text-muted)]">Daily Cap:</span> <span className="text-[var(--text)] ml-1">{formatCents(campaign.dailySpendCapCents)}</span></div>
            <div><span className="text-[var(--text-muted)]">eBay Fee:</span> <span className="text-[var(--text)] ml-1">{formatPct(campaign.ebayFeePct)}</span></div>
            <div><span className="text-[var(--text-muted)]">Sourcing Fee:</span> <span className="text-[var(--text)] ml-1">{formatCents(campaign.psaSourcingFeeCents)}</span></div>
            <div><span className="text-[var(--text-muted)]">Sport:</span> <span className="text-[var(--text)] ml-1">{campaign.sport || 'Pokemon'}</span></div>
            {campaign.yearRange && <div><span className="text-[var(--text-muted)]">Year Range:</span> <span className="text-[var(--text)] ml-1">{campaign.yearRange}</span></div>}
            {campaign.gradeRange && <div><span className="text-[var(--text-muted)]">Grade Range:</span> <span className="text-[var(--text)] ml-1">PSA {campaign.gradeRange}</span></div>}
            {campaign.clConfidence && <div><span className="text-[var(--text-muted)]">CL Confidence:</span> <span className="text-[var(--text)] ml-1">{campaign.clConfidence}</span></div>}
            {campaign.priceRange && <div><span className="text-[var(--text-muted)]">Price Range:</span> <span className="text-[var(--text)] ml-1">${campaign.priceRange}</span></div>}
            {campaign.expectedFillRate > 0 && <div><span className="text-[var(--text-muted)]">Expected Fill Rate:</span> <span className="text-[var(--text)] ml-1">{campaign.expectedFillRate}%</span></div>}
            {campaign.inclusionList && <div className="col-span-2 md:col-span-3"><span className="text-[var(--text-muted)]">{campaign.exclusionMode ? 'Exclusion List:' : 'Inclusion List:'}</span> <span className="text-[var(--text)] ml-1">{campaign.inclusionList}</span></div>}
          </div>
        </div>

        {/* Activation Readiness */}
        {showChecklist && (
          <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider">Activation Readiness</h3>
              {checklistLoading ? (
                <span className="text-xs text-[var(--text-muted)]">Loading...</span>
              ) : checklist?.allPassed ? (
                <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-[var(--success-bg)] text-[var(--success)]">
                  Ready to activate
                </span>
              ) : (
                <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-[var(--warning-bg)] text-[var(--warning)]">
                  Review before activating
                </span>
              )}
            </div>
            {checklistLoading ? (
              <p className="text-sm text-[var(--text-muted)]">Checking activation readiness...</p>
            ) : checklist ? (
              <div className="space-y-2">
                {(checklist.checks ?? []).map(check => (
                  <div key={check.name} className="flex items-start gap-2 text-sm">
                    <span className={`mt-0.5 shrink-0 ${check.passed ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                      {check.passed ? '\u2713' : '\u2717'}
                    </span>
                    <div>
                      <span className="font-medium text-[var(--text)]">{check.name}</span>
                      <span className="text-[var(--text-muted)] ml-1">— {check.message}</span>
                    </div>
                  </div>
                ))}
                {checklist.warnings?.length > 0 && (
                  <div className="mt-3 pt-3 border-t border-[var(--surface-2)] space-y-1">
                    {(checklist.warnings ?? []).map((warning, i) => (
                      <p key={i} className="text-sm text-[var(--warning)]">Warning: {warning}</p>
                    ))}
                  </div>
                )}
              </div>
            ) : null}
          </div>
        )}

        {/* Danger Zone */}
        <div className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--danger-border)]">
          <h3 className="text-sm font-semibold text-[var(--danger)] uppercase tracking-wider mb-2">Danger Zone</h3>
          <p className="text-sm text-[var(--text-muted)] mb-3">
            Permanently delete this campaign and all its purchases, sales, and analytics data. This cannot be undone.
          </p>
          <Button variant="danger" size="sm" onClick={() => setShowDeleteConfirm(true)}>
            Delete Campaign
          </Button>
          <ConfirmDialog
            open={showDeleteConfirm}
            title="Delete Campaign"
            message={`Type "${campaign.name}" to confirm deletion. All purchases, sales, and analytics data will be permanently removed.`}
            confirmLabel="Delete Forever"
            variant="danger"
            loading={deleting}
            disabled={deleteConfirmText !== campaign.name}
            onConfirm={handleDelete}
            onCancel={() => { setShowDeleteConfirm(false); setDeleteConfirmText(''); }}
          >
            <Input
              type="text"
              placeholder={campaign.name}
              value={deleteConfirmText}
              onChange={e => setDeleteConfirmText(e.target.value)}
              className="mt-3"
            />
          </ConfirmDialog>
        </div>
      </div>
    );
  }

  return (
    <form onSubmit={handleSave} className="p-4 bg-[var(--surface-1)] rounded-xl border border-[var(--surface-2)]">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-4">Edit Settings</h3>
      <CampaignFormFields
        values={form}
        onChange={(field, value) => setForm(prev => ({ ...prev, [field]: value }))}
        inputSize="sm"
        showPhase
        showFees
      />
      <div className="mt-4 flex gap-3 justify-end">
        <Button variant="ghost" size="sm" onClick={() => { setEditing(false); setForm(campaign); }}>Cancel</Button>
        <Button type="submit" size="sm" loading={saving}>Save Changes</Button>
      </div>
    </form>
  );
}
