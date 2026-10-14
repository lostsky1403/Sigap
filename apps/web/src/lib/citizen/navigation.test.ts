import { describe, expect, it, beforeEach } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, relative } from 'node:path';
import { CITIZEN_DESTINATIONS, isActiveDestination } from './navigation';
import CitizenHeader from './CitizenHeader.svelte';
import CitizenBottomNav from './CitizenBottomNav.svelte';
import CitizenDesktopNav from './CitizenDesktopNav.svelte';
import AccountMenu from './AccountMenu.svelte';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';

/**
 * The citizen shell contract.
 *
 * These tests exist because the shell is the one piece of the app that is on
 * every citizen page. A shell defect is not a one-page bug: it is a broken
 * tab bar, an unreachable destination, or a duplicated "you are here" marker
 * repeated across the whole product.
 *
 * The responsive rules are asserted in two ways, deliberately:
 *
 * 1. Rendered assertions, for the things jsdom can actually observe — the
 *    destination count, the `aria-current` markers, accessible names.
 * 2. Source assertions, for the things jsdom cannot — media queries, because
 *    jsdom does not evaluate CSS and a `display: none` toggle is invisible to
 *    it. Asserting "the bottom nav is hidden at 1440px" by rendering would
 *    pass no matter what the stylesheet said, which is a test that cannot fail.
 *
 * The source assertions are therefore the real check for the breakpoint split,
 * and they are written to fail if someone deletes the media query.
 */

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = join(here, '..', '..', '..');

const headerSource = readFileSync(join(here, 'CitizenHeader.svelte'), 'utf8');
const bottomNavSource = readFileSync(join(here, 'CitizenBottomNav.svelte'), 'utf8');
const desktopNavSource = readFileSync(join(here, 'CitizenDesktopNav.svelte'), 'utf8');
const accountSource = readFileSync(join(here, 'AccountMenu.svelte'), 'utf8');
const layoutSource = readFileSync(join(webRoot, 'src', 'routes', '+layout.svelte'), 'utf8');

describe('citizen navigation contract', () => {
	it('declares exactly four destinations', () => {
		expect(CITIZEN_DESTINATIONS).toHaveLength(4);
		expect(CITIZEN_DESTINATIONS.map((d) => d.label)).toEqual([
			'Beranda',
			'Faskes',
			'Check-In',
			'Status'
		]);
	});

	it('maps each destination to its canonical route', () => {
		const byLabel = Object.fromEntries(CITIZEN_DESTINATIONS.map((d) => [d.label, d.href]));
		expect(byLabel).toEqual({
			Beranda: '/',
			Faskes: '/faskes',
			'Check-In': '/appointments/check-in',
			Status: '/patient/status'
		});
	});

	it('carries no wallet, admin, role-switcher, or notification destination', () => {
		const hrefs = CITIZEN_DESTINATIONS.map((d) => d.href);
		for (const forbidden of ['/wallet', '/admin']) {
			expect(hrefs).not.toContain(forbidden);
		}
		for (const destination of CITIZEN_DESTINATIONS) {
			expect(destination.href).not.toContain('wallet');
			expect(destination.href).not.toContain('admin');
		}
	});

	it('gives every destination an accessible name that stands alone', () => {
		for (const destination of CITIZEN_DESTINATIONS) {
			expect(destination.ariaLabel.trim().length).toBeGreaterThan(0);
		}
		// "Check-In" alone is ambiguous, so it must be expanded.
		const checkIn = CITIZEN_DESTINATIONS.find((d) => d.label === 'Check-In');
		expect(checkIn?.ariaLabel).toBe('Check-In Janji Temu');
	});

	it('marks exactly one destination active per path', () => {
		for (const path of ['/', '/faskes', '/appointments/check-in', '/patient/status']) {
			const active = CITIZEN_DESTINATIONS.filter((d) => isActiveDestination(path, d.href));
			expect(active, `for ${path}`).toHaveLength(1);
		}
	});

	it('does not mark Beranda active on every route', () => {
		// A prefix match would make `/` current everywhere, because `/` is a
		// prefix of every string. This is the regression that rule guards.
		expect(isActiveDestination('/faskes', '/')).toBe(false);
		expect(isActiveDestination('/patient/status', '/')).toBe(false);
	});

	it('tolerates a trailing slash without double-matching', () => {
		expect(isActiveDestination('/faskes/', '/faskes')).toBe(true);
		expect(isActiveDestination('/', '/')).toBe(true);
	});
});

