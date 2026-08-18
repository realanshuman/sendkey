import { chromium } from 'playwright';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
console.log('width | mode rules | mode links | sec rules (row1/row2)');
let bad=0;
for (const w of [1512,1440,1280,1100,1024,960,921]) {
  const p = await browser.newPage({ viewport:{width:w,height:900} });
  await p.goto(process.env.BASE+'/'); await p.waitForTimeout(250);
  const m = await p.evaluate(() => {
    const T=(sel)=>[...document.querySelectorAll(sel)].map(e=>Math.round(e.getBoundingClientRect().top));
    const sec=T('.sec-cell .spec');
    return { mode:T('.mode-cell .spec'), link:T('.mode-link'), r1:sec.slice(0,3), r2:sec.slice(3,6) };
  });
  const u=(a)=>new Set(a).size<=1;
  const ok = u(m.mode) && u(m.r1) && u(m.r2);
  if(!ok) bad++;
  console.log(`${String(w).padEnd(6)}| ${(u(m.mode)?'aligned':'RAGGED '+m.mode).padEnd(11)}| ${(u(m.link.slice(0,3))?'aligned':'ragged').padEnd(11)}| ${u(m.r1)?'ok':'RAGGED '+m.r1} / ${u(m.r2)?'ok':'RAGGED '+m.r2}`);
  await p.close();
}
await browser.close();
console.log(bad?`\n${bad} widths ragged`:'\nevery spec rule aligned across its row');
