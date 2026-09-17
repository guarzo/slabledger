/* global URL */
const assert = require('node:assert/strict');

// Test-only observer. Context events include every page, including fresh pages
// and popups; do not depend on whichever page the driver currently focuses.
function createPriceReviewObserver(app) {
  const origin = new URL(app).origin;
  const wire = [], failures = [], errors = [];
  const pending = new Set();
  function observe(context, label) {
    context.on('request', request => wire.push({ context: label, method: request.method(), url: request.url(), body: request.postData() }));
    context.on('weberror', error => errors.push({ context: label, message: error.error().message, page: error.page()?.url() }));
    context.on('response', response => {
      if (new URL(response.url()).origin !== origin || response.status() < 400) return;
      const failure = { context: label, method: response.request().method(), url: response.url(), status: response.status(), marker: null, controlled: false };
      // Retain the response immediately; failed/late header inspection must not
      // erase it or turn it into an allowed fault.
      failures.push(failure);
      const work = (async () => {
        try {
          failure.marker = await response.headerValue('x-price-review-controlled-fault');
          failure.controlled = failure.marker === 'inventory-read' && failure.method === 'GET'
            && new URL(failure.url).pathname === '/api/inventory' && failure.status === 503;
        } catch (error) { failure.observationError = String(error); }
      })();
      pending.add(work);
      void work.then(() => pending.delete(work));
    });
  }
  async function drain() {
    // Inspection can yield while other responses arrive. Join each new batch.
    while (pending.size) await Promise.all([...pending]);
  }
  function assertNoUnexpected() {
    assert.deepEqual(failures.filter(failure => !failure.controlled), [], 'Unexpected application HTTP failures');
    assert.deepEqual(errors, [], 'Unexpected browser page errors');
  }
  return { wire, failures, errors, observe, drain, assertNoUnexpected };
}
module.exports = { createPriceReviewObserver };
