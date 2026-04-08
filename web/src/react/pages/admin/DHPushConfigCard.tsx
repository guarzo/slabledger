import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { useToast } from '../../contexts/ToastContext';
import type { DHPushConfig } from '../../../types/apiStatus';
import { formatCents } from '../../utils/formatters';

function ConfigField({ id, label, value, onChange, suffix }: {
  id: string;
  label: string;
  value: number;
  onChange: (v: number) => void;
  suffix: string;
}) {
  const inputId = `cfg-${id}`;
  const descId = `cfg-${id}-desc`;
  return (
    <div>
      <label htmlFor={inputId} className="block text-xs text-[var(--text-muted)] mb-1">{label}</label>
      <div className="flex items-center gap-2">
        <input
          id={inputId}
          type="number"
          min={0}
          value={value}
          onChange={(e) => onChange(parseInt(e.target.value, 10) || 0)}
          aria-describedby={descId}
          className="w-24 px-2 py-1.5 text-sm rounded-lg bg-[var(--surface-0)] border border-[var(--surface-2)] text-[var(--text)]"
        />
        <span id={descId} className="text-xs text-[var(--text-muted)]">{suffix}</span>
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
          id="swing-pct"
          label="Price Swing %"
          value={form.swingPctThreshold}
          onChange={(v) => setForm({ ...form, swingPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          id="swing-min"
          label="Price Swing Min"
          value={form.swingMinCents}
          onChange={(v) => setForm({ ...form, swingMinCents: v })}
          suffix={`(${formatCents(form.swingMinCents)})`}
        />
        <hr className="border-[var(--surface-2)] col-span-full" />
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide col-span-full">Source &amp; CL Rules</p>
        <ConfigField
          id="disagreement-pct"
          label="Source Disagreement %"
          value={form.disagreementPctThreshold}
          onChange={(v) => setForm({ ...form, disagreementPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          id="unreviewed-pct"
          label="Unreviewed CL Change %"
          value={form.unreviewedChangePctThreshold}
          onChange={(v) => setForm({ ...form, unreviewedChangePctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          id="unreviewed-min"
          label="Unreviewed CL Change Min"
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
