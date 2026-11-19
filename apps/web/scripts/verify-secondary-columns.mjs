/**
 * Verifies the secondary-column hiding at 768px, header AND cell together.
 *
 * The claim under test is that the Antrean table hides three columns below
 * 1280px, and that one of them leaks because its cell lacks the class its header
 * carries. That is only answerable from computed style, so this reads it.
 *
 * Not a test. Usage: node scripts/verify-secondary-columns.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const browser = await chromium.launch();

for (const width of [1440, 1280, 1279, 768]) {
	const context = await browser.newContext({ viewport: { width, height: 1000 } });
	const page = await context.newPage();
	await page.goto(BASE + '/admin/queues', { waitUntil: 'networkidle', timeout: 30000 }).catch(() => {});
	await page.waitForTimeout(1200);

	const r = await page.evaluate(() => {
		const table = document.querySelector('table');
		if (!table) return { note: 'no table' };
		const th = Array.from(table.querySelectorAll('thead th')).map((el) => ({
			label: el.textContent.trim(),
			shown: getComputedStyle(el).display !== 'none'
		}));
		const firstRow = table.querySelector('tbody tr');
		const td = firstRow
			? Array.from(firstRow.querySelectorAll('td')).map((el) => ({
					text: el.textContent.trim().slice(0, 18),
					shown: getComputedStyle(el).display !== 'none',
					cls: el.className
				}))
			: [];
		return { th, td };
	});

	const shownHeaders = r.th?.filter((h) => h.shown).length ?? 0;
	const shownCells = r.td?.filter((c) => c.shown).length ?? 0;
	console.log(
		`\n${width}px  headersShown=${shownHeaders}/${r.th?.length}  cellsShown=${shownCells}/${r.td?.length}` +
			(shownHeaders !== shownCells ? '   <-- MISMATCH' : '   (aligned)')
	);
	console.log('  headers:', r.th?.map((h) => `${h.label}${h.shown ? '' : '(hidden)'}`).join(' | '));
	console.log(
		'  cells  :',
		r.td?.map((c) => `${c.text}${c.shown ? '' : '(hidden)'}`).join(' | ')
	);

	await context.close();
}

await browser.close();
