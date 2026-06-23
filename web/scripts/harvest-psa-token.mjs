// Headless harvester for the PSA Buyer Campaign Manager access token.
//
// Logs into Collectors SSO (password-only) with stored credentials, lands back
// on psacard.com, reads the `accessToken` cookie, and prints JSON to stdout:
//   {"accessToken":"<jwt>","expiresAt":"<RFC3339>"}
//
// The Go harvest trigger execs this and persists the result via the encrypted
// token store. On any failure it writes a debug screenshot + HTML and exits 1.
//
// Env:
//   PSA_PORTAL_EMAIL     (required)
//   PSA_PORTAL_PASSWORD  (required)
//   PSA_PORTAL_START_URL (optional, default the buyer campaign manager home)
//   PSA_HARVEST_DEBUG_DIR(optional, default /tmp)
//
// Run:  node web/scripts/harvest-psa-token.mjs   (from repo root, after `npm --prefix web ci`)

import { chromium } from '@playwright/test';

const EMAIL = process.env.PSA_PORTAL_EMAIL;
const PASSWORD = process.env.PSA_PORTAL_PASSWORD;
const START_URL =
  process.env.PSA_PORTAL_START_URL || 'https://www.psacard.com/buyercampaignmanager/';
const DEBUG_DIR = process.env.PSA_HARVEST_DEBUG_DIR || '/tmp';
const UA =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36';

function fail(msg) {
  console.error(`harvest-psa-token: ${msg}`);
  process.exit(1);
}

if (!EMAIL || !PASSWORD) fail('PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required');

function jwtExpiry(token) {
  // Returns RFC3339 expiry from the JWT `exp` claim, or null.
  const parts = token.split('.');
  if (parts.length < 2) return null;
  try {
    const payload = JSON.parse(Buffer.from(parts[1], 'base64url').toString('utf8'));
    return payload.exp ? new Date(payload.exp * 1000).toISOString() : null;
  } catch {
    return null;
  }
}

// firstVisible returns the first locator (from candidates) that becomes visible
// within timeout, or null. Lets us tolerate small DOM variations in the SSO form.
async function firstVisible(scope, candidates, timeout = 15000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    for (const c of candidates) {
      const loc = typeof c === 'function' ? c(scope) : scope.locator(c);
      if (await loc.first().isVisible().catch(() => false)) return loc.first();
    }
    await scope.waitForTimeout(250);
  }
  return null;
}

async function dumpDebug(page, label) {
  try {
    await page.screenshot({ path: `${DEBUG_DIR}/psa-harvest-${label}.png`, fullPage: true });
    const html = await page.content();
    await import('node:fs').then((fs) =>
      fs.writeFileSync(`${DEBUG_DIR}/psa-harvest-${label}.html`, html),
    );
    console.error(`harvest-psa-token: wrote debug ${DEBUG_DIR}/psa-harvest-${label}.{png,html} (url=${page.url()})`);
  } catch (e) {
    console.error('harvest-psa-token: debug dump failed:', e.message);
  }
}

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ userAgent: UA });
const page = await context.newPage();

try {
  await page.goto(START_URL, { waitUntil: 'domcontentloaded', timeout: 60000 });
  // The portal bounces unauthenticated users to app.collectors.com/signin.
  await page.waitForURL(/collectors\.com\/signin/i, { timeout: 30000 }).catch(() => {});

  // --- Email step ---
  const emailField = await firstVisible(page, [
    (p) => p.getByLabel(/email/i),
    'input[type="email"]',
    'input[name*="email" i]',
    'input[autocomplete="username"]',
  ]);
  if (!emailField) {
    await dumpDebug(page, 'no-email-field');
    fail('could not find the email field on the sign-in page');
  }
  await emailField.fill(EMAIL);

  // Some flows reveal the password only after a "Continue"/"Next" click.
  let passwordField = await firstVisible(page, ['input[type="password"]'], 2500);
  if (!passwordField) {
    const cont = await firstVisible(page, [
      (p) => p.getByRole('button', { name: /continue|next|sign in|log ?in/i }),
    ], 5000);
    if (cont) await cont.click().catch(() => {});
    passwordField = await firstVisible(page, ['input[type="password"]'], 15000);
  }
  if (!passwordField) {
    await dumpDebug(page, 'no-password-field');
    fail('could not find the password field on the sign-in page');
  }
  await passwordField.fill(PASSWORD);

  // --- Submit ---
  const submit = await firstVisible(page, [
    (p) => p.getByRole('button', { name: /sign in|log ?in|continue|submit/i }),
    'button[type="submit"]',
  ]);
  if (!submit) {
    await dumpDebug(page, 'no-submit');
    fail('could not find the submit button on the sign-in page');
  }
  await submit.click();

  // --- Wait for return to the portal ---
  await page.waitForURL(/psacard\.com\/buyercampaignmanager/i, { timeout: 60000 }).catch(() => {});

  // Read the accessToken cookie (set on psacard.com after the SSO round-trip).
  const cookies = await context.cookies(['https://www.psacard.com']);
  const at = cookies.find((c) => c.name === 'accessToken');
  if (!at || !at.value) {
    await dumpDebug(page, 'no-access-cookie');
    fail('login completed but no accessToken cookie was set');
  }

  const expiresAt = jwtExpiry(at.value);
  if (!expiresAt) {
    await dumpDebug(page, 'bad-jwt');
    fail('accessToken cookie is not a decodable JWT');
  }

  process.stdout.write(JSON.stringify({ accessToken: at.value, expiresAt }) + '\n');
  await browser.close();
} catch (e) {
  await dumpDebug(page, 'exception');
  await browser.close();
  fail(`unexpected error: ${e.message}`);
}
