import { describe, it, expect } from 'vitest';
import { formatPL } from './utils';

describe('formatPL', () => {
  it('adds exactly one + sign for positive values', () => {
    const out = formatPL(917751);
    expect(out).toBe('+$9,177.51');
  });

  it('adds exactly one - sign for negative values via currency formatter', () => {
    const out = formatPL(-12500);
    expect(out).toBe('-$125.00');
  });

  it('renders zero with a + prefix (the contract)', () => {
    const out = formatPL(0);
    expect(out).toBe('+$0.00');
  });
});
