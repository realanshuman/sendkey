import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const B=process.env.BASE;
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
const p = await browser.newPage({ viewport:{width:1280,height:900}, deviceScaleFactor:2 });
const errs=[]; p.on('pageerror',e=>errs.push(String(e)));
await p.goto(B+'/'); await p.waitForTimeout(500);
for (const [sel,name] of [['#features','features'],['#dev','dev']]) {
  await p.locator(sel).scrollIntoViewIfNeeded(); await p.waitForTimeout(300);
  await p.locator(sel).screenshot({ path:`${D}/rv-${name}.png` });
}
console.log('JS errors:', errs.length ? errs.join('; ') : 'none');
await browser.close();
