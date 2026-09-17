/* global URL, fetch, document, window */
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const http = require('node:http');
const { chromium } = require('@playwright/test');
const { createPriceReviewObserver } = require('./price-review-browser-observer.cjs');

// These focused harness tests complement the actual Go/PG/browser retry probe.
test('context observers retain a recovered mobile503 and another page failure while allowing only a marked fault', async t => {
  let mobileAttempts = 0;
  const server = http.createServer((request, response) => {
    const url = new URL(request.url, 'http://127.0.0.1');
    if (url.pathname === '/api/inventory') {
      const mode = url.searchParams.get('mode');
      if (mode === 'marked') {
        response.setHeader('X-Price-Review-Controlled-Fault', 'inventory-read');
        response.writeHead(503); response.end('controlled'); return;
      }
      if (mode === 'mobile' && ++mobileAttempts === 1) {
        response.writeHead(503); response.end('unmarked'); return;
      }
      if (mode === 'popup') { response.writeHead(500); response.end('popup failure'); return; }
      response.end('recovered'); return;
    }
    response.setHeader('Content-Type', 'text/html');
    response.end('<!doctype html><output>waiting</output>');
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())));
  const app = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch();
  const observer = createPriceReviewObserver(app);
  try {
    const desktop = await browser.newContext(); observer.observe(desktop, 'desktop');
    const page = await desktop.newPage(); await page.goto(app);
    assert.equal(await page.evaluate(async () => (await fetch('/api/inventory?mode=marked')).status), 503);
    const popupReady = desktop.waitForEvent('page');
    await page.evaluate(() => window.open('/'));
    const popup = await popupReady; await popup.waitForLoadState();
    assert.equal(await popup.evaluate(async () => (await fetch('/api/inventory?mode=popup')).status), 500);
    const mobile = await browser.newContext({ viewport: { width: 390, height: 844 } }); observer.observe(mobile, 'mobile');
    const phone = await mobile.newPage(); await phone.goto(app);
    await phone.evaluate(async () => {
      let response = await fetch('/api/inventory?mode=mobile');
      if (!response.ok) response = await fetch('/api/inventory?mode=mobile');
      document.querySelector('output').textContent = response.ok ? 'Recovered' : 'Failed';
    });
    assert.equal(await phone.locator('output').textContent(), 'Recovered');
    assert.equal(mobileAttempts, 2);
    await observer.drain(); await browser.close(); await observer.drain();
    assert.deepEqual(observer.failures.map(({ context, status, marker, controlled }) => ({ context, status, marker, controlled })), [
      { context: 'desktop', status: 503, marker: 'inventory-read', controlled: true },
      { context: 'desktop', status: 500, marker: null, controlled: false },
      { context: 'mobile', status: 503, marker: null, controlled: false },
    ]);
    assert.throws(() => observer.assertNoUnexpected(), /Unexpected application HTTP failures/);
    assert.equal(observer.wire.filter(request => request.context === 'mobile' && request.url.includes('/api/inventory')).length, 2);
  } finally {
    await browser.close(); await observer.drain();
  }
});

// Controlled Response-header boundary isolates the async drain itself, rather
// than depending on browser transport timing to happen to leave work pending.
function response({ marker = 'inventory-read', status = 503, method = 'GET', route = '/api/inventory', origin = 'http://127.0.0.1:4000' } = {}) {
  return { url: () => origin + route, status: () => status, request: () => ({ method: () => method }),
    headerValue: async () => marker };
}
test('drain joins late-added header work and retains attribution errors as unexpected', async () => {
  const context = new EventEmitter(); const observer = createPriceReviewObserver('http://127.0.0.1:4000');
  observer.observe(context, 'mobile');
  let complete, fail;
  context.emit('response', { ...response(), headerValue: () => new Promise(resolve => { complete = resolve; }) });
  let settled = false;
  const draining = observer.drain().then(() => { settled = true; });
  await Promise.resolve(); assert.equal(settled, false);
  context.emit('response', { ...response(), headerValue: () => new Promise((_resolve, reject) => { fail = reject; }) });
  complete('inventory-read');
  await Promise.resolve(); await Promise.resolve(); assert.equal(settled, false);
  fail(new Error('header transport closed'));
  await draining;
  assert.equal(observer.failures[0].controlled, true);
  assert.deepEqual(observer.failures[1], { context: 'mobile', method: 'GET', url: 'http://127.0.0.1:4000/api/inventory',
    status: 503, marker: null, controlled: false, observationError: 'Error: header transport closed' });
  assert.throws(() => observer.assertNoUnexpected(), /Unexpected application HTTP failures/);
});

test('only the exact marked GET inventory503 is allowed, not unrelated status, path, method or marker', async () => {
  for (const input of [{}, { marker: null }, { marker: 'other-fault' }, { status: 500 }, { route: '/api/other' }, { method: 'PATCH' }]) {
    const context = new EventEmitter(); const observer = createPriceReviewObserver('http://127.0.0.1:4000');
    observer.observe(context, 'desktop'); context.emit('response', response(input)); await observer.drain();
    if (!Object.keys(input).length) assert.doesNotThrow(() => observer.assertNoUnexpected());
    else assert.throws(() => observer.assertNoUnexpected(), /Unexpected application HTTP failures/);
  }
});

test('application origin matching is exact and page errors from any context stay fatal', async () => {
  const context = new EventEmitter(); const observer = createPriceReviewObserver('http://127.0.0.1:4000');
  observer.observe(context, 'mobile');
  context.emit('response', response({ origin: 'http://127.0.0.1:40001', marker: null }));
  context.emit('weberror', { error: () => new Error('popup script failed'), page: () => ({ url: () => 'http://127.0.0.1:4000/popup' }) });
  await observer.drain(); assert.deepEqual(observer.failures, []);
  assert.deepEqual(observer.errors, [{ context: 'mobile', message: 'popup script failed', page: 'http://127.0.0.1:4000/popup' }]);
  assert.throws(() => observer.assertNoUnexpected(), /Unexpected browser page errors/);
});
