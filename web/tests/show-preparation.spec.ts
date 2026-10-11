import { expect, test } from '@playwright/test';
import { detail, evaluation, inventoryItem, listId, member, purchaseId } from '../src/react/pages/show-preparation/fixtures.test-support';
import type { ScanCommand, ScanResult } from '../src/types/showprep';

const widths = [{ name: 'mobile', width: 390, height: 844 }, { name: 'tablet', width: 820, height: 1180 }, { name: 'desktop', width: 1440, height: 1000 }];
test.beforeEach(async ({ page, baseURL }) => {
  await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
});
for (const viewport of widths) {
  test(`scan-only confirmation, explicit recovery and history: ${viewport.name}`, async ({ page }, info) => {
    await page.setViewportSize(viewport);
    let saved = detail([]); saved.summary.totalCount = 0; let uncertain = false;
    const commands: ScanCommand[] = []; const receipts = new Map<string, ScanResult>();
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const request = route.request(); const path = new URL(request.url()).pathname; const method = request.method();
      const body = request.postData() ? request.postDataJSON() : {};
      let response: unknown;
      if (path === '/api/auth/user') response = { id: 1, username: 'Show operator', email: 'show@example.test', avatar_url: '', is_admin: false, last_login_at: null };
      else if (path === '/api/show-prep/lists') response = { lists: [saved.list] };
      else if (path.endsWith('/scan-confirm')) {
        commands.push(body);
        const existing = receipts.get(body.commandId);
        if (existing) response = { ...existing, replay: true, current: { state: saved.items.length ? 'present' : 'removed' } };
        else {
          const result: ScanResult = { original: { commandId: body.commandId, listId, certNumber: body.certNumber, outcome: 'confirmed', code: '', itemId: member().id, purchaseId, cardName: evaluation().cardName,
            confirmedAt: '2026-10-09T12:00:00Z', acknowledgedPriceCents: 30000, acknowledgedStatus: 'supported' }, replay: false, current: { state: 'present', evaluation: evaluation({ certNumber: body.certNumber }) } };
          saved = detail([member({ packedAt: result.original.confirmedAt, certNumber: body.certNumber })]); saved.summary.totalCount = 1; saved.summary.packedCount = 1; receipts.set(body.commandId, result); response = result;
        }
        if (uncertain) { uncertain = false; await route.fulfill({ status: 200, json: { partial: true } }); return; }
      } else if (path.includes('/items/') && method === 'DELETE') {
        saved = { ...detail([]), list: { ...saved.list, withdrawalGeneration: saved.list.withdrawalGeneration + 1 } }; saved.summary.totalCount = 0; response = { removed: true };
      } else if (path === `/api/show-prep/lists/${listId}`) response = saved;
      else if (path.includes('/evidence/')) response = { evaluation: saved.items[0]?.evaluation || evaluation(), sales: [] };
      else { await route.fulfill({ status: 404, json: { error: `Unexpected fixture API: ${method} ${path}` } }); return; }
      await route.fulfill({ json: response });
    });
    await page.goto(`/shows?list=${listId}`);
    const scanner = page.getByRole('textbox', { name: 'Scan slab barcode' }); await expect(scanner).toBeEnabled(); await expect(scanner).toBeFocused();
    await scanner.fill('12345678'); await scanner.press('Enter'); await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Confirmed ready asking value')).toContainText('$300.00');
    await expect(page.getByRole('checkbox', { name: /Packed/ })).toHaveCount(0); await expect(page.getByRole('button', { name: /Add.*show|Acknowledge/i })).toHaveCount(0);
    await expect(page.getByRole('heading', { name: evaluation().cardName })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath(`confirmed-${viewport.name}.png`), fullPage: true });
    uncertain = true; await expect(scanner).toBeEnabled(); await scanner.fill('12345678'); await scanner.press('Enter');
    await expect(page.getByRole('button', { name: 'Retry scan 12345678' })).toBeVisible(); const original = commands[1];
    await page.reload(); await expect(page.getByRole('button', { name: 'Recover scan 12345678' })).toBeVisible(); expect(commands).toHaveLength(2);
    await page.getByRole('button', { name: 'Remove 12345678 from show' }).click(); await expect(page.getByRole('article', { name: 'Show member 12345678' })).toHaveCount(0);
    await page.getByRole('button', { name: 'Recover scan 12345678' }).click();
    await expect(page.getByText(/Confirmed previously. Current state: removed/)).toBeVisible(); expect(commands[2]).toEqual(original);
    await expect(page.getByRole('article', { name: 'Show member 12345678' })).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Packing list' }).locator('dl > div').filter({ has: page.getByText('Members', { exact: true }) }).locator('dd')).toHaveText('0');
    await page.screenshot({ path: info.outputPath(`recovered-removed-${viewport.name}.png`), fullPage: true });
  });
}
for (const failed of [true, false]) {
  test(`Inventory financial selection preserves no-price evidence health (failed=${failed})`, async ({ page }, info) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const value = evaluation({ status: 'no_listed_price', reason: 'No positive SlabLedger asking price', localPriceCents: 0, listedPriceCents: 40000,
      evidenceNeedsReview: failed, evidenceReason: failed ? 'CardLadder refresh failed' : '', recent: { ...evaluation().recent, gapPct: null } });
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const path = new URL(route.request().url()).pathname; let response: unknown;
      if (path === '/api/auth/user') response = { id: 1, username: 'Show operator', email: 'show@example.test', avatar_url: '', is_admin: false, last_login_at: null };
      else if (path === '/api/inventory') response = { items: [inventoryItem(value, { reviewedPriceCents: 0, overridePriceCents: 0 })], warnings: [] };
      else if (path === '/api/show-prep/evaluate') response = { evaluations: [value] };
      else if (path.includes('/evidence/')) response = { evaluation: value, sales: [] };
      else { await route.fulfill({ status: 404, json: { error: 'Unexpected fixture API' } }); return; }
      await route.fulfill({ json: response });
    });
    await page.goto('/inventory'); await page.getByRole('checkbox', { name: 'Select 12345678', exact: true }).check();
    const actions = page.getByRole('region', { name: 'Bulk actions for selected cards' });
    await expect(actions.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled(); await expect(actions.getByRole('button', { name: 'List on DH (1)' })).toBeEnabled();
    await expect(actions.getByRole('button', { name: /Add.*show/ })).toHaveCount(0);
    await page.getByRole('link', { name: /Review price 12345678/ }).click();
    const evidence = page.getByRole('region', { name: 'Price details' }); await expect(evidence.getByText('No positive SlabLedger asking price', { exact: true })).toBeVisible();
    await expect(evidence.getByText(/Stored sales may be partial or stale/)).toHaveCount(failed ? 1 : 0);
    await page.screenshot({ path: info.outputPath(`no-price-${failed}.png`), fullPage: true });
  });
}
