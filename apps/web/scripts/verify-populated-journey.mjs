import { chromium } from '@playwright/test';
const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
const page = await ctx.newPage();
await page.goto('http://127.0.0.1:5173/auth/login', { waitUntil: 'domcontentloaded' });
await page.waitForSelector('input[name="email"]');
await page.fill('input[name="email"]', 'smoke@sigap.local');
await page.fill('input[name="password"]', 'Smoke12345!');
await page.click('button[type="submit"]');
// Wait for the session cookie (the enhanced form applies it after the action 303).
await ctx.waitForEvent === undefined ? null : null;
for (let i = 0; i < 20; i++) {
  const c = await ctx.cookies();
  if (c.some(x => x.name.includes('session_token'))) break;
  await page.waitForTimeout(500);
}
const cookies = await ctx.cookies();
console.log('session cookie:', cookies.some(c => c.name.includes('session_token')) ? 'set' : 'MISSING');
await page.goto('http://127.0.0.1:5173/patient/appointments', { waitUntil: 'domcontentloaded' });
await page.waitForSelector('h1', { timeout: 30000 });
await page.waitForTimeout(1500);
const body = await page.locator('body').innerText();
console.log('body has "Silakan masuk":', body.includes('Silakan masuk'));
console.log('has facility A:', body.includes('RS Sehat Nusantara'));
console.log('has facility B:', body.includes('Puskesmas Prima'));
await page.screenshot({ path: '../../review-shots/better-auth-janji-saya-populated.png' });
await browser.close();
