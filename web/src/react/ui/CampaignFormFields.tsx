import type { Phase, SubjectRef } from '../../types/campaigns';
import { useId, useEffect, useState, type ReactNode } from 'react';
import { phaseOptions, targetLanguageOptions, subjectFilterModeOptions, SUBJECT_FILTER_EXCLUDE } from '../utils/campaignConstants';
import { Input, Select } from '../ui';
import { Segmented } from './Segmented';
import ConfidenceRating from './ConfidenceRating';
import GradeRangeSlider from './GradeRangeSlider';
import SubjectListEditor from './SubjectListEditor';

export interface CampaignFormValues {
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  targetLanguages: string[];
  subjectFilterMode: string;
  subjects: SubjectRef[];
  deniedSpecs: SubjectRef[];
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  expectedFillRate?: number;
  phase?: Phase;
}

interface CampaignFormFieldsProps {
  values: CampaignFormValues;
  onChange: (field: string, value: string | number | string[] | SubjectRef[]) => void;
  inputSize?: 'sm';
  showPhase?: boolean;
  showFees?: boolean;
  nameError?: string;
  onNameBlur?: () => void;
}

function FormSection({
  icon,
  title,
  accent,
  children,
}: {
  icon: ReactNode;
  title: string;
  accent: string;
  children: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-[var(--surface-2)]/60 bg-[var(--surface-0)]/40 p-4 md:p-5 space-y-4">
      <div className="flex items-center gap-2.5 pb-3 border-b border-[var(--surface-2)]/40">
        <div className={`flex items-center justify-center w-7 h-7 rounded-lg ${accent}`}>
          {icon}
        </div>
        <h3 className="text-sm font-semibold text-[var(--text)] tracking-wide">
          {title}
        </h3>
      </div>
      {children}
    </div>
  );
}

function EconomicsSection({
  values, onChange, inputSize, showFees,
}: {
  values: CampaignFormValues;
  onChange: (field: string, value: string | number | string[] | SubjectRef[]) => void;
  inputSize?: 'sm';
  showFees?: boolean;
}) {
  const [buyTermsInput, setBuyTermsInput] = useState(() =>
    values.buyTermsCLPct == null ? '' : String(Math.round(values.buyTermsCLPct * 1000) / 10),
  );
  const [dailySpendCapInput, setDailySpendCapInput] = useState(() =>
    values.dailySpendCapCents == null ? '' : String(values.dailySpendCapCents / 100),
  );
  const [ebayFeeInput, setEbayFeeInput] = useState(() =>
    values.ebayFeePct == null ? '' : String(Math.round(values.ebayFeePct * 10000) / 100),
  );
  const [psaSourcingFeeInput, setPsaSourcingFeeInput] = useState(() =>
    values.psaSourcingFeeCents == null ? '' : String(values.psaSourcingFeeCents / 100),
  );
  const [expectedFillRateInput, setExpectedFillRateInput] = useState(() =>
    values.expectedFillRate == null ? '' : String(values.expectedFillRate),
  );

  // Sync local inputs when parent form values change (e.g. after setForm(campaign))
  useEffect(() => {
    setBuyTermsInput(values.buyTermsCLPct == null ? '' : String(Math.round(values.buyTermsCLPct * 1000) / 10));
    setDailySpendCapInput(values.dailySpendCapCents == null ? '' : String(values.dailySpendCapCents / 100));
    setEbayFeeInput(values.ebayFeePct == null ? '' : String(Math.round(values.ebayFeePct * 10000) / 100));
    setPsaSourcingFeeInput(values.psaSourcingFeeCents == null ? '' : String(values.psaSourcingFeeCents / 100));
    setExpectedFillRateInput(values.expectedFillRate == null ? '' : String(values.expectedFillRate));
  }, [values.buyTermsCLPct, values.dailySpendCapCents, values.ebayFeePct, values.psaSourcingFeeCents, values.expectedFillRate]);

  return (
    <FormSection
      title="Economics"
      accent="bg-[var(--warning)]/15 text-[var(--warning)]"
      icon={
        <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
          <line x1="12" y1="1" x2="12" y2="23" />
          <path d="M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6" />
        </svg>
      }
    >
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Input label="Buy Terms (%)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 78"
          value={buyTermsInput}
          onChange={e => setBuyTermsInput(e.target.value)}
          onBlur={() => { const v = parseFloat(buyTermsInput); onChange('buyTermsCLPct', Number.isNaN(v) ? 0 : v / 100); }} />
        <Input label="Daily Spend Cap ($)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 500"
          value={dailySpendCapInput}
          onChange={e => setDailySpendCapInput(e.target.value)}
          onBlur={() => { const v = parseFloat(dailySpendCapInput); onChange('dailySpendCapCents', Number.isNaN(v) ? 0 : Math.round(v * 100)); }} />
        {showFees && (
          <>
            <Input label="Expected Fill Rate (%)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 80"
              value={expectedFillRateInput}
              onChange={e => setExpectedFillRateInput(e.target.value)}
              onBlur={() => { const v = parseFloat(expectedFillRateInput); onChange('expectedFillRate', Number.isNaN(v) ? 0 : v); }} />
            <Input label="eBay Fee %" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 12.35"
              value={ebayFeeInput}
              onChange={e => setEbayFeeInput(e.target.value)}
              onBlur={() => { const v = parseFloat(ebayFeeInput); onChange('ebayFeePct', Number.isNaN(v) ? 0 : v / 100); }} />
            <Input label="PSA Sourcing Fee ($)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 3.00"
              value={psaSourcingFeeInput}
              onChange={e => setPsaSourcingFeeInput(e.target.value)}
              onBlur={() => { const v = parseFloat(psaSourcingFeeInput); onChange('psaSourcingFeeCents', Number.isNaN(v) ? 0 : Math.round(v * 100)); }} />
          </>
        )}
        <div className="md:col-span-2">
          <ConfidenceRating label="CL Confidence" value={values.clConfidence ? parseFloat(values.clConfidence) : 1}
            onChange={(val) => onChange('clConfidence', String(val))} />
        </div>
      </div>
    </FormSection>
  );
}

