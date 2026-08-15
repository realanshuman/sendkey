import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad/mono';
const B=process.env.BASE;
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
for (const theme of ['light','dark']) {
  const p = await browser.newPage({ viewport:{width:1000,height:800}, deviceScaleFactor:3 });
  await p.goto(B+'/');
  await p.evaluate((t)=>document.documentElement.setAttribute('data-theme',t), theme);
  await p.waitForTimeout(400);
  await p.locator('.nav .logo').screenshot({ path:`${D}/nav-${theme}.png` });
  await p.locator('.footer .logo').scrollIntoViewIfNeeded(); await p.waitForTimeout(200);
  await p.locator('.footer .logo').screenshot({ path:`${D}/foot-${theme}.png` });
  await p.close();
}
const p = await browser.newPage({ viewport:{width:260,height:70}, deviceScaleFactor:8 });
await p.setContent(`<body style="margin:0;background:#f2efe9;display:flex;gap:16px;align-items:center;padding:14px">
<img src="${B}/assets/mark.svg" width="16" height="16">
<img src="${B}/assets/mark.svg" width="32" height="32">
<img src="${B}/assets/logo.svg" style="height:26px">
<span style="background:#141412;padding:8px;display:flex"><img src="${B}/assets/logo-inv.svg" style="height:26px"></span></body>`);
await p.waitForTimeout(700);
await p.screenshot({ path:`${D}/assets.png` });
await browser.close();
