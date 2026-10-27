import { afterEach, describe, expect, it, vi } from 'vitest';
import { gotoStart, logNoPasswordSummary, safePageURL } from './psa-harvest-start.mjs';

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

// Only navigation is fake: form actions after the helper are never retried.
describe('initial navigation', () => {
  const url = 'https://www.psacard.com/buyercampaignmanager/';
  const transient = [
    'ERR_TUNNEL_CONNECTION_FAILED', 'ERR_PROXY_CONNECTION_FAILED',
    'ERR_CONNECTION_RESET', 'ERR_CONNECTION_CLOSED', 'ERR_EMPTY_RESPONSE', 'ERR_NETWORK_CHANGED',
  ];
  it.each([null, ...transient])('continues once after %s', async (code) => {
    const response = { status: () => 200 };
    const events = [];
    const page = {
      goto: async (target, options) => {
        expect(target).toBe(url);
        expect(options).toEqual({ waitUntil: 'domcontentloaded', timeout: 60000 });
        events.push('goto');
        if (code && events.length === 1) throw new Error(`page.goto: net::${code} at ${url}?secret=private`);
        return response;
      },
      waitForTimeout: async (ms) => { expect(ms).toBe(5000); events.push('wait'); },
      locator: () => { throw new Error('navigation helper must not interact with forms'); },
    };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    expect(await gotoStart(page, url)).toBe(response);
    events.push('fill email', 'fill password', 'submit');
    expect(events).toEqual(code
      ? ['goto', 'wait', 'goto', 'fill email', 'fill password', 'submit']
      : ['goto', 'fill email', 'fill password', 'submit']);
    expect(stderr.mock.calls.flat().join(' ')).not.toContain('private');
  });
  it('propagates the second failure without a third navigation', async () => {
    let attempts = 0;
    const failure = new Error('net::ERR_CONNECTION_RESET');
    const page = { goto: async () => { attempts++; throw failure; }, waitForTimeout: async () => {} };
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(gotoStart(page, url)).rejects.toBe(failure);
    expect(attempts).toBe(2);
  });
  it.each([
    ['TimeoutError', 'net::ERR_CONNECTION_RESET'], ['TimeoutError', 'Timeout 60000ms exceeded'],
    ['Error', 'net::ERR_TIMED_OUT'], ['Error', 'net::ERR_CONNECTION_TIMED_OUT'],
    ['Error', 'net::ERR_FAILED'], ['Error', 'HTTP 403'], ['Error', 'unknown failure'],
  ])('does not retry %s: %s', async (name, message) => {
    let attempts = 0;
    const failure = Object.assign(new Error(message), { name });
    const page = { goto: async () => { attempts++; throw failure; }, waitForTimeout: () => { throw new Error('unexpected retry'); } };
    await expect(gotoStart(page, url)).rejects.toBe(failure);
    expect(attempts).toBe(1);
  });
});

