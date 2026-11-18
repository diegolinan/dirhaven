import {test,expect} from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
let fixture;
const failures=new WeakMap();
test.beforeEach(async({request,page})=>{fixture=await(await request.get('/__fixture')).json();await request.post('/__fixture/reset');const errors=[];failures.set(page,errors);page.on('pageerror',e=>errors.push(e.message));});
test.afterEach(async({page})=>expect(failures.get(page)).toEqual([]));
async function open(page){await page.goto('/#'+fixture.token,{waitUntil:'commit'});await expect(page.locator('.app-shell')).toHaveAttribute('data-ready','true');await expect(page.locator('#sessionExpired')).toBeHidden();}
async function fillFolders(page){await page.locator('#manualPaths').evaluate(e=>e.open=true);await page.locator('#paths').fill(fixture.root);}
async function prepare(page){await open(page);await fillFolders(page);await page.locator('#generate').click();await expect(page.locator('#state')).toHaveText('LOCAL SCOPE READY');}
async function connection(page){await prepare(page);await page.locator('#nextStep').click();await expect(page.locator('#panel-2')).toBeVisible();}

test('navigation never grants access, writes preferences or starts diagnostics',async({page})=>{
 const calls=[];page.on('request',r=>{if(r.url().includes('/api/'))calls.push(new URL(r.url()).pathname)});
 await open(page);expect(new URL(page.url()).hash).toBe('');await expect(page.locator('#screenTitle')).toHaveText('Choose Your Folders.');await expect(page.locator('#nextStep')).toBeDisabled();await expect(page.locator('#stepNav [data-step="2"]')).toBeDisabled();
 await fillFolders(page);await page.locator('#generate').click();await expect(page.locator('#nextStep')).toBeEnabled();
 await page.locator('#nextStep').click();await expect(page.locator('#screenTitle')).toBeFocused();await page.locator('#nextStep').click();await expect(page.locator('#panel-3')).toBeVisible();await page.locator('#nextStep').click();await expect(page.locator('#panel-manage')).toBeVisible();
 expect(calls).toEqual(['/api/saved','/api/validate']);await expect(page.locator('#manageLocalStatus')).toHaveText('Prepared in this session');await expect(page.locator('#panel-manage')).toContainText('Not verified here');
});

test('save, return, load and forget use real temporary persistence and explicit actions',async({page})=>{
 await prepare(page);await page.locator('#saveScope').click();await expect(page.locator('#saveFeedback')).toContainText('Saved locally');
 await page.goto('/?reopened=1#'+fixture.token,{waitUntil:'commit'});await expect(page.locator('.app-shell')).toHaveAttribute('data-ready','true');await expect(page.locator('#panel-manage')).toBeVisible();await expect(page.locator('#savedFolderList')).toContainText(fixture.root);await expect(page.locator('#manageLocalStatus')).toHaveText('Needs validation');
 await page.locator('#loadSaved').click();await expect(page.locator('#panel-1')).toBeVisible();await expect(page.locator('#paths')).toHaveValue(fixture.root);await expect(page.locator('#nextStep')).toBeDisabled();await page.locator('#generate').click();await expect(page.locator('#nextStep')).toBeEnabled();
 await page.locator('#overviewLink').click();await page.getByText('Manage saved preferences',{exact:true}).click();page.once('dialog',d=>d.dismiss());await page.locator('#revokeSaved').click();await expect(page.locator('#savedFolderList')).toContainText(fixture.root);
 page.once('dialog',d=>d.accept());await page.locator('#revokeSaved').click();await expect(page.locator('#savedStatus')).toContainText('Running connections are unchanged');await expect(page.locator('#savedFolderList li')).toHaveCount(0);
});

test('broad access needs fresh acknowledgement after loading a saved choice',async({page})=>{
 await open(page);await page.locator('#broaderAccess > summary').click();await page.locator('input[name="mode"][value="all"]').check();await page.locator('#generate').click();await expect(page.locator('#validationError')).toBeVisible();await expect(page.locator('#nextStep')).toBeDisabled();
 await page.locator('#ack').check();await page.locator('#generate').click();await expect(page.locator('#state')).toHaveText('LOCAL SCOPE READY');await page.locator('#saveScope').click();await expect(page.locator('#saveFeedback')).toContainText('Saved locally');
 await page.goto('/?reopened=1#'+fixture.token,{waitUntil:'commit'});await expect(page.locator('.app-shell')).toHaveAttribute('data-ready','true');await expect(page.locator('#panel-manage')).toBeVisible();await page.locator('#loadSaved').click();await expect(page.locator('#ack')).not.toBeChecked();await expect(page.locator('#nextStep')).toBeDisabled();
});

