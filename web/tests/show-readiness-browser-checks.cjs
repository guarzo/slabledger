/* global process, console, __filename, Buffer */
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { chromium } = require('@playwright/test');
const { withFixtureCleanup, assertLastRowClearance } = require('./show-readiness-browser-helpers.cjs');

async function cleanupChild(mode, directory) {
  const browser = await chromium.launch();
  try {
    await withFixtureCleanup(async () => {
      await browser.newContext();
      if (mode === 'primary') throw new Error('PRIMARY_EXERCISE_FAILURE');
    }, [
      // A genuine failed artifact write, not a mocked filesystem expectation.
      () => fs.writeFile(directory, 'cannot write a file over a directory'),
      () => fs.writeFile(path.join(directory, 'later-diagnostic'), 'ran'),
    ], () => browser.close());
  } catch (error) {
    console.log(`REPORTED_FAILURE=${error.message}`);
    process.exitCode = 1;
  } finally {
    console.log(`BROWSER_CLOSED=${!browser.isConnected()}`);
    // Test safety only: a RED must not leak Chromium. Assert the state before
    // this fallback, so the regression cannot pass on the fallback's cleanup.
    if (browser.isConnected()) await browser.close();
  }
}

if (process.argv[2] === 'cleanup-child') {
  cleanupChild(process.argv[3], process.argv[4]).catch(error => { console.error(error); process.exitCode = 1; });
} else {
  for (const mode of ['primary', 'diagnostic']) {
    test(`failed diagnostic closes the real browser and preserves ${mode} failure/exit`, async () => {
      const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'readiness-cleanup-'));
      try {
        const child = spawn(process.execPath, [__filename, 'cleanup-child', mode, directory], { timeout: 15000 });
        const output = [];
        child.stdout.on('data', chunk => output.push(chunk));
        child.stderr.on('data', chunk => output.push(chunk));
        const result = await new Promise((resolve, reject) => {
          child.on('error', reject);
          child.on('exit', (code, signal) => resolve({ code, signal }));
        });
        const text = Buffer.concat(output).toString();
        assert.deepEqual(result, { code: 1, signal: null }, text);
        assert.match(text, /BROWSER_CLOSED=true/, text);
        assert.match(text, mode === 'primary' ? /REPORTED_FAILURE=PRIMARY_EXERCISE_FAILURE/ : /REPORTED_FAILURE=.*EISDIR/, text);
        assert.equal(await fs.readFile(path.join(directory, 'later-diagnostic'), 'utf8'), 'ran');
      } finally {
        await fs.rm(directory, { recursive: true, force: true });
      }
    });
  }

  for (const owner of ['article', 'div role="row"']) {
    test(`clearance checks the entire ${owner}, not its checkbox`, async () => {
      const browser = await chromium.launch();
      try {
        const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
        await page.setContent(`<${owner} style="position:absolute;top:0;height:200px;width:300px">
          <input type="checkbox" aria-label="Last slab" style="position:absolute;top:10px">
          <button style="position:absolute;bottom:0">Last row action</button>
          </${owner.split(' ')[0]}>
          <section aria-label="Selection bar" style="position:absolute;top:100px;height:100px;width:390px"></section>`);
        const last = page.getByRole('checkbox', { name: 'Last slab' });
        const bar = page.getByRole('region', { name: 'Selection bar' });
        await assert.rejects(assertLastRowClearance(last, bar), /toBeLessThanOrEqual/);
        await bar.evaluate(element => { element.style.top = '250px'; });
        await assertLastRowClearance(last, bar);
      } finally {
        await browser.close();
      }
    });
  }
}
