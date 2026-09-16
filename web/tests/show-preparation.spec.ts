import { expect, test } from '@playwright/test';
import { detail, evaluation, inventoryItem, listId, member, purchaseId } from '../src/react/pages/show-preparation/fixtures.test-support';
import type { ShowListDetail, ShowListItem } from '../src/types/showprep';

test.beforeEach(async ({ page, baseURL }) => {
  // Local fixture exercise only, including third-party fonts and thumbnails.
  await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
});

const planningId = '44444444-4444-4444-8444-444444444444';
const ambiguousId = '55555555-5555-4555-8555-555555555555';
const widths = [{ name: 'mobile', width: 390, height: 844 }, { name: 'tablet', width: 820, height: 1180 }, { name: 'desktop', width: 1440, height: 1000 }];
const recent = { ...evaluation().recent, saleIds: ['sale-1', 'sale-2'], latestSaleCount: 1, latestSaleMinCents: 27000, latestSaleMaxCents: 27000 };

for (const failed of [true, false]) {
  test(`no asking price preserves independent stored evidence health (failed=${failed})`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const current = evaluation({ status: 'no_listed_price', reason: 'No positive SlabLedger asking price', localPriceCents: 0, listedPriceCents: 40000,
      recent: { ...recent, gapPct: null }, evidenceNeedsReview: failed, evidenceReason: failed ? 'CardLadder refresh failed' : '' });
    let refreshes = 0;
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const path = new URL(route.request().url()).pathname;
      let response: unknown;
      if (path === '/api/auth/user') response = { id: 1, username: 'Show operator', email: 'show@example.test', avatar_url: '', is_admin: false, last_login_at: null };
      else if (path === '/api/inventory') response = { items: [inventoryItem(current, { reviewedPriceCents: 0, overridePriceCents: 0 })], warnings: [] };
      else if (path === '/api/show-prep/lists') response = { lists: [] };
      else if (path === '/api/show-prep/evaluate') response = { evaluations: [current] };
      else if (path === '/api/show-prep/refresh') {
        refreshes++;
        await route.fulfill({ status: 410, json: { error: 'retired' } }); return;
      } else if (path === `/api/show-prep/evidence/${purchaseId}`) response = { evaluation: current, sales: [
        { id: 'sale-1', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: 'https://example.test/sale/1', listingType: 'Auction' },
        { id: 'sale-2', date: '2026-09-12', priceCents: 29000, platform: 'eBay', url: 'https://example.test/sale/2', listingType: 'BestOffer' },
      ] };
      else { await route.fulfill({ status: 404, json: { error: 'Unexpected intercepted API' } }); return; }
      await route.fulfill({ json: response });
    });
    await page.goto('/inventory');
    await expect(page.getByRole('button', { name: /Show 30-day evidence 12345678: No asking price/ })).toBeVisible();
    await page.getByRole('checkbox', { name: 'Select 12345678', exact: true }).check();
    const actions = page.getByRole('region', { name: 'Bulk actions for selected cards' });
    await expect(actions.getByRole('button', { name: 'Add to show (1)' })).toBeEnabled();
    await page.getByRole('button', { name: 'Show 30-day evidence 12345678' }).click();
    const evidence = page.getByRole('region', { name: '30-day evidence 12345678' });
    await expect(page.getByText('No asking price', { exact: true }).last()).toBeVisible();
    await expect(evidence.getByText('No positive SlabLedger asking price', { exact: true })).toBeVisible();
    await expect(evidence.getByRole('list', { name: 'Individual matching sales' }).getByRole('listitem')).toHaveCount(2);
    if (failed) {
      await expect(evidence.getByText('CardLadder refresh failed', { exact: true })).toBeVisible();
      await expect(evidence.getByText(/Stored sales may be partial or stale/)).toBeVisible();
    } else await expect(evidence.getByText(/Stored sales may be partial or stale/)).toHaveCount(0);
    expect(refreshes).toBe(0);
    await page.screenshot({ path: testInfo.outputPath(`no-price-health-${failed ? 'failed' : 'complete'}.png`), fullPage: true });
  });
}

