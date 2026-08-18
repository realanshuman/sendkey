import { chromium } from 'playwright';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
console.log('width | nav gap | page overflow | spec baselines aligned');
let bad=0;
for (const w of [1512,1440,1280,1100,1024,960,921,920,768,560,390,320]) {
  const p = await browser.newPage({ viewport:{width:w,height:900} });
  await p.goto(process.env.BASE+'/'); await p.waitForTimeout(220);
  const m = await p.evaluate((vw) => {
    const links=document.querySelector('.nav-links'), cta=document.querySelector('.nav-cta');
    const vis=getComputedStyle(links).display!=='none';
    const gap = vis ? Math.round(cta.getBoundingClientRect().left - links.getBoundingClientRect().right) : null;
    const tops=[...document.querySelectorAll('.mode-cell .spec')].map(e=>Math.round(e.getBoundingClientRect().top));
    return { gap, vis, ovf: document.documentElement.scrollWidth>vw+1,
             aligned: new Set(tops).size<=1, tops };
  }, w);
  const ok = !m.ovf && (!m.vis || m.gap>=12) && (w<=920 || m.aligned);
  if(!ok) bad++;
  console.log(`${String(w).padEnd(6)}| ${m.vis?String(m.gap).padEnd(8):'(hidden)'}| ${(m.ovf?'OVERFLOW':'ok').padEnd(14)}| ${w<=920?'(stacked)':(m.aligned?'yes':'NO '+m.tops)}`);
  await p.close();
}
await browser.close();
console.log(bad?`\n${bad} problem widths`:'\nnav and spec grid clean at every width');
