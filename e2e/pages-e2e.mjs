// Dedicated /send and /drop pages: same composer, path-driven mode.
import { chromium } from 'playwright';
import { writeFileSync, readFileSync } from 'fs';
import { randomBytes } from 'crypto';

const B = process.env.BASE || 'http://127.0.0.1:8404';
let fails = 0;
const check = (n, ok, x = '') => { console.log(`${ok ? '  PASS' : '  FAIL'}  ${n}${x ? ' — ' + x : ''}`); if (!ok) fails++; };
const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || undefined,
  args: ['--no-sandbox'],
});

// /send: focused text flow
const p = await browser.newPage();
const errs = [];
p.on('pageerror', (e) => errs.push(String(e)));
p.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });
await p.goto(B + '/send');
check('/send shows the composer', await p.locator('#compose-form').isVisible());
check('/send headline', (await p.textContent('#pageTitle')).trim() === 'Send a secret');
await p.fill('#secret', 'from the dedicated page');
await p.click('#go');
await p.waitForSelector('#compose-result:not(.hidden)', { timeout: 10000 });
const link = await p.inputValue('#link');
check('/send creates a link', /\/s\/[\w-]+#[\w-]{43}$/.test(link));
check('no JS errors on /send', errs.length === 0, errs.join('; '));

const v = await (await browser.newContext()).newPage();
await v.goto(link);
await v.waitForSelector('#gate:not(.hidden)');
await v.click('#reveal');
await v.waitForSelector('#out:not(.hidden)');
check('link from /send opens', (await v.textContent('#secret')).trim() === 'from the dedicated page');

// /drop: file-first arrangement plus a real file round trip
const FILE = '/tmp/sk-pages-payload.bin';
const CONTENT = randomBytes(600_000);
writeFileSync(FILE, CONTENT);

const d = await browser.newPage();
const errsD = [];
d.on('pageerror', (e) => errsD.push(String(e)));
await d.goto(B + '/drop');
check('/drop retitles the page', (await d.textContent('#pageTitle')).trim() === 'Send a file');
const order = await d.evaluate(() => {
  const kids = [...document.getElementById('compose-form').children].map((el) => el.id);
  return kids.indexOf('dropzone') < kids.indexOf('secretWrap');
});
check('/drop promotes the drop strip above the textarea', order);
await d.setInputFiles('#fileInput', FILE);
await d.waitForSelector('#fileCard:not(.hidden)');
await d.click('#go');
await d.waitForSelector('#compose-result:not(.hidden)', { timeout: 30000 });
const flink = await d.inputValue('#link');
check('no JS errors on /drop', errsD.length === 0, errsD.join('; '));

const fv = await (await browser.newContext()).newPage();
await fv.goto(flink);
await fv.waitForSelector('#gate:not(.hidden)');
const dl = fv.waitForEvent('download', { timeout: 30000 });
await fv.click('#reveal');
const got = readFileSync(await (await dl).path());
check('file from /drop round trips', Buffer.compare(got, CONTENT) === 0);

await browser.close();
console.log(fails === 0 ? '\nALL PAGES CHECKS PASSED' : `\n${fails} FAILED`);
process.exit(fails ? 1 : 0);
