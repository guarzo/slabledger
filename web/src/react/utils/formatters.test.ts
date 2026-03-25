import { describe, it, expect } from 'vitest';
import {
  currency,
  formatCents, formatPct, getErrorMessage, daysHeldColor, formatTrend,
} from './formatters';

describe('currency', () => {
  it('formats a number as USD', () => {
    expect(currency(123.45)).toBe('$123.45');
    expect(currency(0)).toBe('$0.00');
    expect(currency(1000)).toBe('$1,000.00');
  });

  it('handles null/undefined', () => {
    expect(currency(null)).toBe('$0.00');
    expect(currency(undefined)).toBe('$0.00');
  });
});

describe('formatCents', () => {
  it('converts cents to dollar string', () => {
    expect(formatCents(1250)).toBe('$12.50');
    expect(formatCents(0)).toBe('$0.00');
    expect(formatCents(99)).toBe('$0.99');
  });
});

describe('formatPct', () => {
  it('formats ratio as percentage', () => {
    expect(formatPct(0.125)).toBe('12.5%');
    expect(formatPct(1)).toBe('100.0%');
  });
});

describe('getErrorMessage', () => {
  it('extracts error message', () => {
    expect(getErrorMessage(new Error('test'))).toBe('test');
    expect(getErrorMessage('string error')).toBe('string error');
    expect(getErrorMessage(42)).toBe('Something went wrong');
    expect(getErrorMessage(null, 'fallback')).toBe('fallback');
  });
});

describe('daysHeldColor', () => {
  it('returns correct color classes', () => {
    expect(daysHeldColor(10)).toBe('text-[var(--text)]');
    expect(daysHeldColor(45)).toBe('text-yellow-400');
    expect(daysHeldColor(90)).toBe('text-red-400');
  });
});

describe('formatTrend', () => {
  it('formats positive trends', () => {
    const result = formatTrend(0.15);
    expect(result).toEqual({ text: '+15%', colorClass: 'text-green-400' });
  });

  it('formats negative trends', () => {
    const result = formatTrend(-0.1);
    expect(result).toEqual({ text: '-10%', colorClass: 'text-red-400' });
  });

  it('returns null for zero/null', () => {
    expect(formatTrend(0)).toBeNull();
    expect(formatTrend(null)).toBeNull();
    expect(formatTrend(undefined)).toBeNull();
  });
});
