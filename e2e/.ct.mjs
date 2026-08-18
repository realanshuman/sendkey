import { chromium } from 'playwright';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
const lum=([r,g,b])=>{const f=c=>(c/=255)<=0.03928?c/12.92:Math.pow((c+0.055)/1.055,2.4);return .2126*f(r)+.7152*f(g)+.0722*f(b);};
const rgb=s=>s.match(/\d+/g).slice(0,3).map(Number);
let bad=0;
for (const theme of ['light','dark']) {
  const p = await browser.newPage({ viewport:{width:1280,height:900} });
  await p.goto(process.env.BASE+'/');
  await p.evaluate(t=>document.documentElement.setAttribute('data-theme',t), theme);
  await p.waitForTimeout(300);
  const r = await p.evaluate(()=>[...document.querySelectorAll('.win-title')].map(e=>{
    const c=getComputedStyle(e);
    return { t:e.textContent.trim().slice(0,18), fg:c.color, bg:c.backgroundColor };
  }));
  for (const x of r) {
    const ratio=(()=>{const [a,b]=[lum(rgb(x.fg)),lum(rgb(x.bg))].sort((m,n)=>n-m);return (a+0.05)/(b+0.05);})();
    const ok = ratio>=4.5; if(!ok) bad++;
    console.log(`${ok?'PASS':'FAIL'}  ${theme.padEnd(5)} "${x.t}" ${ratio.toFixed(2)}:1`);
  }
  await p.close();
}
await browser.close();
console.log(bad?`\n${bad} unreadable titles`:'\nevery window title readable in both themes');
