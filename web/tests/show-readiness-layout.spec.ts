import { expect, test } from '@playwright/test';
import { writeFile } from 'node:fs/promises';
import { evaluation, inventoryItem } from '../src/react/pages/show-preparation/fixtures.test-support';

for (const width of [1440, 390]) {
  for (const notice of ['coverage', 'attention', 'quiet'] as const) {
    for (const reset of ['clear', 'view'] as const) {
      test(`${notice} footprint at ${width}, reset by ${reset}`, async ({ page, baseURL }, info) => {
        await page.setViewportSize({ width, height: 1000 });
        // Only these API fixtures are replaced. All React, layout, query timers,
        // filtering, selection and observed-version fencing are production code.
        let published = false;
        const calls: string[] = [];
        const before = evaluation();
        const item = { ...inventoryItem(before), currentMarket: { lastSoldCents: 30000, gradePriceCents: 30000 } };
        await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
        await page.route(url => url.pathname.startsWith('/api/'), async route => {
          const path = new URL(route.request().url()).pathname;
          calls.push(path);
          let response: unknown;
          if (path === '/api/auth/user') response = { id: 1, username: 'Operator', email: 'fixture@example.test', is_admin: false };
          else if (path === '/api/inventory') response = { items: [item], warnings: [] };
          else if (path === '/api/show-prep/evaluate') response = { evaluations: [published ? evaluation({ status: 'below_target', version: 'published' }) : before] };
          else if (path === '/api/show-prep/coverage') response = {
            enabled: true, configured: true, state: 'idle', eligibleIdentities: 1,
            currentIdentities: notice === 'coverage' && !published ? 0 : 1,
            missingIdentities: notice === 'coverage' && !published ? 1 : 0,
            staleIdentities: 0, failedIdentities: 0, eligibleCards: 1,
            currentCards: notice === 'coverage' && !published ? 0 : 1,
            unresolvedCards: 0, lastSweepAt: '', retryAt: '', error: '',
          };
          else if (path === '/api/show-prep/lists') response = { lists: [] };
          else { await route.fulfill({ status: 500, json: { error: 'Unexpected fixture request' } }); return; }
          await route.fulfill({ json: response });
        });
        await page.clock.install();
        await page.goto('/inventory');
        const checkbox = page.getByRole('checkbox', { name: 'Select 12345678', exact: true });
        await expect(checkbox).toBeVisible();
        await page.getByRole('button', { name: /^All\s*\d+$/ }).click();
        if (notice !== 'attention') {
          await page.getByLabel('Search cards').fill('12345678');
          await expect(page.getByText('1 of 1 cards', { exact: true })).toBeVisible();
        }
        await page.getByLabel('Price support', { exact: true }).selectOption('supported');
        await expect(page.getByText('1 card shown', { exact: true })).toBeVisible();
        const coverage = page.getByLabel('Comp data coverage');
        const attention = page.getByRole('button', { name: /card.*need.*attention.*Review/ });
        await expect(coverage).toHaveCount(notice === 'coverage' ? 1 : 0);
        await expect(attention).toHaveCount(notice === 'attention' ? 1 : 0);
        // Fast-forward the installed production minute poll, not a new test poll.
        const initial = await checkbox.boundingBox();
        const tableStart = page.getByRole('checkbox', { name: 'Select all visible cards' });
        const initialTable = await tableStart.boundingBox();
        const requestStart = calls.length;
        await checkbox.check();
        await expect(checkbox).toBeChecked();
        const selected = await checkbox.boundingBox();
        expect(selected).toEqual(initial); // no checkbox-induced header row
        expect(calls.slice(requestStart)).toEqual([]);
        published = true;
        await page.clock.fastForward(60_001);
        await page.evaluate(() => window.dispatchEvent(new Event('focus')));
        await expect(page.getByText(/Review and reselect/)).toBeVisible();
        await expect(page.getByRole('button', { name: 'Add to show (1)' })).toBeDisabled();
        await expect(checkbox).toBeChecked();
        const after = await checkbox.boundingBox();
        const beforeReset = { initial, selected, published: after, initialTable, publishedTable: await tableStart.boundingBox(), calls: [...calls] };
        await writeFile(info.outputPath('geometry-before-reset.json'), JSON.stringify(beforeReset, null, 2));
        expect(after).toEqual(selected);
        if (notice === 'coverage') {
          await expect(coverage).toContainText('1/1 identities current');
          await expect(coverage).toContainText('0 missing');
        }
        if (notice === 'attention') await expect(attention).toContainText('0 cards need attention');
        await expect(page.getByRole('button', { name: /\$250–500/ })).toContainText('0');
        const resetStart = calls.length;
        if (reset === 'clear') await page.getByRole('region', { name: 'Bulk actions for selected cards' }).getByRole('button', { name: 'Clear', exact: true }).click();
        else await page.getByRole('button', { name: /\$250–500/ }).click();
        await expect(coverage).toHaveCount(0);
        await expect(attention).toHaveCount(0);
        if (reset === 'clear') {
          await expect(page.getByText('No current matches under these filters.', { exact: true })).toBeVisible();
          await expect(page.getByRole('button', { name: /\$250–500/ })).toHaveCount(0);
        } else {
          await expect(checkbox).toHaveCount(0);
          await expect(page.getByText('1 selected outside this view')).toBeVisible();
        }
        expect(calls.slice(resetStart)).toEqual([]);
        expect(calls.filter(path => path.endsWith('/refresh'))).toEqual([]);
        const resetTable = await tableStart.boundingBox();
        if (notice !== 'quiet') expect(resetTable!.y).toBeLessThan(initialTable!.y);
        await writeFile(info.outputPath('geometry-reset.json'), JSON.stringify({ table: resetTable, checkbox: await checkbox.count() ? await checkbox.boundingBox() : null, coverage: await coverage.count(), attention: await attention.count(), requests: calls.slice(resetStart) }, null, 2));
      });
    }
  }
}
