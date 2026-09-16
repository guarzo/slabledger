/* global process, URL, fetch, AbortSignal, window, Event, document, console */
// Owned Chromium -> real local HTTP/auth/router/services/PostgreSQL. Only
// non-application origins are blocked. Controlled faults live in the Go owner.
const { chromium, expect } = require('@playwright/test');
const fs = require('node:fs/promises');
const path = require('node:path');
const { withFixtureCleanup } = require('./show-readiness-browser-helpers.cjs');
const app = process.env.SHOW_READINESS_APP;
const control = process.env.SHOW_READINESS_CONTROL;
const artifacts = process.env.SHOW_READINESS_ARTIFACTS;
const headers = { Authorization: `Bearer ${process.env.SHOW_READINESS_TOKEN}`, 'Content-Type': 'application/json' };
const id = i => `11111111-1111-4111-8111-${String(i).padStart(12, '0')}`;
for (const url of [app, control]) if (new URL(url).hostname !== '127.0.0.1') throw Error('owned loopback fixture required');
const wire = [], snapshots = {}, errors = [], failures = [];
async function json(url, method = 'GET', body, status = 200) {
  const response = await fetch(url, { method, headers, body: body && JSON.stringify(body), signal: AbortSignal.timeout(15000) });
  wire.push({ method, url, status: response.status, body });
  expect(response.status, `${method} ${url}`).toBe(status);
  return response.json();
}
const state = () => json(`${control}/state`);
const command = route => json(`${control}/${route}`, 'POST');
const evidence = i => json(`${app}/api/show-prep/evidence/${id(i)}`);
(async () => {
  const browser = await chromium.launch({ headless: true });
  let page;
  await withFixtureCleanup(async () => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, extraHTTPHeaders: headers });
    await context.route(url => url.origin !== new URL(app).origin, route => route.abort());
    page = await context.newPage();
    page.on('pageerror', error => errors.push(error.message));
    page.on('response', response => {
      if (response.url().startsWith(app) && response.status() >= 400) failures.push({ url: response.url(), status: response.status() });
    });
    page.on('request', request => wire.push({ method: request.method(), url: request.url(), body: request.postData() }));
    const saved = () => page.getByRole('region', { name: 'Saved price assessment' });
    const trial = () => page.getByRole('region', { name: 'Trial price assessment' });
    const filters = () => page.getByRole('group', { name: 'Price assessment filters' });
    const review = name => page.getByRole('button', { name: `Review ${name}`, exact: true }).click();
    const bar = () => page.getByRole('region', { name: 'Bulk actions for selected cards' });
    async function capture(name) {
      await page.screenshot({ path: path.join(artifacts, `${name}.png`), fullPage: true });
      await page.screenshot({ path: path.join(artifacts, `${name}-viewport.png`) });
      expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth), name).toBe(false);
    }
    const before = await state(); snapshots.initial = before;
    expect(before).toMatchObject({ mode: 'cached', workerEnabled: false, sourceCalls: 0, providerRequests: 0 });
    expect((await fetch(`${app}/api/show-prep/preview`, { method: 'POST' })).status).toBe(401);
    await page.goto(`${app}/inventory?retained=price-review`);
    await expect(page.getByRole('heading', { name: 'Inventory', exact: true })).toBeVisible();
    await page.getByRole('button', { name: /^All\s*7$/ }).click();
    // Normal operational forms remain available, but cancelling is not a sale.
    await page.getByLabel('Search cards', { exact: true }).fill('91000001');
    await expect(page.getByRole('row')).toHaveCount(1);
    const normalRow = page.getByRole('row').filter({ has: page.getByRole('checkbox', { name: 'Select 91000001', exact: true }) });
    await normalRow.getByRole('button', { name: 'Sell', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Record Sale', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await normalRow.getByRole('button', { name: 'More actions', exact: true }).click();
    await expect(page.getByRole('menuitem', { name: 'Fix DH Match', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await page.getByLabel('Search cards', { exact: true }).fill('91000002');
    await expect(page.getByRole('button', { name: 'List on DH', exact: true })).toBeEnabled();
    await page.getByLabel('Search cards', { exact: true }).fill('');
    await page.getByRole('button', { name: 'Price review', exact: true }).click();
    const cases = [
      ['Declining fixture', 'Asking above comps', '3,200.00'], ['Supported fixture', 'Supported', '2,400.00'],
      ['Mixed fixture', 'Mixed evidence', '2,540.00'], ['Single sale fixture', 'Limited evidence', '3,200.00'],
      ['Old sale fixture', 'Limited evidence', '2,400.00'], ['Unpriced fixture', 'No asking price', null],
      ['Failed fixture', 'Evidence unavailable', '2,400.00'],
    ];
    for (const [name, label, price] of cases) {
      await review(name);
      await expect(saved().getByText(label, { exact: true })).toBeVisible();
      await expect(page.getByLabel('Saved asking price')).toHaveText(price ? `$${price}` : 'Not set');
      await capture(`case-${name.split(' ')[0].toLowerCase()}`);
    }
    for (const label of ['Above comps 1', 'Mixed 1', 'Limited 2', 'Supported 1', 'Unavailable 1', 'Unpriced 1', 'All 7']) {
      await filters().getByRole('button', { name: label, exact: true }).click();
    }
    for (const sort of ['asking', 'recent', 'gap', 'supported', 'attention']) await page.getByLabel('Price review sort').selectOption(sort);
    await page.getByLabel('Search cards', { exact: true }).fill('Declining');
    await expect(page.getByRole('button', { name: /^Review / })).toHaveCount(1);
    await page.getByLabel('Search cards', { exact: true }).fill('');
    await review('Declining fixture');
    const original = await evidence(1); snapshots.original = original;
    expect(original.evaluation).toMatchObject({ localPriceCents: 320000, listedPriceCents: 350000, status: 'below_target', policyVersion: 'recent-sales-v1', recent: { count: 8, medianCents: 232000, latestSaleCount: 2, latestSaleMinCents: 202500, latestSaleMaxCents: 231500 } });
    expect(original.sales.map(sale => sale.priceCents)).toEqual([231500,202500,309937,242500,220000,233012,222500,232500,...Array(8).fill(400000)]);
    await expect(page.getByLabel('Recent median', { exact: true })).toHaveText('$2,320.00');
    await page.getByLabel('Asking price', { exact: true }).fill('2540');
    await expect(trial().getByText('Mixed evidence', { exact: true })).toBeVisible();
    await page.getByLabel('Asking price', { exact: true }).fill('2400');
    await expect(trial().getByText('Supported', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Saved asking price')).toHaveText('$3,200.00');
    await page.getByRole('checkbox', { name: 'Select Declining fixture', exact: true }).check();
    await review('Supported fixture');
    await page.getByLabel('Asking price', { exact: true }).fill('2300');
    await review('Declining fixture');
    await expect(page.getByLabel('Asking price', { exact: true })).toHaveValue('2400');
    await capture('readonly-trial');
    snapshots.readOnly = await command('read-only-complete');
    expect(snapshots.readOnly.rows).toEqual(before.rows); // Entire ledger/list/hold/evidence rows.
    expect(snapshots.readOnly.dh).toEqual({ wire: [], syncs: {}, lists: {}, syncCalls: 0, listCalls: 0 });
    // Separate, explicitly attributed other-actor write. Draft and selected
    // observed version are retained; neither is silently replaced by fresh data.
    snapshots.background = await command('background');
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await expect(page.getByText(/Saved asking changed from \$3,200.00 to \$3,100.00/)).toBeVisible();
    await expect(page.getByLabel('Asking price', { exact: true })).toHaveValue('2400');
    await expect(page.getByRole('checkbox', { name: 'Select Declining fixture', exact: true })).toBeChecked();
    await expect(bar()).toContainText('Review and reselect');
    await expect(page.getByRole('button', { name: 'Add to show (1)', exact: true })).toBeDisabled();
    // Local save is intentionally NOT a no-write phase. Inventory read failure
    // after the committed PATCH must leave the owner/draft and outcome mounted.
    await command('fail-inventory-read');
    await page.getByRole('button', { name: 'Save price', exact: true }).click();
    await expect(page.getByText(/Price saved; assessment could not be refreshed/)).toBeVisible({ timeout: 15000 });
    await expect(page.getByLabel('Asking price', { exact: true })).toHaveValue('2400');
    await capture('saved-read-failed');
    await command('recover-inventory-read');
    await page.getByRole('button', { name: 'Recheck saved state', exact: true }).click();
    await expect(page.getByLabel('Saved asking price')).toHaveText('$2,400.00');
    await expect(saved().getByText('Supported', { exact: true })).toBeVisible();
    for (const [name, amount] of [['Supported fixture','2300'], ['Mixed fixture','2400']]) {
      await review(name); await page.getByLabel('Asking price', { exact: true }).fill(amount);
      await page.getByRole('button', { name: 'Save price', exact: true }).click();
      await expect(page.getByText(/Price saved locally.*Saved state rechecked/)).toBeVisible();
    }
    snapshots.saved = await command('save-complete');
    expect(snapshots.saved.dh.wire).toHaveLength(3);
    expect(snapshots.saved.dh).toMatchObject({ syncCalls: 3, listCalls: 3 });
    expect(Object.keys(snapshots.saved.dh.syncs)).toHaveLength(3);
    expect(snapshots.saved.dh.lists[id(2)]).toMatchObject({ listed: 1, synced: 1, total: 1 });
    expect(snapshots.saved.rows.showprep_items).toBe(before.rows.showprep_items);
    expect(snapshots.saved.rows.showprep_lists).toBe(before.rows.showprep_lists);
    expect(snapshots.saved.rows.showprep_price_holds).toBe(before.rows.showprep_price_holds);
    const requests = snapshots.saved.requests.filter(r => r.method === 'PATCH');
    expect(requests.map(r => r.path)).toEqual([1,2,3].map(i => `/api/purchases/${id(i)}/review-price`));
    // Explicit list commands: stale observed Add fails; fresh Add and Pack
    // acknowledge the authoritative saved asking, never a trial amount.
    const listID = '22222222-2222-4222-8222-222222222222';
    await json(`${app}/api/show-prep/lists`, 'POST', { id: listID, name: 'Price review packing' });
    await json(`${app}/api/show-prep/lists/${listID}/items`, 'POST', { items: [{ purchaseId: id(1), evaluationVersion: original.evaluation.version }] }, 409);
    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    await page.getByRole('button', { name: 'Inventory', exact: true }).click();
    await page.getByLabel('Search cards', { exact: true }).fill('91000001');
    await page.getByRole('checkbox', { name: 'Select 91000001', exact: true }).check();
    await page.getByRole('button', { name: 'Add to show (1)', exact: true }).click();
    await page.getByRole('combobox', { name: 'Show list', exact: true }).selectOption(listID);
    await page.getByRole('button', { name: 'Add to show (1)', exact: true }).click();
    await page.getByRole('link', { name: 'Open packing list →' }).click();
    const packed = page.getByRole('checkbox', { name: 'Packed 91000001', exact: true });
    await expect(packed).toBeEnabled();
    const added = await json(`${app}/api/show-prep/lists/${listID}`);
    await json(`${app}/api/show-prep/lists/${listID}/items/${added.items[0].id}`, 'PUT', {
      version: added.items[0].version, evaluationVersion: original.evaluation.version, packed: true,
    }, 409);
    expect((await json(`${app}/api/show-prep/lists/${listID}`)).items).toEqual(added.items);
    await packed.focus(); await packed.press('Space'); await expect(packed).toBeChecked();
    await page.reload(); await expect(packed).toBeChecked();
    snapshots.packed = await state();
    const item = JSON.parse(snapshots.packed.rows.showprep_items)[0];
    expect(item.acknowledged_price_cents).toBe(240000);
    expect(item.acknowledged_status).toBe('supported'); expect(item.packed_at).toBeTruthy();
    await capture('packed-authoritative-price');
    // Fresh context has no in-memory drafts. The real deep link opens the
    // saved identity on mobile; Back restores the queue's explicit control.
    const mobile = await browser.newContext({ viewport: { width: 390, height: 844 }, extraHTTPHeaders: headers });
    await mobile.route(url => url.origin !== new URL(app).origin, route => route.abort());
    const desktop = page; page = await mobile.newPage();
    page.on('pageerror', error => errors.push(error.message));
    await page.goto(`${app}/inventory?view=pricing&review=${id(1)}&retained=price-review`);
    await expect(page.getByRole('heading', { name: 'Declining fixture', exact: true })).toBeVisible();
    await expect(page.getByLabel('Saved asking price')).toHaveText('$2,400.00');
    await expect(page.getByLabel('Asking price', { exact: true })).toHaveValue('2400.00');
    await capture('mobile-deep-link');
    await page.getByRole('button', { name: 'Back to inventory list', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Review Declining fixture', exact: true })).toBeFocused();
    expect(page.url()).toContain('retained=price-review'); await capture('mobile-return-queue');
    await mobile.close(); page = desktop;
    snapshots.final = await state();
    for (const [table, rows] of Object.entries(snapshots.saved.rows)) if (!['showprep_items','showprep_lists'].includes(table)) expect(snapshots.final.rows[table], table).toBe(rows);
    expect(snapshots.final.sourceCalls).toBe(0); expect(snapshots.final.providerRequests).toBe(0);
    expect(failures.length).toBeGreaterThan(0);
    expect(failures.every(f => new URL(f.url).pathname === '/api/inventory' && f.status === 503)).toBe(true);
    expect(errors).toEqual([]);
    expect(snapshots.final.requests.filter(r => !['GET','HEAD'].includes(r.method) && !r.path.startsWith('/api/show-prep/') && !r.path.endsWith('/review-price'))).toEqual([]);
    console.log('PASS seven-case cached real-wire price review: whole-row immutable navigation/trials; controlled background+503; three explicit reviewed saves; real DH sync/list results and persisted effects; stale Add/Pack409, explicit packing, mobile deep link/focus; provider acquisition=0');
  }, [
    () => fs.writeFile(path.join(artifacts, 'price-review-wire.json'), JSON.stringify({ wire, snapshots, errors, failures }, null, 2)),
    () => page?.screenshot({ path: path.join(artifacts, 'last-view.png'), fullPage: true }),
  ], () => browser.close());
})().catch(error => { console.error(error); process.exitCode = 1; });
