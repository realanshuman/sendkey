import { chromium } from 'playwright';
const D='/tmp/claude-0/-home-user-sendkey/49739a7f-3a7b-529f-ac9d-a5407ace6fac/scratchpad/mono';
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
const p = await browser.newPage({ viewport:{width:1200,height:1400}, deviceScaleFactor:2 });
await p.goto('file://'+D+'/preview.html'); await p.waitForTimeout(700);
await p.screenshot({ path: `${D}/sheet.png`, fullPage:true });
await browser.close();