function LanguageMultiSelect({
  value, onChange,
}: {
  value: string[];
  onChange: (next: string[]) => void;
}) {
  const groupId = useId();
  const selected = new Set(value);
  const known = new Set<string>(targetLanguageOptions.map(o => o.value));
  // Tokens the backend sent that this copy of the closed set doesn't know.
  // They are preserved through every toggle rather than silently dropped —
  // dropping one would narrow what a live, money-spending campaign buys.
  const unknown = value.filter(t => !known.has(t));

  function toggleLanguage(token: string) {
    onChange(selected.has(token) ? value.filter(t => t !== token) : [...value, token]);
  }

  const labels = value
    .filter(t => known.has(t))
    .map(t => targetLanguageOptions.find(o => o.value === t)?.label ?? t);
  // Derived from value.length, not labels.length: an unrecognized token means
  // the set is non-empty, so it is NOT an open net even though no known
  // checkbox is checked. Conflating the two would contradict the warning
  // below for the same state.
  let summary: string;
  if (value.length === 0) {
    summary = 'None selected — open net: this campaign buys any language.';
  } else if (labels.length === 0) {
    // Every token in the set is unrecognized — there's nothing to enumerate,
    // and the set is not open (see above). Point at the warning below rather
    // than assert an empty buy scope.
    summary = 'No recognized language selected — see warning below.';
  } else {
    summary = `Buys ${labels.join(' and ')} cards only.`;
  }

  return (
    <fieldset className="space-y-1.5">
      <legend className="block text-xs text-[var(--text-muted)] mb-1">Languages</legend>
      <div className="flex flex-wrap items-center gap-4">
        {targetLanguageOptions.map(o => (
          <label
            key={o.value}
            htmlFor={`${groupId}-${o.value}`}
            className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer"
          >
            <input
              id={`${groupId}-${o.value}`}
              type="checkbox"
              checked={selected.has(o.value)}
              onChange={() => toggleLanguage(o.value)}
              className="rounded border-[var(--surface-2)]"
            />
            {o.label}
          </label>
        ))}
      </div>
      <p className="text-xs text-[var(--text-subtle)]">{summary}</p>
      {unknown.map(t => (
        <p key={t} className="text-xs text-[var(--warning)] flex flex-wrap items-center gap-1.5">
          {/* normalizeTargetLanguages (validation.go) rejects the whole save on
              the first unknown token — nothing with this token is pushed. */}
          <span>
            Unrecognized language token: {t} — not recognized by this UI; the
            campaign will be rejected on save until it is reconciled.
          </span>
          {/* Toggling a known checkbox preserves unknown tokens, so without an
              explicit action here the campaign cannot be saved at all and the
              operator has no way out of the form. Removing is deliberate and
              operator-initiated, which is the distinction that matters: the
              silent drop this component avoids is the one that happens as a
              side effect of an unrelated toggle. */}
          <button
            type="button"
            onClick={() => onChange(value.filter(v => v !== t))}
            className="underline underline-offset-2 hover:text-[var(--danger)]"
          >
            Remove {t}
          </button>
        </p>
      ))}
    </fieldset>
  );
}

