import { describe, it, expect } from 'vitest';
import { jwtExpiry, reusableToken, selectAccessToken, REUSE_MARGIN_MS } from './psa-token-expiry.mjs';

// Builds an unsigned JWT whose `exp` claim is `epochSeconds`. Only the payload
// segment is read, so a placeholder header/signature is enough.
function tokenExpiring(epochSeconds) {
  const payload = Buffer.from(JSON.stringify({ exp: epochSeconds })).toString('base64url');
  return `hhh.${payload}.sss`;
}

describe('jwtExpiry', () => {
  it('reads the exp claim as RFC3339', () => {
    expect(jwtExpiry(tokenExpiring(1786500000))).toBe(new Date(1786500000 * 1000).toISOString());
  });

  const bad = [
    { name: 'not a JWT', input: 'nope' },
    { name: 'undecodable payload', input: 'hhh.!!!.sss' },
    { name: 'no exp claim', input: `hhh.${Buffer.from('{"sub":"x"}').toString('base64url')}.sss` },
    { name: 'empty', input: '' },
  ];
  for (const c of bad) {
    it(`returns null for ${c.name}`, () => {
      expect(jwtExpiry(c.input)).toBeNull();
    });
  }
});

describe('reusableToken', () => {
  const now = Date.parse('2026-08-17T22:00:00Z');
  const hours = (n) => now + n * 3600_000;

  const cases = [
    { name: 'valid with room to spare', token: tokenExpiring(hours(4) / 1000), want: true },
    { name: 'already expired', token: tokenExpiring(hours(-1) / 1000), want: false },
    { name: 'inside the safety margin', token: tokenExpiring((now + 60_000) / 1000), want: false },
    // Boundary: the reuse check is strictly greater-than, so a token expiring
    // exactly REUSE_MARGIN_MS from now is not reusable. Mirrors the Go side's
    // "exactly at the margin is not reusable" case (session_test.go).
    { name: 'exactly at the safety margin', token: tokenExpiring((now + REUSE_MARGIN_MS) / 1000), want: false },
    { name: 'undecodable token', token: 'nope', want: false },
    { name: 'empty token', token: '', want: false },
  ];
  for (const c of cases) {
    it(c.name, () => {
      expect(reusableToken(c.token, now)).toBe(c.want);
    });
  }
});

describe('selectAccessToken', () => {
  const sso = { name: 'accessToken', value: 'sso-tok', domain: 'app.collectors.com' };
  const ssoParent = { name: 'accessToken', value: 'sso-tok', domain: '.collectors.com' };
  const injected = { name: 'accessToken', value: 'injected-tok', domain: 'www.psacard.com' };
  const noise = { name: 'cf_clearance', value: 'x', domain: '.psacard.com' };
  const thirdParty = { name: 'accessToken', value: 'analytics', domain: '.capig.stape.us' };

  it('prefers the SSO cookie when both SSO and injected portal cookies are present', () => {
    // Injected cookie first in the jar — the ordering that made a name-only
    // .find() return the stale injection instead of the fresh SSO token.
    expect(selectAccessToken([injected, sso])?.value).toBe('sso-tok');
  });

  it('matches an SSO cookie scoped to the parent .collectors.com domain', () => {
    expect(selectAccessToken([injected, ssoParent])?.value).toBe('sso-tok');
  });

  it('falls back to the portal cookie when SSO was skipped (reuse fast-path)', () => {
    expect(selectAccessToken([injected])?.value).toBe('injected-tok');
  });

  it('does not treat a third-party analytics accessToken as the SSO cookie', () => {
    // .capig.stape.us must not match the collectors.com preference; falls back
    // to first-by-name (still the injected one here).
    expect(selectAccessToken([injected, thirdParty])?.value).toBe('injected-tok');
  });

  it('ignores non-accessToken and valueless cookies', () => {
    expect(selectAccessToken([noise, { name: 'accessToken', value: '', domain: 'app.collectors.com' }])).toBeNull();
  });

  it('returns null for an empty jar', () => {
    expect(selectAccessToken([])).toBeNull();
  });
});
