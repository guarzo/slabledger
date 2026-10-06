import { expect, test } from '@playwright/test';
import type { ConfirmedReturnState, Purchase, Sale } from '../src/types/campaigns';

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
      // Spheal exercises a linked local sale; Charizard keeps the nullable CAS case.
      const sale: Sale | null = card.cert === '162787413' ? {
        id: '30000000-0000-4000-8000-000000000848', purchaseId: p.id,
        saleChannel: 'ebay', salePriceCents: 2600, saleFeeCents: 465,
        saleDate: '2026-09-26', daysToSell: 25, netProfitCents: 1135,
        forcedLiquidation: false, orderId: 'ext-848', createdAt: stamp, updatedAt: stamp,
      } : null;
      const expectedSaleId = sale?.id ?? null;
      let durable: ConfirmedReturnState = {
        operation: null, expectedSaleId, awaitingListing: false,
        precedingAttempt: null, purchase: p, sale,
      };
      let priceReviewSucceeded = false;
      let checkCalls = 0;
      const returnButton = page.getByRole('button', { name: sale ? 'Return' : 'Resolve DH sale', exact: true });
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
        if (path.endsWith('/dh-sale-check')) {
          checkCalls++;
          return route.fulfill({ json: { status: 'sold', resolvable: true, reason: '',
            target: { dhInventoryId: card.inventoryId, certNumber: card.cert, grader: 'PSA' },
          } });
        }
        if (path.endsWith('/confirm-return')) {
          expect(request.postDataJSON()).toEqual({ returnConfirmed: true, expectedSaleId,
            expectedTarget: { dhInventoryId: card.inventoryId, certNumber: card.cert, grader: 'PSA' },
          });
          durable = { ...durable, expectedSaleId: null, sale: null, awaitingListing: true, outcome: 'completed',
            operation: {
              id: `operation-${card.inventoryId}`, purchaseId: p.id, capturedPurchaseId: p.id,
              dhInventoryId: card.inventoryId, certNumber: card.cert, grader: 'PSA',
              expectedSaleId, capturedOrderId: sale?.orderId ?? '', returnedOrderId: `ext-${card.externalSale}`,
              state: 'completed', createdAt: stamp, completedAt: stamp,
            },
          };
          return route.fulfill({ json: durable });
        }
        if (path.endsWith('/review-price')) {
          expect(request.postDataJSON()).toEqual({ priceCents: 4000, source: 'market' });
          await route.fulfill({ json: { success: true, reviewedAt: stamp } });
          priceReviewSucceeded = true;
          return;
        }
        if (path.endsWith('/list-on-dh')) {
          if (!priceReviewSucceeded) {
            return route.fulfill({ status: 409, json: { error: 'Review the price before listing on DH' } });
          }
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
      if (!sale) {
        await page.getByRole('button', { name: 'Show card details' }).click();
        expect(checkCalls).toBe(0);
        await page.getByRole('button', { name: 'Check DH sale' }).click();
        await expect(page.getByRole('button', { name: 'Resolve DH sale' })).toBeVisible();
        expect(checkCalls).toBe(1);
      }
      await expect(returnButton).toBeVisible();
      await expect(page.getByText(`${sale ? 'Return' : 'Resolve'} confirms slab in hand and refund resolved`)).toBeVisible();
      await page.screenshot({ path: info.outputPath(`ready-${card.cert}-${width}.png`), fullPage: true });
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))).toHaveLength(0);
      await returnButton.click();
      await expect(page.getByRole('alertdialog')).toHaveCount(0);
      await expect(page.getByText('Returned; review the price and list explicitly.')).toBeVisible();
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))).toHaveLength(1);
      expect(mutations.filter(r => r.path.endsWith('/list-on-dh'))).toHaveLength(0);
      expect(mutations.filter(r => r.path.endsWith('/review-price'))).toHaveLength(0);
      await page.reload();
      await expect(page.getByText('Returned; review the price and list explicitly.')).toBeVisible();
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))).toHaveLength(1);
      await page.getByRole('button', { name: 'List on DH', exact: true }).click();
      await expect(page.getByText('Returned; review the price and list explicitly.')).toHaveCount(0);
      expect(mutations.filter(r => r.path.endsWith('/review-price'))).toHaveLength(1);
      expect(mutations.filter(r => r.path.endsWith('/list-on-dh'))).toHaveLength(1);
      expect(mutations.filter(r => r.path.endsWith('/review-price') || r.path.endsWith('/list-on-dh'))
        .map(r => r.path.split('/').at(-1))).toEqual(['review-price', 'list-on-dh']);
      expect(mutations.filter(r => r.path.endsWith('/confirm-return'))[0].body?.expectedSaleId).toBe(expectedSaleId);
      expect(mutations.some(r => r.path.endsWith('/sale'))).toBe(false);
      expect(errors).toEqual([]);
      await page.screenshot({ path: info.outputPath(`listed-${card.cert}-${width}.png`), fullPage: true });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
    });
  }
}
