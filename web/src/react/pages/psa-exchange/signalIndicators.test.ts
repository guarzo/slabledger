import { describe, expect, it } from 'vitest';
import { daysTier, velocityTier, confidenceTier } from './signalIndicators';

describe('daysTier', () => {
  it('returns "fast" for ≤6 days', () => {
    expect(daysTier(0)).toBe('fast');
    expect(daysTier(6)).toBe('fast');
  });

  it('returns "medium" for 7–15 days', () => {
    expect(daysTier(7)).toBe('medium');
    expect(daysTier(15)).toBe('medium');
  });

  it('returns "slow" for >15 days or non-finite', () => {
    expect(daysTier(16)).toBe('slow');
    expect(daysTier(Number.POSITIVE_INFINITY)).toBe('slow');
    expect(daysTier(NaN)).toBe('slow');
  });
});

describe('velocityTier', () => {
  it('returns 3 for ≥10 sales/mo', () => {
    expect(velocityTier(10)).toBe(3);
    expect(velocityTier(50)).toBe(3);
  });

  it('returns 2 for 3–9 sales/mo', () => {
    expect(velocityTier(3)).toBe(2);
    expect(velocityTier(9)).toBe(2);
  });

  it('returns 1 for <3 sales/mo', () => {
    expect(velocityTier(0)).toBe(1);
    expect(velocityTier(2.99)).toBe(1);
  });
});

describe('confidenceTier', () => {
  it('returns "high" for ≥7', () => {
    expect(confidenceTier(7)).toBe('high');
    expect(confidenceTier(10)).toBe('high');
  });

  it('returns "medium" for 5–6', () => {
    expect(confidenceTier(5)).toBe('medium');
    expect(confidenceTier(6)).toBe('medium');
  });

  it('returns "low" for <5', () => {
    expect(confidenceTier(0)).toBe('low');
    expect(confidenceTier(4.9)).toBe('low');
  });
});
