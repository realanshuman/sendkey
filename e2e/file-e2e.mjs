// End-to-end: encrypted file drop. Upload through the composer, download
// through the view page, byte-compare, verify burn semantics.
import { chromium } from 'playwright';
import { readFileSync, writeFileSync } from 'fs';
import { randomBytes } from 'crypto';

const B = process.env.BASE || 'http://127.0.0.1:8404';
let fails = 0;
const check = (n, ok, x = '') => { console.log(`${ok ? '  PASS' : '  FAIL'}  ${n}${x ? ' — ' + x : ''}`); if (!ok) fails++; };

const FILE = '/tmp/sk-e2e-payload.bin';
const CONTENT = randomBytes(1_300_000); // 3 chunks
writeFileSync(FILE, CONTENT);

const browser = await chromium.launch({ args: ['--no-sandbox'] });

// --- sender uploads a file ---
const s = await browser.newPage();
const errsS = [];
s.on('pageerror', (e) => errsS.push(String(e)));
s.on('console', (m) => { if (m.type() === 'error') errsS.push(m.text()); });
await s.goto(B + '/');
await s.setInputFiles('#fileInput', FILE);
await s.waitForSelector('#fileCard:not(.hidden)');
check('file card appears', (await s.textContent('#fName')).includes('sk-e2e-payload.bin'));
check('textarea hidden in file mode', await s.locator('#secretWrap').isHidden());
await s.click('#go');
await s.waitForSelector('#compose-result:not(.hidden)', { timeout: 30000 });
const link = await s.inputValue('#link');
check('file link generated', /\/s\/[\w-]+#[\w-]{43}$/.test(link), link.slice(0, 55) + '…');
check('no sender JS errors', errsS.length === 0, errsS.join('; '));

// --- recipient downloads ---
const ctx = await browser.newContext();
const v = await ctx.newPage();
const errsV = [];
v.on('pageerror', (e) => errsV.push(String(e)));
await v.goto(link);
await v.waitForSelector('#gate:not(.hidden)');
const dl = v.waitForEvent('download', { timeout: 30000 });
await v.click('#reveal');
await v.waitForSelector('#fileOut:not(.hidden)', { timeout: 30000 });
const download = await dl;
const path = await download.path();
const got = readFileSync(path);
check('downloaded bytes identical', Buffer.compare(got, CONTENT) === 0, `${got.length} bytes`);
check('suggested filename preserved', download.suggestedFilename() === 'sk-e2e-payload.bin', download.suggestedFilename());
await v.waitForSelector('#saveBtn:not(.hidden)');
check('save-again button present', true);
check('fragment stripped after open', !v.url().includes('#'));
check('no recipient JS errors', errsV.length === 0, errsV.join('; '));

// --- link burned for the next visitor ---
const p3 = await ctx.newPage();
await p3.goto(link);
await p3.waitForSelector('#gone:not(.hidden)', { timeout: 10000 });
check('second visit finds it burned', true);

// --- file + passphrase ---
const s2 = await browser.newPage();
await s2.goto(B + '/');
await s2.setInputFiles('#fileInput', FILE);
await s2.check('#usePass');
await s2.fill('#pass', 'open sesame');
await s2.click('#go');
await s2.waitForSelector('#compose-result:not(.hidden)', { timeout: 30000 });
const plink = await s2.inputValue('#link');

const v2 = await (await browser.newContext()).newPage();
await v2.goto(plink);
await v2.waitForSelector('#gate:not(.hidden)');
await v2.click('#reveal');
await v2.waitForSelector('#passgate:not(.hidden)', { timeout: 15000 });
check('passphrase gate shown for file', true);
const dl2 = v2.waitForEvent('download', { timeout: 30000 });
await v2.fill('#pass', 'open sesame');
await v2.click('#unlock');
await v2.waitForSelector('#fileOut:not(.hidden)', { timeout: 30000 });
const got2 = readFileSync(await (await dl2).path());
check('passphrase-protected file round trips', Buffer.compare(got2, CONTENT) === 0);

// --- text secrets unaffected (inline regression) ---
const t = await browser.newPage();
await t.goto(B + '/');
await t.fill('#secret', 'text still works');
await t.click('#go');
await t.waitForSelector('#compose-result:not(.hidden)', { timeout: 10000 });
const tlink = await t.inputValue('#link');
const tv = await (await browser.newContext()).newPage();
await tv.goto(tlink);
await tv.waitForSelector('#gate:not(.hidden)');
await tv.click('#reveal');
await tv.waitForSelector('#out:not(.hidden)');
check('text secret still renders as text', (await tv.textContent('#secret')).trim() === 'text still works');
check('file block stays hidden for text', await tv.locator('#fileOut').isHidden());

await browser.close();
console.log(fails === 0 ? '\nALL FILE CHECKS PASSED' : `\n${fails} FAILED`);
process.exit(fails ? 1 : 0);
