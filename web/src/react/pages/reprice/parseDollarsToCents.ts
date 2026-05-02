/**
 * Parse a "12.34" dollar string into integer cents. Strict: rejects
 * anything that isn't an integer, optional ".", and up to two fractional
 * digits — `parseInt` would silently coerce "12abc" → 12. Returns null
 * for empty, lone ".", or non-decimal input.
 */
export function parseDollarsToCents(val: string | undefined): number | null {
  if (!val || val === '.') return null;
  if (!/^\d*(\.\d{0,2})?$/.test(val)) return null;
  const parts = val.split('.');
  const d = Number(parts[0] || '0');
  const frac = (parts[1] || '0').slice(0, 2).padEnd(2, '0');
  const cents = d * 100 + Number(frac);
  return Number.isNaN(cents) || cents < 0 ? null : cents;
}
