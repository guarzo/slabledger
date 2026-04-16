import { describe, it, expect } from 'vitest';
import { dhBadgeFor } from './dhBadge';

describe('dhBadgeFor', () => {
  it('prioritizes dh_status=sold over push status', () => {
    expect(dhBadgeFor('matched', 'sold')).toBe('sold');
    expect(dhBadgeFor('pending', 'sold')).toBe('sold');
  });

  it('maps dh_status=listed', () => {
    expect(dhBadgeFor('matched', 'listed')).toBe('listed');
  });

  it('maps dh_status=in_stock', () => {
    expect(dhBadgeFor('matched', 'in_stock')).toBe('in stock');
  });

  it('falls through to push status when dh_status is empty', () => {
    expect(dhBadgeFor('held', undefined)).toBe('held');
    expect(dhBadgeFor('unmatched', undefined)).toBe('unmatched');
    expect(dhBadgeFor('dismissed', undefined)).toBe('dismissed');
    expect(dhBadgeFor('pending', undefined)).toBe('pending');
    expect(dhBadgeFor('matched', undefined)).toBe('pushed');
  });

  it('returns unenrolled for empty or unknown push status', () => {
    expect(dhBadgeFor(undefined, undefined)).toBe('unenrolled');
    expect(dhBadgeFor('', '')).toBe('unenrolled');
    expect(dhBadgeFor('bogus', undefined)).toBe('unenrolled');
  });
});
