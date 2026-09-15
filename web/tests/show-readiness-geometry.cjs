/* global document, window */
const { expect } = require('@playwright/test');

async function assertRowsSeparated(page, label, metrics) {
  // Let ResizeObserver and the virtualizer settle, then inspect actual adjacent
  // mounted row bounds, not estimates or only the last row's viewport clearance.
  await page.waitForTimeout(350);
  const rows = await page.locator('.show-inventory [data-index]').evaluateAll(elements => elements
    .filter(e => e.getClientRects().length)
    .map(e => ({ index: Number(e.dataset.index), top: e.getBoundingClientRect().top, bottom: e.getBoundingClientRect().bottom }))
    .sort((a, b) => a.index - b.index));
  metrics.push({ name: label, rows });
  expect(rows.length, label).toBeGreaterThan(1);
  for (let i = 1; i < rows.length; i++) {
    expect(rows[i].top, `${label}: row ${rows[i].index} overlaps row ${rows[i - 1].index}`).toBeGreaterThanOrEqual(rows[i - 1].bottom - 1);
  }
}

async function exerciseGeometry(page, context, capture, metrics, cert) {
  const cdp = await context.newCDPSession(page);
  try {
    for (const mode of [
      { name: 'desktop-fine', width: 1440, height: 1000, touch: false },
      { name: 'desktop-coarse', width: 1440, height: 1000, touch: true },
      { name: 'mobile-coarse', width: 390, height: 844, touch: true },
      { name: 'desktop-coarse-return', width: 1440, height: 1000, touch: true },
    ]) {
      // Start with Chromium's real fine-pointer default. Disabling CDP touch
      // emulation produces pointer:none, not fine, so never mislabel that mode.
      if (mode.touch) await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 1 });
      await page.setViewportSize({ width: mode.width, height: mode.height });
      const pointer = await page.evaluate(() => ({ coarse: window.matchMedia('(pointer: coarse)').matches, fine: window.matchMedia('(pointer: fine)').matches }));
      metrics.push({ name: `${mode.name}-pointer`, ...pointer });
      expect(pointer.coarse).toBe(mode.touch); expect(pointer.fine).toBe(!mode.touch);
      await page.evaluate(() => { document.querySelectorAll('.show-inventory .overflow-y-auto').forEach(e => { e.scrollTop = 0; }); window.scrollTo(0, 0); });
      for (let turn = 0; turn < 3; turn++) {
        await page.getByRole('button', { name: new RegExp(`Show 30-day evidence ${cert}`) }).click();
        await expect(page.getByRole('region', { name: `30-day evidence ${cert}` })).toContainText('$270.00');
        await assertRowsSeparated(page, `${mode.name}-open-${turn}`, metrics);
        await page.getByRole('button', { name: new RegExp(`Hide 30-day evidence ${cert}`) }).click();
        await assertRowsSeparated(page, `${mode.name}-collapsed-${turn}`, metrics);
      }
      const owningRow = page.locator('.show-inventory [data-index="0"]');
      await expect(owningRow).toContainText('DH listed $300.00');
      if (mode.width > 768) await expect(owningRow).toContainText('$400.00');
      await owningRow.scrollIntoViewIfNeeded();
      const trigger = await owningRow.locator('.show-evidence-trigger').boundingBox();
      metrics.push({ name: `${mode.name}-trigger`, height: trigger.height });
      if (mode.touch) expect(trigger.height).toBeGreaterThanOrEqual(44);
      await capture(`${mode.name}-price-context-collapse`, cdp);
      expect(await page.evaluate(() => window.matchMedia('(pointer: coarse)').matches), 'capture must preserve the measured pointer mode').toBe(mode.touch);
      expect((await owningRow.locator('.show-evidence-trigger').boundingBox()).height, 'capture must preserve the measured trigger height').toBe(trigger.height);
      for (const fraction of [0.5, 1, 0]) {
        await page.evaluate(fraction => document.querySelectorAll('.show-inventory .overflow-y-auto').forEach(e => { e.scrollTop = e.scrollHeight * fraction; }), fraction);
        await assertRowsSeparated(page, `${mode.name}-scroll-${fraction}`, metrics);
      }
    }
  } finally { await cdp.detach(); }
}
module.exports = { assertRowsSeparated, exerciseGeometry };