describe('citizen shell: mobile at 390px', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	it('renders the bottom navigation with exactly four links', async () => {
		render(CitizenBottomNav, { path: '/' });
		await tick();
		const nav = screen.getByRole('navigation', { name: 'Navigasi bawah' });
		const links = nav.querySelectorAll('a');
		expect(links).toHaveLength(4);
	});

	it('marks exactly one tab as the current page', async () => {
		render(CitizenBottomNav, { path: '/faskes' });
		await tick();
		const current = document.querySelectorAll('[aria-current="page"]');
		expect(current).toHaveLength(1);
		expect(current[0].getAttribute('href')).toBe('/faskes');
	});

	it('marks no tab current on an unrelated route', async () => {
		// A route outside the four destinations must not light up a tab, or the
		// bar would claim the citizen is somewhere they are not.
		render(CitizenBottomNav, { path: '/appointments/new' });
		await tick();
		expect(document.querySelectorAll('[aria-current="page"]')).toHaveLength(0);
	});

	it('gives every tab link an accessible name', async () => {
		render(CitizenBottomNav, { path: '/' });
		await tick();
		const links = Array.from(document.querySelectorAll('nav a'));
		for (const link of links) {
			const name =
				link.getAttribute('aria-label') ?? link.textContent?.trim() ?? '';
			expect(name.length, `tab ${link.getAttribute('href')}`).toBeGreaterThan(0);
		}
	});

	it('is reachable by keyboard as real links', async () => {
		render(CitizenBottomNav, { path: '/' });
		await tick();
		const links = Array.from(document.querySelectorAll('nav a'));
		for (const link of links) {
			// A real anchor with an href is focusable and activates with Enter.
			// A div with a click handler would be neither.
			expect(link.tagName).toBe('A');
			expect(link.getAttribute('href')).toBeTruthy();
			expect(link.hasAttribute('tabindex')).toBe(false);
		}
	});

	it('is fixed to the bottom with safe-area clearance reserved', () => {
		// jsdom does not evaluate CSS, so the responsive and positioning
		// guarantees are asserted against the source. Deleting the media query
		// or the safe-area padding fails here rather than silently in a browser.
		expect(bottomNavSource).toMatch(/position:\s*fixed/);
		expect(bottomNavSource).toMatch(/bottom:\s*0/);
		expect(bottomNavSource).toMatch(/env\(safe-area-inset-bottom/);
	});

	it('reserves content clearance so the bar cannot cover the page', () => {
		// The layout, not the nav, owns the reserved space: the bar is fixed, so
		// without padding on the content the last row would sit underneath it.
		expect(layoutSource).toMatch(/sigap-citizen-main/);
		expect(layoutSource).toMatch(/padding-bottom/);
		expect(layoutSource).toMatch(/safe-area-inset-bottom/);
	});

	it('hides the bottom nav at desktop widths and shows the desktop nav', () => {
		expect(bottomNavSource).toMatch(/@media \(max-width: 1023px\)/);
		expect(desktopNavSource).toMatch(/@media \(min-width: 1024px\)/);
	});

	it('keeps citizen touch targets at or above 44px', () => {
		// Tab rows must clear the citizen minimum. Parsed from the stylesheet
		// because jsdom reports no layout.
		const heights = Array.from(
			bottomNavSource.matchAll(/min-height:\s*(\d+)px/g)
		).map((m) => Number(m[1]));
		expect(heights.length).toBeGreaterThan(0);
		for (const height of heights) {
			expect(height).toBeGreaterThanOrEqual(44);
		}
	});
});

