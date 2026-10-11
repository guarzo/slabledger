/* global process, URL, fetch, AbortSignal, document, console, innerWidth, sessionStorage, setTimeout, clearTimeout */
// Actual browser -> authenticated Go routes -> PostgreSQL. Interception may
// abort an actual committed response, NEVER fabricate an application result.
const { chromium, expect } = require('@playwright/test');
const fs = require('node:fs/promises');
const path = require('node:path');
const { randomUUID } = require('node:crypto');
const { withFixtureCleanup } = require('./show-readiness-browser-helpers.cjs');
const app = process.env.SHOW_READINESS_APP;
const control = process.env.SHOW_READINESS_CONTROL;
const artifacts = process.env.SHOW_READINESS_ARTIFACTS;
const headers = { Authorization: `Bearer ${process.env.SHOW_READINESS_TOKEN}`, 'Content-Type': 'application/json' };
for (const url of [app, control]) if (new URL(url).hostname !== '127.0.0.1') throw Error('owned loopback fixture required');
const wire = []; const snapshots = {}; const errors = []; const metrics = [];
async function json(url, method = 'GET', body, status = 200) {
  const response = await fetch(url, { method, headers, body: body && JSON.stringify(body), signal: AbortSignal.timeout(15000) });
  expect(response.status, `${method} ${url}`).toBe(status);
  const result = await response.json(); wire.push({ path: new URL(url).pathname, method, body, status, result }); return result;
}
const members = rows => [...rows].sort((a, b) => a.id.localeCompare(b.id));
const state = async () => { const result = await json(`${control}/state`); return { ...result, items: members(result.items) }; };
function bounded(promise, label) {
  let timer;
  const deadline = new Promise((_, reject) => { timer = setTimeout(() => reject(Error(`${label} exceeded 15s`)), 15000); });
  return Promise.race([promise, deadline]).finally(() => clearTimeout(timer));
}
const command = (route, body) => json(`${control}/${route}`, 'POST', body);
const cert = i => `910000${String(i).padStart(2,'0')}`;
(async () => {
  const browser = await chromium.launch({ headless: true }); let page;
  await withFixtureCleanup(async () => {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, extraHTTPHeaders: headers });
    await context.route(url => url.origin !== new URL(app).origin, route => route.abort());
    page = await context.newPage(); page.on('pageerror', error => errors.push(error.message));
    let intercept = null;
    await context.route('**/scan-confirm', async route => {
      const body = route.request().postDataJSON(); const chosen = intercept; intercept = null;
      try {
        if (chosen?.kind === 'before') { chosen.resolve(body); await bounded(chosen.wait, 'scan delivery hold'); await route.continue(); return; }
        const response = await route.fetch({ timeout: 15000 }); const result = await response.json();
        wire.push({ path: new URL(route.request().url()).pathname, body, status: response.status(), result });
        if (chosen) { chosen.resolve({ body, result }); if (chosen.wait) await bounded(chosen.wait, 'scan response hold'); await route.abort('failed'); }
        else await route.fulfill({ response }); // forwards the ACTUAL backend bytes/status
      } catch (error) {
        chosen?.reject(error); errors.push(error.message);
        await route.abort('failed').catch(() => {});
      }
    });
    function interceptNext(kind, hold = false) {
      let resolve, reject, release; const received = new Promise((r, fail) => { resolve = r; reject = fail; });
      const wait = hold ? new Promise(r => { release = r; }) : null;
      intercept = { kind, resolve, reject, wait }; return { received: bounded(received, 'scan interception'), release };
    }
    const scanner = () => page.getByRole('textbox', { name: 'Scan slab barcode' });
    const member = i => page.getByRole('article', { name: `Show member ${cert(i)}`, exact: true });
    const recent = () => page.getByRole('region', { name: 'Recent scans' });
    const pendingStorage = () => page.evaluate(() => Object.keys(sessionStorage).filter(k => k.startsWith('slabledger:show-scans:')).map(k => JSON.parse(sessionStorage.getItem(k))));
    async function scan(i) { await scanner().focus(); await page.keyboard.type(cert(i)); await page.keyboard.press('Enter'); }
    async function capture(name) {
      await page.screenshot({ path:path.join(artifacts,`${name}-viewport.png`) });
      await page.screenshot({ path:path.join(artifacts,`${name}.png`),fullPage:true });
      const geometry = await page.evaluate(() => ({ width:innerWidth, overflow:document.documentElement.scrollWidth>innerWidth,
        controls:[...document.querySelectorAll('.show-prep-page button,.show-prep-page input,.show-prep-page select')].filter(e=>e.getClientRects().length).map(e=>({ label:e.getAttribute('aria-label')||e.textContent,height:e.getBoundingClientRect().height })) }));
      metrics.push({name,...geometry}); expect(geometry.overflow,name).toBe(false);
      if(geometry.width<=768) expect(geometry.controls.every(c=>c.height>=44),`${name} touch controls`).toBe(true);
    }
    snapshots.initial=await state();
    await page.goto(`${app}/shows`); await expect(page.getByRole('heading',{name:'No show lists yet'})).toBeVisible(); await capture('desktop-empty');
    await page.getByLabel('New show name').fill('Real scan show');
    await page.getByRole('button', { name:'Create show list',exact:true }).click();
    await expect(scanner()).toBeEnabled(); await expect(scanner()).toBeFocused();
    const firstHeld = interceptNext('after',true);
    await scan(1); const firstCommit = await firstHeld.received;
    await scanner().fill(cert(2)); await scanner().press('Enter');
    await expect(recent()).toContainText('Pending');
    await expect(page.getByRole('combobox',{name:'Show list',exact:true})).toBeDisabled();
    const queued = await pendingStorage(); expect(queued[0].scans.map(s=>s.command.certNumber)).toEqual([cert(1),cert(2)]);
    firstHeld.release(); await expect(page.getByRole('button',{name:`Retry scan ${cert(1)}`})).toBeVisible();
    // Uncertain first response stops FIFO; queued second has not reached backend.
    expect((await state()).items).toHaveLength(1);
    await page.getByRole('button',{name:`Retry scan ${cert(1)}`} ).click();
    await expect(member(2)).toBeVisible(); await expect(member(1)).toBeVisible();
    let s = await state(); const listID=s.lists[0].id; snapshots.consecutive=s;
    expect(s.items.every(i=>i.packed_at&&i.acknowledged_price_cents===30000)).toBe(true);
    expect(s.receipts).toHaveLength(2); expect(wire.filter(w=>w.body?.commandId===firstCommit.body.commandId).map(w=>w.body)).toEqual([firstCommit.body,firstCommit.body]);
    await expect(page.getByRole('checkbox',{name:/Packed/})).toHaveCount(0);
    await expect(page.getByRole('button',{name:/Add.*show|Acknowledge/i})).toHaveCount(0);
    await page.reload(); await expect(member(2)).toBeVisible();
    expect(await command('restart')).toMatchObject({restarted:true,restarts:1});
    await page.reload(); await expect(member(2)).toBeVisible(); expect((await state()).items).toEqual(s.items);
    // Hold the actual post-scan detail response while the next barcode arrives.
    // Keyboard delivery never waits for an enabled/editable input, unlike fill.
    let readStarted, releaseRead;
    const readReceived = bounded(new Promise(resolve => { readStarted = resolve; }), 'detail interception');
    const readHold = new Promise(resolve => { releaseRead = resolve; });
    await context.route(`**/api/show-prep/lists/${listID}`, async route => {
      try {
        const response = await route.fetch({ timeout: 15000 }); readStarted();
        await bounded(readHold, 'detail response hold'); await route.fulfill({ response });
      } catch (error) { errors.push(error.message); await route.abort('failed').catch(() => {}); }
    }, { times: 1 });
    await scan(1); await readReceived;
    await page.keyboard.type(cert(2)); await page.keyboard.press('Enter');
    await expect.poll(() => wire.filter(w => w.body?.certNumber === cert(2)).length).toBe(2);
    await expect(scanner()).toHaveValue('');
    releaseRead(); await expect(recent()).toContainText('Already confirmed');
    expect((await state()).items).toEqual(s.items);
    // Lost committed response, price drift, read-only warning and exact replay.
    const lost=interceptNext('after'); await scan(1); const committed=await lost.received;
    await expect(page.getByRole('button',{name:`Retry scan ${cert(1)}`})).toBeVisible();
    snapshots.attributedAskingDrift=await command('asking-drift'); await page.reload();
    await expect(page.getByRole('button',{name:`Recover scan ${cert(1)}`})).toBeVisible();
    await expect(member(1)).toContainText('Check sticker and rescan');
    const beforeReplay=(await state()).items;
    await page.getByRole('button',{name:`Recover scan ${cert(1)}`} ).click();
    await expect(recent()).toContainText('Already confirmed');
    s=await state(); expect(s.items).toEqual(beforeReplay);
    const replay=wire.filter(w=>w.body?.commandId===committed.body.commandId).at(-1);
    expect(replay.body).toEqual(committed.body); expect(replay.result.original).toEqual(committed.result.original); expect(replay.result.replay).toBe(true);
    await scan(1); await expect(recent()).toContainText('Rechecked');
    s=await state(); expect(s.items.find(i=>i.cert_number===cert(1))).toMatchObject({packed_at:beforeReplay.find(i=>i.cert_number===cert(1)).packed_at,acknowledged_price_cents:32500});
    await expect(member(1)).not.toContainText('Check sticker and rescan');
    for(const viewport of [{name:'desktop',width:1440,height:1000},{name:'tablet',width:768,height:1024},{name:'mobile',width:390,height:844}]) {
      await page.setViewportSize(viewport); await capture(`${viewport.name}-confirmed`);
    }
    // An attributed inventory grade correction retains saved identity/history,
    // but cannot claim current confirmation or let a scan/replay approve it.
    const identityHistory=(await state()).items;
    snapshots.attributedGradeDrift=await command('grade-drift'); await page.reload();
    await expect(member(1)).toContainText('Identity changed. Confirmation history retained');
    await expect(member(1)).toContainText('Check inventory identity before scanning');
    await expect(member(1)).toContainText('Confirmed');
    const identityDetail=await json(`${app}/api/show-prep/lists/${listID}`);
    expect(identityDetail.items.find(i=>i.certNumber===cert(1))).toMatchObject({identityChanged:true,grade:10,evaluation:{grade:9}});
    expect(identityDetail.summary).toMatchObject({packedCount:2,confirmedReadyCount:1,confirmedKnownValueCents:30000,knownValueCents:62500});
    await expect(page.getByLabel('Confirmed ready asking value')).toContainText('$300.00');
    await scan(1); await expect(recent()).toContainText('Card identity changed');
    expect(await json(`${app}/api/show-prep/lists/${listID}/scan-confirm`,'POST',firstCommit.body)).toMatchObject({replay:true,current:{state:'unavailable'},original:firstCommit.result.original});
    expect((await state()).items).toEqual(identityHistory); await capture('mobile-identity-history-warning');
    snapshots.attributedGradeRestore=await command('grade-restore'); await page.reload();
    await expect(member(1)).not.toContainText('Identity changed');
    await expect(page.getByLabel('Confirmed ready asking value')).toContainText('$625.00');
    expect((await state()).items).toEqual(identityHistory);
    // Real NOWAIT contention: no receipt. Removal advances generation and old
    // recovery remains immutable across restart, including removal/re-add.
    await command('hold-purchase'); await scan(3);
    await expect(page.getByRole('button',{name:`Retry scan ${cert(3)}`})).toBeVisible();
    expect(wire.filter(w=>w.body?.certNumber===cert(3)).at(-1)).toMatchObject({status:503,result:{code:'scan_contended',retryable:true}});
    const oldPending=(await pendingStorage())[0].scans[0]; s=await state();
    expect(s.receipts.some(r=>r.command_id===oldPending.command.commandId)).toBe(false);
    await command('release-purchase'); await page.getByRole('button',{name:`Remove ${cert(1)} from show`} ).click(); await expect(member(1)).toHaveCount(0);
    const absentBefore=(await state()).items;
    expect(await json(`${app}/api/show-prep/lists/${listID}/scan-confirm`,'POST',firstCommit.body)).toMatchObject({replay:true,current:{state:'removed'},original:firstCommit.result.original});
    expect((await state()).items).toEqual(absentBefore);
    await command('restart'); await page.reload();
    await page.getByRole('button',{name:`Recover scan ${cert(3)}`} ).click();
    await expect(recent()).toContainText('Show contents changed after this scan; rescan to confirm.');
    expect(wire.filter(w=>w.body?.commandId===oldPending.command.commandId).at(-1).body).toEqual(oldPending.command);
    expect((await state()).items.some(i=>i.cert_number===cert(3))).toBe(false);
    await scan(1); await expect(member(1)).toBeVisible();
    const newMember=(await state()).items.find(i=>i.cert_number===cert(1));
    expect(newMember.id).not.toBe(firstCommit.result.original.itemId);
    const removedReplay=await json(`${app}/api/show-prep/lists/${listID}/scan-confirm`,'POST',firstCommit.body);
    expect(removedReplay).toMatchObject({replay:true,current:{state:'removed'},original:firstCommit.result.original});
    expect((await state()).items.find(i=>i.id===newMember.id)).toEqual(newMember);
    await scan(3); await expect(member(3)).toBeVisible();
    // Legacy unconfirmed rows are seeded ONLY through the fixture, never Add.
    await command('legacy-member',{listId:listID}); await page.reload();
    await expect(member(4)).toContainText('Unconfirmed. Scan the checked slab');
    await scan(4); await expect(member(4)).toContainText('Confirmed');
    // Actual not-received retains historical confirmation without eligible value.
    snapshots.attributedUnreceive=await command('unreceive'); await page.reload(); await expect(member(2)).toContainText('Confirmation history retained');
    await scan(2); await expect(recent()).toContainText('Not received. Record receipt');
    const detail=await json(`${app}/api/show-prep/lists/${listID}`);
    expect(detail.summary).toMatchObject({totalCount:4,packedCount:4,confirmedReadyCount:3,confirmedKnownValueCents:92500,notReceivedCount:1});
    await scan(26); await expect(member(26)).toBeVisible(); await expect(member(26)).toContainText('No asking price');
    await scan(99); await expect(recent()).toContainText('No inventory match');
    await expect(scanner()).toBeEnabled(); await capture('mobile-history-warnings');
    // Stale-client retirement is real authenticated JSON410, with no row writes.
    s=await state();
    for(const [method,route,body] of [['POST',`/lists/${listID}/items`,{items:[]}],['PUT',`/lists/${listID}/items/${newMember.id}`,{packed:true}]]) {
      expect((await fetch(`${app}/api/show-prep${route}`,{method})).status).toBe(401);
      const gone=await json(`${app}/api/show-prep${route}`,method,body,410); expect(gone.error).toBeTruthy();
    }
    expect((await state()).items).toEqual(s.items);
    // Navigation does not retarget a captured/delayed command to another show.
    const other=randomUUID(); await json(`${app}/api/show-prep/lists`,'POST',{id:other,name:'Other scan show'});
    const held=interceptNext('after',true); await scan(5); const oldShow=await held.received;
    await page.goto(`${app}/shows?list=${other}`); held.release();
    await expect(page.getByRole('button',{name:`Recover scan ${cert(5)}`})).toBeVisible();
    await page.getByRole('button',{name:`Recover scan ${cert(5)}`} ).click();
    await expect(recent()).toContainText('Confirmed');
    await expect(member(5)).toHaveCount(0); expect((await json(`${app}/api/show-prep/lists/${other}`)).items).toEqual([]);
    expect(wire.filter(w=>w.body?.commandId===oldShow.body.commandId).every(w=>w.path.includes(listID))).toBe(true);
    // Retiring a show fences delayed create/scan retries, not inventory rows.
    const otherDetail=await json(`${app}/api/show-prep/lists/${other}`);
    const deletion={commandId:randomUUID(),expectedState:otherDetail.stateFingerprint};
    await json(`${app}/api/show-prep/lists/${other}`,'DELETE',deletion);
    expect(await json(`${app}/api/show-prep/lists/${other}`,'DELETE',deletion)).toMatchObject({deleted:true,replay:true});
    await json(`${app}/api/show-prep/lists`,'POST',{id:other,name:'Other scan show'},410);
    await json(`${app}/api/show-prep/lists/${other}/scan-confirm`,'POST',{commandId:randomUUID(),certNumber:cert(1),withdrawalGeneration:0},410);
    await page.goto(`${app}/shows?list=${listID}`);
    // Capacity uses actual UI capture while the first request is held BEFORE
    // delivery. The 201st cert is retained, never silently dropped or submitted.
    const capacity=interceptNext('before',true); await scan(99); await capacity.received;
    for(let i=1;i<200;i++) { await scanner().fill(cert(99)); await scanner().press('Enter'); }
    expect((await pendingStorage())[0].scans).toHaveLength(200);
    await scanner().fill('capacity-overflow'); await scanner().press('Enter');
    await expect(page.getByRole('alert')).toContainText('200 unsettled scans');
    await expect(scanner()).toHaveValue('capacity-overflow'); await capture('mobile-capacity');
    const receiptCount=(await state()).receipts.length;
    // Real logout clears tab recovery before the request resolves; old account
    // work cannot be recovered after re-entering the authenticated route.
    await page.getByRole('button',{name:'User menu for local-api'}).click();
    await page.getByRole('menuitem',{name:'Log out',exact:true}).click();
    await expect.poll(pendingStorage).toEqual([]);
    await page.waitForURL(`${app}/login`);
    // LocalAPIToken is deliberately still valid; the real Login page resolves
    // it and redirects home. Join that auth navigation before opening Shows.
    await page.waitForURL(`${app}/`); capacity.release();
    await page.goto(`${app}/shows?list=${listID}`); await expect(scanner()).toBeEnabled();
    await expect(page.getByRole('button',{name:/Recover scan|Retry scan/})).toHaveCount(0);
    expect((await state()).receipts.length).toBeLessThanOrEqual(receiptCount+1);
    snapshots.final=await state(); expect(snapshots.final).toMatchObject({sourceCalls:0,providerRequests:0,ledgerUnchanged:true,restarts:2});
    expect(snapshots.final.requests.filter(r=>!['GET','HEAD'].includes(r.method)&&!r.path.startsWith('/api/show-prep/')&&r.path!=='/api/auth/logout')).toEqual([]);
    expect(errors).toEqual([]);
    console.log('PASS real scan FIFO/atomic persistence/reload/2 full restarts; committed lost-response exact-ID replay; drift/rescan; grade correction identity warning/excluded totals/history/read-only replay; NOWAIT no receipt/remove/recovery; remove-readd old replay; legacy/unreceived/no-price/no-match; auth401/logout discard/200-bound; old410/tombstone; navigation destination; whole financial rows unchanged outside named fixture mutations; provider=0');
  }, [
    () => fs.writeFile(path.join(artifacts,'show-scan-proof.json'),JSON.stringify({wire,snapshots,errors,metrics},null,2)),
    () => page?.screenshot({path:path.join(artifacts,'last-view.png'),fullPage:true}),
  ], () => browser.close());
})().catch(error=>{console.error(error);process.exitCode=1;});
