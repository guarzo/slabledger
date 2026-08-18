import { describe, it, expect } from 'vitest';
import { jwtExpiry, reusableToken } from './psa-token-expiry.mjs';

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
    { name: 'undecodable token', token: 'nope', want: false },
    { name: 'empty token', token: '', want: false },
  ];
  for (const c of cases) {
    it(c.name, () => {
      expect(reusableToken(c.token, now)).toBe(c.want);
    });
  }
});
