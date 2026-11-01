import { afterEach, expect, it, vi } from 'vitest';

const originalExitCode = process.exitCode;
afterEach(() => {
  process.exitCode = originalExitCode;
  vi.restoreAllMocks();
  vi.unstubAllEnvs();
  vi.doUnmock('@playwright/test');
  vi.resetModules();
});

// Exercise the executable script; only browser I/O is replaced, never the login helpers.
async function runSignIn({ passwordAfter = Infinity, emailVisible = true, clickFails = false } = {}) {
  vi.resetModules();
  for (const [key, value] of Object.entries({
    PSA_PORTAL_EMAIL: 'offline-email', PSA_PORTAL_PASSWORD: 'offline-password',
    PSA_PORTAL_ACCESS_TOKEN: '', PSA_PORTAL_PROXY_URL: '', PSA_PORTAL_START_URL: '',
  })) vi.stubEnv(key, value);
  process.exitCode = undefined;
  let now = 0;
  let continuedAt;
  const events = [];
  const listeners = {};
  vi.spyOn(Date, 'now').mockImplementation(() => now);
  const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
  const stdout = vi.spyOn(process.stdout, 'write').mockImplementation(() => true);
  const field = (kind, visible) => ({
    first() { return this; }, isVisible: async () => visible(),
    fill: async (value) => { events.push([kind, 'fill', value]); },
    click: async () => {
      events.push([kind, 'click']);
      if (kind === 'continue') {
        if (clickFails) throw new Error('offline-click-stop');
        continuedAt = now;
      }
    },
  });
  const email = field('email', () => emailVisible);
  const password = field('password', () => continuedAt !== undefined && now - continuedAt >= passwordAfter);
  const cont = field('continue', () => true);
  const submit = field('submit', () => true);
  const invisible = field('missing', () => false);
  const page = {
    on: (event, callback) => { listeners[event] = callback; },
    goto: async () => {
      events.push(['goto']);
      for (const url of ['https://assets.example/app.js?token=hidden', 'data:text/plain,hidden']) {
        listeners.requestfailed?.({ url: () => url });
      }
    },
    waitForURL: async () => {}, url: () => 'https://app.collectors.com/signin?token=hidden',
    waitForTimeout: async (ms) => { now += ms; }, title: async () => 'Sign in',
    locator: (selector) => {
      if (selector === 'input, button, [role="button"]') return { all: async () => [] };
      if (selector === 'input[type="password"]') return password;
      return selector === '#email' ? email : invisible;
    },
    getByLabel: () => invisible,
    getByRole: (_role, { name }) => name.test('Next') ? cont : submit,
    // Stop before content()/fs import, and before the stdin loop/handshake respectively.
    screenshot: async () => { throw new Error('offline-dump-stop'); },
  };
  const context = { newPage: async () => page, cookies: async () => { events.push(['cookies']); throw new Error('offline-cookie-stop'); } };
  vi.doMock('@playwright/test', () => ({ chromium: { launch: async () => ({
    newContext: async () => context, close: async () => { events.push(['close']); },
  }) } }));
  await import('./harvest-psa-token.mjs');
  expect(stdout).not.toHaveBeenCalled();
  return { events, now, stderr: stderr.mock.calls.map(([line]) => line) };
}

it('waits for a password 45s after Continue without replaying credentials or clicks', async () => {
  const { events, now, stderr } = await runSignIn({ passwordAfter: 45000 });
  expect(events).toEqual([
    ['goto'], ['email', 'fill', 'offline-email'], ['continue', 'click'],
    ['password', 'fill', 'offline-password'], ['submit', 'click'], ['cookies'], ['close'],
  ]);
  expect(now).toBe(47500); // unchanged 2.5s probe plus the late password step
  expect(stderr).toContain('harvest-psa-token: offline-cookie-stop');
  expect(stderr.some((line) => line.includes('no-password-field'))).toBe(false);
});

it.each([false, true])('fails once after the 60s password boundary (Continue rejected: %s)', async (clickFails) => {
  const { events, now, stderr } = await runSignIn({ clickFails });
  expect(events).toEqual([['goto'], ['email', 'fill', 'offline-email'], ['continue', 'click'], ['close']]);
  expect(now).toBe(62500);
  expect(process.exitCode).toBe(1);
  expect(stderr).toContain('harvest-psa-token: could not find the password field on the sign-in page');
  const lines = stderr.filter((line) => line.includes('"phase":"no-password-field"'));
  expect(lines).toHaveLength(1);
  expect(JSON.parse(lines[0].slice(lines[0].indexOf('{')))).toMatchObject({
    controls: [], continueFound: true, continueClicked: !clickFails,
    failedRequestCount: 2, failedRequestOrigins: ['https://assets.example'],
  });
});

it('keeps the initial email deadline at 15s without clicking or filling anything', async () => {
  const { events, now, stderr } = await runSignIn({ emailVisible: false });
  expect(events).toEqual([['goto'], ['close']]);
  expect(now).toBe(15000);
  expect(process.exitCode).toBe(1);
  expect(stderr).toContain('harvest-psa-token: could not find the email field on the sign-in page');
  expect(stderr.some((line) => line.includes('no-password-field'))).toBe(false);
});
