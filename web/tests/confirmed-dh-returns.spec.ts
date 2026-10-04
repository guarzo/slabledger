import { expect, test } from '@playwright/test';
import type { ConfirmedReturnState, Purchase } from '../src/types/campaigns';

const cards = [
  { cert: '160944741', inventoryId: 147840, externalSale: 443, grade: 3, name: 'Charizard' },
  { cert: '162787413', inventoryId: 364577, externalSale: 848, grade: 6, name: 'Spheal' },
];
const stamp = '2026-10-03T12:00:00Z';
const market = { lastSoldCents: 4000, gradePriceCents: 4000, clValueCents: 5000 };

function purchase(card: typeof cards[number]): Purchase {
  return {
    id: `10000000-0000-4000-8000-${String(card.externalSale).padStart(12, '0')}`,
    campaignId: '20000000-0000-4000-8000-000000000001', cardName: card.name,
    certNumber: card.cert, grader: 'PSA', gradeValue: card.grade,
    clValueCents: 5000, buyCostCents: 1000, psaSourcingFeeCents: 0,
    purchaseDate: '2026-09-01', receivedAt: stamp, createdAt: stamp, updatedAt: stamp,
    dhInventoryId: card.inventoryId, dhCardId: 100, dhStatus: 'in_stock', dhPushStatus: 'pending',
  };
}

for (const width of [1440, 390]) {
  for (const card of cards) {
    test(`confirmed return ${card.cert} stays separate from List at ${width}`, async ({ page, baseURL }, info) => {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 });
      const p = purchase(card);
      let durable: ConfirmedReturnState = {
        operation: null, expectedSaleId: null, awaitingListing: false,
        precedingAttempt: null, purchase: p, sale: null,
      };
      const mutations: { path: string; body: Record<string, unknown> | null }[] = [];
      const errors: string[] = [];
      page.on('pageerror', error => errors.push(error.message));
      // Browser coverage uses API fixtures; real HTTP/store/DH composition is
      // exercised separately by the Go integration facade. No external traffic.
      await page.route(url => url.origin !== new URL(baseURL!).origin, route => route.abort());
      await page.route(url => url.pathname.startsWith('/api/'), async route => {
        const request = route.request();
        const path = new URL(request.url()).pathname;
        if (request.method() !== 'GET') mutations.push({ path, body: request.postData() ? request.postDataJSON() : null });
        if (path === '/api/auth/user') return route.fulfill({ json: { id: 1, username: 'Operator', email: 'fixture@example.test', is_admin: false } });
        if (path === '/api/admin/psa-sync/pending') return route.fulfill({ json: { items: [] } });
        if (path.endsWith('/scan-cert')) return route.fulfill({ json: {
          status: 'existing', ...durable.purchase, purchaseId: p.id, market,
        } });
        if (path.endsWith('/confirmed-return')) return route.fulfill({ json: durable });
        if (path.endsWith('/confirm-return')) {
          expect(request.postDataJSON()).toEqual({ returnConfirmed: true, expectedSaleId: null });
          durable = { ...durable, awaitingListing: true, outcome: 'completed',
            operation: {
              id: `operation-${card.inventoryId}`, purchaseId: p.id, capturedPurchaseId: p.id,
              dhInventoryId: card.inventoryId, certNumber: card.cert, grader: 'PSA',
              expectedSaleId: null, capturedOrderId: '', returnedOrderId: `ext-${card.externalSale}`,
              state: 'completed', createdAt: stamp, completedAt: stamp,
            },
          };
          return route.fulfill({ json: durable });
        }
        if (path.endsWith('/review-price')) {
          expect(request.postDataJSON()).toEqual({ priceCents: 4000, source: 'market' });
          return route.fulfill({ json: { success: true, reviewedAt: stamp } });
        }
        if (path.endsWith('/list-on-dh')) {
          durable = { ...durable, awaitingListing: false,
            purchase: { ...p, dhStatus: 'listed', dhPushStatus: 'matched' } };
          return route.fulfill({ json: { listed: 1, synced: 2, skipped: 0, total: 1 } });
        }
        return route.fulfill({ status: 400, json: { error: `Unexpected fixture request: ${path}` } });
      });
      await page.goto('/scan');
      const input = page.getByPlaceholder('Scan or type cert number…');
      await expect(page.getByText('No pending items.')).toBeVisible();
      await input.fill(card.cert); await input.press('Enter');
      await page.getByRole('button', { name: 'Confirm DH return', exact: true }).click();
      const dialog = page.getByRole('alertdialog');
      await expect(dialog).toContainText(card.cert);
      await expect(dialog).toContainText('physically back');
      await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused();
      await page.screenshot({ path: info.outputPath(`confirm-${card.cert}-${width}.png`), fullPage: true });
      await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))).toHaveLength(0);
      await page.getByRole('button', { name: 'Confirm DH return', exact: true }).click();
      await dialog.getByRole('button', { name: 'Confirm return', exact: true }).click();
      await expect(page.getByText('Returned; review the price and list explicitly.')).toBeVisible();
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))).toHaveLength(1);
      expect(mutations.filter(r => r.path.endsWith('/list-on-dh'))).toHaveLength(0);
      await page.reload();
      await expect(page.getByText('Returned; review the price and list explicitly.')).toBeVisible();
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))).toHaveLength(1);
      await page.getByRole('button', { name: 'List on DH', exact: true }).click();
      await expect(page.getByText('Returned; review the price and list explicitly.')).toHaveCount(0);
      expect(mutations.filter(r => r.path.endsWith('/list-on-dh'))).toHaveLength(1);
      expect(mutations.some(r => r.path.endsWith('/sale'))).toBe(false);
      expect(errors).toEqual([]);
      await page.screenshot({ path: info.outputPath(`listed-${card.cert}-${width}.png`), fullPage: true });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    });
  }
}
