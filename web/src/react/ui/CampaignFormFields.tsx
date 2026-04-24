import type { Phase } from '../../types/campaigns';
import { useEffect, useState, useId, type ReactNode } from 'react';
import { Checkbox } from 'radix-ui';
import { phaseOptions } from '../utils/campaignConstants';
import { Input, Select } from '../ui';
import ConfidenceRating from './ConfidenceRating';
import GradeRangeSlider from './GradeRangeSlider';

export interface CampaignFormValues {
  name: string;
  sport: string;
  yearRange: string;
  gradeRange: string;
  priceRange: string;
  clConfidence: string;
  buyTermsCLPct: number;
  dailySpendCapCents: number;
  inclusionList: string;
  exclusionMode: boolean;
  psaSourcingFeeCents: number;
  ebayFeePct: number;
  expectedFillRate?: number;
  phase?: Phase;
}

interface CampaignFormFieldsProps {
  values: CampaignFormValues;
  onChange: (field: string, value: string | number | boolean) => void;
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
  onChange: (field: string, value: string | number | boolean) => void;
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

  // Sync local inputs when parent form values change (e.g. after setForm(campaign))
  useEffect(() => {
    setBuyTermsInput(values.buyTermsCLPct == null ? '' : String(Math.round(values.buyTermsCLPct * 1000) / 10));
    setDailySpendCapInput(values.dailySpendCapCents == null ? '' : String(values.dailySpendCapCents / 100));
    setEbayFeeInput(values.ebayFeePct == null ? '' : String(Math.round(values.ebayFeePct * 10000) / 100));
    setPsaSourcingFeeInput(values.psaSourcingFeeCents == null ? '' : String(values.psaSourcingFeeCents / 100));
  }, [values.buyTermsCLPct, values.dailySpendCapCents, values.ebayFeePct, values.psaSourcingFeeCents]);

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
            <Input label="Expected Fill Rate (%)" type="text" inputMode="decimal" inputSize={inputSize} placeholder="e.g. 80" value={values.expectedFillRate != null ? String(values.expectedFillRate) : ''}
              onChange={e => { const v = parseFloat(e.target.value); onChange('expectedFillRate', Number.isNaN(v) ? 0 : v); }} />
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

export default function CampaignFormFields({
  values, onChange, inputSize, showPhase, showFees, nameError, onNameBlur,
}: CampaignFormFieldsProps) {
  const exclusionModeId = useId();
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
          <div className="md:col-span-2 space-y-2">
            <Input label={values.exclusionMode ? 'Exclusion List' : 'Inclusion List'} type="text" inputSize={inputSize} placeholder="e.g. charizard pikachu blastoise" value={values.inclusionList}
              onChange={e => onChange('inclusionList', e.target.value)} />
            <label htmlFor={exclusionModeId} className="inline-flex items-center gap-2.5 text-sm text-[var(--text-muted)] cursor-pointer group select-none">
              <Checkbox.Root id={exclusionModeId} checked={values.exclusionMode}
                onCheckedChange={(checked) => onChange('exclusionMode', checked === true)}
                className="flex items-center justify-center w-4 h-4 rounded
                           border border-[var(--surface-3)] bg-[var(--surface-2)] transition-colors
                           data-[state=checked]:bg-[var(--brand-500)] data-[state=checked]:border-[var(--brand-500)]
                           focus-visible:ring-2 focus-visible:ring-[var(--brand-500)]/40
                           focus-visible:ring-offset-1 focus-visible:ring-offset-[var(--surface-1)]
                           group-hover:border-[var(--brand-500)]/50">
                <Checkbox.Indicator className="text-white">
                  <svg className="w-2.5 h-2.5" fill="none" stroke="currentColor" strokeWidth="3"
                       viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round"
                       aria-hidden="true" focusable="false">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                </Checkbox.Indicator>
              </Checkbox.Root>
              Use as exclusion list
            </label>
          </div>
        </div>
      </FormSection>

      {/* Economics */}
      <EconomicsSection values={values} onChange={onChange} inputSize={inputSize} showFees={showFees} />
    </div>
  );
}
