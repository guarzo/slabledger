import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { useToast } from '../../contexts/ToastContext';
import type { DHPushConfig } from '../../../types/apiStatus';
import { formatCents } from '../../utils/formatters';

function ConfigField({ label, description, value, onChange, suffix }: {
  label: string;
  description: string;
  value: number;
  onChange: (v: number) => void;
  suffix: string;
}) {
  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-[var(--text)]">{label}</label>
      <p className="text-xs text-[var(--text-muted)]">{description}</p>
      <div className="flex items-center gap-2 mt-1">
        <input
          type="number"
          min={0}
          value={value}
          onChange={(e) => onChange(parseInt(e.target.value, 10) || 0)}
          className="w-24 px-2 py-1.5 text-sm rounded-lg bg-[var(--surface-0)] border border-[var(--surface-2)] text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--brand-500)]"
        />
        <span className="text-xs text-[var(--text-muted)]">{suffix}</span>
      </div>
    </div>
  );
}

export function DHPushConfigCard() {
  const toast = useToast();
  const queryClient = useQueryClient();

  const { data: config, isLoading, isError, error, refetch } = useQuery({
    queryKey: ['admin', 'dh-push-config'],
    queryFn: () => api.getDHPushConfig(),
  });

  const [form, setForm] = useState<DHPushConfig | null>(null);

  const saveMutation = useMutation({
    mutationFn: (cfg: DHPushConfig) => api.saveDHPushConfig(cfg),
    onSuccess: () => {
      toast.success('DH push config saved');
      queryClient.invalidateQueries({ queryKey: ['admin', 'dh-push-config'] });
    },
    onError: () => toast.error('Failed to save config'),
  });

  useEffect(() => {
    if (config) setForm(config);
  }, [config]);

  if (isError) {
    return (
      <CardShell padding="lg">
        <div className="text-center">
          <p className="text-red-500 mb-2">Failed to load config: {String(error)}</p>
          <Button onClick={() => refetch()}>Retry</Button>
        </div>
      </CardShell>
    );
  }

  if (isLoading || !form) {
    return <CardShell padding="lg"><p className="text-[var(--text-muted)]">Loading...</p></CardShell>;
  }

  return (
    <CardShell padding="lg">
      <h4 className="text-sm font-semibold text-[var(--text)] mb-1">Listing Push Safety Rules</h4>
      <p className="text-xs text-[var(--text-muted)] mb-5">
        Price updates that exceed these thresholds are held for manual review before being pushed to your DoubleHolo listings.
      </p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide col-span-full">Price Swing Rules</p>
        <ConfigField
          label="Max Price Change"
          description="Flag re-pushes where the new price differs from the current listing by more than this percentage."
          value={form.swingPctThreshold}
          onChange={(v) => setForm({ ...form, swingPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          label="Min Dollar Amount"
          description="Only apply the swing rule when the absolute price difference exceeds this amount."
          value={form.swingMinCents}
          onChange={(v) => setForm({ ...form, swingMinCents: v })}
          suffix={`(${formatCents(form.swingMinCents)})`}
        />
        <hr className="border-[var(--surface-2)] col-span-full" />
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide col-span-full">Source &amp; CL Rules</p>
        <ConfigField
          label="Source Disagreement Limit"
          description="Hold a push when your pricing sources disagree by more than this percentage."
          value={form.disagreementPctThreshold}
          onChange={(v) => setForm({ ...form, disagreementPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          label="Max Unreviewed CL Shift"
          description="Flag pushes where the Card Ladder value has changed by more than this since your last review."
          value={form.unreviewedChangePctThreshold}
          onChange={(v) => setForm({ ...form, unreviewedChangePctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          label="Min CL Change Amount"
          description="Only apply the CL rule when the dollar shift exceeds this amount."
          value={form.unreviewedChangeMinCents}
          onChange={(v) => setForm({ ...form, unreviewedChangeMinCents: v })}
          suffix={`(${formatCents(form.unreviewedChangeMinCents)})`}
        />
      </div>
      <div className="mt-4">
        <Button
          size="sm"
          onClick={() => saveMutation.mutate(form)}
          loading={saveMutation.isPending}
        >
          Save
        </Button>
      </div>
    </CardShell>
  );
}
