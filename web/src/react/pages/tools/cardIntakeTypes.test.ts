import { describe, it, expect } from 'vitest';
import { importErrorStatus, rowAwaitingSync } from './cardIntakeTypes';
import type { CertRow } from './cardIntakeTypes';

describe('importErrorStatus', () => {
  it('stages transient (retryable) failures for retry', () => {
    expect(importErrorStatus({ retryable: true })).toBe('retry');
  });

  it('marks permanent failures as terminally failed', () => {
    expect(importErrorStatus({ retryable: false })).toBe('failed');
  });

  it('treats a missing retryable flag as permanent', () => {
    // Backend omits `retryable` (omitempty) for permanent errors like
    // "cert not found", so undefined must NOT be staged for retry.
    expect(importErrorStatus({})).toBe('failed');
  });
});

describe('rowAwaitingSync', () => {
  const base: CertRow = { certNumber: '12345678', status: 'resolved' };

  it('does not poll rows staged for retry', () => {
    // A retry row is staged for re-import, not awaiting a background sync —
    // polling it would be wasted work and could mask its retry state.
    expect(rowAwaitingSync({ ...base, status: 'retry' })).toBe(false);
  });

  it('does not poll terminally failed rows', () => {
    expect(rowAwaitingSync({ ...base, status: 'failed' })).toBe(false);
  });

  it('polls rows still resolving', () => {
    expect(rowAwaitingSync({ ...base, status: 'resolving' })).toBe(true);
  });
});
