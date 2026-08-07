import type { Campaign, CreateCampaignInput } from '../../types/campaigns';

/**
 * CreateCampaignInput has no `expectedFillRate` key (it isn't set at creation
 * time), but the edit form renders that field via `showFees` and must seed
 * and round-trip it like every other server-owned value.
 */
export type EditCampaignFormValues = CreateCampaignInput & { expectedFillRate: number };

/**
 * Seeds the campaign edit form from an existing campaign.
 *
 * Every key of CreateCampaignInput is assigned explicitly, `phase` included.
 * The save path sends `{ ...freshCampaign, ...formValues }` to a full-row PUT,
 * so a key that is present-but-undefined here would override the campaign's
 * real value with undefined — which is why this does not spread the campaign.
 *
 * Subject ids ride through verbatim: portal ids span 4xxx/8xxx/22xxx
 * generations and are never re-derived from a name. -1
 * (LegacyUnreconciledSubjectID) and 0 (operator-typed, resolved at push time)
 * are preserved as-is too; SubjectListEditor is what treats them differently.
 *
 * Arrays are copied, not aliased, so editing the form cannot mutate the cached
 * campaign object React Query is holding.
 *
 * The `?? []` fallbacks are load-bearing: Go marshals a nil slice as JSON null,
 * so a campaign that never had targeting set arrives with null, not [].
 */
export function toFormValues(c: Campaign): EditCampaignFormValues {
  return {
    name: c.name,
    sport: c.sport,
    yearRange: c.yearRange,
    gradeRange: c.gradeRange,
    priceRange: c.priceRange,
    clConfidence: c.clConfidence,
    buyTermsCLPct: c.buyTermsCLPct,
    dailySpendCapCents: c.dailySpendCapCents,
    targetLanguages: [...(c.targetLanguages ?? [])],
    subjectFilterMode: c.subjectFilterMode,
    subjects: (c.subjects ?? []).map(s => ({ ...s })),
    deniedSpecs: (c.deniedSpecs ?? []).map(s => ({ ...s })),
    psaSourcingFeeCents: c.psaSourcingFeeCents,
    ebayFeePct: c.ebayFeePct,
    expectedFillRate: c.expectedFillRate,
    phase: c.phase,
  };
}
