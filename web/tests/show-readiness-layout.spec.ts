import { expect, test } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
import { evaluation, inventoryItem } from '../src/react/pages/show-preparation/fixtures.test-support';

// The retired support/readiness header has no footprint to pin. Preserve the
// meaningful regression: selection causes no layout jump or collection request,
// and the existing UTC observer updates evidence without clearing financial selection.
for (const width of [1440, 390]) {
  for (const pricing of [false, true]) {
    test(`cached observation and selection without old header at ${width}, pricing=${pricing}`, async ({ page, baseURL }, info) => {
      await page.setViewportSize({ width, height: 1000 });
      let published = false;
      const calls: string[] = [];
      const before = evaluation();
      const changed = evaluation({ status: 'below_target', version: 'published' });
      const item = { ...inventoryItem(before), currentMarket: { lastSoldCents: 30000, gradePriceCents: 30000 } };
      await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
      await page.route(url => url.pathname.startsWith('/api/'), async route => {
        const path = new URL(route.request().url()).pathname; calls.push(path);
        let response: unknown;
        if (path === '/api/auth/user') response = { id: 1, username: 'Operator', email: 'fixture@example.test', is_admin: false };
        else if (path === '/api/inventory') response = { items: [item], warnings: [] };
        else if (path === '/api/show-prep/evaluate') response = { evaluations: [published ? changed : before] };
        else if (path.includes('/evidence/')) response = { evaluation: published ? changed : before, sales: [] };
        else if (path === '/api/show-prep/lists') response = { lists: [] };
        else { await route.fulfill({ status: 400, json: { error: 'Unexpected fixture request' } }); return; }
        await route.fulfill({ json: response });
      });
      await page.clock.install({ time: new Date('2026-09-16T23:59:50Z') });
      await page.goto(`/inventory${pricing ? '?view=pricing' : ''}`);
      const checkbox = page.getByRole('checkbox', { name: pricing ? `Select ${before.cardName}` : 'Select 12345678', exact: true });
      await expect(checkbox).toBeVisible();
      if (!pricing) await expect(page.getByRole('link', { name: /Review price 12345678: Supported/ })).toBeVisible();
      else await expect(page.getByRole('button', { name: /^Supported 1$/ })).toBeVisible();
      await expect(page.getByLabel('Price support', { exact: true })).toHaveCount(0);
      await expect(page.getByLabel('Comp data coverage')).toHaveCount(0);
      const initial = await checkbox.boundingBox(); const requestStart = calls.length;
      await checkbox.check(); await expect(checkbox).toBeChecked();
      const selected = await checkbox.boundingBox(); expect(selected).toEqual(initial);
      expect(calls.slice(requestStart)).toEqual([]);
      // Cross the next UTC date without dispatching a synthetic application event.
      published = true; await page.clock.fastForward(11_000);
      if (!pricing) await expect(page.getByRole('link', { name: /Review price 12345678: Asking above comps/ })).toBeVisible();
      else await expect(page.getByRole('button', { name: /^Above comps 1$/ })).toBeVisible();
      await expect(page.getByRole('button', { name: /Add.*show/ })).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled();
      await expect(page.getByRole('button', { name: 'List on DH (1)' })).toBeEnabled();
      await expect(checkbox).toBeChecked();
      expect(calls.slice(requestStart).some(path => path.endsWith('/evaluate'))).toBe(true);
      expect(calls.some(path => /refresh|coverage|review-price|override/.test(path))).toBe(false);
      await writeFile(info.outputPath('selection-observation.json'), JSON.stringify({ initial, selected, after: await checkbox.boundingBox(), calls }, null, 2));
      // Financial selection remains explicit and usable after evidence publication.
      await checkbox.uncheck(); await checkbox.check();
      await expect(page.getByText(/Review and reselect/)).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Record sale (1)' })).toBeEnabled();
      await page.screenshot({ path: info.outputPath(`selection-${width}-${pricing}.png`), fullPage: true });
    });
  }
}
