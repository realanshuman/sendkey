import { chromium } from 'playwright';

const BASE = process.env.BASE || 'http://127.0.0.1:8392';
const SECRET = 'browser-e2e: sk-live-9f8a7b6c5d4e3f2a1b';
const PASS = 'open sesame';
let failures = 0;

function check(name, ok, extra = '') {
  console.log(`${ok ? '  PASS' : '  FAIL'}  ${name}${extra ? ' — ' + extra : ''}`);
  if (!ok) failures++;
}

const browser = await chromium.launch({
  args: ['--no-sandbox'],
});
const ctx = await browser.newContext();

// Record every request the browser makes, so we can prove the key fragment
// is never transmitted.
const requests = [];
ctx.on('request', (r) => requests.push(r.url()));

const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(String(e)));
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });

console.log('\n--- compose page ---');
await page.goto(BASE + '/');
await page.fill('#secret', SECRET);
await page.click('#go');
await page.waitForSelector('#compose-result:not(.hidden)', { timeout: 10000 });

const link = await page.inputValue('#link');
check('link generated', /\/s\/[\w-]+#[\w-]{43}$/.test(link), link.slice(0, 60) + '…');
check('plaintext cleared from textarea', (await page.inputValue('#secret')) === '');
check('no JS errors on compose', errors.length === 0, errors.join('; '));

console.log('\n--- key never leaves the browser ---');
const key = link.split('#')[1];
const leaked = requests.filter((u) => u.includes(key));
check('key absent from every request URL', leaked.length === 0, leaked.join(', '));

// Also confirm the server-side stored blob does not contain the plaintext.
const id = link.split('/s/')[1].split('#')[0];
const meta = await (await fetch(`${BASE}/api/secret/${id}/meta`)).json();
check('server metadata has no plaintext', !JSON.stringify(meta).includes('sk-live'));

console.log('\n--- view page (burn on reveal) ---');
const p2 = await ctx.newPage();
const err2 = [];
p2.on('pageerror', (e) => err2.push(String(e)));
await p2.goto(link);
await p2.waitForSelector('#gate:not(.hidden)', { timeout: 10000 });
check('gate shown before burning', true);

await p2.click('#reveal');
await p2.waitForSelector('#out:not(.hidden)', { timeout: 10000 });
const shown = (await p2.textContent('#secret')).trim();
check('decrypted secret matches', shown === SECRET, shown);
check('key stripped from URL after reveal', !p2.url().includes('#'), p2.url());
check('no JS errors on view', err2.length === 0, err2.join('; '));

console.log('\n--- second visit is dead (burned) ---');
const p3 = await ctx.newPage();
await p3.goto(link);
await p3.waitForSelector('#gone:not(.hidden)', { timeout: 10000 });
check('burned link shows gone state', true);

console.log('\n--- passphrase flow ---');
const p4 = await ctx.newPage();
await p4.goto(BASE + '/');
await p4.fill('#secret', SECRET);
await p4.check('#usePass');
await p4.fill('#pass', PASS);
await p4.click('#go');
await p4.waitForSelector('#compose-result:not(.hidden)', { timeout: 10000 });
const plink = await p4.inputValue('#link');

const p5 = await ctx.newPage();
await p5.goto(plink);
await p5.waitForSelector('#gate:not(.hidden)', { timeout: 10000 });
await p5.click('#reveal');
await p5.waitForSelector('#passgate:not(.hidden)', { timeout: 10000 });
check('passphrase gate shown', true);

// Wrong passphrase must be retryable without re-fetching (already burned).
await p5.fill('#pass', 'wrong');
await p5.click('#unlock');
await p5.waitForSelector('#passerr:not(.hidden)', { timeout: 10000 });
check('wrong passphrase shows retryable error', true);

await p5.fill('#pass', PASS);
await p5.click('#unlock');
await p5.waitForSelector('#out:not(.hidden)', { timeout: 10000 });
const shown2 = (await p5.textContent('#secret')).trim();
check('retry after wrong passphrase succeeds', shown2 === SECRET, shown2);

await page.screenshot({ path: 'shot-compose.png', fullPage: true });
await p2.screenshot({ path: 'shot-revealed.png', fullPage: true });

await browser.close();
console.log(failures === 0 ? '\nALL BROWSER CHECKS PASSED' : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
