import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
const p = await browser.newPage({ viewport:{width:390,height:844}, deviceScaleFactor:2 });
await p.goto(process.env.BASE+'/'); await p.waitForTimeout(400);
for (const [sel,name] of [['#features','feat'],['#dev','dev']]) {
  await p.locator(sel).scrollIntoViewIfNeeded(); await p.waitForTimeout(250);
  await p.locator(sel).screenshot({ path:`${D}/ph2-${name}.png` });
}
await browser.close();
