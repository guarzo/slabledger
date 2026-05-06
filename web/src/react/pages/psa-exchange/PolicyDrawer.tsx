import { useEffect, useState } from 'react';
import { Dialog } from 'radix-ui';
import { clsx } from 'clsx';
import Button from '../../ui/Button';
import { useUpdatePsaExchangePolicy } from '../../queries/usePsaExchangeQueries';
import type { PsaExchangePolicy, PsaExchangePolicySettings } from '../../../types/psaExchange';

interface PolicyDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  settings: PsaExchangePolicySettings;
}

interface FormState {
  highLiquidityVelocity: string;
  highLiquidityConfidence: string;
  highLiquidityOfferPct: string; // expressed as % in the form (e.g. "75")
  defaultOfferPct: string;
  minConfidence: string;
  minQuarterVelocity: string;
}

function policyToForm(p: PsaExchangePolicy): FormState {
  return {
    highLiquidityVelocity: String(p.highLiquidityVelocity),
    highLiquidityConfidence: String(p.highLiquidityConfidence),
    highLiquidityOfferPct: String(+(p.highLiquidityOfferPct * 100).toFixed(2)),
    defaultOfferPct: String(+(p.defaultOfferPct * 100).toFixed(2)),
    minConfidence: String(p.minConfidence),
    minQuarterVelocity: String(p.minQuarterVelocity),
  };
}

interface FormErrors {
  highLiquidityVelocity?: string;
  highLiquidityConfidence?: string;
  highLiquidityOfferPct?: string;
  defaultOfferPct?: string;
  minConfidence?: string;
  minQuarterVelocity?: string;
  cross?: string;
}

function validateForm(f: FormState): { policy?: PsaExchangePolicy; errors: FormErrors } {
  const errors: FormErrors = {};
  const intField = (raw: string, lo: number, hi: number, label: string): number | undefined => {
    if (raw === '' || Number.isNaN(Number(raw))) {
      return undefined;
    }
    const n = Number(raw);
    if (!Number.isInteger(n)) {
      return undefined;
    }
    if (n < lo || n > hi) {
      return undefined;
    }
    void label;
    return n;
  };
  const pctField = (raw: string): number | undefined => {
    if (raw === '' || Number.isNaN(Number(raw))) return undefined;
    const n = Number(raw);
    if (n <= 0 || n > 100) return undefined;
    return n / 100;
  };

  const hv = intField(f.highLiquidityVelocity, 0, 1000, 'highLiquidityVelocity');
  if (hv === undefined) errors.highLiquidityVelocity = 'integer ≥ 0';
  const hc = intField(f.highLiquidityConfidence, 0, 10, 'highLiquidityConfidence');
  if (hc === undefined) errors.highLiquidityConfidence = '0–10';
  const hp = pctField(f.highLiquidityOfferPct);
  if (hp === undefined) errors.highLiquidityOfferPct = '0 < pct ≤ 100';
  const dp = pctField(f.defaultOfferPct);
  if (dp === undefined) errors.defaultOfferPct = '0 < pct ≤ 100';
  const mc = intField(f.minConfidence, 0, 10, 'minConfidence');
  if (mc === undefined) errors.minConfidence = '0–10';
  const mq = intField(f.minQuarterVelocity, 0, 1000, 'minQuarterVelocity');
  if (mq === undefined) errors.minQuarterVelocity = 'integer ≥ 0';

  if (hp !== undefined && dp !== undefined && hp < dp) {
    errors.cross = 'High liquidity % must be ≥ default %';
  }

  if (Object.keys(errors).length > 0) {
    return { errors };
  }
  return {
    errors,
    policy: {
      highLiquidityVelocity: hv!,
      highLiquidityConfidence: hc!,
      highLiquidityOfferPct: hp!,
      defaultOfferPct: dp!,
      minConfidence: mc!,
      minQuarterVelocity: mq!,
    },
  };
}

