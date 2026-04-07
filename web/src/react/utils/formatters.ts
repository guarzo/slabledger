/**
 * Shared utility functions for formatting data
 * Eliminates duplication across components
 */

/**
 * Return today's date as an ISO date string (YYYY-MM-DD) in UTC.
 */
export function today(): string {
  return new Date().toISOString().split('T')[0];
}

/**
 * Return today's date as YYYY-MM-DD in the browser's local timezone.
 * Use this instead of today() when the date must match the user's calendar day
 * (e.g., sale dates, purchase dates).
 */
export function localToday(): string {
  const d = new Date();
  const yyyy = d.getFullYear();
  const mm = String(d.getMonth() + 1).padStart(2, '0');
  const dd = String(d.getDate()).padStart(2, '0');
  return `${yyyy}-${mm}-${dd}`;
}

/**
 * Format a number as USD currency
 * @param value - Number to format (in dollars - backend always sends dollars)
 * @returns Formatted currency string (e.g., "$123.45")
 *
 * IMPORTANT: Backend always sends prices in dollars, not cents.
 * No unit conversion is performed.
 */
export function currency(value: number | undefined | null): string {
  if (value === undefined || value === null) {
    return '$0.00';
  }

  try {
    // Backend always sends prices in dollars - use value as-is
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  } catch {
    return `$${value.toFixed(2)}`;
  }
}

/**
 * Format cents as USD currency string
 * @param cents - Amount in cents (integer)
 * @returns Formatted string (e.g., "$12.50")
 */
export function formatCents(cents: number | null | undefined): string {
  return ((cents ?? 0) / 100).toLocaleString('en-US', { style: 'currency', currency: 'USD' });
}

/**
 * Convert cents to a plain dollar string without currency symbol (e.g., "250.00").
 * Use for editable price fields where the "$" is rendered separately.
 */
export function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2);
}

/**
 * Parse a dollar string to cents (e.g., "250.00" → 25000).
 * Use for converting user-entered dollar values back to cents.
 */
export function dollarsToCents(dollars: string): number {
  const n = parseFloat(dollars);
  return isNaN(n) ? 0 : Math.round(n * 100);
}

/**
 * Format a decimal ratio as a percentage string
 * @param pct - Ratio value (e.g., 0.125 for 12.5%)
 * @returns Formatted percentage (e.g., "12.5%")
 */
export function formatPct(pct: number | null | undefined): string {
  return `${((pct ?? 0) * 100).toFixed(1)}%`;
}

/**
 * Extract a human-readable message from an unknown error value.
 */
export function getErrorMessage(err: unknown, fallback = 'Something went wrong'): string {
  if (err instanceof Error) return err.message;
  if (typeof err === 'string') return err;
  return fallback;
}

/**
 * Return a Tailwind color class based on how long a card has been held.
 */
export function daysHeldColor(days: number): string {
  if (days > 60) return 'text-red-400';
  if (days > 30) return 'text-yellow-400';
  return 'text-[var(--text)]';
}

/**
 * Format a 30-day trend value as text + color class.
 * Returns null if no meaningful trend.
 */
export function formatTrend(trend30d: number | null | undefined): { text: string; colorClass: string } | null {
  if (trend30d == null || trend30d === 0) return null;
  const sign = trend30d > 0 ? '+' : '';
  return {
    text: `${sign}${(trend30d * 100).toFixed(0)}%`,
    colorClass: trend30d > 0 ? 'text-green-400' : 'text-red-400',
  };
}

/**
 * Format a token count with K/M suffixes.
 */
export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

/**
 * Format a latency value in ms to a human-readable string.
 */
export function formatLatency(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms)}ms`;
}

/**
 * Convert ALL CAPS card names to Title Case for readability.
 * Preserves known card game acronyms (EX, GX, V, VMAX, etc.) in uppercase.
 */
const PRESERVE_UPPER = new Set(['EX', 'GX', 'V', 'VMAX', 'VSTAR', 'DX', 'PM', 'HP', 'TG', 'FA', 'AA', 'SIR', 'SR', 'AR', 'SAR', 'IR']);

export function toTitleCase(text: string): string {
  return text.replace(/\p{L}+/gu, (word) => {
    if (PRESERVE_UPPER.has(word.toUpperCase())) return word.toUpperCase();
    return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
  });
}

/**
 * Format a raw price range string (e.g. "10-50") into a display format ("$10 to $50").
 */
export function formatPriceRange(raw: string): string {
  if (!raw) return '';
  const parts = raw.split(/\s*[-\u2013\u2014]\s*/);
  return parts.map(p => {
    const n = p.replace(/[^0-9.]/g, '');
    return n ? `$${n}` : p;
  }).join(' to ');
}

export type SignalDirection = 'rising' | 'falling' | 'stable';

/** Display label for a market signal direction. */
export function signalLabel(direction: SignalDirection | string): string {
  switch (direction) {
    case 'rising': return 'Rising';
    case 'falling': return 'Falling';
    default: return 'Stable';
  }
}

/** Background color class for a signal badge. */
export function signalBgColor(direction: SignalDirection | string): string {
  switch (direction) {
    case 'rising': return 'bg-green-400/15 text-green-400';
    case 'falling': return 'bg-red-400/15 text-red-400';
    default: return 'bg-[var(--surface-2)] text-[var(--text-muted)]';
  }
}

/** Format weeks-to-cover for display. Returns '—' when no recovery data. */
export function formatWeeksToCover(weeks: number, hasRecoveryData: boolean): string {
  if (!hasRecoveryData) return '—';
  if (weeks > 20) return '20+';
  return `~${Math.round(weeks)}`;
}

