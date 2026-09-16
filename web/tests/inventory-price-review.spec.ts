import { expect, test } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
import { evaluation, inventoryItem, otherId, preview, purchaseId, sales } from '../src/react/pages/price-review/fixtures.test-support';

for (const width of [1440, 390]) {
  test(`normal/review integration and read-only save recovery at ${width}`, async ({ page, baseURL }, info) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
    let values = [evaluation(), evaluation({ purchaseId: otherId, cardName: 'Moon Tortoise', certNumber: '00000002', version: 'saved-b' })];
    const requests: { method: string; path: string; body: Record<string, unknown> }[] = [];
    const pageErrors: string[] = []; page.on('pageerror', error => pageErrors.push(error.message));
    let failInventory = false;
    let releaseInventory: (() => void) | undefined;
    let holdRead = false;
    await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const request = route.request(); const path = new URL(request.url()).pathname; const method = request.method();
      const body = method === 'GET' ? {} : request.postDataJSON(); requests.push({ method, path, body });
      if (path === '/api/auth/user') return route.fulfill({ json: { id: 1, username: 'Operator', email: 'fixture@example.test', is_admin: false } });
      if (path === '/api/inventory') {
        if (holdRead) { holdRead = false; await new Promise<void>(resolve => { releaseInventory = resolve; }); }
        return route.fulfill({ status: failInventory ? 400 : 200, json: failInventory ? { error: 'Inventory read unavailable' }
          : { items: values.map(e => inventoryItem(e, { dhPushStatus: 'matched' })), warnings: [] } });
      }
      if (path.endsWith('/evaluate')) return route.fulfill({ json: { evaluations: values } });
      if (path.includes('/evidence/')) return route.fulfill({ json: { evaluation: values.find(e => path.endsWith(e.purchaseId)), sales } });
      if (path.endsWith('/preview')) return route.fulfill({ json: preview(body.priceCents, values.find(e => e.purchaseId === body.purchaseId)) });
      if (path.endsWith('/review-price') && method === 'PATCH') {
        values = values.map(e => path.includes(e.purchaseId) ? { ...e, localPriceCents: body.priceCents, version: 'saved-new', status: 'supported',
          reason: 'Recent sales support this saved asking price.', recent: { ...e.recent, gapPct: 0 } } : e);
        return route.fulfill({ json: { success: true, reviewedAt: '2026-09-16T12:00:00Z' } });
      }
      if (path.endsWith('/lists')) return route.fulfill({ json: { lists: [] } });
      return route.fulfill({ status: 400, json: { error: `Unexpected fixture request: ${method} ${path}` } });
    });
    await page.goto('/inventory?keep=fixture');
    await expect(page.getByRole('link', { name: /Review price 00000001/ })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Sell', exact: true })).toHaveCount(2);
    await expect(page.getByLabel('Price support', { exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: 'More actions', exact: true }).first().click();
    await expect(page.getByRole('menuitem', { name: 'Fix DH Match', exact: true })).toBeVisible();
    await expect(page.getByRole('menuitem', { name: 'Remove DH Match', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: 'Sell', exact: true }).first().click();
    await expect(page.getByRole('button', { name: 'Cancel', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.getByRole('checkbox', { name: 'Select 00000001', exact: true }).check();
    await expect(page.getByRole('button', { name: 'List on DH (1)' })).toBeEnabled();
    await expect(page.getByRole('region', { name: 'Bulk actions for selected cards' })).toHaveCSS('position', 'fixed');
    await page.screenshot({ path: info.outputPath(`normal-${width}.png`), fullPage: true });
    await page.getByRole('link', { name: /Review price 00000002/ }).click();
    await expect(page).toHaveURL(new RegExp(`keep=fixture&view=pricing&review=${otherId}`));
    const input = page.getByRole('textbox', { name: 'Asking price', exact: true });
    async function openCard(name: string) {
      if (width === 390) await page.getByRole('button', { name: 'Back to inventory list', exact: true }).click();
      await page.getByRole('button', { name: `Review ${name}`, exact: true }).click();
    }
    await expect(input).toHaveValue('2800.00');
    if (width === 390) {
      const back = await page.getByRole('button', { name: 'Back to inventory list', exact: true }).boundingBox();
      const banner = await page.getByRole('banner').boundingBox();
      expect(back!.y).toBeGreaterThanOrEqual(banner!.y + banner!.height);
    }
    await input.fill('2500');
    await page.getByRole('button', { name: 'Previous card' }).click(); await expect(input).toHaveValue('2800.00'); await input.fill('2400');
    await expect(page.getByRole('region', { name: 'Trial price assessment' })).toContainText('Supported');
    await page.screenshot({ path: info.outputPath(`review-${width}.png`), fullPage: true });
    failInventory = true; holdRead = true;
    await page.getByRole('button', { name: 'Save price', exact: true }).click();
    await expect.poll(() => !!releaseInventory).toBe(true);
    await openCard('Moon Tortoise'); await expect(input).toHaveValue('2500'); await input.focus();
    releaseInventory!();
    await expect(page.getByText(/Inventory could not be refreshed/)).toBeVisible();
    await expect(input).toBeFocused(); await expect(input).toHaveValue('2500');
    await openCard('Aurora Dragon');
    await expect(page.getByText(/Price saved; assessment could not be refreshed/)).toBeVisible();
    await expect(input).toHaveValue('2400');
    await expect(page.getByRole('region', { name: 'Saved price assessment' })).toContainText('Assessment pending refresh');
    await expect(page.getByRole('button', { name: 'Save price', exact: true })).toBeDisabled();
    await page.screenshot({ path: info.outputPath(`failed-read-${width}.png`), fullPage: true });
    await page.getByRole('button', { name: 'Inventory', exact: true }).click();
    await expect(page.getByRole('checkbox', { name: 'Select 00000001', exact: true })).toBeChecked();
    await page.getByRole('button', { name: 'Price review', exact: true }).click();
    await page.getByRole('button', { name: 'Review Aurora Dragon', exact: true }).click();
    await expect(input).toHaveValue('2400'); await expect(page.getByText(/Price saved; assessment could not be refreshed/)).toBeVisible();
    const readStart = requests.length; failInventory = false;
    await page.getByRole('button', { name: 'Recheck saved state', exact: true }).click();
    await expect(page.getByText(/Saved state rechecked/)).toBeVisible(); await expect(input).toHaveValue('2400.00');
    await expect(page.getByText(/Inventory could not be refreshed/)).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Saved price assessment' }).getByText('Supported', { exact: true })).toBeVisible();
    expect(requests.slice(readStart).every(r => r.method === 'GET' || r.path.endsWith('/evaluate'))).toBe(true);
    expect(requests.filter(r => r.method === 'PATCH')).toEqual([{ method: 'PATCH', path: `/api/purchases/${purchaseId}/review-price`, body: { priceCents: 240000, source: 'manual' } }]);
    await openCard('Moon Tortoise'); await expect(input).toHaveValue('2500');
    if (width === 390) {
      await page.getByRole('button', { name: 'Back to inventory list', exact: true }).focus(); await page.keyboard.press('Enter');
      await expect(page.getByRole('button', { name: 'Review Moon Tortoise', exact: true })).toBeFocused();
      await expect(page.getByRole('checkbox', { name: 'Select Aurora Dragon', exact: true })).toBeChecked();
      await expect(page.getByRole('region', { name: 'Price details' })).toBeHidden();
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    expect(pageErrors).toEqual([]);
    expect(requests.some(r => /refresh|override|list-on-dh/.test(r.path))).toBe(false);
    await page.screenshot({ path: info.outputPath(`recovered-${width}.png`), fullPage: true });
    await writeFile(info.outputPath('http-log.json'), JSON.stringify({ requests, pageErrors }, null, 2));
  });
}
