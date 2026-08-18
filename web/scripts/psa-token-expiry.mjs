// Expiry helpers for the harvested PSA portal access token.
//
// Extracted so the "may I reuse this token?" decision is testable without a
// browser. It exists because the harvester used to inject a stored token
// unconditionally, then read that same cookie back and report it as freshly
// harvested — so an expired session looked like a successful login while every
// portal fetch 403'd (2026-08-17).

// Refuse to reuse a token this close to its expiry: a harvest run takes a few
// minutes, and a token that dies mid-run fails the same way an expired one does.
// Keep in sync with tokenReuseMargin in
// internal/adapters/clients/psaportal/session.go. The Go side gates before the
// subprocess spawns; this one re-checks after Chromium has launched, which is
// where startup drift would show up.
export const REUSE_MARGIN_MS = 5 * 60 * 1000;

// jwtExpiry returns the RFC3339 expiry from a JWT `exp` claim, or null when the
// token is not a decodable JWT or carries no `exp`.
export function jwtExpiry(token) {
  if (!token) return null;
  const parts = token.split('.');
  if (parts.length < 2) return null;
  try {
    const payload = JSON.parse(Buffer.from(parts[1], 'base64url').toString('utf8'));
    return payload.exp ? new Date(payload.exp * 1000).toISOString() : null;
  } catch {
    return null;
  }
}

// reusableToken reports whether a stored token still has enough life left to be
// worth injecting. An undecodable token is never reusable: falling through to a
// full SSO login is always the safe direction.
export function reusableToken(token, nowMs = Date.now()) {
  const expiry = jwtExpiry(token);
  if (!expiry) return false;
  return Date.parse(expiry) - nowMs > REUSE_MARGIN_MS;
}

// selectAccessToken picks which `accessToken` cookie to harvest from the browser
// context. Prefer the one Collectors SSO minted (its domain is under
// collectors.com) over a portal-host cookie: when a still-valid stored token was
// injected onto the portal host AND a fresh SSO login also ran, both are named
// `accessToken`, so a name-only match can return the injected copy instead of
// the freshly harvested one — re-persisting the old token instead of advancing
// (the laundering this guard exists to prevent, 2026-08-17). Fall back to the
// first accessToken so the reuse fast-path — SSO skipped, only the injected
// portal cookie present — still works. Returns null when none carries a value.
//
// `cookies` must already be scoped to the portal + SSO origins (third-party
// analytics hosts also drop an `accessToken`, and one of those would be worse
// than finding nothing).
export function selectAccessToken(cookies) {
  const named = (cookies || []).filter((c) => c.name === 'accessToken' && c.value);
  return named.find((c) => /(^|\.)collectors\.com$/.test(c.domain)) ?? named[0] ?? null;
}
