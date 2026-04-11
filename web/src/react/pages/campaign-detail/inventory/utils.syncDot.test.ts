import { describe, it, expect, vi, afterEach } from 'vitest';
import { syncDotProps } from './utils';

const NOW = new Date('2026-04-11T12:00:00Z').getTime();

describe('syncDotProps', () => {
  afterEach(() => vi.useRealTimers());

  function run(cl?: string, mm?: string, dh?: string) {
    vi.setSystemTime(NOW);
    return syncDotProps(cl, mm, dh);
  }

  it('green when all 3 synced within 24h', () => {
    const ts = new Date(NOW - 2 * 3600000).toISOString(); // 2h ago
    const { color } = run(ts, ts, ts);
    expect(color).toBe('#22c55e');
  });

  it('yellow when 1 synced within 24h', () => {
    const fresh = new Date(NOW - 3600000).toISOString();   // 1h ago
    const stale = new Date(NOW - 48 * 3600000).toISOString(); // 2d ago
    const { color } = run(fresh, stale, stale);
    expect(color).toBe('#f59e0b');
  });

  it('red when none synced within 24h', () => {
    const stale = new Date(NOW - 48 * 3600000).toISOString();
    const { color } = run(stale, stale, stale);
    expect(color).toBe('#ef4444');
  });

  it('red when all timestamps undefined', () => {
    const { color } = run(undefined, undefined, undefined);
    expect(color).toBe('#ef4444');
  });

  it('tooltip includes CL, MM, DH labels', () => {
    const ts = new Date(NOW - 2 * 3600000).toISOString();
    const { tooltip } = run(ts, ts, ts);
    expect(tooltip).toContain('CL ·');
    expect(tooltip).toContain('MM ·');
    expect(tooltip).toContain('DH ·');
  });
});
