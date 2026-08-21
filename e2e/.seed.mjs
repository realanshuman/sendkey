import { chromium } from 'playwright';
const B = process.env.BASE;
// seed through the real composer so the counters see real traffic shapes
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN, args:['--no-sandbox'] });
const links = [];
for (const [ttl, views, n] of [['3600','1',3], ['86400','1',5], ['86400','2',2], ['604800','5',2]]) {
  for (let i = 0; i < n; i++) {
    const p = await browser.newPage();
    await p.goto(B + '/');
    await p.fill('#secret', `seed ${ttl}/${views}/${i} ${'x'.repeat(200 + i * 37)}`);
    await p.selectOption('#ttl', ttl);
    await p.selectOption('#views', views);
    await p.click('#go');
    await p.waitForSelector('#compose-result:not(.hidden)', { timeout: 15000 });
    links.push(await p.inputValue('#link'));
    await p.close();
  }
}
// open some: 6 opens, of which 5 burn (one was a 2-view link opened once)
for (const link of links.slice(0, 5)) {
  const p = await browser.newPage();
  await p.goto(link);
  await p.click('#reveal');
  await p.waitForSelector('#out:not(.hidden), #fileOut:not(.hidden)', { timeout: 15000 });
  await p.close();
}
await browser.close();
console.log('seeded', links.length, 'links, opened 5');