test('picker cancellation and failure preserve manual selection and do not authorize',async({page})=>{
 await open(page);await page.route('**/api/pick-directory',r=>r.fulfill({json:{cancelled:true}}));await page.locator('#pickDirectory').click();await expect(page.locator('#pickerStatus')).toContainText('cancelled');await expect(page.locator('#nextStep')).toBeDisabled();
 await page.unroute('**/api/pick-directory');await page.locator('#pickDirectory').click();await expect(page.locator('#selectedFolderList')).toContainText(fixture.root);await expect(page.locator('#nextStep')).toBeDisabled();
 await page.locator('#generate').click();await expect(page.locator('#nextStep')).toBeEnabled();await page.getByRole('button',{name:'Remove folder '+fixture.root,exact:true}).click();await expect(page.locator('#nextStep')).toBeDisabled();await expect(page.locator('#pickDirectory')).toBeFocused();
 await page.route('**/api/pick-directory',r=>r.fulfill({status:500,body:'unavailable'}));await page.locator('#pickDirectory').click();await expect(page.locator('#pickerStatus')).toContainText('Paste an absolute path');await fillFolders(page);await expect(page.locator('#paths')).toHaveValue(fixture.root);
});

test('a stale folder validation cannot validate a newer selection',async({page})=>{
 await open(page);await fillFolders(page);let release;let arrived;const requested=new Promise(r=>arrived=r);
 await page.route('**/api/validate',async r=>{await new Promise(resolve=>{release=resolve;arrived()});await r.fulfill({json:{ok:true,roots:[fixture.root],command:'old-command',mcpClientJSON:'old-json'}})});
 await page.locator('#generate').click();await requested;await page.locator('#paths').fill(fixture.second);release();await expect(page.locator('#generate')).toBeEnabled();await expect(page.locator('#nextStep')).toBeDisabled();await expect(page.locator('#results')).toBeHidden();await expect(page.locator('#state')).toHaveText('NOT VALIDATED');
});

test('invalid paths and failed validation keep the user in folders',async({page})=>{
 await open(page);await page.locator('#generate').click();await expect(page.locator('#validationError')).toContainText('at least one');await expect(page.locator('#nextStep')).toBeDisabled();await fillFolders(page);
 await page.route('**/api/validate',r=>r.abort());await page.locator('#generate').click();await expect(page.locator('#validationError')).toBeVisible();await expect(page.locator('#nextStep')).toBeDisabled();
});

test('the local MCP branch shows its own configuration and copying retains focus',async({page})=>{
 await page.addInitScript(()=>Object.defineProperty(navigator,'clipboard',{value:{writeText:async text=>window.lastCopy=text},configurable:true}));await prepare(page);await page.locator('input[name="destination"][value="stdio"]').check();await page.locator('#nextStep').click();await expect(page.locator('#clientExample')).toHaveAttribute('open','');await expect(page.locator('#diagnostics')).toBeHidden();
 await page.locator('[data-copy="client"]').click();await expect(page.locator('[data-copy="client"]')).toBeFocused();expect(await page.evaluate(()=>JSON.parse(window.lastCopy).mcpServers)).toBeTruthy();await page.locator('#previousStep').click();await expect(page.locator('#paths')).toHaveValue(fixture.root);
});

test('diagnostics stay opt-in and distinguish metadata, checksums and signatures',async({page})=>{
 const calls=[];page.on('request',r=>{if(r.url().includes('/api/tunnel-'))calls.push(new URL(r.url()).pathname)});
 await connection(page);await page.locator('#diagnostics > summary').click();expect(calls).toEqual([]);
 await page.route('**/api/tunnel-selected',r=>r.fulfill({json:{executableName:'tunnel-client',nextStep:'Metadata only.'}}));await page.locator('#tunnelPath').fill('/chosen/tunnel-client');await page.locator('#checkChosenTunnel').click();await expect(page.locator('#chosenTunnelStatus')).toContainText('NOT CHECKED');
 await page.route('**/api/tunnel-fingerprint',r=>r.fulfill({json:{ok:true,sha256:'a'.repeat(64)}}));await page.locator('#fingerprintTunnel').click();await expect(page.locator('#fingerprintStatus')).toContainText('Authenticity: NOT VERIFIED');
 await page.route('**/api/pick-tunnel-file',r=>r.fulfill({json:{cancelled:true}}));await page.locator('#browseTunnel').click();await expect(page.locator('#chosenTunnelStatus')).toContainText('cancelled');await expect(page.locator('#tunnelPath')).toHaveValue('/chosen/tunnel-client');await page.unroute('**/api/pick-tunnel-file');await page.locator('#browseTunnel').click();await expect(page.locator('#tunnelPath')).toHaveValue(/tunnel-client\.exe$/);await expect(page.locator('#fingerprintStatus')).toContainText('not calculated');
 await page.locator('#archivePath').fill('/chosen/release.zip');await page.locator('#manifestPath').fill('/chosen/SHA256SUMS.txt');await page.route('**/api/tunnel-archive-verify',r=>r.fulfill({json:{matches:false}}));await page.locator('#verifyArchive').click();await expect(page.locator('#archiveStatus')).toContainText('CHECKSUM MISMATCH');
 await page.locator('#bundlePath').fill('/chosen/bundle.json');await page.route('**/api/tunnel-provenance',r=>r.fulfill({json:{signatureVerified:true,identity:'publisher',releaseTag:'v1',sourceDigestPinned:false}}));await page.locator('#verifyProvenance').click();await expect(page.locator('#provenanceStatus')).toContainText('exact source commit NOT pinned');await expect(page.locator('#provenanceStatus')).toContainText('ChatGPT connection: NOT CHECKED');
 await page.locator('#sourceDigest').fill('b'.repeat(40));await expect(page.locator('#provenanceStatus')).toHaveText('NOT VERIFIED for these selections.');
});