export default function PolicyDrawer({ open, onOpenChange, settings }: PolicyDrawerProps) {
  const [form, setForm] = useState<FormState>(() => policyToForm(settings.active));
  const update = useUpdatePsaExchangePolicy();

  // Reset form whenever the drawer is reopened or upstream settings change.
  useEffect(() => {
    if (open) setForm(policyToForm(settings.active));
  }, [open, settings.active]);

  const { policy, errors } = validateForm(form);
  const canSave = !!policy && !update.isPending;

  const onChange = (key: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm((prev) => ({ ...prev, [key]: e.target.value }));
  };

  const onReset = () => setForm(policyToForm(settings.defaults));

  const onSave = () => {
    if (!policy) return;
    update.mutate(policy, {
      onSuccess: () => onOpenChange(false),
    });
  };

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in" />
        <Dialog.Content
          aria-describedby={undefined}
          className="fixed inset-y-0 right-0 z-50 w-full max-w-md overflow-y-auto bg-[var(--surface-1)] border-l border-[var(--surface-2)] shadow-2xl flex flex-col"
        >
          <div className="px-5 py-4 border-b border-[var(--surface-2)] flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-base font-semibold text-[var(--text)]">Tune PSA-Exchange policy</Dialog.Title>
              <p className="text-xs text-[var(--text-muted)] mt-1 leading-relaxed">
                Target offer = comp × tier %. Listings below the filter thresholds are dropped before scoring.
              </p>
            </div>
            <Dialog.Close asChild>
              <button
                type="button"
                aria-label="Close"
                className="text-[var(--text-muted)] hover:text-[var(--text)] text-xl leading-none -mt-1"
              >
                ×
              </button>
            </Dialog.Close>
          </div>

          <div className="px-5 py-4 space-y-5 flex-1">
            <Section title="Tier breakpoints" hint="A listing qualifies for the high-liquidity tier when both thresholds are met.">
              <Field label="Velocity ≥" suffix="/ month" value={form.highLiquidityVelocity} onChange={onChange('highLiquidityVelocity')} error={errors.highLiquidityVelocity} />
              <Field label="Confidence ≥" value={form.highLiquidityConfidence} onChange={onChange('highLiquidityConfidence')} error={errors.highLiquidityConfidence} />
            </Section>

            <Section title="Offer percentages" hint="Percent of comp used to compute the target offer for each tier.">
              <Field label="High liquidity %" suffix="%" value={form.highLiquidityOfferPct} onChange={onChange('highLiquidityOfferPct')} error={errors.highLiquidityOfferPct} />
              <Field label="Default %" suffix="%" value={form.defaultOfferPct} onChange={onChange('defaultOfferPct')} error={errors.defaultOfferPct} />
              {errors.cross && <p className="text-xs text-[var(--danger)] -mt-1">{errors.cross}</p>}
            </Section>

            <Section title="Pre-filter thresholds" hint="Listings below either are dropped from the table entirely.">
              <Field label="Min confidence" value={form.minConfidence} onChange={onChange('minConfidence')} error={errors.minConfidence} />
              <Field label="Min quarter velocity" value={form.minQuarterVelocity} onChange={onChange('minQuarterVelocity')} error={errors.minQuarterVelocity} />
            </Section>

            {update.isError && (
              <div className="text-xs text-[var(--danger)]">
                Save failed: {update.error?.message ?? 'unknown error'}
              </div>
            )}
          </div>

          <div className="px-5 py-4 border-t border-[var(--surface-2)] flex items-center justify-between gap-2">
            <Button variant="secondary" onClick={onReset} disabled={update.isPending}>
              Reset to defaults
            </Button>
            <div className="flex items-center gap-2">
              <Dialog.Close asChild>
                <Button variant="secondary" disabled={update.isPending}>Cancel</Button>
              </Dialog.Close>
              <Button onClick={onSave} disabled={!canSave}>
                {update.isPending ? 'Saving…' : 'Save'}
              </Button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <div>
        <div className="text-[11px] uppercase tracking-wide font-semibold text-[var(--text)]">{title}</div>
        {hint && <div className="text-[11px] text-[var(--text-muted)] leading-snug mt-0.5">{hint}</div>}
      </div>
      <div className="space-y-2">{children}</div>
    </div>
  );
}

function Field({
  label,
  suffix,
  value,
  onChange,
  error,
}: {
  label: string;
  suffix?: string;
  value: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  error?: string;
}) {
  return (
    <label className="flex items-center justify-between gap-3 text-xs">
      <span className="text-[var(--text)] flex-1">{label}</span>
      <span className="inline-flex items-center gap-1.5">
        <input
          type="number"
          step="any"
          value={value}
          onChange={onChange}
          aria-invalid={!!error}
          className={clsx(
            'w-24 px-2 py-1 rounded-md text-sm bg-[var(--surface-2)] border tabular-nums text-[var(--text)] focus:outline-none focus:ring-1',
            error
              ? 'border-[var(--danger)] focus:border-[var(--danger)] focus:ring-[var(--danger)]/30'
              : 'border-[var(--surface-2)] focus:border-[var(--brand-500)] focus:ring-[var(--brand-500)]/30',
          )}
        />
        {suffix && <span className="text-[var(--text-muted)] text-xs w-10">{suffix}</span>}
      </span>
      {error && <span className="text-[10px] text-[var(--danger)] w-24 -ml-3">{error}</span>}
    </label>
  );
}
