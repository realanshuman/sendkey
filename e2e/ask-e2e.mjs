import { chromium } from 'playwright';
const B = process.env.BASE || 'http://127.0.0.1:8404';
const ANSWER = 'ask-e2e: postgres://prod:sw0rdf1sh@db.internal:5432';
let fails = 0;
const check = (n, ok, x = '') => { console.log(`${ok ? '  PASS' : '  FAIL'}  ${n}${x ? ' — ' + x : ''}`); if (!ok) fails++; };
const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || undefined,
  args: ['--no-sandbox'] });

// requester context: create the ask
const ctxA = await browser.newContext();
const reqs = [];
ctxA.on('request', r => reqs.push(r.url()));
const a = await ctxA.newPage();
const errsA = [];
a.on('pageerror', e => errsA.push(String(e)));
a.on('console', m => { if (m.type() === 'error') errsA.push(m.text()); });
await a.goto(B + '/ask');
await a.waitForSelector('#create:not(.hidden)');
await a.click('#askCreate');
await a.waitForSelector('#owner:not(.hidden)', { timeout: 10000 });
const link = await a.inputValue('#askLink');
check('ask link created', /\/a\/[\w-]{22}#[\w-]{87}$/.test(link), link.slice(0, 70) + '…');
check('URL rewritten to the ask id', a.url().includes('/a/'));
const ownerIconPixels = await a.evaluate(() => document.querySelectorAll('#ownerIcon rect').length);
check('owner fingerprint rendered', ownerIconPixels > 0, `${ownerIconPixels} px`);

// responder context: a different browser answers
const ctxB = await browser.newContext();
const b = await ctxB.newPage();
const errsB = [];
b.on('pageerror', e => errsB.push(String(e)));
b.on('console', m => { if (m.type() === 'error') errsB.push(m.text()); });
await b.goto(link);
await b.waitForSelector('#respond:not(.hidden)', { timeout: 10000 });
check('responder view shown to a stranger', true);
const respIconPixels = await b.evaluate(() => document.querySelectorAll('#respIcon rect').length);
check('responder sees a fingerprint', respIconPixels > 0);
const sameIcon = await Promise.all([
  a.evaluate(() => document.getElementById('ownerIcon').innerHTML),
  b.evaluate(() => document.getElementById('respIcon').innerHTML),
]);
check('fingerprints match on both sides', sameIcon[0] === sameIcon[1]);

await b.fill('#ansSecret', ANSWER);
await b.click('#ansGo');
await b.waitForSelector('#answered:not(.hidden)', { timeout: 10000 });
check('answer delivered', true);
check('no JS errors on responder', errsB.length === 0, errsB.join('; '));

// a third visitor now finds the ask closed
const ctxC = await browser.newContext();
const c = await ctxC.newPage();
await c.goto(link);
await c.waitForSelector('#dead:not(.hidden)', { timeout: 10000 });
const deadMsg = await c.textContent('#deadMsg');
check('second responder is refused', /already been answered/.test(deadMsg), deadMsg.trim());

// requester's page notices the answer via polling and opens it
await a.waitForSelector('#askReveal:not(.hidden)', { timeout: 15000 });
check('owner page noticed the answer', true);
await a.click('#askReveal');
await a.waitForSelector('#opened:not(.hidden)', { timeout: 10000 });
const shown = (await a.textContent('#askSecret')).trim();
check('answer decrypts to the exact secret', shown === ANSWER, shown.slice(0, 40));
check('no JS errors on owner', errsA.length === 0, errsA.join('; '));

// the private key must never appear in any request this context made
const privLeak = reqs.filter(u => u.includes('"d"') || u.includes(link.split('#')[1]));
check('no key material in any request URL', privLeak.length === 0, privLeak.join(', '));

// mailbox burned after the read
const mailboxGone = await a.evaluate(async () => {
  const id = location.pathname.slice(3);
  const enc = new TextEncoder().encode('sendkey/ask/v1|');
  const rid = Uint8Array.from(atob(id.replace(/-/g, '+').replace(/_/g, '/')), ch => ch.charCodeAt(0));
  const buf = new Uint8Array(enc.length + rid.length);
  buf.set(enc); buf.set(rid, enc.length);
  const dig = new Uint8Array(await crypto.subtle.digest('SHA-256', buf)).subarray(0, 16);
  let s = ''; for (const x of dig) s += String.fromCharCode(x);
  const mb = btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return (await fetch(`/api/secret/${mb}/meta`)).status;
});
check('mailbox burned after reading', mailboxGone === 404, `status ${mailboxGone}`);

// screenshots for the user
await a.setViewportSize({ width: 1440, height: 900 });
await browser.close();
console.log(fails === 0 ? '\nALL ASK CHECKS PASSED' : `\n${fails} FAILED`);
process.exit(fails ? 1 : 0);