describe('citizen shell: desktop at 1440px', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	it('renders a one-line desktop navigation with the four destinations', async () => {
		render(CitizenDesktopNav, { path: '/' });
		await tick();
		const nav = screen.getByRole('navigation', { name: 'Navigasi utama' });
		const links = Array.from(nav.querySelectorAll('a'));
		// Four destinations plus the booking CTA, which is an action rather than
		// a destination and therefore lives outside the list.
		expect(links).toHaveLength(5);
		expect(links[0].getAttribute('href')).toBe('/');
		expect(links[4].textContent).toContain('Buat Janji Temu');
	});

	it('marks exactly one desktop link as the current page', async () => {
		render(CitizenDesktopNav, { path: '/patient/status' });
		await tick();
		const current = document.querySelectorAll('[aria-current="page"]');
		expect(current).toHaveLength(1);
		expect(current[0].getAttribute('href')).toBe('/patient/status');
	});

	it('does not render the bottom nav inside the desktop nav', async () => {
		render(CitizenDesktopNav, { path: '/' });
		await tick();
		expect(screen.queryByRole('navigation', { name: 'Navigasi bawah' })).toBeNull();
	});

	it('presents the four destinations on a single row', () => {
		// The list is a flex row, so the destinations cannot wrap to a second
		// line. `flex-wrap` appearing here would break the frozen layout.
		expect(desktopNavSource).toMatch(/\.sigap-desktop-nav__list\s*\{[^}]*display:\s*flex/);
		expect(desktopNavSource).not.toMatch(/\.sigap-desktop-nav__list\s*\{[^}]*flex-wrap/);
	});
});

describe('citizen shell: header composition', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	it('renders a banner landmark with the desktop nav and the account control', async () => {
		render(CitizenHeader, { path: '/', hasSession: false, userEmail: '' });
		await tick();
		const banner = screen.getByRole('banner');
		expect(banner).toBeTruthy();
		expect(banner.querySelector('nav[aria-label="Navigasi utama"]')).toBeTruthy();
		expect(banner.querySelector('.sigap-citizen-header__account')).toBeTruthy();
	});

	it('renders exactly one banner and two named navigation landmarks', async () => {
		render(CitizenHeader, { path: '/', hasSession: false, userEmail: '' });
		await tick();
		// One banner, not two: a duplicated header landmark would make "skip to
		// main" and landmark navigation ambiguous.
		expect(screen.getAllByRole('banner')).toHaveLength(1);

		// The bottom bar is a sibling of the header, not a child, because it is
		// fixed to the viewport rather than part of the header's border context.
		const navs = screen.getAllByRole('navigation');
		const names = navs.map((n) => n.getAttribute('aria-label'));
		expect(names).toContain('Navigasi utama');
		expect(names).toContain('Navigasi bawah');
		// Each navigation is named, so a screen reader can tell them apart.
		for (const name of names) {
			expect(name, 'every nav landmark needs a name').toBeTruthy();
		}
	});

	it('exposes a single brand link back to Beranda', async () => {
		render(CitizenHeader, { path: '/faskes', hasSession: false, userEmail: '' });
		await tick();
		const brand = document.querySelector('.sigap-citizen-header__brand');
		expect(brand?.getAttribute('href')).toBe('/');
		// The mark glyph alone would be announced as "S", so the link carries a
		// full name.
		expect(brand?.getAttribute('aria-label')).toBe('Sigap, kembali ke Beranda');
	});
});