describe('safe no-password summary', () => {
  it('reports visible control metadata without values, snapshot children, secrets or URL queries', async () => {
    document.body.innerHTML = '<input type="email" name="email" id="email" value="never-print-value">' +
      '<button id="next" type="button">Continue</button><input type="hidden" value="never-print-hidden">';
    const snapshots = [
      '- textbox "Email": never-print-value\n  - text: never-print-child',
      '- button "Continue \\"now\\" https://app.collectors.com/signin?code=never-print-query"',
      '- textbox "never-print-hidden"',
    ];
    const controls = [...document.querySelectorAll('input, button')].map((element, index) => ({
      isVisible: async () => element.type !== 'hidden',
      evaluate: async (fn) => fn(element),
      ariaSnapshot: async () => snapshots[index],
    }));
    const page = {
      url: () => 'https://user:never-print-userinfo@app.collectors.com/signin?token=never-print-url#never-print-fragment',
      title: async () => 'Sign in never-print-password never-print-email never-print-token',
      locator: () => ({ all: async () => controls }),
    };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    const stdout = vi.spyOn(process.stdout, 'write');
    await logNoPasswordSummary(page, ['never-print-password', 'never-print-email', 'never-print-token']);
    expect(stderr).toHaveBeenCalledTimes(1);
    const line = stderr.mock.calls[0][0];
    const summary = JSON.parse(line.slice(line.indexOf('{')));
    expect(summary.time).toMatch(/^\d{4}-\d\d-\d\dT.*Z$/);
    expect(summary.phase).toBe('no-password-field');
    expect(summary.url).toBe('https://app.collectors.com/signin');
    expect(summary.title).toBe('Sign in [REDACTED] [REDACTED] [REDACTED]');
    expect(summary.controls).toEqual([
      { type: 'email', name: 'email', id: 'email', role: 'textbox', accessibleName: 'Email' },
      { type: 'button', name: '', id: 'next', role: 'button', accessibleName: 'Continue "now" https://app.collectors.com/signin' },
    ]);
    expect(line).not.toContain('never-print');
    expect(line).not.toMatch(/[\r\n]/);
    expect(stdout).not.toHaveBeenCalled();
  });
  it('redacts known credentials in control identifiers and accessible names as well as the title', async () => {
    const page = {
      url: () => 'https://app.collectors.com/signin', title: async () => 'Sign in',
      locator: () => ({ all: async () => [{
        isVisible: async () => true,
        evaluate: async (fn) => { const el = document.createElement('button'); el.id = 'credential'; el.name = 'credential'; return fn(el); },
        ariaSnapshot: async () => '- button "credential"',
      }] }),
    };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    await logNoPasswordSummary(page, ['credential']);
    expect(stderr.mock.calls[0][0]).not.toContain('credential');
  });
  it('redacts case-varied and overlapping credentials without exposing their suffixes', async () => {
    const page = {
      url: () => 'https://app.collectors.com/signin',
      title: async () => 'user@example.com user%40example.com User@Example.com-password-tail',
      locator: () => ({ all: async () => [] }),
    };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    await logNoPasswordSummary(page, ['User@Example.com', 'User@Example.com-password-tail']);
    const line = stderr.mock.calls[0][0];
    expect(line).not.toMatch(/user@|user%40|password-tail/i);
  });
  it('does not let hidden controls exhaust the visible-control budget', async () => {
    const button = document.createElement('button'); button.id = 'next';
    const hidden = { isVisible: async () => false };
    const visible = { isVisible: async () => true, evaluate: async (fn) => fn(button), ariaSnapshot: async () => '- button "Continue"' };
    const page = {
      url: () => 'https://app.collectors.com/signin', title: async () => 'Sign in',
      locator: () => ({ all: async () => [...Array(21).fill(hidden), visible] }),
    };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    await logNoPasswordSummary(page, []);
    expect(stderr.mock.calls[0][0]).toContain('Continue');
  });
  it('does not propagate a diagnostic failure or log its sensitive error', async () => {
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(logNoPasswordSummary({ url: () => { throw new Error('secret'); } }, [])).resolves.toBeUndefined();
    expect(stderr).toHaveBeenCalledTimes(1);
    expect(stderr.mock.calls[0][0]).not.toContain('secret');
  });
  it('handles encoded credentials, empty secrets, YAML-quoted names and malformed URLs in page text', async () => {
    const el = document.createElement('button');
    const page = {
      url: () => 'https://app.collectors.com/signin?secret=hidden',
      title: async () => 'user%40example.com proxy-password https://% ?secret=hidden',
      locator: () => ({ all: async () => [{
        isVisible: async () => true, evaluate: async (fn) => fn(el),
        ariaSnapshot: async () => '- \'button "Continue: Tom\'\'s account"\'',
      }] }),
    };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(logNoPasswordSummary(page, ['', 'user@example.com', 'proxy-password'])).resolves.toBeUndefined();
    const line = stderr.mock.calls[0][0];
    expect(line).toContain('Continue: Tom');
    expect(line).not.toMatch(/user%40|proxy-password|secret=hidden/);
  });
  it('bounds a hanging diagnostic without suppressing subsequent failure handling', async () => {
    vi.useFakeTimers();
    const page = { locator: () => ({ all: () => new Promise(() => {}) }) };
    const stderr = vi.spyOn(console, 'error').mockImplementation(() => {});
    let dumpReached = false;
    const failedLogin = (async () => { await logNoPasswordSummary(page, []); dumpReached = true; throw new Error('no password'); })();
    const result = expect(failedLogin).rejects.toThrow('no password');
    await vi.advanceTimersByTimeAsync(5000);
    await result;
    expect(dumpReached).toBe(true);
    expect(stderr).toHaveBeenCalledTimes(1);
  });
  it.each([
    ['https://user:password@app.collectors.com/signin?code=secret#fragment', 'https://app.collectors.com/signin'],
    ['https://%', '[invalid-url]'],
  ])('sanitizes %s', (url, expected) => { expect(safePageURL(url)).toBe(expected); });
});
