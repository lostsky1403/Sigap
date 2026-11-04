import { chromium } from '@playwright/test';
const B = 'http://127.0.0.1:5173';
const OUT = 'F:/sigap-design-v2/review-shots/evolution';
const b = await chromium.launch();

const p = await (await b.newContext({ viewport:{width:1440,height:900} })).newPage();
await p.goto(`${B}/admin/queues`, { waitUntil:'domcontentloaded' });
await p.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, { timeout: 30000 });
await p.waitForTimeout(3000);

// Count Panggil buttons in the Menunggu band.
const panggil = p.getByRole('button', { name: /Panggil/i });
const n = await panggil.count();
console.log('Panggil buttons:', n);
if (n === 0) { console.log('no waiting rows; aborting focus probe'); await b.close(); process.exit(0); }

// Record focus before mutation (should be body / not on a row button after idle).
console.log('focus before click:', await p.evaluate(() => document.activeElement?.textContent?.trim().slice(0,30) || document.activeElement?.tagName));

await panggil.first().click();
await p.waitForTimeout(1500);

const active = await p.evaluate(() => {
  const el = document.activeElement;
  if (!el) return 'none';
  if (el.tagName === 'BODY') return 'BODY';
  return `${el.tagName}:${(el.textContent||'').trim().slice(0,30) || el.getAttribute('aria-label')||el.id||el.className}`;
});
console.log('focus after Panggil:', active);

await p.screenshot({ path: `${OUT}/after-queue-1440.png` });

// 390 capture
const m = await (await b.newContext({ viewport:{width:390,height:844} })).newPage();
await m.goto(`${B}/admin/queues`, { waitUntil:'domcontentloaded' });
await m.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, { timeout:30000 });
await m.waitForTimeout(2500);
await m.screenshot({ path: `${OUT}/after-queue-390.png` });

// Beranda shots (masthead removed, chips full-height)
const h = await (await b.newContext({ viewport:{width:1440,height:900} })).newPage();
await h.goto(`${B}/`, { waitUntil:'domcontentloaded' });
await h.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, { timeout:30000 });
await h.waitForTimeout(1500);
console.log('masthead present (should be 0):', await h.locator('.sigap-beranda__masthead').count());
console.log('brand in header (should be >=1):', await h.locator('.sigap-citizen-header__brand').count());
await h.screenshot({ path: `${OUT}/after-beranda-1440.png` });
const h2 = await (await b.newContext({ viewport:{width:390,height:844} })).newPage();
await h2.goto(`${B}/`, { waitUntil:'domcontentloaded' });
await h2.waitForFunction(() => document.documentElement.dataset.sigapHydrated === 'true', null, { timeout:30000 });
await h2.waitForTimeout(1500);
await h2.screenshot({ path: `${OUT}/after-beranda-390.png` });

await b.close();
console.log('shots written');
