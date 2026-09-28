import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import AdminSidebar from './AdminSidebar.svelte';
import AdminShell from './AdminShell.svelte';
import {
	ADMIN_ACCOUNT_HREF,
	ADMIN_DESTINATIONS,
	isActiveDestination
} from './navigation';

/**
 * T-3B4-01: the admin shell contract.
 *
 * Three claims are pinned here, and each maps to a way the shell can go wrong
 * that is invisible in a screenshot:
 *
 *  1. Exactly six destinations, in the frozen order, with exactly ONE
 *     `aria-current="page"` at any path. Two current markers is a screen-reader
 *     user being told they are on two pages at once.
 *  2. Every rail link carries an accessible NAME. At 1024px the visible label is
 *     hidden by CSS, so an `aria-label` is the only thing distinguishing six
 *     icon-only links. Without it, jsdom cannot catch the regression — the CSS
 *     hides the text but the accessible name is a DOM property, and testing it
 *     directly is the only way to prove the rail is usable.
 *  3. The account area is a STATIC link to Beranda. It must not vary by role,
 *     because the client has no idea what role anyone holds.
 */

beforeEach(() => {
	document.body.innerHTML = '';
});

describe('admin navigation: the frozen six destinations', () => {
	it('declares exactly six destinations in the frozen order', () => {
		expect(ADMIN_DESTINATIONS).toHaveLength(6);
		expect(ADMIN_DESTINATIONS.map((d) => d.label)).toEqual([
			'Ringkasan',
			'Antrean',
			'Janji Temu',
			'Jadwal',
			'Fasilitas',
			'Notifikasi'
		]);
		expect(ADMIN_DESTINATIONS.map((d) => d.href)).toEqual([
			'/admin',
			'/admin/queues',
			'/admin/appointments',
			'/admin/schedules',
			'/admin/facilities',
			'/admin/notifications'
		]);
	});

	it('gives every destination a distinct href and an icon', () => {
		const hrefs = new Set(ADMIN_DESTINATIONS.map((d) => d.href));
		expect(hrefs.size, 'two destinations sharing an href would make one unreachable').toBe(6);
		for (const destination of ADMIN_DESTINATIONS) {
			expect(destination.icon, `${destination.label} needs an icon`).toBeTruthy();
		}
	});

	it('links the account area statically to Beranda, with no role detection', () => {
		expect(ADMIN_ACCOUNT_HREF).toBe('/');
		// The account href is a constant, not a function of anything. A shell that
		// computed it from a role claim would make this a function instead, which
		// is exactly the client-side authorization inference that is forbidden.
		expect(typeof ADMIN_ACCOUNT_HREF).toBe('string');
	});
});

describe('isActiveDestination', () => {
	it('matches on exact path, not prefix', () => {
		// The prefix case is the important one: `/admin` is a prefix of every
		// admin route, so a prefix match would mark Ringkasan current on all six
		// pages simultaneously.
		expect(isActiveDestination('/admin', '/admin')).toBe(true);
		expect(isActiveDestination('/admin', '/admin/queues')).toBe(false);
		expect(isActiveDestination('/admin/queues', '/admin')).toBe(false);
		expect(isActiveDestination('/admin/queues', '/admin/queues')).toBe(true);
	});

	it('treats a trailing slash as the same page', () => {
		// SvelteKit may normalise to a trailing slash, and that names the same
		// page; without this, a redirect would silently clear the current marker.
		expect(isActiveDestination('/admin/queues/', '/admin/queues')).toBe(true);
		expect(isActiveDestination('/admin/', '/admin')).toBe(true);
	});
});

describe('AdminSidebar', () => {
	it('renders exactly six navigation links', () => {
		render(AdminSidebar, { path: '/admin' });
		const nav = screen.getByRole('navigation', { name: 'Menu operasi' });
		expect(within(nav).getAllByRole('link')).toHaveLength(6);
	});

	it('marks exactly one destination as the current page', () => {
		for (const destination of ADMIN_DESTINATIONS) {
			document.body.innerHTML = '';
			render(AdminSidebar, { path: destination.href });
			const current = document.querySelectorAll('[aria-current="page"]');
			expect(
				current.length,
				`${destination.href} must mark exactly one current destination`
			).toBe(1);
			expect(current[0].getAttribute('href')).toBe(destination.href);
		}
	});

	it('gives every link an accessible name, so the collapsed rail stays usable', () => {
		render(AdminSidebar, { path: '/admin' });
		const nav = screen.getByRole('navigation', { name: 'Menu operasi' });
		// At 1024px CSS hides the visible label, so the aria-label is the ONLY
		// thing left to identify each link. This is the assertion that keeps six
		// icon-only links from becoming six anonymous links.
		for (const destination of ADMIN_DESTINATIONS) {
			const link = within(nav).getByRole('link', { name: destination.label });
			expect(link).toBeTruthy();
		}
	});

	it('exposes the six destinations as a named navigation landmark', () => {
		render(AdminSidebar, { path: '/admin' });
		// A named landmark is what distinguishes this nav from any other on the
		// page; two unlabelled navs are indistinguishable to assistive tech.
		expect(screen.getByRole('complementary', { name: 'Navigasi panel operasi' })).toBeTruthy();
		expect(screen.getByRole('navigation', { name: 'Menu operasi' })).toBeTruthy();
	});

	it('offers a static Beranda link in the account area', () => {
		render(AdminSidebar, { path: '/admin' });
		const account = screen.getByRole('link', { name: 'Beranda' });
		expect(account.getAttribute('href')).toBe('/');
	});
});

describe('AdminShell', () => {
	it('composes the sidebar and renders its content', () => {
		render(AdminShell, { path: '/admin/queues' });
		expect(screen.getByRole('navigation', { name: 'Menu operasi' })).toBeTruthy();
		// Exactly one current marker across the whole shell, not one per region.
		expect(document.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
	});

	it('never renders a citizen bottom navigation', () => {
		render(AdminShell, { path: '/admin' });
		// There is no admin mobile design. A bottom tab bar here would be the
		// citizen shell leaking into the admin product.
		expect(document.querySelector('.sigap-bottom-nav')).toBeNull();
	});
});