test('late fingerprint and provenance results cannot certify different selections',async({page})=>{
 await connection(page);await page.locator('#diagnostics > summary').click();await page.locator('#tunnelPath').fill('/first/tunnel-client');let release;let started;const pending=new Promise(r=>started=r);
 await page.route('**/api/tunnel-fingerprint',async r=>{await new Promise(resolve=>{release=resolve;started()});await r.fulfill({json:{ok:true,sha256:'a'.repeat(64)}})});await page.locator('#fingerprintTunnel').click();await pending;await page.locator('#tunnelPath').fill('/second/tunnel-client');release();await expect(page.locator('#fingerprintTunnel')).toBeEnabled();await expect(page.locator('#fingerprintStatus')).not.toContainText('SHA-256:');
 await page.locator('#archivePath').fill('/first/a.zip');await page.locator('#bundlePath').fill('/first/b.json');let releaseProvenance;let requested;const wait=new Promise(r=>requested=r);await page.route('**/api/tunnel-provenance',async r=>{await new Promise(resolve=>{releaseProvenance=resolve;requested()});await r.fulfill({json:{signatureVerified:true,identity:'publisher',releaseTag:'old'}})});await page.locator('#verifyProvenance').click();await wait;await page.locator('#archivePath').fill('/second/a.zip');releaseProvenance();await expect(page.locator('#verifyProvenance')).toBeEnabled();await expect(page.locator('#provenanceStatus')).toHaveText('NOT VERIFIED for these selections.');
});

test('expired sessions explain recovery and never call protected APIs',async({page})=>{
 const calls=[];page.on('request',r=>{if(r.url().includes('/api/'))calls.push(r.url())});await page.goto('/');await expect(page.locator('#sessionExpired')).toBeVisible();await fillFolders(page);await page.locator('#generate').click();await expect(page.locator('#validationError')).toContainText('expired');expect(calls).toEqual([]);
});

test('all stages and the return overview remain accessible and fit narrow screens',async({page})=>{
 await prepare(page);for(const step of [1,2,3]){if(step>1)await page.locator('#nextStep').click();expect((await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa','wcag22aa']).analyze()).violations).toEqual([]);}
 await page.locator('#nextStep').click();expect((await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze()).violations).toEqual([]);
 await page.locator('#configureFresh').click();for(const width of [320,390,768,1024,1440]){await page.setViewportSize({width,height:900});expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false)}
 await page.setViewportSize({width:800,height:900});await page.evaluate(()=>document.documentElement.style.zoom='2');expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false);await page.emulateMedia({forcedColors:'active',reducedMotion:'reduce'});await expect(page.locator('#generate')).toBeVisible();
});
test('a delayed saved selection never replaces work already being entered',async({page})=>{
 let release;let arrived;const pending=new Promise(r=>arrived=r);
 await page.route('**/api/saved',async r=>{await new Promise(resolve=>{release=resolve;arrived()});await r.fulfill({json:{ok:true,exists:true,mode:'scoped',roots:[fixture.root],revision:'saved'}})});
 await open(page);await pending;await page.locator('#manualPaths').evaluate(e=>e.open=true);await page.locator('#paths').fill(fixture.second);release();await expect(page.locator('#overviewLink')).toBeVisible();await expect(page.locator('#panel-1')).toBeVisible();await expect(page.locator('#paths')).toHaveValue(fixture.second);
});

test('preference read and save errors stay visible without implying persistence',async({page})=>{
 await page.route('**/api/saved',r=>r.fulfill({status:503,body:'unavailable'}));await prepare(page);await expect(page.locator('#savedReadNotice')).toContainText('Existing data was not changed');
 await page.route('**/api/save',r=>r.fulfill({status:409,json:{ok:false,error:'Saved selection changed. Review again.'}}));await page.locator('#saveScope').click();await expect(page.locator('#saveFeedback')).toHaveText('Saved selection changed. Review again.');await page.locator('#brandHome').click();await expect(page.locator('#overviewStatus')).toHaveText('Configuration prepared');await expect(page.locator('#savedFolderList li')).toHaveCount(0);
});