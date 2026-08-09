import { describe, expect, it } from 'vitest';

/**
 * Guards the timer sweep in tests/setup.js.
 *
 * Without it, a pending window.setTimeout survives its test, then its own test
 * file, and finally fires against a torn-down jsdom — which vitest reports as an
 * unhandled error and fails the run with exit code 1 even though every test
 * passed. @radix-ui/react-toast leaks exactly this way: its auto-dismiss effect
 * arms a 5000ms timer and returns no cleanup, so unmounting a toast does not
 * disarm it.
 *
 * The two tests below must stay in this order and in this file: the first arms a
 * timer and finishes, the second asserts the sweep in between disarmed it.
 * Deleting the sweep makes the second one fail.
 */
describe('setup.js timer sweep', () => {
  let fired = false;

  it('arms a timer that outlives the test', () => {
    window.setTimeout(() => {
      fired = true;
    }, 20);
    expect(fired).toBe(false);
  });

  it('disarms timers left pending by the previous test', async () => {
    await new Promise((resolve) => window.setTimeout(resolve, 60));
    expect(fired).toBe(false);
  });
});
