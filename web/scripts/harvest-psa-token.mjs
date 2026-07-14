// Headless harvester for the PSA Buyer Campaign Manager access token.
//
// Logs into Collectors SSO (password-only) with stored credentials, lands back
// on psacard.com, reads the `accessToken` cookie, and prints JSON to stdout:
//   {"accessToken":"<jwt>","expiresAt":"<RFC3339>","analyticsData":"<raw __data.json body>"}
//
// The Go harvest trigger execs this and persists the result via the encrypted
// token store. On any failure it writes a debug screenshot + HTML and exits 1.
//
// Env:
//   PSA_PORTAL_EMAIL     (required)
//   PSA_PORTAL_PASSWORD  (required)
//   PSA_PORTAL_START_URL (optional, default the buyer campaign manager home)
//   PSA_PORTAL_ACCESS_TOKEN (optional) previously harvested token; injected as a
//                            cookie so a still-valid session skips the SSO login
//   PSA_HARVEST_DEBUG_DIR(optional, default /tmp)
//   PSA_PORTAL_CHROME_PATH    (optional) absolute path to an installed chrome/chromium binary
//   PSA_PORTAL_CHROME_CHANNEL (optional) branded channel, e.g. "chrome" or "msedge" (no download)
//
// Run:  node web/scripts/harvest-psa-token.mjs   (from repo root, after `npm --prefix web ci`)
// If Playwright's browser download fails, set PSA_PORTAL_CHROME_CHANNEL=chrome (uses installed
// Google Chrome) or PSA_PORTAL_CHROME_PATH=/path/to/chromium — no Playwright download needed.

import { chromium } from '@playwright/test';

const EMAIL = process.env.PSA_PORTAL_EMAIL;
const PASSWORD = process.env.PSA_PORTAL_PASSWORD;
const START_URL =
  process.env.PSA_PORTAL_START_URL || 'https://www.psacard.com/buyercampaignmanager/';
const ACCESS_TOKEN = process.env.PSA_PORTAL_ACCESS_TOKEN || '';
const DEBUG_DIR = process.env.PSA_HARVEST_DEBUG_DIR || '/tmp';
const UA =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36';

function fail(msg) {
  console.error(`harvest-psa-token: ${msg}`);
  process.exit(1);
}

if (!EMAIL || !PASSWORD) fail('PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required');

// The accessToken cookie must be scoped to whatever host START_URL actually
// points at (PSA_PORTAL_START_URL is operator-configurable), not a hardcoded
// default — otherwise an injected cookie silently never applies.
const COOKIE_DOMAIN = new URL(START_URL).hostname;
if (!COOKIE_DOMAIN.endsWith('psacard.com')) {
  fail(`PSA_PORTAL_START_URL host "${COOKIE_DOMAIN}" is not a psacard.com host`);
}

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
    const png = `${DEBUG_DIR}/psa-harvest-${label}.png`;
    await page.screenshot({ path: png, fullPage: true });
    const html = await page.content();
    await import('node:fs').then((fs) => {
      // Restrict to owner-only — dumps can contain login-page / session state.
      fs.chmodSync(png, 0o600);
      fs.writeFileSync(`${DEBUG_DIR}/psa-harvest-${label}.html`, html, { mode: 0o600 });
    });
    console.error(`harvest-psa-token: wrote debug ${DEBUG_DIR}/psa-harvest-${label}.{png,html} (url=${page.url()})`);
  } catch (e) {
    console.error('harvest-psa-token: debug dump failed:', e.message);
  }
}

// loginWithPassword drives the Collectors SSO password form. Selectors and
// fallbacks are unchanged from the original inline flow.
async function loginWithPassword(page) {
  // --- Email step ---
  const emailField = await firstVisible(page, [
    '#email',
    'input[name="email"]',
    (p) => p.getByLabel(/email/i),
    'input[type="email"]',
    'input[name*="email" i]',
    'input[autocomplete="username"]',
  ]);
  if (!emailField) {
    await dumpDebug(page, 'no-email-field');
    throw new Error('could not find the email field on the sign-in page');
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
    throw new Error('could not find the password field on the sign-in page');
  }
  await passwordField.fill(PASSWORD);

  // --- Submit ---
  // Collectors' password step uses a Vaadin button labelled "Verify" (no type=submit).
  const submit = await firstVisible(page, [
    (p) => p.getByRole('button', { name: /verify|sign ?in|log ?in|continue|submit/i }),
    'button[type="submit"]',
  ]);
  if (!submit) {
    await dumpDebug(page, 'no-submit');
    throw new Error('could not find the submit button on the sign-in page');
  }
  await submit.click();
}

