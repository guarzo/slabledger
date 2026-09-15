/* global console */
const { expect } = require('@playwright/test');

// Test-harness resource ownership, not application behavior.
async function withFixtureCleanup(exercise, diagnostics, close) {
  let failed = false;
  let failure;
  const recordFailure = error => {
    if (!failed) { failed = true; failure = error; }
    else console.error('Secondary fixture cleanup failure:', error);
  };
  try {
    try { await exercise(); } catch (error) { recordFailure(error); }
    // One broken read/write must not prevent the remaining diagnostic attempts.
    for (const diagnostic of diagnostics) {
      try { await diagnostic(); } catch (error) { recordFailure(error); }
    }
  } finally {
    try { await close(); } catch (error) { recordFailure(error); }
  }
  if (failed) throw failure;
}

async function assertLastRowClearance(last, bar) {
  await expect(last).toBeVisible();
  const owner = last.locator('xpath=ancestor::*[self::article or @role="row"][1]');
  await expect(owner).toBeVisible();
  const lastBox = await owner.boundingBox();
  const barBox = await bar.boundingBox();
  expect(lastBox.y + lastBox.height).toBeLessThanOrEqual(barBox.y);
}

module.exports = { withFixtureCleanup, assertLastRowClearance };
