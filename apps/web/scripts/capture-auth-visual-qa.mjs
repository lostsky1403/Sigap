/**
 * Auth + role-aware navigation visual-QA capture.
 *
 * Produces, at four widths (1440 desktop / 768 tablet / 390 + 360 mobile):
 *   1. /auth/login                    — signed out
 *   2. /auth/register                 — signed out
 *   3. /patient/appointments          — signed out, i.e. the "Silakan masuk" panel
 *   4. /admin                         — the role-aware admin shell/nav
 *
 * WHAT THIS HARNESS IS, AND IS NOT
 * --------------------------------
 * It is a geometry + artifact producer. For every shot it measures four things
 * and writes them down; it never inspects the PNGs it writes:
 *
 *   - documentElement.scrollWidth vs clientWidth  → is the page itself
 *     horizontally overflowing, and by how many pixels?
 *   - every element whose right edge passes the viewport, EXCLUDING elements
 *     that live inside a horizontally-scrollable ancestor (a table in its own
 *     scroll region is the intended design, not a defect)
 *   - the number of interactive controls whose box is smaller than 36px on
 *     either axis (the touch-target floor)
 *   - whether the view's primary action is inside the first viewport
 *
 * Judging the pixels — colour, contrast, hierarchy, whether the thing looks
 * right — is a human (or vision-capable) review step on the PNGs. This script
 * deliberately makes no claim about how the screens look.
 *
 * ADMIN SURFACE
 * -------------
 * In the dev-mode preview the web tier arms `SIGAP_LOCAL_E2E_ACTOR`
 * (default `local-global-super-admin`) and that subject is the local admin
 * actor — the Go API resolves its roles from the database per request. So the
 * `/admin` shot needs no sign-in form; what it needs is a server started with
 * that actor armed (`scripts/dev/Start-V22Preview.ps1`, no `-NoAdminActor`).
 * The capture asserts the dev banner naming that subject, so an unarmed server
 * is reported as such instead of silently producing an empty-but-pretty shell.
 *
 * Run:
 *   node scripts/capture-auth-visual-qa.mjs [baseUrl] [outDir]
 * Defaults: http://127.0.0.1:5173 and ../../review-shots/auth-qa
 */
import { chromium } from '@playwright/test';
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

const BASE = process.argv[2] ?? 'http://127.0.0.1:5173';
const OUT = process.argv[3] ?? join(process.cwd(), '..', '..', 'review-shots', 'auth-qa');
mkdirSync(OUT, { recursive: true });

const VIEWPORTS = [
	{ name: '1440', width: 1440, height: 900 },
	{ name: '768', width: 768, height: 900 },
	{ name: '390', width: 390, height: 844 },
	{ name: '360', width: 360, height: 780 }
];

/**
 * One entry per screen under test.
 *
 * `ready` is a selector that only exists once the screen has left its loading
 * state — waiting on `h1` alone would photograph a skeleton. `primary` names
 * the affordance the screen exists to offer, and is what "visible in the first
 * viewport" is measured against.
 */
const VIEWS = [
	{
		name: 'login',
		path: '/auth/login',
		ready: '[data-testid="auth-form"], [data-testid="auth-unavailable"]',
		primary: '[data-testid="auth-form"] button[type="submit"]',
		expectText: null
	},
	{
		name: 'register',
		path: '/auth/register',
		ready: '[data-testid="auth-form"], [data-testid="auth-unavailable"]',
		primary: '[data-testid="auth-form"] button[type="submit"]',
		expectText: null
	},
	{
		name: 'appointments-signed-out',
		path: '/patient/appointments',
		ready: '.sigap-unauth',
		primary: '.sigap-unauth__cta',
		expectText: 'Silakan masuk'
	},
	{
		name: 'admin',
		path: '/admin',
		ready: 'aside.sigap-admin-sidebar',
		primary: 'aside.sigap-admin-sidebar nav a',
		expectText: null
	}
];

/** Selector set for the touch-target census. Broad on purpose: a control is a control. */
const INTERACTIVE =
	'a[href], button, [role="button"], [role="link"], [role="tab"], input:not([type="hidden"]), select, textarea, summary, [tabindex]:not([tabindex="-1"])';

/**
 * Runs in the page. Returns only numbers and small descriptors, never DOM.
 */