export default function CampaignFormFields({
  values, onChange, inputSize, showPhase, showFees, nameError, onNameBlur,
}: CampaignFormFieldsProps) {
  return (
    <div className="space-y-4">
      {/* Identity */}
      <FormSection
        title="Identity"
        accent="bg-[var(--brand-500)]/15 text-[var(--brand-400)]"
        icon={
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
            <path d="M20.59 13.41l-7.17 7.17a2 2 0 01-2.83 0L2 12V2h10l8.59 8.59a2 2 0 010 2.82z" />
            <line x1="7" y1="7" x2="7.01" y2="7" />
          </svg>
        }
      >
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Input label="Name" required type="text" inputSize={inputSize} value={values.name}
            onChange={e => onChange('name', e.target.value)}
            onBlur={onNameBlur}
            error={nameError} />
          {showPhase && values.phase !== undefined && (
            <Select label="Phase" selectSize={inputSize} value={values.phase}
              onChange={e => onChange('phase', e.target.value)}
              options={[...phaseOptions]} />
          )}
          <Input label="Year Range" type="text" inputSize={inputSize} placeholder="e.g. 1999-2003" value={values.yearRange}
            onChange={e => onChange('yearRange', e.target.value)} />
        </div>
      </FormSection>

      {/* Targeting */}
      <FormSection
        title="Targeting"
        accent="bg-[var(--success)]/15 text-[var(--success)]"
        icon={
          <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
            <circle cx="12" cy="12" r="10" />
            <circle cx="12" cy="12" r="6" />
            <circle cx="12" cy="12" r="2" />
          </svg>
        }
      >
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="md:col-span-2">
            <GradeRangeSlider label="Grade Range" value={values.gradeRange}
              onChange={(val) => onChange('gradeRange', val)} />
          </div>
          <Input label="Price Range" type="text" inputSize={inputSize} placeholder="e.g. 250-1500" value={values.priceRange}
            onChange={e => onChange('priceRange', e.target.value)} />
          <LanguageMultiSelect
            value={values.targetLanguages}
            onChange={(next) => onChange('targetLanguages', next)}
          />
          <div className="space-y-1.5">
            <span className="block text-xs text-[var(--text-muted)] mb-1">Subject Mode</span>
            <Segmented
              ariaLabel="Subject filter mode"
              options={subjectFilterModeOptions}
              value={(values.subjectFilterMode || 'Target') as 'Target' | 'Exclude'}
              onChange={(v) => onChange('subjectFilterMode', v)}
            />
          </div>
          <div className="md:col-span-2">
            <SubjectListEditor
              label={values.subjectFilterMode === SUBJECT_FILTER_EXCLUDE ? 'Excluded Subjects' : 'Targeted Subjects'}
              value={values.subjects}
              onChange={(next) => onChange('subjects', next)}
              inputSize={inputSize}
            />
          </div>
          {values.deniedSpecs.length > 0 && (
            <div className="md:col-span-2 space-y-1.5">
              <span className="block text-xs text-[var(--text-muted)] mb-1">
                Denied Specs (portal-managed — add or remove in the PSA portal)
              </span>
              <div className="flex flex-wrap gap-1.5">
                {values.deniedSpecs.map((spec, i) => (
                  <span key={`${spec.id}-${spec.name}-${i}`} title={`id: ${spec.id}`}
                    className="inline-flex items-center rounded-full bg-[var(--surface-2)] text-[var(--text-muted)] text-xs px-2.5 py-1">
                    {spec.name}
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      </FormSection>

      {/* Economics */}
      <EconomicsSection values={values} onChange={onChange} inputSize={inputSize} showFees={showFees} />
    </div>
  );
}
