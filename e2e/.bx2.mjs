import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const B=process.env.BASE;
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
for (const [w,h,dsf,tag,theme] of [[1280,900,2,'desk','dark'],[390,844,2,'phone','light']]) {
  const p = await browser.newPage({ viewport:{width:w,height:h}, deviceScaleFactor:dsf });
  await p.goto(B+'/'); await p.evaluate(t=>document.documentElement.setAttribute('data-theme',t), theme);
  await p.waitForTimeout(500);
  if (tag==='desk') { await p.locator('#features').scrollIntoViewIfNeeded(); await p.waitForTimeout(300);
    await p.locator('#features').screenshot({ path:`${D}/bx-${tag}-${theme}.png` }); }
  else { await p.screenshot({ path:`${D}/bx-${tag}-${theme}.png`, fullPage:true }); }
  await p.close();
}
const p = await browser.newPage({ viewport:{width:1280,height:900} });
await p.goto(B+'/'); await p.waitForTimeout(400);
console.log('page height:', await p.evaluate(()=>document.documentElement.scrollHeight), 'px (was 5076 unboxed)');
await browser.close();
