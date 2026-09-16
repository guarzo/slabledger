/* global process, URL, fetch, AbortSignal, Buffer, document, innerWidth, innerHeight, window, Event, console, performance, requestAnimationFrame */
// Cached-use acceptance: real browser -> real Go router/service -> PostgreSQL.
// Go finishes real worker population BEFORE launching this process in worker mode.
// Cached mode instead uses explicitly seeded preconditions. In BOTH modes the
// worker is stopped/joined and provider blocked; acquisition history is preserved.
// No application API response is intercepted.
const { chromium, expect } = require('@playwright/test');
const fs = require('node:fs/promises');
const path = require('node:path');
const { withFixtureCleanup, assertLastRowClearance } = require('./show-readiness-browser-helpers.cjs');
const { exerciseGeometry } = require('./show-readiness-geometry.cjs');
const app = process.env.SHOW_READINESS_APP;
const control = process.env.SHOW_READINESS_CONTROL;
const token = process.env.SHOW_READINESS_TOKEN;
const artifacts = process.env.SHOW_READINESS_ARTIFACTS;
const id = i => `11111111-1111-4111-8111-${String(i).padStart(12, '0')}`;
const cert = i => `91000${String(i).padStart(3, '0')}`;
const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
const explicitRequests = [];
for (const url of [app, control]) {
  if (new URL(url).hostname !== '127.0.0.1' || new URL(url).port === '4173') throw Error('owned local fixture URLs required');
}
async function json(url, method = 'GET', body, status = 200) {
  const response = await fetch(url, { method, headers, body: body && JSON.stringify(body), signal: AbortSignal.timeout(15000) });
  explicitRequests.push({ method, path: new URL(url).pathname, status: response.status, ...(body && { body }) });
  expect(response.status, `${method} ${url}`).toBe(status); return response.json();
}
const state = () => json(`${control}/state`);
const command = route => json(`${control}/${route}`, 'POST');
const evidence = i => json(`${app}/api/show-prep/evidence/${id(i)}`);
const listWrites = s => s.requests.filter(r => r.path.startsWith('/api/show-prep/lists') && r.method !== 'GET');
(async () => {
  const browser = await chromium.launch({ headless: true });
  let page; const metrics = []; const snapshots = {}; const acquisitionRequests = []; const checkboxRequests = [];
  await withFixtureCleanup(async () => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, extraHTTPHeaders: headers });
    page = await context.newPage(); const errors = []; const pageRequests = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', request => {
      const requestPath = new URL(request.url()).pathname;
      pageRequests.push({ method: request.method(), path: requestPath, ...(request.postData() && { body: request.postData() }) });
      if (requestPath === '/api/show-prep/refresh') acquisitionRequests.push(requestPath);
    });
    const fonts = new Map();
    await context.route(url => url.origin !== new URL(app).origin, async route => {
      const url = route.request().url();
      if (['fonts.googleapis.com', 'fonts.gstatic.com'].includes(new URL(url).hostname) && route.request().method() === 'GET') {
        try {
          if (!fonts.has(url)) {
            const response = await fetch(url, { redirect: 'error', signal: AbortSignal.timeout(8000) });
            if (!response.ok) throw Error(`font HTTP ${response.status}`);
            fonts.set(url, { body: Buffer.from(await response.arrayBuffer()), contentType: response.headers.get('content-type') });
          }
          await route.fulfill(fonts.get(url));
        } catch { await route.abort(); }
      } else await route.abort();
    });
    async function capture(name, cdp) {
      await page.evaluate(() => document.fonts.ready);
      if (cdp) {
        const shot = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
        await fs.writeFile(path.join(artifacts, `${name}-viewport.png`), Buffer.from(shot.data, 'base64'));
      } else {
        await page.screenshot({ path: path.join(artifacts, `${name}.png`), fullPage: true });
        await page.screenshot({ path: path.join(artifacts, `${name}-viewport.png`) });
      }
      const geometry = await page.evaluate(() => ({ width: innerWidth, height: innerHeight,
        fonts: [...document.fonts].map(f => ({ family: f.family, status: f.status, weight: f.weight, style: f.style, unicodeRange: f.unicodeRange })),
        renderedFonts: [...document.querySelectorAll('.page-title, .tabular-nums, .num')]
          .filter(e => e.getClientRects().length && e.textContent.trim()).map(e => {
            const style = window.getComputedStyle(e); const font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
            return { font, loaded: document.fonts.check(font, e.textContent.trim()) };
          }),
        overflow: document.documentElement.scrollWidth > innerWidth,
        controls: [...document.querySelectorAll('.show-toolbar select, .show-toolbar button, .show-selection-bar:not([hidden]) button')]
          .filter(e => e.getClientRects().length).map(e => ({ text: e.textContent, height: e.getBoundingClientRect().height })),
      }));
      metrics.push({ name, ...geometry }); expect(geometry.overflow, name).toBe(false);
      expect(geometry.renderedFonts.every(f => f.loaded), `${name} rendered webfonts loaded`).toBe(true);
      if (geometry.width <= 768) expect(geometry.controls.every(c => c.height >= 44), `${name} touch targets`).toBe(true);
    }
    const checkbox = i => page.getByRole('checkbox', { name: `Select ${cert(i)}`, exact: true });
    const add = n => page.getByRole('button', { name: `Add to show (${n})`, exact: true });
    const bar = () => page.getByRole('region', { name: 'Bulk actions for selected cards' });
    async function idle() { await page.waitForLoadState('networkidle'); }
    async function pure(label, action) {
      await idle(); const before = pageRequests.length; await action();
      await page.waitForTimeout(150);
      const requests = pageRequests.slice(before); checkboxRequests.push({ label, requests });
      expect(requests, `${label} must not make a request`).toEqual([]);
    }
    async function inventory() {
      await expect(page.getByRole('heading', { name: 'Inventory', exact: true })).toBeVisible();
      await page.getByRole('button', { name: /^All\s*\d+$/ }).click();
      await page.getByRole('button', { name: 'Sort by Card', exact: true }).click();
      await expect(page.getByText('155 cards shown', { exact: true })).toBeVisible(); await idle();
    }
    const before = await state(); snapshots.precondition = before;
    const workerMode = before.mode === 'worker';
    const sourceBaseline = before.calls.length; const providerBaseline = before.providerRequests.length;
    expect(sourceBaseline).toBe(workerMode ? 142 : 0); expect(providerBaseline).toBe(workerMode ? 143 : 0);
    expect(before.evidence.length).toBe(workerMode ? 142 : 141);
    const assertNoSource = s => {
      expect(s.calls).toEqual(before.calls); expect(s.providerRequests).toEqual(before.providerRequests);
    };
    expect(before.lists).toEqual([]); expect(before.items).toEqual([]);
    expect((await fetch(`${app}/api/show-prep/refresh`, { method: 'POST' })).status).toBe(401);
    expect((await fetch(`${app}/api/show-prep/refresh`, { method: 'POST', headers: { Authorization: 'Bearer wrong' } })).status).toBe(401);
    const gone = await json(`${app}/api/show-prep/refresh`, 'POST', { purchaseIds: [id(1)] }, 410);
    expect(gone.error).toContain('server-managed');
    const afterGone = await state();
    for (const key of ['evidence', 'lists', 'items', 'holds']) expect(afterGone[key]).toEqual(before[key]);
    assertNoSource(afterGone); expect(afterGone.ledgerUnchanged).toBe(true);
    // Actual Admin UI/status, with the same disabled production composition used
    // through restart. Unrelated integration panels can honestly be unconfigured.
    await page.goto(`${app}/admin`);
    const workerPanel = page.getByRole('region', { name: 'Show evidence', exact: true });
    await expect(workerPanel).toContainText('Disabled by SHOW_PREP_REFRESH_ENABLED');
    await expect(workerPanel).toContainText(workerMode ? '153/155 cards with current evidence' : '151/155 cards with current evidence');
    await expect(workerPanel.getByRole('button', { name: 'Run now', exact: true })).toBeDisabled();
    await workerPanel.scrollIntoViewIfNeeded(); await capture('desktop-admin-worker');
    await page.setViewportSize({ width: 390, height: 844 }); await workerPanel.scrollIntoViewIfNeeded(); await capture('mobile-admin-worker');
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`${app}/shows`);
    await expect(page.getByRole('heading', { name: 'No show lists yet' })).toBeVisible(); await capture('desktop-empty-shows');
    await page.getByRole('link', { name: 'Inventory →', exact: true }).click(); await inventory();
    await expect(page.getByLabel('Comp data coverage')).toContainText(workerMode ? '153/155 cards with current evidence' : '151/155 cards with current evidence');
    await capture('desktop-cached-inventory');
    const e = await evidence(1);
    expect(e.evaluation).toMatchObject({ status: 'supported', listedPriceCents: 30000, medianCents: 28000, compCount: 2, readiness: { state: 'current' } });
    expect(e.sales.map(s => s.priceCents)).toEqual([27000, 29000]);
    // UI labels and stored evidence use the real evaluator, including incomplete coverage.
    for (const [i, label] of [[25,workerMode ? 'Supported' : 'No comp data'], [26,'No DH price'], [28,workerMode ? 'Supported' : 'Out of date'], [29,'Data unavailable'], [30,'Needs matching'], [31,'No recent sales'], [32,'Thin evidence'], [33,'Below target']]) {
      await page.getByLabel('Search cards', { exact: true }).fill(cert(i));
      const trigger = page.getByRole('button', { name: new RegExp(`Show 30-day evidence ${cert(i)}: ${label}`) });
      await expect(trigger).toBeVisible();
      if (i === 29 || (!workerMode && i === 28)) {
        await trigger.click();
        await expect(page.getByRole('region', { name: `30-day evidence ${cert(i)}` })).toContainText('$270.00');
        await capture(`desktop-retained-${i}`);
      }
    }
    await page.getByLabel('Search cards', { exact: true }).fill(''); await expect(page.getByText('155 cards shown', { exact: true })).toBeVisible();
    await pure('Supported filter', () => page.getByLabel('Price support', { exact: true }).selectOption('supported'));
    await expect(page.getByText(`${workerMode ? 149 : 147} cards shown`, { exact: true })).toBeVisible();
    // Measure event -> next rendered frame inside the browser, excluding driver latency.
    const interaction = await page.evaluate(async () => {
      const select = document.querySelector('select[aria-label="Price support"]');
      const timed = async action => { const start = performance.now(); action(); await new Promise(requestAnimationFrame); return performance.now() - start; };
      const filterMs = await timed(() => { select.value = 'all'; select.dispatchEvent(new Event('change', { bubbles: true })); });
      const check = document.querySelector('input[aria-label="Select 91000001"]');
      const selectMs = await timed(() => check.click()); const clearMs = await timed(() => check.click());
      return { filterMs, selectMs, clearMs };
    });
    metrics.push({ name: '155-card-interaction-ms', ...interaction });
    // Report local measurements against the <100ms goal, not a flaky CI percentile gate.
    metrics.push({ name: '155-card-target', targetMs: 100, met: Object.values(interaction).every(ms => ms < 100) });
    await pure('select all 155', () => page.getByRole('checkbox', { name: 'Select all visible cards', exact: true }).check());
    await expect(add(155)).toBeEnabled(); await pure('clear all 155', () => page.getByRole('button', { name: 'Clear', exact: true }).click());
    await pure('Supported filter again', () => page.getByLabel('Price support', { exact: true }).selectOption('supported'));
    // Existing geometry assertions exercise stable virtual rows with stored sales.
    await exerciseGeometry(page, context, capture, metrics, cert(1));
    await pure('checkbox', () => checkbox(1).check()); await expect(add(1)).toBeEnabled();
    await expect(bar()).toContainText('$310.00 CL value');
    await expect(bar()).not.toContainText('$310.00 list');
    await expect(bar().getByRole('button', { name: 'Record sale (1)' })).toBeEnabled();
    await expect(bar().getByRole('button', { name: 'List on DH (1)' })).toBeEnabled();
    await expect(page.getByRole('button', { name: /Show selection|Check selected|Cancel checking|Check comps|Retry.*checking/ })).toHaveCount(0);
    await page.getByRole('button', { name: new RegExp(`Show 30-day evidence ${cert(1)}`) }).click();
    await expect(page.getByRole('region', { name: `30-day evidence ${cert(1)}` })).toContainText('$290.00');
    await capture('desktop-selected-evidence');
    await add(1).click(); await expect(page.getByLabel('New show name')).toBeVisible();
    await page.getByRole('button', { name: 'Close destination', exact: true }).click(); await expect(add(1)).toBeFocused();
    await add(1).click(); await page.getByLabel('New show name').fill('Cached local show');
    await page.getByRole('button', { name: 'Create show list', exact: true }).click();
    await expect(page.getByRole('combobox', { name: 'Show list', exact: true })).not.toHaveValue('');
    await add(1).click(); await page.getByRole('link', { name: 'Open packing list →' }).click();
    const packed = i => page.getByRole('checkbox', { name: `Packed ${cert(i)}`, exact: true });
    await packed(1).focus(); await packed(1).press('Space'); await expect(packed(1)).toBeChecked();
    await page.reload(); await expect(packed(1)).toBeChecked(); await capture('desktop-packed');
    let s = await state(); snapshots.packed = s;
    const listID = s.lists[0].id; const packedRow = s.items[0];
    expect(listWrites(s).map(r => r.method)).toEqual(['POST','POST','PUT']);
    await command('restart'); await page.reload(); await expect(packed(1)).toBeChecked();
    await page.getByRole('link', { name: 'Inventory →', exact: true }).click(); await inventory();
    await checkbox(2).check(); await add(1).click();
    await page.getByRole('combobox', { name: 'Show list', exact: true }).selectOption(listID); await add(1).click();
    await page.getByRole('link', { name: 'Open packing list →' }).click();
    await expect(packed(2)).not.toBeChecked();
    s = await state(); snapshots.existingListAdded = s;
    const second = s.items.find(item => item.purchase_id === id(2)); const old = await evidence(2);
    const third = await evidence(3);
    // A clock change invalidates selected versions, without changing stored prices.
    await page.getByRole('link', { name: 'Inventory →', exact: true }).click(); await inventory();
    await checkbox(3).check(); await page.getByLabel('Price support', { exact: true }).selectOption('supported');
    s = await command('rollover'); await page.clock.setFixedTime(new Date(s.now));
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(bar()).toContainText('Review and reselect'); await expect(checkbox(3)).toBeChecked(); await expect(add(1)).toBeDisabled();
    await json(`${app}/api/show-prep/lists/${listID}/items`, 'POST', { items: [{ purchaseId: id(3), evaluationVersion: third.evaluation.version }] }, 409);
    await json(`${app}/api/show-prep/lists/${listID}/items/${second.id}`, 'PUT', { version: second.version, evaluationVersion: old.evaluation.version, packed: true }, 409);
    s = await state(); expect(s.items).toEqual(snapshots.existingListAdded.items); assertNoSource(s);
    await capture('desktop-changed-selection');
    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    await expect(page.getByText('No current matches under these filters.', { exact: true })).toBeVisible();
    await expect(page.getByText(/Comp data coverage is incomplete/)).toBeVisible(); await capture('desktop-incomplete-supported');
    await page.getByLabel('Price support', { exact: true }).selectOption('all');
    for (const viewport of [{ name: 'mobile', width: 390, height: 844 }, { name: 'tablet', width: 768, height: 1024 }]) {
      await page.setViewportSize(viewport); await page.evaluate(() => window.scrollTo(0,0));
      await checkbox(1).check(); await checkbox(1).scrollIntoViewIfNeeded();
      await page.evaluate(() => window.scrollBy(0,240)); await capture(`${viewport.name}-selected`);
      await add(1).click(); await capture(`${viewport.name}-destination`);
      await page.getByRole('button', { name: 'Close destination', exact: true }).click();
      for (let turn = 0; turn < 3; turn++) {
        await page.evaluate(() => { document.querySelectorAll('.show-inventory .overflow-y-auto').forEach(e => { e.scrollTop = e.scrollHeight; }); window.scrollTo(0,document.body.scrollHeight); });
        await page.waitForTimeout(180);
      }
      await assertLastRowClearance(checkbox(24), bar()); await capture(`${viewport.name}-last-row-clearance`);
      await page.keyboard.press('Escape'); await expect(bar()).toHaveCount(0);
      await page.evaluate(() => document.querySelectorAll('.show-inventory .overflow-y-auto').forEach(e => { e.scrollTop = 0; }));
    }
    await page.goto(`${app}/shows?list=${listID}`); await expect(packed(1)).toBeChecked();
    // Weak/stale evidence is not a blanket packing prohibition; current version is explicit.
    await expect(packed(2)).toBeEnabled(); await packed(2).focus(); await packed(2).press('Space'); await expect(packed(2)).toBeChecked(); await capture('tablet-packing-history');
    s = await state(); snapshots.final = s;
    expect(s.items.find(item => item.purchase_id === id(1))).toEqual(packedRow);
    assertNoSource(s); expect(s.evidence).toEqual(before.evidence); expect(s.ledgerUnchanged).toBe(true);
    expect(acquisitionRequests).toEqual([]); expect(errors).toEqual([]);
    expect(s.requests.filter(r => !['GET','HEAD'].includes(r.method) && !r.path.startsWith('/api/show-prep/'))).toEqual([]);
    snapshots.cachedOperatorComplete = await command('cached-complete');
    console.log(`PASS ${before.mode} 155-card real Go/Postgres workflow; prior source calls=${sourceBaseline}, prior all provider requests=${providerBaseline}; operator added provider requests=0, browser refresh POSTs=0, checkbox/filter requests=0; financial+legacy rows unchanged`);
    if (workerMode) {
      // A SEPARATE phase, after archiving decisive B. This intentionally permits
      // one real background repair; never count its source calls as cached use.
      const concurrentStart = await command('publication-start');
      await page.clock.setFixedTime(new Date(concurrentStart.now));
      await page.setViewportSize({ width: 1440, height: 1000 });
      await page.goto(`${app}/inventory`); await inventory();
      await page.getByLabel('Search cards', { exact: true }).fill(cert(29));
      await page.getByLabel('Price support', { exact: true }).selectOption('needs_review');
      await expect(page.getByText('1 card shown', { exact: true })).toBeVisible();
      const held = await evidence(29);
      await json(`${app}/api/show-prep/lists/${listID}/items`, 'POST', { items: [{ purchaseId: id(29), evaluationVersion: held.evaluation.version }] });
      const conflictList = '22222222-2222-4222-8222-222222222222';
      await json(`${app}/api/show-prep/lists`, 'POST', { id: conflictList, name: 'Publication conflict target' });
      const added = await state(); const heldItem = added.items.find(item => item.purchase_id === id(29));
      await checkbox(29).check(); await expect(add(1)).toBeEnabled();
      await json(`${app}/api/admin/show-prep/worker/retry`, 'POST', undefined, 202);
      await expect.poll(async () => (await state()).calls.length).toBe(sourceBaseline + 1);
      expect((await json(`${app}/api/admin/show-prep/worker`)).state).toBe('running');
      await expect(bar().getByRole('button', { name: 'Record sale (1)' })).toBeEnabled();
      await expect(bar().getByRole('button', { name: 'List on DH (1)' })).toBeEnabled();
      // Opening and cancelling a real financial form must not wait for collection.
      await bar().getByRole('button', { name: 'Record sale (1)' }).click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await page.getByRole('dialog').getByRole('button', { name: /Cancel|Close/ }).first().click();
      await expect(checkbox(29)).toBeChecked();
      await capture('desktop-worker-active-selection');
      const boxBefore = await checkbox(29).boundingBox();
      await command('source?mode=complete');
      await expect.poll(async () => (await json(`${app}/api/admin/show-prep/worker`)).currentIdentities).toBe(142);
      await expect.poll(async () => (await json(`${app}/api/admin/show-prep/worker`)).state).toBe('idle');
      await command('publication-stop');
      await page.evaluate(() => window.dispatchEvent(new Event('focus')));
      await expect(bar()).toContainText('Review and reselect');
      await expect(checkbox(29)).toBeChecked(); await expect(add(1)).toBeDisabled();
      const boxAfter = await checkbox(29).boundingBox();
      metrics.push({ name: 'background-publication-held-row', before: boxBefore, after: boxAfter });
      expect(Math.abs(boxAfter.y - boxBefore.y)).toBeLessThan(2);
      // A new destination exercises stale Add, rather than the intentionally
      // idempotent replay of an already-added membership.
      await json(`${app}/api/show-prep/lists/${conflictList}/items`, 'POST', { items: [{ purchaseId: id(29), evaluationVersion: held.evaluation.version }] }, 409);
      await json(`${app}/api/show-prep/lists/${listID}/items/${heldItem.id}`, 'PUT', { version: heldItem.version, evaluationVersion: held.evaluation.version, packed: true }, 409);
      const published = await state(); const changed = await evidence(29);
      expect(changed.evaluation.status).toBe('supported'); expect(changed.evaluation.version).not.toBe(held.evaluation.version);
      expect(published.items).toEqual(added.items); expect(published.ledgerUnchanged).toBe(true);
      expect(published.calls.length).toBe(sourceBaseline + 1); expect(published.providerRequests.length).toBe(providerBaseline + 2);
      snapshots.concurrentPublication = { held, changed, before: added, after: published };
      await capture('desktop-background-publication-conflict');
      console.log('PASS separate concurrent-publication phase: actual runtime +1 source/+1 token, held selection/geometry retained, explicit Add/Pack409, financial form usable, list history and financial rows unchanged');
    }
    snapshots.browserRequests = pageRequests;
    expect(acquisitionRequests).toEqual([]); expect(errors).toEqual([]);
  }, [
    () => fs.writeFile(path.join(artifacts, 'metrics.json'), JSON.stringify(metrics,null,2)),
    async () => fs.writeFile(path.join(artifacts, 'wire-snapshots.json'), JSON.stringify({ snapshots, explicitRequests, acquisitionRequests, checkboxRequests, lastState: await state() },null,2)),
    () => page?.screenshot({ path: path.join(artifacts, 'last-view.png'), fullPage: true }),
  ], () => browser.close());
})().catch(error => { console.error(error); process.exitCode = 1; });
