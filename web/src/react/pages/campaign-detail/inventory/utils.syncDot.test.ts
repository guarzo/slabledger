import { describe, it, expect, vi, afterEach } from 'vitest';
import { syncDotProps } from './syncDot';

const NOW = new Date('2026-04-11T12:00:00Z').getTime();

describe('syncDotProps', () => {
  afterEach(() => vi.useRealTimers());

  function iso(hoursAgo: number): string {
    return new Date(NOW - hoursAgo * 3600000).toISOString();
  }

  it('green when DH has price and synced within 24h', () => {
    vi.setSystemTime(NOW);
    const ts = iso(2);
    const { color } = syncDotProps({
      clSyncedAt: ts,
      mmValueUpdatedAt: ts,
      dhLastSyncedAt: ts,
      clHasValue: true,
      hasMMValue: true,
      hasDHPrice: true,
    });
    expect(color).toBe('#22c55e');
  });

  it('yellow when DH synced recently but has no price', () => {
    vi.setSystemTime(NOW);
    const { color } = syncDotProps({
      dhLastSyncedAt: iso(2),
      hasDHPrice: false,
      clSyncedAt: iso(2),
      clHasValue: true,
    });
    expect(color).toBe('#f59e0b');
  });

  it('yellow when DH stale but CL has a fresh value', () => {
    vi.setSystemTime(NOW);
    const { color } = syncDotProps({
      dhLastSyncedAt: iso(48),
      hasDHPrice: false,
      clSyncedAt: iso(2),
      clHasValue: true,
    });
    expect(color).toBe('#f59e0b');
  });

  it('red when everything is stale', () => {
    vi.setSystemTime(NOW);
    const { color } = syncDotProps({
      clSyncedAt: iso(48),
      mmValueUpdatedAt: iso(48),
      dhLastSyncedAt: iso(48),
      clHasValue: true,
      hasMMValue: true,
      hasDHPrice: true,
    });
    expect(color).toBe('#ef4444');
  });

  it('grey when nothing has ever synced', () => {
    vi.setSystemTime(NOW);
    const { color } = syncDotProps({});
    expect(color).toBe('#6b7280');
  });

  it('tooltip distinguishes CL matched-without-value from fresh match', () => {
    vi.setSystemTime(NOW);
    const { tooltip } = syncDotProps({
      clSyncedAt: iso(2),
      hasDHPrice: false,
      dhLastSyncedAt: iso(2),
      clReason: 'no_value',
    });
    expect(tooltip).toContain('CL · matched, no value');
  });

  it('tooltip labels catalog fallback distinctly', () => {
    vi.setSystemTime(NOW);
    const { tooltip } = syncDotProps({
      clSyncedAt: iso(2),
      clReason: 'catalog_fallback',
    });
    expect(tooltip).toContain('CL · catalog fallback');
  });
});
