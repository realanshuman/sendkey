import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const B=process.env.BASE;
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
// full page, desktop light + phone
for (const [w,h,dsf,tag,theme] of [[1280,900,1,'desk','light'],[390,844,2,'phone','light'],[1280,900,1,'desk','dark']]) {
  const p = await browser.newPage({ viewport:{width:w,height:h}, deviceScaleFactor:dsf });
  await p.goto(B+'/');
  await p.evaluate(t=>document.documentElement.setAttribute('data-theme',t), theme);
  await p.waitForTimeout(600);
  await p.screenshot({ path:`${D}/full-${tag}-${theme}.png`, fullPage:true });
  await p.close();
}
// page length before vs after matters: the user asked repeatedly for a short page
const p = await browser.newPage({ viewport:{width:1280,height:900} });
await p.goto(B+'/'); await p.waitForTimeout(400);
const len = await p.evaluate(()=>document.documentElement.scrollHeight);
const secs = await p.evaluate(()=>[...document.querySelectorAll('body > section, body > header')].map(s=>s.id||s.className.split(' ')[0]));
console.log('page height:', len, 'px');
console.log('sections:', secs.join(' > '));
await browser.close();
