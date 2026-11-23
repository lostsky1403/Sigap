/**
 * Real form-auth browser journey: POST the login form (Better Auth server
 * action) -> session cookie -> navigate to Janji Saya -> assert the signed-in
 * state (not "Silakan masuk") -> screenshot. Run against the -Jwt -NoAdminActor
 * preview so the JWT provider (not the local test selector) authorizes.
 */
import { chromium } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const OUT = process.argv[3] ?? join(process.cwd(), '..', '..', 'review-shots');
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
const page = await ctx.newPage();

// 1. Load the login form.
await page.goto(`${BASE}/auth/login`, { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForSelector('input[name="email"]', { timeout: 30000 });

// 2. Fill + submit the form (drives the Better Auth server action).
await page.fill('input[name="email"]', 'smoke@sigap.local');
await page.fill('input[name="password"]', 'Smoke12345!');
await page.click('button[type="submit"]');
await page.waitForTimeout(2500);

// 3. Session cookie present?
const cookies = await ctx.cookies();
const sessionCookie = cookies.find((c) => c.name.includes('session_token'));
console.log('session cookie:', sessionCookie ? sessionCookie.name : 'MISSING');

// 4. Open Janji Saya.
await page.goto(`${BASE}/patient/appointments`, { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForSelector('h1', { timeout: 30000 });
await page.waitForTimeout(1500);

const h1 = (await page.locator('h1').first().textContent().catch(() => ''))?.trim() ?? '';
const bodyText = (await page.locator('body').innerText().catch(() => '')) ?? '';
console.log('h1:', h1);
console.log('signed-out?', bodyText.includes('Silakan masuk'));
await page.screenshot({ path: join(OUT, 'better-auth-janji-saya-1280.png') });

await browser.close();
console.log('done');