import { useState, useEffect } from 'react';
import { CardShell } from '../../ui/CardShell';
import Button from '../../ui/Button';
import { useToast } from '../../contexts/ToastContext';
import { useDHPushConfig, useSaveDHPushConfig } from '../../queries/useAdminQueries';
import type { DHPushConfig } from '../../../types/apiStatus';
import { formatCents, getErrorMessage } from '../../utils/formatters';
import PokeballLoader from '../../PokeballLoader';

function ConfigField({ id, label, value, onChange, suffix }: {
  id: string;
  label: string;
  value: number;
  onChange: (v: number) => void;
  suffix: string;
}) {
  const inputId = `cfg-${id}`;
  const descId = `cfg-${id}-desc`;
  // The input keeps its own draft string so a cleared box stays cleared. The
  // parent only ever receives a real non-negative integer; an empty or
  // half-typed value simply doesn't propagate. The effect re-syncs the draft
  // when the value changes from outside (a refetch after save), but leaves a
  // draft that already parses to the same number alone so "07" isn't rewritten
  // mid-typing.
  const [draft, setDraft] = useState(String(value));
  useEffect(() => {
    setDraft((d) => (Number.parseInt(d, 10) === value ? d : String(value)));
  }, [value]);
  return (
    <div>
      <label htmlFor={inputId} className="block text-xs text-[var(--text-muted)] mb-1">{label}</label>
      <div className="flex items-center gap-2">
        <input
          id={inputId}
          type="number"
          min={0}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            const parsed = Number.parseInt(e.target.value, 10);
            if (Number.isFinite(parsed) && parsed >= 0) onChange(parsed);
          }}
          onBlur={() => {
            if (draft.trim() === '') setDraft(String(value));
          }}
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

  const { data: config, isLoading, isError, error, refetch } = useDHPushConfig();
  const saveMutation = useSaveDHPushConfig();

  const [form, setForm] = useState<DHPushConfig | null>(null);
  // Guards against DHListingsPauseControl's writes: its save seeds this same
  // query cache (useAdminQueries.ts:214-216), which produces a new `config`
  // identity and would otherwise re-fire this effect mid-edit, silently
  // discarding an operator's unsaved threshold changes. Once the form has any
  // local edit, stop re-seeding from the query until this card's own save
  // completes.
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (config && !dirty) setForm(config);
  }, [config, dirty]);

  // The card owns exactly these five fields. `listingsPaused` is deliberately
  // absent: Task 4 moves that control into DHListingsPauseControl, and because
  // useSaveDHPushConfig merges patches server-side-first, the card must never
  // transmit a key it does not render. Sending it back would be precisely the
  // clobber the merging mutation exists to prevent.
  const thresholdPatch = (cfg: DHPushConfig): Partial<DHPushConfig> => ({
    swingPctThreshold: cfg.swingPctThreshold,
    swingMinCents: cfg.swingMinCents,
    disagreementPctThreshold: cfg.disagreementPctThreshold,
    unreviewedChangePctThreshold: cfg.unreviewedChangePctThreshold,
    unreviewedChangeMinCents: cfg.unreviewedChangeMinCents,
  });

  // Every field edit routes through here so a single flag tracks "has unsaved
  // local state" without repeating setDirty(true) at each ConfigField callsite.
  const updateField = (patch: Partial<DHPushConfig>) => {
    setForm((prev) => (prev ? { ...prev, ...patch } : prev));
    setDirty(true);
  };

  const save = (patch: Partial<DHPushConfig>) => {
    saveMutation.mutate(patch, {
      onSuccess: () => {
        setDirty(false);
        toast.success('DH push config saved');
      },
      onError: () => toast.error('Failed to save config'),
    });
  };

  if (isError) {
    return (
      <CardShell padding="lg">
        <div className="text-center">
          <p className="text-[var(--danger)] mb-2">
            Failed to load config: {getErrorMessage(error, 'Unknown error')}
          </p>
          <Button onClick={() => refetch()}>Retry</Button>
        </div>
      </CardShell>
    );
  }

  if (isLoading || !form) {
    return <CardShell padding="lg"><PokeballLoader size="sm" /></CardShell>;
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
          onChange={(v) => updateField({ swingPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          id="swing-min"
          label="Price Swing Min"
          value={form.swingMinCents}
          onChange={(v) => updateField({ swingMinCents: v })}
          suffix={`(${formatCents(form.swingMinCents)})`}
        />
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wide col-span-full">Source &amp; CL Rules</p>
        <ConfigField
          id="disagreement-pct"
          label="Source Disagreement %"
          value={form.disagreementPctThreshold}
          onChange={(v) => updateField({ disagreementPctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          id="unreviewed-pct"
          label="Unreviewed CL Change %"
          value={form.unreviewedChangePctThreshold}
          onChange={(v) => updateField({ unreviewedChangePctThreshold: v })}
          suffix="%"
        />
        <ConfigField
          id="unreviewed-min"
          label="Unreviewed CL Change Min"
          value={form.unreviewedChangeMinCents}
          onChange={(v) => updateField({ unreviewedChangeMinCents: v })}
          suffix={`(${formatCents(form.unreviewedChangeMinCents)})`}
        />
      </div>
      <div className="mt-4">
        <Button
          size="sm"
          onClick={() => save(thresholdPatch(form))}
          loading={saveMutation.isPending}
        >
          Save
        </Button>
      </div>
    </CardShell>
  );
}
