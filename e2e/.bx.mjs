import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
const p = await browser.newPage({ viewport:{width:1280,height:900} });
const errs=[]; p.on('pageerror',e=>errs.push(String(e)));
p.on('console',m=>{ if(m.type()==='error') errs.push(m.text()); });
await p.goto(process.env.BASE+'/'); await p.waitForTimeout(600);
// structural sanity: nothing orphaned by the surgery
const chk = await p.evaluate(() => ({
  wins: document.querySelectorAll('.section-win').length,
  burn: !!document.querySelector('#burn .burn-pane'),
  grids: document.querySelectorAll('.section-win .hgrid').length,
  faq: document.querySelectorAll('.section-win .faq details').length,
  dev: !!document.querySelector('.section-win .dev-grid .terminal'),
  strayEyebrow: document.querySelectorAll('.section-head .eyebrow').length,
  ovf: document.documentElement.scrollWidth > window.innerWidth,
}));
console.log(JSON.stringify(chk, null, 0));
console.log('JS errors:', errs.length ? errs.join('; ') : 'none');
await p.screenshot({ path:`${D}/boxed-full.png`, fullPage:true });
await browser.close();