describe('account menu: identity only', () => {
	beforeEach(() => {
		document.body.innerHTML = '';
	});

	it('offers Masuk and Daftar when signed out', async () => {
		render(AccountMenu, { hasSession: false });
		await tick();
		expect(screen.getByRole('link', { name: /Masuk/ })).toBeTruthy();
		expect(screen.getByRole('link', { name: /Daftar/ })).toBeTruthy();
		expect(document.querySelector('form[action="/auth/logout"]')).toBeNull();
	});

	it('offers Keluar when signed in', async () => {
		render(AccountMenu, { hasSession: true, userEmail: 'warga@example.test' });
		await tick();

		// The signed-in presentation is a disclosure, so the panel does not exist
		// until it is opened. Opening it first is what a user does, and testing
		// without opening would assert against a closed menu.
		const trigger = document.querySelector<HTMLButtonElement>(
			'.sigap-account__trigger'
		);
		expect(trigger, 'the account trigger must exist when signed in').toBeTruthy();
		expect(trigger?.getAttribute('aria-expanded')).toBe('false');
		await fireEvent.click(trigger as HTMLButtonElement);
		await tick();

		expect(trigger?.getAttribute('aria-expanded')).toBe('true');
		const logout = document.querySelector('form[action="/auth/logout"] button');
		expect(logout?.textContent).toContain('Keluar');
	});

	it('closes the disclosure on Escape and restores focus', async () => {
		render(AccountMenu, { hasSession: true, userEmail: 'warga@example.test' });
		await tick();
		const trigger = document.querySelector<HTMLButtonElement>('.sigap-account__trigger');
		await fireEvent.click(trigger as HTMLButtonElement);
		await waitFor(() => expect(document.querySelector('#sigap-account-menu')).toBeTruthy());

		// Dispatched on the document because that is where a real keypress
		// lands: the listener is registered on <svelte:window>, and jsdom's
		// event dispatch does not bubble a window-targeted event back down.
		await fireEvent.keyDown(document, { key: 'Escape' });

		// waitFor, not tick. The panel is removed by an outro transition, and
		// that removal is a second, later DOM update than the state change that
		// started it — a single tick observes the panel still present and reports
		// a component bug where there is only an animation in the way.
		await waitFor(() => expect(document.querySelector('#sigap-account-menu')).toBeNull());
		// Focus returns to the trigger so keyboard position is not lost.
		expect(document.activeElement).toBe(trigger);
	});

	it('closes the disclosure on an outside click', async () => {
		// Escape alone is not enough: a menu that only closes by keyboard strands
		// anyone using a pointer or a screen reader in reading mode.
		render(AccountMenu, { hasSession: true, userEmail: 'warga@example.test' });
		await tick();
		const trigger = document.querySelector<HTMLButtonElement>('.sigap-account__trigger');
		await fireEvent.click(trigger as HTMLButtonElement);
		await waitFor(() => expect(document.querySelector('#sigap-account-menu')).toBeTruthy());

		await fireEvent.click(document.body);
		await waitFor(() => expect(document.querySelector('#sigap-account-menu')).toBeNull());
	});

	it('moves focus into the panel once it has opened', async () => {
		// Without this, opening the menu leaves focus on the trigger and the next
		// Tab lands on the page behind the panel instead of its first action.
		render(AccountMenu, { hasSession: true, userEmail: 'warga@example.test' });
		await tick();
		const trigger = document.querySelector<HTMLButtonElement>('.sigap-account__trigger');
		await fireEvent.click(trigger as HTMLButtonElement);

		await waitFor(() => {
			const panel = document.querySelector('#sigap-account-menu');
			expect(panel, 'panel should be open').toBeTruthy();
			expect(panel?.contains(document.activeElement), 'focus should be inside the panel').toBe(
				true
			);
		});
	});

	it('labels the disclosure as a menu with correct expanded state', async () => {
		render(AccountMenu, { hasSession: true, userEmail: 'warga@example.test' });
		await tick();
		const trigger = document.querySelector<HTMLButtonElement>('.sigap-account__trigger');
		expect(trigger?.getAttribute('aria-haspopup')).toBe('menu');
		expect(trigger?.getAttribute('aria-controls')).toBe('sigap-account-menu');
	});

	it('never links to an admin route', async () => {
		// The citizen shell must not offer a way into the admin product, and the
		// account menu is the most tempting place for such a link to appear.
		render(AccountMenu, { hasSession: true, userEmail: 'admin@example.test' });
		await tick();
		const hrefs = Array.from(document.querySelectorAll('a')).map((a) =>
			a.getAttribute('href')
		);
		for (const href of hrefs) {
			expect(href).not.toContain('admin');
		}
	});

	it('does not reference role, permission, or scope concepts at all', () => {
		// A stronger statement than "no admin link": the account surface must
		// not even know these words. A menu that could branch on them is a menu
		// that could one day leak them.
		//
		// Two things are excluded before scanning, and both exclusions are
		// deliberate:
		//
		// 1. Comments. This component's own documentation names the concepts in
		//    order to explain why they are absent, and a raw substring scan would
		//    flag that explanation as the violation it is forbidding.
		// 2. ARIA attributes. `role="menu"` and `role="menuitem"` are the
		//    accessibility role, not the authorization role. Confusing the two
		//    would mean deleting a correct ARIA attribute, so the scan targets
		//    the authorization words specifically rather than the bare token
		//    "role".
		const code = accountSource
			.replace(/<!--[\s\S]*?-->/g, ' ')
			.replace(/\/\*[\s\S]*?\*\//g, ' ')
			.replace(/(^|[^:])\/\/.*$/gm, '$1')
			.replace(/role="[^"]*"/g, ' ')
			.toLowerCase();
		for (const forbidden of [
			'super_admin',
			'superadmin',
			'facilityscope',
			'permission',
			'is_admin',
			'user_role',
			'role:',
			'.role'
		]) {
			expect(code, `account menu must not mention ${forbidden}`).not.toContain(forbidden);
		}
	});

	it('discloses the email without parsing it', async () => {
		render(AccountMenu, { hasSession: true, userEmail: 'warga@example.test' });
		await tick();
		const trigger = document.querySelector<HTMLButtonElement>('.sigap-account__trigger');
		await fireEvent.click(trigger as HTMLButtonElement);
		await tick();
		// Rendered verbatim. Nothing inspects the domain or the local part.
		expect(document.body.textContent).toContain('warga@example.test');
	});
});

describe('no navigation path to /wallet', () => {
	function collectSvelteFiles(dir: string): string[] {
		const out: string[] = [];
		for (const entry of readdirSync(dir)) {
			const full = join(dir, entry);
			if (statSync(full).isDirectory()) {
				out.push(...collectSvelteFiles(full));
			} else if (full.endsWith('.svelte')) {
				out.push(full);
			}
		}
		return out;
	}

	it('has no link to /wallet in any shell or navigation component', () => {
		// The wallet page still exists, but nothing navigates to it. This walks
		// the real component tree rather than trusting a hand-written list, so a
		// new shell component with a wallet link fails without anyone editing a
		// second file.
		const shellFiles = collectSvelteFiles(join(webRoot, 'src', 'lib', 'citizen'));
		const offenders: string[] = [];
		for (const file of shellFiles) {
			const source = readFileSync(file, 'utf8');
			if (/["'`]\/wallet/.test(source)) {
				offenders.push(relative(webRoot, file).replace(/\\/g, '/'));
			}
		}
		expect(offenders).toEqual([]);
	});

	it('has no wallet link in the root layout', () => {
		expect(layoutSource).not.toMatch(/["'`]\/wallet/);
	});

	it('keeps the wallet page itself', () => {
		// Removal from navigation is not removal of the route. The page must
		// still exist; it simply has no normal path to it.
		expect(
			statSync(join(webRoot, 'src', 'routes', 'wallet', '+page.svelte')).isFile()
		).toBe(true);
	});
});
