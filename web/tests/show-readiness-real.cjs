/* global process, URL, fetch, AbortSignal, setTimeout, Buffer, document, innerWidth, innerHeight, window, Event, console */
// Driven by TestShowReadinessRealBrowser. No application API route is stubbed:
// only external public fonts are fetched without credentials; all other external
// requests are blocked. Assertions cross browser -> Go -> PG -> local source.
const { chromium, expect } = require('@playwright/test');
const fs = require('node:fs/promises');
const path = require('node:path');
const { withFixtureCleanup, assertLastRowClearance } = require('./show-readiness-browser-helpers.cjs');

const app = process.env.SHOW_READINESS_APP;
const control = process.env.SHOW_READINESS_CONTROL;
const token = process.env.SHOW_READINESS_TOKEN;
const artifacts = process.env.SHOW_READINESS_ARTIFACTS;
const id = '11111111-1111-4111-8111-000000000001';
const cert = '91000001';
const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
for (const url of [app, control]) {
  if (new URL(url).hostname !== '127.0.0.1' || new URL(url).port === '4173') throw Error('local fixture URLs required');
}
async function json(url, method = 'GET', body) {
  const response = await fetch(url, { method, headers, body: body && JSON.stringify(body), signal: AbortSignal.timeout(15000) });
  expect(response.ok, `${method} ${url}: ${response.status}`).toBe(true);
  return response.json();
}
const state = () => json(`${control}/state`);
const command = route => json(`${control}/${route}`, 'POST');
const evidence = () => json(`${app}/api/show-prep/evidence/${id}`);
const refreshes = s => s.requests.filter(r => r.path === '/api/show-prep/refresh');
const listWrites = s => s.requests.filter(r => r.path.startsWith('/api/show-prep/lists') && r.method !== 'GET');
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));