// Browser selection. Playwright's bundled-browser download is unreliable in some
// environments, so allow pointing at an already-installed Chrome/Chromium:
//   PSA_PORTAL_CHROME_PATH    — absolute path to a chrome/chromium binary (executablePath)
//   PSA_PORTAL_CHROME_CHANNEL — branded channel, e.g. "chrome" or "msedge" (no download)
// If neither is set, Playwright uses its bundled Chromium (requires `playwright install`).
const launchOpts = { headless: true };
if (process.env.PSA_PORTAL_CHROME_PATH) {
  launchOpts.executablePath = process.env.PSA_PORTAL_CHROME_PATH;
} else if (process.env.PSA_PORTAL_CHROME_CHANNEL) {
  launchOpts.channel = process.env.PSA_PORTAL_CHROME_CHANNEL;
}
const browser = await chromium.launch(launchOpts);
const context = await browser.newContext({ userAgent: UA });
const page = await context.newPage();

try {
  // Inject a previously harvested token so a still-valid session skips SSO.
  if (ACCESS_TOKEN) {
    await context.addCookies([{
      name: 'accessToken',
      value: ACCESS_TOKEN,
      domain: COOKIE_DOMAIN,
      path: '/',
      secure: true,
    }]);
  }

  await page.goto(START_URL, { waitUntil: 'domcontentloaded', timeout: 60000 });
  // Authenticated sessions stay on the portal; everyone else bounces to
  // app.collectors.com/signin. Wait for either outcome, then check where we are.
  await Promise.race([
    page.waitForURL(/collectors\.com\/signin/i, { timeout: 30000 }),
    page.waitForURL(/psacard\.com\/buyercampaignmanager/i, { timeout: 30000 }),
  ]).catch(() => {});

  if (/collectors\.com\/signin/i.test(page.url())) {
    await loginWithPassword(page);
    await page.waitForURL(/psacard\.com\/buyercampaignmanager/i, { timeout: 60000 }).catch(() => {});
  }

  // Read the accessToken cookie (set on psacard.com after the SSO round-trip).
  const cookies = await context.cookies(['https://www.psacard.com']);
  const at = cookies.find((c) => c.name === 'accessToken');
  if (!at || !at.value) {
    // The two-outcome URL race above assumes we land on /signin or the portal.
    // Include the actual landing URL so an unexpected third page (interstitial,
    // consent, changed path) is diagnosable rather than hidden behind a generic
    // "no accessToken cookie" error.
    console.error(`harvest-psa-token: no accessToken cookie; landed on ${page.url()}`);
    await dumpDebug(page, 'no-access-cookie');
    throw new Error('login completed but no accessToken cookie was set');
  }

  const expiresAt = jwtExpiry(at.value);
  if (!expiresAt) {
    await dumpDebug(page, 'bad-jwt');
    throw new Error('accessToken cookie is not a decodable JWT');
  }

  // Fetch the analytics __data.json from inside the page: the browser context
  // already holds cf_clearance, so this bypasses the Cloudflare gate that
  // blocks plain HTTP clients on datacenter IPs. Its embedUrl carries a
  // Lightdash embed JWT minted fresh per request (~1h TTL) — the Go side
  // exchanges it for rows immediately.
  const analytics = await page.evaluate(async (path) => {
    const r = await fetch(path, { credentials: 'include' });
    return { status: r.status, body: await r.text() };
  }, '/buyercampaignmanager/analytics/__data.json?x-sveltekit-invalidated=001');
  if (analytics.status !== 200 || !analytics.body.includes('embedUrl')) {
    await dumpDebug(page, 'analytics-fetch');
    throw new Error(`analytics __data.json fetch failed (status ${analytics.status})`);
  }

  process.stdout.write(
    JSON.stringify({ accessToken: at.value, expiresAt, analyticsData: analytics.body }) + '\n'
  );
} catch (e) {
  await dumpDebug(page, 'exception');
  // Don't call fail() here — it exits immediately and would skip the finally
  // block, leaking the browser process. Flag failure and let finally close it.
  console.error(`harvest-psa-token: ${e.message}`);
  process.exitCode = 1;
} finally {
  await browser.close();
}