function measure({ primarySelector, interactiveSelector }) {
	const de = document.documentElement;
	const vw = de.clientWidth;
	const vh = window.innerHeight;

	const label = (el) => {
		const cls =
			typeof el.className === 'string' && el.className.trim()
				? '.' + el.className.trim().split(/\s+/).slice(0, 2).join('.')
				: '';
		return `${el.tagName.toLowerCase()}${el.id ? '#' + el.id : ''}${cls}`;
	};

	// An element inside a scroll region that is actually scrolling is where the
	// author meant the overflow to go. Only a region that is scrolled matters:
	// `overflow-x: auto` on a container narrower than its content is the fix,
	// not the bug.
	const insideHorizontalScroller = (el) => {
		let p = el.parentElement;
		while (p && p !== de) {
			const cs = getComputedStyle(p);
			if (
				(cs.overflowX === 'auto' || cs.overflowX === 'scroll') &&
				p.scrollWidth > p.clientWidth + 1
			) {
				return true;
			}
			p = p.parentElement;
		}
		return false;
	};

	const clipped = [];
	for (const el of document.querySelectorAll('body *')) {
		const r = el.getBoundingClientRect();
		if (r.width <= 0 || r.height <= 0) continue;
		if (r.right <= vw + 1) continue;
		if (insideHorizontalScroller(el)) continue;
		clipped.push({
			el: label(el),
			right: Math.round(r.right),
			width: Math.round(r.width),
			overflowPx: Math.round(r.right - vw),
			text: (el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 60)
		});
	}

	let interactiveTotal = 0;
	const smallTargets = [];
	for (const el of document.querySelectorAll(interactiveSelector)) {
		const r = el.getBoundingClientRect();
		if (r.width <= 0 || r.height <= 0) continue;
		const cs = getComputedStyle(el);
		if (cs.display === 'none' || cs.visibility === 'hidden' || cs.opacity === '0') continue;
		interactiveTotal += 1;
		if (r.height < 36 || r.width < 36) {
			smallTargets.push({
				el: label(el),
				w: Math.round(r.width),
				h: Math.round(r.height),
				text: (el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 40)
			});
		}
	}

	const primaryEl = document.querySelector(primarySelector);
	let primary = { selector: primarySelector, found: false, visibleInFirstViewport: false, rect: null };
	if (primaryEl) {
		const r = primaryEl.getBoundingClientRect();
		const cs = getComputedStyle(primaryEl);
		const painted =
			r.width > 0 &&
			r.height > 0 &&
			cs.display !== 'none' &&
			cs.visibility !== 'hidden' &&
			cs.opacity !== '0' &&
			primaryEl.offsetParent !== null;
		primary = {
			selector: primarySelector,
			found: true,
			visibleInFirstViewport:
				painted && r.top >= -1 && r.top < vh && r.bottom > 0 && r.left < vw && r.right > 0,
			rect: {
				top: Math.round(r.top),
				left: Math.round(r.left),
				right: Math.round(r.right),
				bottom: Math.round(r.bottom),
				w: Math.round(r.width),
				h: Math.round(r.height)
			}
		};
	}

	const h1 = document.querySelector('h1');
	const navLinks = [...document.querySelectorAll('aside.sigap-admin-sidebar nav a')].map((a) => ({
		label: a.getAttribute('aria-label') || (a.textContent || '').trim(),
		href: a.getAttribute('href'),
		current: a.getAttribute('aria-current') || null
	}));

	return {
		title: document.title,
		h1: h1 ? (h1.textContent || '').trim() : null,
		scrollWidth: de.scrollWidth,
		clientWidth: de.clientWidth,
		overflowX: de.scrollWidth > de.clientWidth + 1,
		overflowPx: Math.max(0, de.scrollWidth - de.clientWidth),
		scrollHeight: de.scrollHeight,
		viewportHeight: vh,
		clippedCount: clipped.length,
		clipped: clipped.slice(0, 12),
		interactiveTotal,
		smallTargetCount: smallTargets.length,
		smallTargets: smallTargets.slice(0, 12),
		primary,
		expectedTextPresent: null,
		adminActorArmed: document.body.innerText.includes('local-global-super-admin'),
		navLinks
	};
}

const browser = await chromium.launch();
const shots = [];

