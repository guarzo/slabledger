import { describe, it, expect } from 'vitest';
import { toFormValues } from './campaignFormValues';
import type { Campaign } from '../../types/campaigns';

function makeCampaign(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: 'c1',
    name: 'Vintage Core',
    sport: 'Pokemon',
    yearRange: '1999-2003',
    gradeRange: '8-10',
    priceRange: '50-500',
    clConfidence: 'high',
    buyTermsCLPct: 0.78,
    dailySpendCapCents: 50000,
    targetLanguages: ['english', 'japanese'],
    subjectFilterMode: 'Exclude',
    subjects: [{ id: 22210, name: 'Machamp' }, { id: 0, name: 'Mewtwo' }],
    deniedSpecs: [{ id: 4807, name: 'Charizard' }],
    phase: 'closed',
    psaSourcingFeeCents: 300,
    ebayFeePct: 0.1235,
    expectedFillRate: 42,
    psaCampaignRequestId: 'req-1',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-02-02T00:00:00Z',
    ...overrides,
  } as Campaign;
}

describe('toFormValues', () => {
  it('seeds all three targeting axes from the campaign', () => {
    const v = toFormValues(makeCampaign());
    expect(v.targetLanguages).toEqual(['english', 'japanese']);
    expect(v.subjectFilterMode).toBe('Exclude');
    expect(v.subjects).toEqual([{ id: 22210, name: 'Machamp' }, { id: 0, name: 'Mewtwo' }]);
    expect(v.deniedSpecs).toEqual([{ id: 4807, name: 'Charizard' }]);
  });

  it('seeds expectedFillRate from the campaign', () => {
    // The field renders on the edit form (showFees) but has no key on
    // CreateCampaignInput; a missing seed here means the form shows a blank
    // input for a campaign with a real stored value.
    const v = toFormValues(makeCampaign({ expectedFillRate: 42 }));
    expect(v.expectedFillRate).toBe(42);
  });

  it('preserves portal-issued subject ids exactly', () => {
    // Ids span 4xxx/8xxx/22xxx portal generations and are never re-derived from
    // a name. A seed that dropped or rewrote one would push wrong targeting.
    const v = toFormValues(makeCampaign({
      subjects: [{ id: 4807, name: 'Charizard' }, { id: 8123, name: 'Blastoise' }, { id: -1, name: 'Venusaur' }],
    }));
    expect(v.subjects).toEqual([
      { id: 4807, name: 'Charizard' },
      { id: 8123, name: 'Blastoise' },
      { id: -1, name: 'Venusaur' },
    ]);
  });

  it('always sets phase, so a later spread cannot blank it', () => {
    // CampaignsPage saves `{ ...freshCampaign, ...formValues }`. `phase` is
    // optional on CreateCampaignInput, and an explicit `phase: undefined` in
    // the spread would override the campaign's real phase with undefined.
    const v = toFormValues(makeCampaign({ phase: 'pending' }));
    expect(v.phase).toBe('pending');
  });

  it('copies arrays rather than aliasing the campaign', () => {
    const c = makeCampaign();
    const v = toFormValues(c);
    (v.subjects as { id: number; name: string }[]).push({ id: 1, name: 'X' });
    v.targetLanguages.push('klingon');
    expect(c.subjects).toHaveLength(2);
    expect(c.targetLanguages).toEqual(['english', 'japanese']);
  });

  it('tolerates null slices from the server', () => {
    // Go marshals a nil slice as JSON null, so a campaign that has never had
    // targeting set arrives with targetLanguages: null, not [].
    const c = makeCampaign({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- simulating the real null the server sends, which the non-nullable TS type disallows
      targetLanguages: null as any,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- ditto
      subjects: null as any,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- ditto
      deniedSpecs: null as any,
    });
    const v = toFormValues(c);
    expect(v.targetLanguages).toEqual([]);
    expect(v.subjects).toEqual([]);
    expect(v.deniedSpecs).toEqual([]);
  });
});
