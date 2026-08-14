// Live burn receipts: the sender's tab learns when their link was opened,
// with no reload, while the recipient's burn UX is untouched.
import { chromium } from 'playwright';

const B = process.env.BASE || 'http://127.0.0.1:8404';
let fails = 0;
const check = (n, ok, x = '') => { console.log(`${ok ? '  PASS' : '  FAIL'}  ${n}${x ? ' — ' + x : ''}`); if (!ok) fails++; };
const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || undefined,
  args: ['--no-sandbox'],
});

// --- sender creates a link and starts watching ---
const sender = await browser.newPage();
const errs = [];
sender.on('pageerror', (e) => errs.push(String(e)));
sender.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

await sender.goto(B + '/send');
check('receipt panel hidden before a link exists', await sender.locator('#receiptPanel').isHidden());

await sender.fill('#secret', 'receipt-e2e payload');
await sender.click('#go');
await sender.waitForSelector('#compose-result:not(.hidden)', { timeout: 10000 });
const link = await sender.inputValue('#link');

await sender.waitForSelector('#receiptPanel:not(.hidden)', { timeout: 10000 });
check('receipt panel appears with the link', true);
check('starts in the waiting state',
  (await sender.textContent('#receiptText')).includes('Waiting'),
  await sender.textContent('#receiptText'));

// --- a different browser context opens it ---
const recipient = await (await browser.newContext()).newPage();
await recipient.goto(link);
await recipient.waitForSelector('#gate:not(.hidden)');
await recipient.click('#reveal');
await recipient.waitForSelector('#out:not(.hidden)', { timeout: 10000 });
check('recipient still sees the secret', (await recipient.textContent('#secret')).trim() === 'receipt-e2e payload');

// --- the sender's ORIGINAL tab updates itself, no reload ---
await sender.waitForFunction(
  () => document.getElementById('receiptText').textContent.startsWith('Opened at'),
  null, { timeout: 20000 });
const receiptText = (await sender.textContent('#receiptText')).trim();
check('sender tab shows the open time live', /^Opened at .+/.test(receiptText), receiptText);
check('status dot marked arrived', await sender.locator('#receiptStatus.arrived').count() === 1);
check('no JS errors on the sender page', errs.length === 0, errs.join('; '));

// --- burn semantics unchanged for everyone else ---
const third = await (await browser.newContext()).newPage();
await third.goto(link);
await third.waitForSelector('#gone:not(.hidden)', { timeout: 10000 });
check('link is still burned for the next visitor', true);

// --- multi-view counts up ---
const s2 = await browser.newPage();
await s2.goto(B + '/send');
await s2.fill('#secret', 'multi view');
await s2.selectOption('#views', '2');
await s2.click('#go');
await s2.waitForSelector('#compose-result:not(.hidden)', { timeout: 10000 });
const link2 = await s2.inputValue('#link');

const r2 = await (await browser.newContext()).newPage();
await r2.goto(link2);
await r2.waitForSelector('#gate:not(.hidden)');
await r2.click('#reveal');
await r2.waitForSelector('#out:not(.hidden)', { timeout: 10000 });

await s2.waitForFunction(
  () => document.getElementById('receiptText').textContent.includes('of 2 views'),
  null, { timeout: 20000 });
const multiText = (await s2.textContent('#receiptText')).trim();
check('multi-view receipt counts opens', /1 of 2 views used/.test(multiText), multiText);

// --- "send another" clears the panel ---
await s2.click('#again');
check('send another hides the receipt panel', await s2.locator('#receiptPanel').isHidden());

await browser.close();
console.log(fails === 0 ? '\nALL RECEIPT CHECKS PASSED' : `\n${fails} FAILED`);
process.exit(fails ? 1 : 0);
