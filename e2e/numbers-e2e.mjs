// End to end: the numbers page. Creates real traffic through the composer,
// then proves /numbers reflects it, stays honest about what it cannot count,
// and holds up on a phone.
import { chromium } from 'playwright';

const B = process.env.BASE || 'http://127.0.0.1:8404';
let fails = 0;
const check = (n, ok, x = '') => { console.log(`${ok ? '  PASS' : '  FAIL'}  ${n}${x ? ' — ' + x : ''}`); if (!ok) fails++; };

const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || undefined,
  args: ['--no-sandbox'] });

console.log('\n--- baseline, then traffic ---');
const before = await (await fetch(`${B}/api/stats`)).json();

const s = await browser.newPage();
await s.goto(B + '/');
await s.fill('#secret', 'numbers-e2e probe');
await s.selectOption('#views', '2');
await s.click('#go');
await s.waitForSelector('#compose-result:not(.hidden)', { timeout: 15000 });
const link = await s.inputValue('#link');
await s.close();

const v = await (await browser.newContext()).newPage();
await v.goto(link);
await v.click('#reveal');
await v.waitForSelector('#out:not(.hidden)', { timeout: 15000 });
await v.close();

const after = await (await fetch(`${B}/api/stats`)).json();
check('created advanced by one', after.created === before.created + 1,
  `${before.created} -> ${after.created}`);
check('opened advanced by one', after.opened === before.opened + 1);
check('a 2-view link burned nothing', after.burned === before.burned,
  `${before.burned} -> ${after.burned}`);
check('sealed grew', after.sealed > before.sealed);
check('ttl buckets sum to created',
  after.ttlHour + after.ttlDay + after.ttlWeek === after.created);
check('view buckets sum to created',
  after.views1 + after.views2 + after.views5 === after.created);

console.log('\n--- the page reflects the counters ---');
const p = await browser.newPage({ viewport: { width: 1280, height: 1000 } });
const errs = [];
p.on('pageerror', (e) => errs.push(String(e)));
p.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });
await p.goto(B + '/numbers');
await p.waitForSelector('.num-status.arrived', { timeout: 10000 });
await p.waitForTimeout(900); // let the count-up land

const human = (n) => n < 1000 ? String(n)
  : n < 1_000_000 ? `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`
  : `${(n / 1_000_000).toFixed(1)}m`;
check('created total rendered', (await p.textContent('#nCreated')).trim() === human(after.created),
  await p.textContent('#nCreated'));
check('opened total rendered', (await p.textContent('#nOpened')).trim() === human(after.opened));
check('since line states the window', /Counting since/.test(await p.textContent('#sinceLine')));
check('honesty panel present', (await p.textContent('.num-cant')).includes('cannot read'));
check('no JS errors on the page', errs.length === 0, errs.join('; '));

const widths = await p.$$eval('#barViews [data-seg]', (els) => els.map((e) => e.style.width));
check('view dial has segment widths', widths.every((w) => /%$/.test(w)), widths.join(' '));

console.log('\n--- phone ---');
const m = await browser.newPage({ viewport: { width: 390, height: 844 } });
await m.goto(B + '/numbers');
await m.waitForSelector('.num-status.arrived', { timeout: 10000 });
const mm = await m.evaluate(() => ({
  ovf: document.documentElement.scrollWidth > window.innerWidth,
  stacked: getComputedStyle(document.querySelector('.num-wall')).gridTemplateColumns.split(' ').length === 1,
}));
check('no horizontal overflow at 390px', !mm.ovf);
check('stat wall stacks to one column', mm.stacked);
await m.close();

await browser.close();
console.log(fails === 0 ? '\nALL NUMBERS CHECKS PASSED' : `\n${fails} FAILED`);
process.exit(fails ? 1 : 0);
