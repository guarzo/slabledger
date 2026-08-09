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

  useEffect(() => {
    if (config) setForm(config);
  }, [config]);

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

  const save = (patch: Partial<DHPushConfig>, onFailure?: () => void) => {
    saveMutation.mutate(patch, {
      onSuccess: () => toast.success('DH push config saved'),
      onError: () => {
        toast.error('Failed to save config');
        onFailure?.();
      },
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
      <div className="mb-6 p-4 rounded-lg border border-[var(--surface-2)] bg-[var(--surface-1)]">
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1">
            <h4 className="text-sm font-semibold text-[var(--text)] mb-1">Pause DH Listings</h4>
            <p className="text-xs text-[var(--text-muted)]">
              When enabled, new and pending purchases are kept in inventory but are not listed on DoubleHolo.
              Use this before a card show so on-hand stock won&apos;t also be live on DH.
            </p>
            {form.listingsPaused && (
              <p className="mt-2 text-xs font-medium text-amber-500">
                Listings are currently paused. Items will accumulate as pending until this is turned off.
              </p>
            )}
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={form.listingsPaused}
            aria-label="Pause DH listings"
            aria-busy={saveMutation.isPending}
            disabled={saveMutation.isPending}
            onClick={() => {
              if (saveMutation.isPending) return;
              const prev = form;
              const next = { ...form, listingsPaused: !form.listingsPaused };
              setForm(next);
              save({ listingsPaused: next.listingsPaused }, () => setForm(prev));
            }}
            className={`relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors disabled:opacity-60 ${
              form.listingsPaused ? 'bg-amber-500' : 'bg-[var(--surface-3,#3a3a3a)]'
            }`}
          >
            <span
              className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                form.listingsPaused ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
        </div>
      </div>
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
          onClick={() => save(thresholdPatch(form))}
          loading={saveMutation.isPending}
        >
          Save
        </Button>
      </div>
    </CardShell>
  );
}