for (const viewport of widths) {
  test(`show preparation workflow and retained warnings: ${viewport.name}`, async ({ page }, testInfo) => {
    await page.setViewportSize(viewport);
    const values = [evaluation({ listedPriceCents: 40000, priceMismatch: true, recent }),
      evaluation({ purchaseId: planningId, cardName: 'Charizard ex Special Art Rare Japanese Edition, not yet received', certNumber: '87654321', availability: 'not_received', canPack: false, status: 'thin_evidence', compCount: 1, medianCents: 27000,
        recent: { ...recent, saleIds: ['sale-1'], count: 1, medianCents: 27000, gapPct: 10 } }),
      evaluation({ purchaseId: ambiguousId, cardName: 'Umbreon VMAX Alternate Art, duplicate certification number across graders', certNumber: '99999999', listedPriceCents: 90000, priceMismatch: true, priceAssociationUnclear: true, recent })];
    const inventory = values.map(e => inventoryItem(e, e.availability === 'not_received' ? { receivedAt: '', dhStatus: '' } : {}));
    let saved: ShowListDetail = detail([]);
    let lists = [saved.list];
    const writes: { path: string; body: Record<string, unknown> }[] = [];
    function totals() {
      saved.summary = { totalCount: saved.items.length, packedCount: saved.items.filter(i => i.packedAt).length,
        notReceivedCount: saved.items.filter(i => i.evaluation.availability === 'not_received').length,
        unavailableCount: saved.items.filter(i => !['ready', 'not_received'].includes(i.evaluation.availability)).length,
        knownValueCents: saved.items.reduce((sum, i) => sum + (i.evaluation.availability === 'ready' && i.evaluation.localPriceCents > 0 ? i.evaluation.localPriceCents : 0), 0),
        missingPriceCount: saved.items.filter(i => i.evaluation.localPriceCents <= 0).length, ambiguousPriceCount: saved.items.filter(i => i.evaluation.priceAssociationUnclear).length };
    }
    // Every API is intercepted before navigation. Unknown writes fail; none can reach a live server.
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      const method = request.method();
      const body = method === 'GET' || method === 'DELETE' ? {} : request.postDataJSON();
      let response: unknown; let status = 200;
      if (method !== 'GET') writes.push({ path, body });
      if (path === '/api/auth/user') response = { id: 1, username: 'Show operator', email: 'show@example.test', avatar_url: '', is_admin: false, last_login_at: null };
      else if (path === '/api/inventory') response = { items: inventory, warnings: [] };
      else if (path === '/api/show-prep/evaluate') response = { evaluations: values.filter(e => body.purchaseIds.includes(e.purchaseId)) };
      else if (path === '/api/show-prep/refresh') { status = 410; response = { error: 'retired' }; }
      else if (path.startsWith('/api/show-prep/evidence/')) {
        const e = values.find(e => path.endsWith(e.purchaseId))!;
        response = { evaluation: e, sales: [
          { id: 'sale-1', date: '2026-09-13', priceCents: 27000, platform: 'eBay', url: 'https://example.test/sale/1', listingType: 'Auction' },
          { id: 'sale-2', date: '2026-09-12', priceCents: 29000, platform: 'eBay', url: 'javascript:alert(1)', listingType: 'Fixed price' },
        ].slice(0, e.compCount) };
      } else if (path === '/api/show-prep/lists') {
        if (method === 'POST') { saved = { ...saved, list: { ...saved.list, id: body.id, name: body.name } }; lists = [...lists, saved.list]; response = saved.list; }
        else response = { lists };
      } else if (path === `/api/show-prep/lists/${saved.list.id}`) {
        if (method === 'PUT') { saved.list.name = body.name; response = saved.list; }
        else { totals(); response = saved; }
      } else if (path === `/api/show-prep/lists/${saved.list.id}/items`) {
        for (const input of body.items) {
          if (saved.items.some(i => i.purchaseId === input.purchaseId)) continue;
          const e = values.find(e => e.purchaseId === input.purchaseId)!;
          expect(input.evaluationVersion).toBe(e.version);
          saved.items.push(member({ id: e.purchaseId, purchaseId: e.purchaseId, cardName: e.cardName, certNumber: e.certNumber, acknowledgedPriceCents: e.localPriceCents, acknowledgedStatus: e.status, evaluation: { ...e } }));
        }
        totals(); response = saved;
      } else if (path.startsWith(`/api/show-prep/lists/${saved.list.id}/items/`)) {
        const item = saved.items.find(i => path.endsWith(i.id))!;
        if (method === 'DELETE') { saved.items = saved.items.filter(i => i !== item); response = { removed: true }; }
        else {
          expect(body.version).toBe(item.version); expect(body.evaluationVersion).toBe(item.evaluation.version);
          if (body.packed !== undefined) item.packedAt = body.packed ? '2026-09-14T11:00:00Z' : '';
          if (body.acknowledge || body.packed) {
            item.acknowledgedPriceCents = item.evaluation.localPriceCents; item.acknowledgedStatus = item.evaluation.status;
            item.priceChanged = false; item.supportChanged = false;
          }
          item.version++; totals(); response = saved;
        }
      } else { status = 404; response = { error: `Unexpected intercepted API: ${method} ${path}` }; }
      await route.fulfill({ status, json: response });
    });
    await page.goto('/inventory');
    await expect(page.getByRole('heading', { name: 'Inventory', exact: true })).toBeVisible();
    await page.getByLabel('Price support', { exact: true }).selectOption('supported');
    await page.getByLabel('Search cards').fill('Charizard');
    await expect(page.getByText('0 cards shown', { exact: true })).toBeVisible();
    await page.getByLabel('Search cards').fill('Pikachu');
    await page.getByRole('button', { name: /^DH Listed/ }).click();
    await expect(page.getByText('1 card shown', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Show 30-day evidence 12345678' }).click();
    const evidence = page.getByRole('region', { name: '30-day evidence 12345678' });
    await expect(evidence.getByText('$270.00', { exact: true })).toBeVisible();
    await expect(evidence.getByRole('link')).toHaveCount(1);
    // A measured row can unmount on scroll or breakpoint changes; disclosure
    // intent must survive that remount rather than silently closing evidence.
    await page.setViewportSize({ width: viewport.width === 390 ? 1440 : 390, height: 844 });
    await expect(evidence.getByText('$270.00', { exact: true })).toBeVisible();
    await page.setViewportSize(viewport);
    await page.evaluate(() => window.scrollTo(0, 0));
    await expect(evidence.getByText('$270.00', { exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`inventory-${viewport.name}.png`), fullPage: true });
    await expect(evidence.getByText('$270.00', { exact: true })).toBeVisible();
    await page.getByRole('checkbox', { name: 'Select all visible cards' }).check();
    await page.getByRole('button', { name: 'Add to show (1)' }).click();
    await page.getByRole('combobox', { name: 'Show list', exact: true }).selectOption(listId);
    await page.getByRole('button', { name: 'Add to show (1)' }).click();
    await expect(page.getByRole('region', { name: 'Bulk actions for selected cards' })).toHaveCount(0);
    await page.getByLabel('Price support', { exact: true }).selectOption('all');
    await page.getByLabel('Search cards').fill('');
    await page.getByRole('button', { name: /^All\s*\d+$/ }).click();
    await page.getByRole('checkbox', { name: 'Select 87654321', exact: true }).check();
    await page.getByRole('checkbox', { name: 'Select 99999999', exact: true }).check();
    await page.getByRole('button', { name: 'Add to show (2)' }).click();
    await expect(page.getByRole('combobox', { name: 'Show list', exact: true })).toHaveValue(listId);
    await page.getByRole('button', { name: 'Add to show (2)' }).click();
    await expect(page.getByRole('region', { name: 'Bulk actions for selected cards' })).toHaveCount(0);
    await page.getByRole('link', { name: 'Open packing list →' }).click();
    await expect(page).toHaveURL(new RegExp(`/shows\\?list=${listId}`));
    const packed = page.getByRole('checkbox', { name: 'Packed 12345678', exact: true });
    await expect(packed).not.toBeChecked();
    await packed.focus();
    await packed.press('Space');
    await expect(packed).toBeChecked();
    await page.reload();
    await expect(packed).toBeChecked();
    await expect(page.getByRole('checkbox', { name: 'Packed 87654321' })).toBeDisabled();
    await expect(page.getByRole('checkbox', { name: 'Packed 99999999' })).toBeEnabled();
    await expect(page.getByLabel('Known ready-to-pack asking value')).toContainText('$600.00');
    await expect(page.getByLabel('DH association warnings')).toContainText('1');
    await expect(page.getByLabel('Missing asking prices')).toContainText('0');
    await expect(page.getByRole('heading', { name: values[0].cardName })).toBeVisible();
    await expect(page.locator('body')).not.toContainText('NaN');
    await expect(page.getByRole('button', { name: /record sale|reprice|delist/i })).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await page.evaluate(() => window.scrollTo(0, 0));
    await expect(page.getByRole('heading', { name: 'Shows', exact: true })).toBeInViewport();
    await page.screenshot({ path: testInfo.outputPath(`packing-${viewport.name}.png`), fullPage: true });
    const changed: ShowListItem = saved.items[0];
    changed.evaluation = evaluation({ localPriceCents: 35000, listedPriceCents: 40000, priceMismatch: true, recent: { ...recent, gapPct: 20 },
      availability: 'refunded', canAdd: false, canPack: false, status: 'below_target', version: 'eval-2' });
    changed.priceChanged = true; changed.supportChanged = true;
    await page.getByRole('button', { name: 'Update list status' }).click();
    await expect(page.getByText(/Price changed: check the physical sticker/)).toBeVisible();
    await expect(page.getByText('Unavailable: refunded', { exact: true })).toBeVisible();
    await expect(packed).toBeChecked();
    await expect(page.getByLabel('Known ready-to-pack asking value')).toContainText('$300.00');
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({ path: testInfo.outputPath(`changed-${viewport.name}.png`), fullPage: true });
    await page.getByRole('button', { name: 'Acknowledge changes 12345678' }).click();
    await expect(page.getByText(/Price changed: check the physical sticker/)).toHaveCount(0);
    await packed.click();
    await expect(packed).not.toBeChecked(); await expect(packed).toBeDisabled();
    await page.getByRole('button', { name: 'Remove 12345678 from show' }).click();
    await expect(packed).toHaveCount(0);
    expect(writes.every(w => w.path.startsWith('/api/show-prep/'))).toBe(true);
    expect(writes.some(w => w.path.endsWith('/refresh'))).toBe(false);
    const added = writes.filter(w => w.path.endsWith('/items')).flatMap(w => w.body.items as { purchaseId: string }[]);
    expect(added.map(i => i.purchaseId)).toEqual([purchaseId, planningId, ambiguousId]);
  });
}
