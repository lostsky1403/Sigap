/**
 * Contact sheet builder for the owner-review bundle.
 *
 * Lays the captured route screenshots into one image so the whole product can
 * be scanned in a single view instead of opened file by file.
 *
 * WHY AN HTML GRID INSTEAD OF AN IMAGE LIBRARY: composing with a native image
 * dependency would add a package for one build-time step. Playwright is already
 * a devDependency and can render a grid and screenshot it, so this stays
 * dependency-free.
 *
 * Not a test. It asserts nothing.
 *
 * Usage:
 *   node scripts/build-contact-sheet.mjs [shotsDir] [outFile]
 */
import { chromium } from '@playwright/test';
import { readdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const SHOTS = resolve(process.argv[2] ?? join(process.cwd(), '..', '..', 'review-shots'));
const OUT = resolve(process.argv[3] ?? join(SHOTS, 'contact-sheet.png'));

/** Desktop captures first, then tablet, then the two mobile widths. */
const ORDER = ['-1440.png', '-768.png', '-390.png', '-360.png'];

const files = readdirSync(SHOTS)
	.filter((f) => f.endsWith('.png') && !f.startsWith('contact-sheet'))
	.sort((a, b) => {
		const ao = ORDER.findIndex((s) => a.endsWith(s));
		const bo = ORDER.findIndex((s) => b.endsWith(s));
		if (ao !== bo) return ao - bo;
		return a.localeCompare(b);
	});

if (files.length === 0) {
	console.error(`no screenshots found in ${SHOTS}`);
	process.exit(1);
}

/*
 * Captions are the filename without the viewport suffix: the route name the
 * capture script derived, which is what a reviewer reads.
 *
 * Images are inlined as data URIs because `page.setContent` renders from
 * `about:blank`, where a relative `src` cannot resolve to a file on disk.
 * Serving the directory over HTTP would work too, but that is a server to
 * start and stop for one screenshot.
 */
const cells = files
	.map((f) => {
		const viewport = ORDER.find((s) => f.endsWith(s))?.replace(/[-.]/g, '').replace('png', '');
		const route = f.replace(/-\d+\.png$/, '');
		const data = readFileSync(join(SHOTS, f)).toString('base64');
		return `<figure><img src="data:image/png;base64,${data}" alt="${route} ${viewport}"><figcaption>${route}<span>${viewport}</span></figcaption></figure>`;
	})
	.join('\n');

const html = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>SIGAP review contact sheet</title>
<style>
  :root { color-scheme: light; }
  body { margin: 0; padding: 24px; background: #F7F9F8; font: 13px/1.4 system-ui, sans-serif; color: #17211F; }
  h1 { margin: 0 0 4px; font-size: 20px; }
  p.meta { margin: 0 0 20px; color: #5F6F6B; font-size: 12px; }
  .grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; }
  figure { margin: 0; background: #fff; border: 1px solid #E0E6E3; border-radius: 8px; overflow: hidden; }
  img { display: block; width: 100%; height: auto; border-bottom: 1px solid #E0E6E3; }
  figcaption { display: flex; justify-content: space-between; gap: 8px; padding: 6px 8px; font-size: 11px; }
  figcaption span { color: #5F6F6B; }
</style></head>
<body>
  <h1>SIGAP — review contact sheet</h1>
  <p class="meta">${files.length} captures · desktop 1440 · tablet 768 · mobile 390 / 360</p>
  <div class="grid">${cells}</div>
</body></html>`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1800, height: 1200 } });
await page.setContent(html, { waitUntil: 'networkidle' });
await page.screenshot({ path: OUT, fullPage: true });
await browser.close();

console.log(`contact sheet: ${OUT} (${files.length} captures)`);
