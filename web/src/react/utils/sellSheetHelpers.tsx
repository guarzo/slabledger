import type { SellSheetItem } from '../../types/campaigns';

/** Upper-case suffixes that should stay uppercase in Pokemon card names. */
const UPPER_SUFFIXES = new Set(['EX', 'GX', 'VMAX', 'V', 'VSTAR']);

/** Title-case a single word, preserving Pokemon suffixes and expanding abbreviations. */
function titleWord(word: string): string {
  const upper = word.toUpperCase();
  if (UPPER_SUFFIXES.has(upper)) return upper;
  if (upper === 'REV.FOIL' || upper === 'REV.' || upper === 'REVFOIL') return 'Reverse Foil';
  if (upper === '1ST' && word.length <= 4) return '1st';
  if (upper === 'ED.' || upper === 'EDITION') return 'Edition';
  if (upper === '1STED.' || upper === '1STEDITION') return '1st Edition';
  return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
}

/** Format a card name for customer-facing display: title-case with Pokemon suffix handling. */
export function formatCardName(name: string): string {
  if (!name) return '';
  // Handle "1ST ED." as a combined token
  const normalized = name.replace(/1ST\s+ED\.?/gi, '1st Edition');
  return normalized.split(/\s+/).map(titleWord).join(' ');
}

/** Format grader prefix and grade for display. */
export function gradeDisplay(item: SellSheetItem): string {
  const prefix = item.grader && item.grader !== 'PSA' ? item.grader : 'PSA';
  return `${prefix} ${item.grade}`;
}

/** Build a subtitle from set name and card number. */
export function cardSubtitle(item: SellSheetItem): string | null {
  const parts: string[] = [];
  if (item.setName) parts.push(item.setName);
  if (item.cardNumber) parts.push(`#${item.cardNumber}`);
  return parts.length > 0 ? parts.join(' · ') : null;
}
