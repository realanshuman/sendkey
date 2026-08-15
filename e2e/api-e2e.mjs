// The API reference page and the cross-origin contract it documents. The Go
// suite already proves the headers; this proves what only a browser can:
// that a page on a foreign origin can really drive the whole store-and-burn
// cycle with fetch, and that the docs page itself renders and copies.
import { chromium } from 'playwright';

const BASE = process.env.BASE || 'http://127.0.0.1:8392';
let failures = 0;

function check(name, ok, extra = '') {
  console.log(`${ok ? '  PASS' : '  FAIL'}  ${name}${extra ? ' — ' + extra : ''}`);
  if (!ok) failures++;
}

const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || undefined,
  args: ['--no-sandbox'],
});

console.log('\n--- the reference page ---');
const ctx = await browser.newContext({ permissions: ['clipboard-read', 'clipboard-write'] });
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(String(e)));
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });

await page.goto(BASE + '/api');
check('page titled as the API reference', (await page.title()).includes('HTTP API'));

const body = await page.textContent('main');
for (const ep of ['/api/secret', '/api/secret/{id}/meta', '/api/secret/{id}/receipt', '/healthz']) {
  check(`documents ${ep}`, body.includes(ep));
}

await page.click('[data-copy="cmdGet"]');
const clip = await page.evaluate(() => navigator.clipboard.readText());
check('copy button yields a runnable command', clip.startsWith('curl '), clip.slice(0, 40));
check('shell prompt kept out of the clipboard', !clip.includes('$'));
check('no JS errors on the page', errors.length === 0, errors.join('; '));

console.log('\n--- the API link is in every footer ---');
for (const path of ['/', '/send', '/ask', '/api']) {
  const p = await ctx.newPage();
  await p.goto(BASE + path);
  const hasLink = await p.locator('footer a[href="/api"]').count();
  check(`footer on ${path} links the API`, hasLink > 0);
  await p.close();
}

console.log('\n--- a foreign origin can use the API ---');
// A browser context parked on an origin that is not the server: about:blank
// pages get an opaque origin, so these fetches are cross-origin in the
// fullest sense. Without the CORS headers every one of them would throw.
const foreign = await browser.newPage();
const result = await foreign.evaluate(async (base) => {
  const enc = (b) => btoa(String.fromCharCode(...b))
    .replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const ct = crypto.getRandomValues(new Uint8Array(48)); // opaque bytes are enough here
  const made = await (await fetch(base + '/api/secret', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ct: enc(ct), iv: enc(iv), ttl: 3600, views: 1 }),
  })).json();
  const meta = await (await fetch(`${base}/api/secret/${made.id}/meta`)).json();
  const got = await (await fetch(`${base}/api/secret/${made.id}`)).json();
  const gone = (await fetch(`${base}/api/secret/${made.id}`)).status;
  const receipt = await (await fetch(`${base}/api/secret/${made.id}/receipt`)).json();
  return {
    id: made.id || '',
    views: meta.views,
    roundTrip: got.ct !== undefined && got.iv !== undefined,
    gone,
    opens: (receipt.opens || []).length,
  };
}, BASE);

check('cross-origin store returns an id', /^[\w-]{22}$/.test(result.id));
check('cross-origin meta readable', result.views === 1);
check('cross-origin consume returns the ciphertext', result.roundTrip);
check('second consume is 404', result.gone === 404, `status ${result.gone}`);
check('receipt records the open', result.opens === 1);

await browser.close();

if (failures > 0) {
  console.error(`\n${failures} CHECK(S) FAILED`);
  process.exit(1);
}
console.log('\nALL API CHECKS PASSED');
