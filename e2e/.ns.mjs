import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
for (const [w,h,tag,theme] of [[1280,1200,'desk','light'],[1280,1200,'desk','dark'],[390,900,'phone','light']]) {
  const p = await browser.newPage({ viewport:{width:w,height:h}, deviceScaleFactor:2 });
  const errs=[]; p.on('pageerror',e=>errs.push(String(e)));
  await p.goto(process.env.BASE+'/numbers');
  await p.evaluate(t=>document.documentElement.setAttribute('data-theme',t), theme);
  await p.waitForTimeout(1400); // let the count-up land
  if (errs.length) console.log(tag, theme, 'JS ERRORS:', errs.join('; '));
  await p.screenshot({ path:`${D}/num-${tag}-${theme}.png`, fullPage:true });
  await p.close();
}
await browser.close(); console.log('shots done');