(async () => {
  const browser = await chromium.launch({ headless: true });
  let page;
  const metrics = [];
  const snapshots = {};
  const refreshBodies = [];
  const refreshReplies = [];
  await withFixtureCleanup(async () => {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, extraHTTPHeaders: headers });
  page = await context.newPage();
  const errors = [];
  page.on('response', async response => {
    if (new URL(response.url()).pathname === '/api/show-prep/refresh') {
      try { refreshReplies.push(await response.json()); } catch { /* Cancellation may prevent a body. */ }
    }
  });
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => {
    if (new URL(request.url()).pathname === '/api/show-prep/refresh') refreshBodies.push(request.postDataJSON());
  });
  const fontCache = new Map();
  await context.route(url => url.origin !== new URL(app).origin, async route => {
    const url = route.request().url();
    const host = new URL(url).hostname;
    if (['fonts.googleapis.com', 'fonts.gstatic.com'].includes(host) && route.request().method() === 'GET') {
      try {
        if (!fontCache.has(url)) {
          const response = await fetch(url, { redirect: 'error', signal: AbortSignal.timeout(8000) });
          if (!response.ok) throw Error(`font HTTP ${response.status}`);
          fontCache.set(url, { body: Buffer.from(await response.arrayBuffer()), contentType: response.headers.get('content-type') });
        }
        await route.fulfill(fontCache.get(url));
      } catch { await route.abort(); }
    } else await route.abort();
  });
  async function capture(name) {
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: path.join(artifacts, `${name}.png`), fullPage: true });
    await page.screenshot({ path: path.join(artifacts, `${name}-viewport.png`) });
    metrics.push({ name, ...await page.evaluate(() => {
      const box = selector => { const e = document.querySelector(selector); if (!e) return null; const b = e.getBoundingClientRect(); return { x: b.x, y: b.y, width: b.width, height: b.height }; };
      return { width: innerWidth, height: innerHeight, overflow: document.documentElement.scrollWidth > innerWidth,
        readiness: box('.show-readiness'), bar: box('.show-selection-bar:not([hidden])'),
        row: box('.show-inventory [role="row"]'), card: box('article'),
        toolbar: box('.show-toolbar'),
        showControls: [...document.querySelectorAll('.show-toolbar select, .show-toolbar button, .show-selection-bar:not([hidden]) button')]
          .filter(e => e.getClientRects().length).map(e => ({ text: e.textContent, height: e.getBoundingClientRect().height })),
        fonts: [...document.fonts].filter(f => f.status === 'loaded').map(f => f.family) };
    }) });
    expect(metrics.at(-1).overflow, name).toBe(false);
    if (metrics.at(-1).width <= 768) expect(metrics.at(-1).showControls.every(c => c.height >= 44), `${name} touch targets`).toBe(true);
  }
  async function scope() {
    await page.getByRole('button', { name: /^All\s*\d+$/ }).click();
    await page.getByLabel('Search cards', { exact: true }).fill('Readiness slab');
    await expect(page.getByText('24 of 26 cards', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Sort by Card', exact: true }).click();
  }
  {
    let s = await state();
    expect(s.evidence).toEqual([]); expect(s.lists).toEqual([]); expect(s.items).toEqual([]); expect(s.calls).toEqual([]);
    expect((await fetch(`${app}/api/show-prep/lists`)).status).toBe(401);
    expect((await fetch(`${app}/api/inventory`, { headers: { Authorization: 'Bearer wrong' } })).status).toBe(401);
    await page.clock.setFixedTime(new Date(s.now));
    await page.goto(`${app}/shows`);
    await expect(page.getByRole('heading', { name: 'No show lists yet' })).toBeVisible();
    await capture('desktop-empty-shows');
    await page.setViewportSize({ width: 390, height: 844 });
    await capture('mobile-empty-shows');
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`${app}/inventory`);
    await expect(page.getByRole('heading', { name: 'Inventory', exact: true })).toBeVisible();
    await scope();
    await page.getByRole('button', { name: `Show 30-day evidence ${cert}` }).click();
    await expect(page.getByRole('button', { name: `Hide 30-day evidence ${cert}` })).toContainText('Not checked');
    await expect(page.getByRole('region', { name: `30-day evidence ${cert}` })).toContainText('No verified CardLadder evidence');
    await expect(page.getByRole('region', { name: `30-day evidence ${cert}` })).not.toContainText('0 sales');
    expect((await evidence()).evaluation.readiness.state).toBe('not_checked');
    await pause(600);
    expect((await state()).calls).toHaveLength(0);
    await capture('desktop-cold-read-only');
    await page.getByRole('button', { name: `Hide 30-day evidence ${cert}` }).click();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.evaluate(() => window.scrollTo(0, 0));
    await capture('mobile-normal');
    await page.setViewportSize({ width: 1440, height: 1000 });

    await command('source?mode=hold');
    // The original cold-init path dead-ended here: no selected IDs or list exist.
    await page.getByLabel('Price support', { exact: true }).selectOption('supported');
    await expect.poll(async () => (await state()).calls.length, { timeout: 12000, message: 'cold Supported-first must reach real CardLadder adapter' }).toBe(1);
    await expect(page.getByLabel('Comps readiness')).toContainText('Checking comps');
    expect((await state()).lists).toEqual([]);
    await expect(page.getByRole('region', { name: 'Show selection actions' })).toHaveCount(0);
    await capture('desktop-cold-checking');
    await command('source?mode=complete');
    await expect(page.getByLabel('Comps readiness')).toContainText('Check complete · 24 cards current', { timeout: 35000 });
    await expect(page.getByText('24 cards shown', { exact: true })).toBeVisible();
    s = await state(); snapshots.coldAcquired = s;
    expect(s.calls).toHaveLength(12); expect(refreshes(s)).toHaveLength(2);
    expect(refreshBodies.map(body => body.purchaseIds.length)).toEqual([10, 2]);
    expect(new Set(refreshBodies.flatMap(body => body.purchaseIds)).size).toBe(12);
    expect(s.evidence).toHaveLength(12); expect(s.evidence.every(e => e.attempt_state === 'complete')).toBe(true);
    expect(s.calls.every(c => !c.filters[0].includes('outside') && !c.filters[0].includes('no-price'))).toBe(true);
    expect(listWrites(s)).toEqual([]); expect(s.holds).toEqual([]);
    let e = await evidence();
    expect(e.evaluation).toMatchObject({ status: 'supported', listedPriceCents: 30000, compCount: 2, readiness: { state: 'current' } });
    expect(e.sales.map(sale => sale.priceCents)).toEqual([27000, 29000]);
    const initialVersion = e.evaluation.version;
    const initialWindow = e.evaluation.windowEnd;
    await capture('desktop-current');
    await page.getByLabel('Price support', { exact: true }).selectOption('below_target');
    await expect(page.getByText('No cards match this price support filter.', { exact: true })).toBeVisible();
    await capture('desktop-complete-empty-filter');
    await page.getByLabel('Price support', { exact: true }).selectOption('supported');

    await page.reload(); await scope();
    await page.getByLabel('Price support', { exact: true }).selectOption('supported');
    await expect(page.getByLabel('Comps readiness')).toContainText('24 cards current');
    await pause(600); expect((await state()).calls).toHaveLength(12);
    await command('restart');
    await page.reload(); await scope();
    await page.getByLabel('Price support', { exact: true }).selectOption('supported');
    await expect(page.getByLabel('Comps readiness')).toContainText('24 cards current');
    expect((await evidence()).evaluation.version).toBe(initialVersion);
    await pause(600); expect((await state()).calls).toHaveLength(12);

    await page.getByRole('button', { name: 'Show selection', exact: true }).click();
    const checkbox = () => page.getByRole('checkbox', { name: `Select ${cert}`, exact: true });
    await checkbox().check();
    const add = () => page.getByRole('button', { name: 'Add selected to show (1)' });
    await expect(add()).toBeEnabled();
    await capture('desktop-selection');
    await add().click();
    await expect(page.getByLabel('New show name')).toBeVisible();
    await page.keyboard.press('Escape'); await expect(add()).toBeFocused();
    await add().click();
    await page.getByLabel('New show name').fill('Local real-wire show');
    await page.getByRole('button', { name: 'Create show list', exact: true }).click();
    await expect(page.getByRole('combobox', { name: 'Show list', exact: true })).not.toHaveValue('');
    await add().click();
    await page.getByRole('link', { name: 'Open packing list →' }).click();
    const packed = () => page.getByRole('checkbox', { name: `Packed ${cert}`, exact: true });
    await packed().focus(); await packed().press('Space'); await expect(packed()).toBeChecked();
    await page.reload(); await expect(packed()).toBeChecked();
    s = await state(); snapshots.packed = s;
    expect(s.lists).toHaveLength(1); expect(s.items).toHaveLength(1);
    expect(listWrites(s).map(r => r.method)).toEqual(['POST', 'POST', 'PUT']);
    const packedRow = s.items[0];
    const listID = s.lists[0].id;
    await capture('desktop-packed');

    await page.goto(`${app}/inventory`); await scope();
    await page.getByLabel('Price support', { exact: true }).selectOption('supported');
    await page.getByRole('button', { name: 'Show selection', exact: true }).click();
    await checkbox().check();
    s = await command('rollover');
    await page.clock.setFixedTime(new Date(s.now));
    e = await evidence();
    expect(e.evaluation.readiness.state).toBe('stale');
    expect(e.evaluation.status).toBe('needs_review');
    expect(e.evaluation.version).not.toBe(initialVersion);
    expect(e.sales).toHaveLength(2);
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(page.getByRole('region', { name: 'Show selection actions' })).toContainText('Review and reselect');
    await expect(checkbox()).toBeChecked(); await expect(add()).toBeDisabled();
    expect((await state()).calls).toHaveLength(12);
    await capture('desktop-rollover-selected');
    await page.getByRole('button', { name: 'Clear selection', exact: true }).click();
    await expect(page.getByLabel('Comps readiness')).toContainText('24 cards current', { timeout: 35000 });
    s = await state(); snapshots.renewed = s;
    expect(s.calls).toHaveLength(24); expect(s.items).toEqual([packedRow]);
    e = await evidence();
    expect(e.evaluation.readiness.state).toBe('current');
    expect(e.evaluation.windowEnd).not.toBe(initialWindow);
    expect(e.evaluation.windowEnd).toBe(s.now.slice(0, 10));
    const start = new Date(`${s.now.slice(0, 10)}T00:00:00Z`);
    start.setUTCDate(start.getUTCDate() - 29);
    expect(e.evaluation.windowStart).toBe(start.toISOString().slice(0, 10));
    expect(e.sales.every(sale => sale.date === s.now.slice(0, 10))).toBe(true);

    // Failure and partial pages must retain the last verified payload, never
    // relabel it current or automatically retry it on focus/reload/selection clear.
    const retained = e.sales;
    for (const mode of ['failed', 'partial']) {
      await command(`source?mode=${mode}`);
      if (mode === 'failed') {
        await checkbox().check();
        await page.getByRole('button', { name: 'Check selected (1)' }).click();
      } else await page.getByRole('button', { name: 'Retry / Continue checking', exact: true }).click();
      await expect.poll(async () => (await evidence()).evaluation.readiness.state).toBe('failed');
      await expect(page.getByRole('button', { name: 'Retry / Continue checking', exact: true })).toBeEnabled();
      e = await evidence(); expect(e.evaluation.status).toBe('needs_review'); expect(e.sales).toEqual(retained);
      s = await state(); const calls = s.calls.length;
      expect(s.evidence.find(row => row.profile_id === 'psa-1').attempt_state).toBe(mode === 'failed' ? 'failed' : 'partial');
      expect(s.items).toEqual([packedRow]);
      await page.evaluate(() => window.dispatchEvent(new Event('focus')));
      await pause(700); expect((await state()).calls).toHaveLength(calls);
      await capture(`desktop-${mode}-retained`);
      if (mode === 'failed') {
        await page.getByRole('button', { name: 'Clear selection', exact: true }).click();
        await page.reload(); await scope();
        await page.getByLabel('Price support', { exact: true }).selectOption('supported');
        await expect(page.getByRole('button', { name: 'Retry / Continue checking', exact: true })).toBeVisible();
        await pause(700); expect((await state()).calls).toHaveLength(calls);
      }
    }
    await command('source?mode=complete');
    await page.getByRole('button', { name: 'Retry / Continue checking', exact: true }).click();
    await expect(page.getByLabel('Comps readiness')).toContainText('24 cards current');
    expect((await evidence()).evaluation.status).toBe('supported');
    await page.getByRole('button', { name: 'Show selection', exact: true }).click();

    for (const viewport of [{ name: 'mobile', width: 390, height: 844 }, { name: 'tablet', width: 768, height: 1024 }]) {
      await page.setViewportSize(viewport);
      await page.evaluate(() => window.scrollTo(0, 0));
      await capture(`${viewport.name}-current`);
      await checkbox().check(); await checkbox().scrollIntoViewIfNeeded();
      // Scroll the document as an operator would; scrollIntoView alone treats
      // a fixed contextual bar as transparent, unlike the user's viewport.
      await page.evaluate(() => window.scrollBy(0, 240));
      await capture(`${viewport.name}-selected`);
      const bar = await page.getByRole('region', { name: 'Show selection actions' }).boundingBox();
      const box = await checkbox().boundingBox(); expect(box.y + box.height).toBeLessThanOrEqual(bar.y);
      await page.getByRole('button', { name: `Show 30-day evidence ${cert}` }).click();
      await expect(page.getByRole('region', { name: `30-day evidence ${cert}` })).toContainText('$270.00');
      await capture(`${viewport.name}-evidence`);
      await page.getByRole('button', { name: `Hide 30-day evidence ${cert}` }).click();
      for (let turn = 0; turn < 3; turn++) {
        await page.evaluate(() => {
          document.querySelectorAll('.show-inventory .overflow-y-auto').forEach(e => { e.scrollTop = e.scrollHeight; });
          window.scrollTo(0, document.body.scrollHeight);
        });
        await pause(180);
      }
      const last = page.getByRole('checkbox', { name: 'Select 91000024', exact: true });
      await assertLastRowClearance(last, page.getByRole('region', { name: 'Show selection actions' }));
      await capture(`${viewport.name}-last-row-clearance`);
      await page.keyboard.press('Escape');
      await expect(page.getByRole('region', { name: 'Show selection actions' })).toHaveCount(0);
      await page.evaluate(() => document.querySelectorAll('.show-inventory .overflow-y-auto').forEach(e => { e.scrollTop = 0; }));
    }
    await page.getByLabel('Search cards', { exact: true }).fill('Readiness');
    await expect(page.getByLabel('Comps readiness')).toContainText('1 without a listed price');
    await pause(700); expect((await state()).calls).toHaveLength(27);
    const unpriced = await json(`${app}/api/show-prep/evidence/11111111-1111-4111-8111-000000000026`);
    expect(unpriced.evaluation).toMatchObject({ status: 'no_listed_price', readiness: { state: 'not_checked' } });
    await page.goto(`${app}/shows?list=${listID}`); await expect(packed()).toBeChecked();
    await capture('tablet-packing-history');
    s = await state(); snapshots.final = s;
    expect(s.items).toEqual([packedRow]); expect(s.ledgerUnchanged).toBe(true);
    expect(listWrites(s)).toHaveLength(3);
    expect(s.calls).toHaveLength(27); expect(refreshes(s)).toHaveLength(7);
    const unexpectedWrites = s.requests.filter(r => !['GET', 'HEAD'].includes(r.method) && !r.path.startsWith('/api/show-prep/'));
    expect(unexpectedWrites).toEqual([]); expect(errors).toEqual([]);
    console.log('PASS real-wire cold upgrade, 12 coalesced identities/2 batches, reload/restart, UTC rollover/selection, list/packing, failed+partial retained evidence, explicit retry; 27 source GETs, 7 refresh POSTs, 3 explicit list writes; ledger unchanged');
  }
  }, [
    () => fs.writeFile(path.join(artifacts, 'metrics.json'), JSON.stringify(metrics, null, 2)),
    async () => fs.writeFile(path.join(artifacts, 'wire-snapshots.json'), JSON.stringify({ snapshots, refreshBodies, refreshReplies, lastEvidence: await evidence(), lastState: await state() }, null, 2)),
    () => page?.screenshot({ path: path.join(artifacts, 'last-view.png'), fullPage: true }),
  ], () => browser.close());
})().catch(error => { console.error(error); process.exitCode = 1; });
