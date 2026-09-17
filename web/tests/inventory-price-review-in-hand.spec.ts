import { expect, test } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
import { evaluation, inventoryItem, otherId, preview, purchaseId, sales } from '../src/react/pages/price-review/fixtures.test-support';

for (const width of [1440, 390]) {
  test(`in-hand-only review preserves intake, shared selections and revoked-intake drafts at ${width}`, async ({ page, baseURL }, info) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
    await page.clock.install();
    const failedId = '33333333-3333-4333-8333-333333333333';
    const values = [evaluation(), evaluation({ purchaseId: otherId, cardName: 'Moon Tortoise', certNumber: '00000002' }),
      evaluation({ purchaseId: failedId, cardName: 'Cloud Fox', certNumber: '00000003', status: 'needs_review',
        evidenceNeedsReview: true, evidenceReason: 'Stored source failed', canPack: false })];
    const unreceived = new Set([otherId]);
    const requests: { method: string; path: string }[] = [];
    const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
    await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const request = route.request(); const path = new URL(request.url()).pathname;
      requests.push({ method: request.method(), path });
      if (path === '/api/auth/user') return route.fulfill({ json: { id: 1, username: 'Operator', is_admin: false } });
      if (path === '/api/inventory') return route.fulfill({ json: {
        items: values.map(e => inventoryItem(e, unreceived.has(e.purchaseId) ? { receivedAt: '' } : {})), warnings: [],
      } });
      // Deliberately retain a healthy, ready assessment for the unreceived card.
      if (path.endsWith('/evaluate')) return route.fulfill({ json: { evaluations: values } });
      if (path.includes('/evidence/')) return route.fulfill({ json: { evaluation: values.find(e => path.endsWith(e.purchaseId)), sales } });
      if (path.endsWith('/preview')) {
        const body = request.postDataJSON();
        return route.fulfill({ json: preview(body.priceCents, values.find(e => e.purchaseId === body.purchaseId)) });
      }
      if (path.endsWith('/lists')) return route.fulfill({ json: { lists: [] } });
      return route.fulfill({ status: 400, json: { error: `Unexpected fixture request: ${path}` } });
    });
    const details = page.getByRole('region', { name: 'Price details', exact: true });
    const input = page.getByRole('textbox', { name: 'Asking price', exact: true });
    const queue = page.getByRole('list', { name: 'Price review queue' });
    const back = page.getByRole('button', { name: 'Back to inventory list', exact: true });
    await page.goto(`/inventory?view=pricing&review=${otherId}&keep=intake`);
    await expect(details.getByText(/Price review is for in-hand inventory only/)).toBeVisible();
    await expect(input).toHaveCount(0);
    expect(requests.some(r => /evidence\/|preview/.test(r.path))).toBe(false);
    await page.getByRole('button', { name: 'Inventory', exact: true }).click();
    await page.getByRole('button', { name: /^All\s*3$/ }).click();
    await expect(page.getByTitle('Awaiting intake', { exact: true })).toBeVisible();
    await page.getByRole('checkbox', { name: 'Select 00000002', exact: true }).check();
    await page.getByRole('button', { name: 'Price review', exact: true }).click();
    await expect(page.getByRole('button', { name: 'All 2', exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Unavailable 1', exact: true })).toBeVisible();
    await expect(queue.getByRole('button', { name: 'Review Moon Tortoise' })).toHaveCount(0);
    await page.getByRole('checkbox', { name: 'Select all visible cards' }).check();
    await page.getByRole('button', { name: 'Supported 0', exact: true }).click();
    await page.getByRole('button', { name: 'Reveal selected', exact: true }).click();
    await expect(queue.getByRole('button')).toHaveCount(2);
    await expect(queue.getByRole('button', { name: 'Review Moon Tortoise' })).toHaveCount(0);
    await page.getByLabel('Search cards', { exact: true }).fill('Moon');
    await expect(page.getByRole('button', { name: 'All 0', exact: true })).toBeVisible();
    await expect(page.getByText(/No in-hand cards match/)).toBeVisible();
    await page.getByLabel('Search cards', { exact: true }).fill('');
    await page.getByRole('button', { name: 'All 2', exact: true }).click();
    for (const sort of ['asking', 'recent', 'gap', 'supported', 'attention']) {
      await page.getByLabel('Price review sort').selectOption(sort);
      await expect(queue.getByRole('button')).toHaveCount(2);
    }
    await queue.getByRole('button', { name: 'Review Aurora Dragon' }).click();
    await input.fill('2400');
    await expect(page.getByRole('region', { name: 'Trial price assessment' })).toContainText('Supported');
    async function refreshInventory() {
      // Global inventory disables focus refetch; a stale-query reconnect is its
      // existing background read path. Do not remount the draft owner.
      await page.clock.fastForward(120001);
      await page.context().setOffline(true);
      const read = page.waitForResponse(response => response.url().endsWith('/api/inventory'));
      await page.context().setOffline(false);
      await read;
    }
    unreceived.add(purchaseId); await refreshInventory();
    await expect(input).toHaveCount(0);
    await expect(details.getByText(/Price review is for in-hand inventory only/)).toBeVisible();
    await expect(page.getByRole('button', { name: 'All 1', exact: true, includeHidden: true })).toHaveCount(1);
    await expect(page.getByRole('button', { name: 'Review Aurora Dragon', includeHidden: true })).toHaveCount(0);
    await page.screenshot({ path: info.outputPath(`intake-revoked-${width}.png`), fullPage: true });
    unreceived.delete(purchaseId); await refreshInventory();
    await expect(input).toHaveValue('2400');
    if (width === 390) { await back.click(); await expect(queue.getByRole('button', { name: 'Review Aurora Dragon' })).toBeFocused(); }
    await page.getByRole('button', { name: 'Inventory', exact: true }).click();
    await expect(page.getByRole('checkbox', { name: 'Select 00000002', exact: true })).toBeChecked();
    await expect(page.getByRole('checkbox', { name: 'Select 00000001', exact: true })).toBeChecked();
    unreceived.add(purchaseId); unreceived.add(failedId);
    await page.goto(`/inventory?view=pricing&review=${otherId}`);
    await expect(details.getByText(/Price review is for in-hand inventory only/)).toBeVisible();
    if (width === 390) await back.click();
    await expect(page.getByText(/No in-hand inventory to review/)).toBeVisible();
    await expect(input).toHaveCount(0);
    expect(requests.some(r => /review-price|override|refresh|list-on-dh/.test(r.path))).toBe(false);
    expect(requests.some(r => r.path.includes(`/evidence/${otherId}`))).toBe(false);
    expect(errors).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    await writeFile(info.outputPath('in-hand-requests.json'), JSON.stringify({ requests, errors }, null, 2));
  });
}