for (const vp of VIEWPORTS) {
	const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
	const page = await ctx.newPage();
	page.setDefaultTimeout(45000);

	for (const view of VIEWS) {
		const file = `auth-qa-${view.name}-${vp.name}.png`;
		const url = `${BASE}${view.path}`;
		const record = {
			view: view.name,
			path: view.path,
			viewport: vp.name,
			width: vp.width,
			height: vp.height,
			url,
			file: join(OUT, file),
			// Every capture runs in a fresh context with no cookies. The admin
			// screen is reached without a session because the dev server arms
			// SIGAP_LOCAL_E2E_ACTOR; the other three are genuinely signed out.
			sessionState:
				view.name === 'admin'
					? 'no session; server-armed local actor (SIGAP_LOCAL_E2E_ACTOR)'
					: 'signed out',
			sessionCookiePresent: false,
			ready: false,
			error: null,
			expectText: view.expectText
		};

		try {
			await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60000 });
			try {
				await page.waitForSelector(view.ready, { timeout: 30000 });
				record.ready = true;
			} catch {
				record.error = `ready selector not found: ${view.ready}`;
			}
			await page.waitForTimeout(1200);
			await page.screenshot({ path: record.file, fullPage: false });

			// `measure` is passed by reference so Playwright serialises its own
			// source into the page. Wrapping it in a closure would evaluate a
			// call to a function that does not exist on the page side.
			const geom = await page.evaluate(measure, {
				primarySelector: view.primary,
				interactiveSelector: INTERACTIVE
			});
			const bodyText = await page
				.locator('body')
				.innerText()
				.catch(() => '');
			geom.expectedTextPresent = view.expectText
				? bodyText.includes(view.expectText)
				: null;
			record.geometry = geom;
		} catch (err) {
			record.error = `${record.error ? record.error + '; ' : ''}${err?.message ?? String(err)}`;
		}

		// Signed-out proof: the capture must not be carrying a session.
		const cookies = await ctx.cookies().catch(() => []);
		record.sessionCookiePresent = cookies.some((c) => c.name.includes('session_token'));
		shots.push(record);
	}

	await ctx.close();
}

await browser.close();

const report = {
	base: BASE,
	outDir: OUT,
	generatedAt: new Date().toISOString(),
	viewports: VIEWPORTS,
	views: VIEWS.map((v) => v.name),
	pixelReview: 'not performed by this harness — geometry and images only',
	shots
};
writeFileSync(join(OUT, 'report.json'), JSON.stringify(report, null, 2));

const pad = (v, n) => String(v).padEnd(n);
const padL = (v, n) => String(v).padStart(n);
console.log('');
console.log(
	`Auth visual QA — ${BASE} → ${OUT}\n` +
		'Geometry only: this harness measured the DOM and wrote PNGs. IT DID NOT INSPECT THE PIXELS.\n'
);
console.log(
	pad('view', 24) +
		padL('w', 5) +
		'  ' +
		pad('pageOverflow', 14) +
		padL('clipped', 8) +
		padL('smallCtl', 9) +
		padL('/total', 7) +
		'  ' +
		pad('primaryVisible', 15) +
		'notes'
);
console.log('-'.repeat(112));
for (const s of shots) {
	const g = s.geometry;
	const notes = [
		s.ready ? '' : 'NOT-READY',
		s.error ? s.error : '',
		g && g.expectedTextPresent === false ? `missing text "${s.expectText}"` : '',
		s.sessionCookiePresent ? 'SESSION COOKIE PRESENT' : ''
	]
		.filter(Boolean)
		.join('; ');
	console.log(
		pad(s.view, 24) +
			padL(s.viewport, 5) +
			'  ' +
			pad(g ? (g.overflowX ? `YES +${g.overflowPx}px` : 'no') : 'n/a', 14) +
			padL(g ? g.clippedCount : '-', 8) +
			padL(g ? g.smallTargetCount : '-', 9) +
			padL(g ? g.interactiveTotal : '-', 7) +
			'  ' +
			pad(g ? String(g.primary.visibleInFirstViewport) : 'n/a', 15) +
			notes
	);
}
console.log('-'.repeat(112));
console.log(`report: ${join(OUT, 'report.json')}`);
console.log('Pixels were NOT visually inspected here; that review is the owner\'s step.');
