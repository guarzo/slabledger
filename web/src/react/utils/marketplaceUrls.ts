import type { GradeKey } from '../../types/pricing';

/** eBay category ID for Pokémon TCG Individual Cards */
const EBAY_POKEMON_CATEGORY = '183454';

export interface SearchableCard {
  name: string;
  setName: string;
  number: string;
  category?: string;
}

export function buildSearchTerms(card: SearchableCard, grade?: GradeKey): string {
  const prefix = card.category || 'pokemon';
  let terms = `${prefix} ${card.name} ${card.setName}`;
  if (card.number) terms += ` #${card.number}`;
  if (grade && grade !== 'raw') {
    const match = grade.match(/^psa(\d+)$/);
    if (match) terms += ` PSA ${match[1]}`;
  }
  return terms;
}

function ebayParams(card: SearchableCard, grade: GradeKey | undefined, extra: Record<string, string>): URLSearchParams {
  return new URLSearchParams({
    '_nkw': buildSearchTerms(card, grade),
    '_sacat': EBAY_POKEMON_CATEGORY,
    'LH_TitleDesc': '0',
    ...extra,
  });
}

export function defaultEbayUrl(card: SearchableCard, grade?: GradeKey): string {
  return `https://www.ebay.com/sch/i.html?${ebayParams(card, grade, { '_sop': '15' }).toString()}`;
}

export function defaultAltUrl(card: SearchableCard, grade?: GradeKey): string {
  const params = new URLSearchParams({ query: buildSearchTerms(card, grade), sortBy: 'newest_first' });
  return `https://alt.xyz/browse?${params.toString()}`;
}

export function defaultCardLadderUrl(card: SearchableCard, grade?: GradeKey): string {
  let q = `${card.name} ${card.setName}`;
  if (grade && grade !== 'raw') {
    const match = grade.match(/^psa(\d+)$/);
    if (match) q += ` psa ${match[1]}`;
  }
  const params = new URLSearchParams({ sort: 'date', direction: 'desc', q });
  return `https://app.cardladder.com/sales-history?${params.toString()}`;
}

export function ebayCompletedUrl(card: SearchableCard, grade?: GradeKey): string {
  return `https://www.ebay.com/sch/i.html?${ebayParams(card, grade, { 'LH_Complete': '1', 'LH_Sold': '1', '_sop': '13' }).toString()}`;
}

export function gradeToGradeKey(grade: number): GradeKey {
  if (!Number.isFinite(grade)) return 'raw';
  const floorGrade = Math.floor(grade);
  if (floorGrade >= 1 && floorGrade <= 10) return `psa${floorGrade}` as GradeKey;
  return 'raw';
}
