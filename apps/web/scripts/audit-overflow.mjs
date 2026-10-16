/**
 * DOM overflow audit.
 *
 * Measures the rendered page instead of reading a screenshot. A screenshot
 * answers "does this look wrong"; this answers "which element is wider than its
 * container", which is the fact that decides whether a defect is real and what
 * causes it.
 *
 * Reports, per route and viewport:
 *   - document scrollWidth vs clientWidth (page-level horizontal overflow)
 *   - every element whose scrollWidth exceeds clientWidth by more than 1px
 *   - every element whose right edge passes the viewport's right edge
 *   - text elements whose scrollHeight exceeds clientHeight (clipped text)
 *
 * Not a test. It asserts nothing and is not part of the suite; it exists so the
 * layout claims have measurements behind them.
 *
 * Usage: node scripts/audit-overflow.mjs [baseURL]
 */
import { chromium } from '@playwright/test';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';

const VIEWPORTS = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '768', width: 768, height: 1024 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

const ROUTES = [
	['/', 'beranda'],
	['/faskes', 'faskes'],
	['/appointments/new', 'booking'],
	['/appointments/check-in', 'check-in'],
	['/queues/new', 'walk-in'],
	['/patient/status', 'status'],
	['/auth/login', 'login'],
	['/auth/register', 'register'],
	['/auth/logout', 'logout'],
	['/admin', 'admin-ringkasan'],
	['/admin/queues', 'admin-antrean'],
	['/admin/appointments', 'admin-janji-temu'],
	['/admin/schedules', 'admin-jadwal'],
	['/admin/facilities', 'admin-fasilitas'],
	['/admin/notifications', 'admin-notifikasi']
];

const browser = await chromium.launch();
let findings = 0;

for (const vp of VIEWPORTS) {
	const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await context.newPage();

	for (const [route, label] of ROUTES) {
		await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 30000 }).catch(() => {});
		await page.waitForTimeout(700);

		const report = await page.evaluate((vw) => {
			const describe = (el) => {
				const cls = typeof el.className === 'string' ? el.className.split(/\s+/).slice(0, 2).join('.') : '';
				const id = el.id ? `#${el.id}` : '';
				return `${el.tagName.toLowerCase()}${id}${cls ? '.' + cls : ''}`;
			};

			/*
				A visually-hidden element is 1px tall with its content clipped on
				purpose, and it is how every icon-only control here keeps an
				accessible name. Measuring it as "clipped text" would flag the
				technique rather than a defect.
			*/
			const isVisuallyHidden = (el) => {
				const r = el.getBoundingClientRect();
				return r.width <= 1 || r.height <= 1;
			};

			/*
				An element inside a horizontal scroller is SUPPOSED to extend past
				its container — that is what the scroller is for. Walking up to
				find one is what separates "the table scrolls" from "the page
				scrolls sideways".
			*/
			const insideScroller = (el) => {
				for (let p = el.parentElement; p && p !== document.body; p = p.parentElement) {
					const ox = getComputedStyle(p).overflowX;
					if (ox === 'auto' || ox === 'scroll') return true;
				}
				return false;
			};

			const overflowing = [];
			const pastRight = [];
			const clipped = [];

			for (const el of document.querySelectorAll('body *')) {
				const style = getComputedStyle(el);
				if (style.display === 'none' || style.visibility === 'hidden') continue;
				const r = el.getBoundingClientRect();
				if (r.width === 0 || r.height === 0) continue;
				if (isVisuallyHidden(el)) continue;

				// Horizontal overflow inside a scroll container is legitimate only
				// when the element is itself allowed to scroll.
				const canScrollX = ['auto', 'scroll'].includes(style.overflowX);
				if (!canScrollX && el.scrollWidth > el.clientWidth + 1) {
					overflowing.push({
						el: describe(el),
						scrollWidth: el.scrollWidth,
						clientWidth: el.clientWidth,
						delta: el.scrollWidth - el.clientWidth
					});
				}

				if (r.right > vw + 1 && !insideScroller(el)) {
					pastRight.push({ el: describe(el), right: Math.round(r.right), over: Math.round(r.right - vw) });
				}

				// Clipped text: a fixed-height box whose content does not fit.
				const hasText = (el.textContent ?? '').trim().length > 0;
				const onlyChildText = Array.from(el.children).every(
					(c) => (c.textContent ?? '').trim().length > 0
				);
				if (
					hasText &&
					onlyChildText &&
					style.overflow !== 'visible' &&
					style.overflowY !== 'visible' &&
					el.scrollHeight > el.clientHeight + 1
				) {
					clipped.push({
						el: describe(el),
						scrollHeight: el.scrollHeight,
						clientHeight: el.clientHeight
					});
				}
			}

			return {
				docScrollWidth: document.documentElement.scrollWidth,
				docClientWidth: document.documentElement.clientWidth,
				overflowing: overflowing.slice(0, 12),
				pastRight: pastRight.slice(0, 12),
				clipped: clipped.slice(0, 12)
			};
		}, vp.width);

		const pageOverflow = report.docScrollWidth - report.docClientWidth;
		const hasIssue =
			pageOverflow > 1 || report.pastRight.length > 0 || report.clipped.length > 0;

		if (hasIssue) {
			findings++;
			console.log(`\n### ${vp.name} ${route} (${label})`);
			if (pageOverflow > 1) {
				console.log(`  PAGE OVERFLOW: scrollWidth ${report.docScrollWidth} > clientWidth ${report.docClientWidth}`);
			}
			for (const o of report.pastRight) console.log(`  PAST RIGHT  +${o.over}px  ${o.el}`);
			for (const c of report.clipped) console.log(`  CLIPPED     ${c.el} (${c.scrollHeight}>${c.clientHeight})`);
			for (const o of report.overflowing) console.log(`  INNER OVF   +${o.delta}px  ${o.el}`);
		}
	}

	await context.close();
}

await browser.close();
console.log(`\n${findings} route/viewport combinations with layout findings`);
