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
  executablePath: process.env.CHROME_BIN || undefined,
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

console.log('\n--- burn demo (landing) ---');
const p6 = await ctx.newPage();
await p6.goto(BASE + '/');
const demoRequests = [];
p6.on('request', (r) => demoRequests.push(r.url()));
await p6.waitForSelector('.burn-pane[data-state="idle"].is-on', { timeout: 10000 });
await p6.click('[data-burn="run"]');
await p6.waitForSelector('.burn-pane[data-state="open"].is-on', { timeout: 10000 });
check('demo opens to staged plaintext',
  (await p6.textContent('.burn-plain')).includes('AKIA'));
await p6.waitForSelector('.burn-pane[data-state="dead"].is-on', { timeout: 10000 });
check('demo link ends dead',
  await p6.$eval('#burn', (el) => el.classList.contains('is-dead')));
const demoAPI = demoRequests.filter((u) => u.includes('/api/'));
check('demo touches no API (pure theater)', demoAPI.length === 0,
  demoAPI.join(', '));
await p6.click('[data-burn="again"]');
await p6.waitForSelector('.burn-pane[data-state="idle"].is-on', { timeout: 10000 });
check('demo resets to idle',
  await p6.$eval('#burn', (el) => !el.classList.contains('is-dead')));

// The hero exists to put the composer in front of people. Stacked under a
// full-height poster its submit button sat at y=927, below the fold on every
// laptop we measured, so the two-column layout is load-bearing rather than
// decorative and copy added to the hero can silently undo it.
console.log('\n--- hero keeps the product above the fold ---');
for (const [w, h, name] of [[1512, 860, 'macbook'], [1280, 720, 'small laptop']]) {
  const hp = await ctx.newPage();
  await hp.setViewportSize({ width: w, height: h });
  await hp.goto(BASE + '/');
  const m = await hp.evaluate(() => {
    const r = (s) => { const b = document.querySelector(s).getBoundingClientRect();
      return { top: b.top + scrollY, bottom: b.bottom + scrollY, left: b.left, right: b.right }; };
    return { go: r('#go'), proof: r('.hero-proof'), composer: r('.composer'),
             scrollWidth: document.documentElement.scrollWidth };
  });
  check(`${name}: submit button above the fold`, m.go.bottom <= h,
    `button ends at ${Math.round(m.go.bottom)}, viewport ${h}`);
  check(`${name}: two columns engaged`, m.proof.right <= m.composer.left,
    'proof list should sit left of the composer');
  check(`${name}: no horizontal overflow`, m.scrollWidth <= w, `${m.scrollWidth}`);
  await hp.close();
}
// Phones cannot fit the whole form, but the product itself must still be the
// thing you land on, and the proof list must not pad the preamble.
for (const [w, h, name] of [[390, 844, 'iphone'], [320, 650, 'small android']]) {
  const hp = await ctx.newPage();
  await hp.setViewportSize({ width: w, height: h });
  await hp.goto(BASE + '/');
  const m = await hp.evaluate(() => {
    const r = (s) => { const b = document.querySelector(s).getBoundingClientRect();
      return { top: b.top + scrollY, bottom: b.bottom + scrollY }; };
    return { ta: r('#secret'), proof: r('.hero-proof'), composer: r('.composer'),
             scrollWidth: document.documentElement.scrollWidth };
  });
  check(`${name}: secret field visible on landing`, m.ta.top < h,
    `field starts at ${Math.round(m.ta.top)}, viewport ${h}`);
  check(`${name}: proof list moved below the composer`,
    m.proof.top >= m.composer.bottom, 'it must not push the composer down');
  check(`${name}: no horizontal overflow`, m.scrollWidth <= w, `${m.scrollWidth}`);
  await hp.close();
}

// The hero ends on the same rhythm every other section boundary uses. Its
// two columns are different heights, so this only reads as padding when the
// shorter one is pinned to the bottom too; left top-aligned it stopped 188px
// short and the trailing space looked like a void instead.
console.log('\n--- hero closes on the section rhythm ---');
for (const [w, h, name] of [[1440, 900, 'desktop'], [920, 800, 'stack edge'], [390, 844, 'iphone']]) {
  const hp = await ctx.newPage();
  await hp.setViewportSize({ width: w, height: h });
  await hp.goto(BASE + '/');
  const m = await hp.evaluate(() => {
    const R = (s) => { const b = document.querySelector(s).getBoundingClientRect();
      return { top: b.top + scrollY, bottom: b.bottom + scrollY }; };
    const hero = R('.hero'), composer = R('.composer'), proof = R('.hero-proof');
    const feat = R('#features'), head = R('#features .section-head');
    return { trailing: Math.round(hero.bottom - Math.max(composer.bottom, proof.bottom)),
             next: Math.round(head.top - feat.top),
             ragged: Math.round(Math.abs(composer.bottom - proof.bottom)) };
  });
  check(`${name}: hero trailing space matches the next section`,
    m.trailing === m.next, `${m.trailing} vs ${m.next}`);
  if (w > 920) {
    check(`${name}: both columns end level`, m.ragged === 0, `${m.ragged}px apart`);
  }
  await hp.close();
}

await page.screenshot({ path: (process.env.TMPDIR || '/tmp') + '/shot-compose.png', fullPage: true });
await p2.screenshot({ path: (process.env.TMPDIR || '/tmp') + '/shot-revealed.png', fullPage: true });

await browser.close();
console.log(failures === 0 ? '\nALL BROWSER CHECKS PASSED' : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
