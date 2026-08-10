import { describe, it, expect } from 'vitest';
import {
  AUTO_RETRY_DELAYS_MS,
  MAX_AUTO_RETRIES,
  autoRetryDelayMs,
  retryPendingMessage,
  batchRetryPendingMessage,
  retryExhaustedMessage,
} from './cardIntakeRetry';

describe('autoRetryDelayMs', () => {
  const cases: { name: string; attempt: number; want: number | null }[] = [
    { name: 'first attempt', attempt: 1, want: AUTO_RETRY_DELAYS_MS[0] },
    { name: 'last funded attempt', attempt: MAX_AUTO_RETRIES, want: AUTO_RETRY_DELAYS_MS[MAX_AUTO_RETRIES - 1] },
    { name: 'one past the budget', attempt: MAX_AUTO_RETRIES + 1, want: null },
    { name: 'zero is not an attempt', attempt: 0, want: null },
  ];

  for (const c of cases) {
    it(c.name, () => {
      expect(autoRetryDelayMs(c.attempt)).toBe(c.want);
    });
  }

  it('backs off monotonically, ending past the 120s breaker timeout', () => {
    for (let i = 1; i < AUTO_RETRY_DELAYS_MS.length; i++) {
      expect(AUTO_RETRY_DELAYS_MS[i]).toBeGreaterThan(AUTO_RETRY_DELAYS_MS[i - 1]);
    }
    // The backend circuit breaker waits 120s before going half-open; a final
    // attempt inside that window could only collect ERR_PROV_CIRCUIT_OPEN.
    expect(AUTO_RETRY_DELAYS_MS[AUTO_RETRY_DELAYS_MS.length - 1]).toBeGreaterThan(120_000);
  });
});

describe('retry messages', () => {
  it('never asks the operator to act while a retry is pending', () => {
    const msg = retryPendingMessage(24, 1, 15_000);
    expect(msg).toMatch(/Retrying automatically in 15s/);
    expect(msg).toMatch(/attempt 1 of 3/);
    expect(msg).toMatch(/don't need to do anything/);
    expect(msg).not.toMatch(/click|press/i);
  });

  it('pluralises a single cert correctly', () => {
    expect(retryPendingMessage(1, 1, 15_000)).toMatch(/for 1 cert\./);
    expect(retryPendingMessage(2, 1, 15_000)).toMatch(/for 2 certs\./);
  });

  it('carries the thrown detail on a whole-batch failure', () => {
    const msg = batchRetryPendingMessage('Network request failed', 2, 60_000);
    expect(msg).toMatch(/^Network request failed\. Retrying automatically in 60s/);
    expect(msg).toMatch(/attempt 2 of 3/);
  });

  it('only asks for a human once automatic recovery is genuinely spent', () => {
    const msg = retryExhaustedMessage(24);
    expect(msg).toMatch(/after 3 automatic retries/);
    expect(msg).toMatch(/nothing was lost/i);
    expect(msg).toMatch(/press Import/);
  });
});
