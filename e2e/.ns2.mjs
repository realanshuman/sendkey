import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
for (const [w,tag,theme] of [[1280,'desk','dark'],[390,'phone','light'],[320,'se','light']]) {
  const p = await browser.newPage({ viewport:{width:w,height:1100}, deviceScaleFactor:2 });
  await p.goto(process.env.BASE+'/numbers');
  await p.evaluate(t=>document.documentElement.setAttribute('data-theme',t), theme);
  await p.waitForTimeout(1300);
  const ovf = await p.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth);
  console.log(`${tag} ${theme}: ${ovf?'OVERFLOW':'no overflow'}`);
  await p.screenshot({ path:`${D}/num2-${tag}-${theme}.png`, fullPage:true });
  await p.close();
}
await browser.close();
