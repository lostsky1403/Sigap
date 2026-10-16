/**
 * The frozen Citizen navigation contract.
 *
 * This module is the single place the four citizen destinations are declared.
 * Both the desktop nav and the bottom nav import it, which is what makes
 * "exactly four destinations" a structural fact rather than something two
 * components each have to remember. Adding a fifth entry here would fail the
 * shell tests, and a route that is not in this list has no business in the
 * citizen shell at all.
 *
 * `/wallet` is deliberately absent. The page still exists, but a wallet is not
 * a citizen destination in the frozen design, so it gets no navigation path.
 * The audit in `navigation.test.ts` asserts no shell link points at it.
 *
 * Check-In and walk-in stay separate contracts. Check-in takes a code for an
 * appointment you already booked; walk-in takes a number on arrival. They are
 * different journeys, and conflating them is how a citizen ends up in a queue
 * with no appointment behind them.
 */

/** A destination in the citizen shell. */
export interface CitizenDestination {
	/** Visible label. Also the accessible name, so it must stand alone. */
	label: string;
	/** Canonical in-app route. Never a full URL; the shell stays same-origin. */
	href: string;
	/**
	 * Expanded accessible name, used where the short label is ambiguous.
	 * "Check-In" alone does not say what you are checking in to.
	 */
	ariaLabel: string;
	/** Lucide component name, resolved by the caller to keep this module data-only. */
	icon: string;
}

/**
 * Exactly four, in the frozen order. `Beranda / Faskes / Check-In / Status`.
 *
 * There is no centre action button. The frozen mobile reference marks the
 * booking CTA as excluded from the tab bar, so a raised middle FAB would
 * contradict the design and would make the tab count ambiguous for anyone
 * counting landmarks.
 */
export const CITIZEN_DESTINATIONS: readonly CitizenDestination[] = [
	{ label: 'Beranda', href: '/', ariaLabel: 'Beranda', icon: 'Home' },
	{ label: 'Faskes', href: '/faskes', ariaLabel: 'Faskes', icon: 'MapPin' },
	{
		label: 'Check-In',
		href: '/appointments/check-in',
		ariaLabel: 'Check-In Janji Temu',
		icon: 'ClipboardCheck'
	},
	{ label: 'Status', href: '/patient/status', ariaLabel: 'Status kunjungan', icon: 'Activity' }
] as const;

/**
 * True when `path` is the destination currently being viewed.
 *
 * Exact match on purpose. A prefix match would mark both `/` and every other
 * route as "Beranda" because `/` is a prefix of everything, which would put
 * several `aria-current="page"` markers on one screen. The one exception is
 * trailing slashes, which SvelteKit may add and which name the same page.
 */
export function isActiveDestination(path: string, href: string): boolean {
	const normalise = (value: string) =>
		value.length > 1 && value.endsWith('/') ? value.slice(0, -1) : value;
	return normalise(path) === normalise(href);
}

/** The one auth control the signed-out masthead carries. */
export interface AuthAlternate {
	label: string;
	href: string;
}

/**
 * The masthead's signed-out auth control, which flips on the auth pages.
 *
 * On the login page it offers "Daftar"; everywhere else — including the
 * register page — it offers "Masuk". That is not a display trick: a citizen
 * standing on the login form does not need a second link to the login form,
 * and a citizen on the register form does not need a second link to register.
 * The control always points at the thing they cannot currently do.
 *
 * The frozen reference shows exactly this: `Daftar` beside the booking CTA on
 * 17 Citizen / Masuk, and `Masuk` beside it on 18 Citizen / Daftar and on
 * every other signed-out page.
 *
 * Sign-in only. A signed-in masthead renders the account disclosure instead,
 * because offering "Masuk" to someone already signed in is the bug this
 * function exists to make impossible.
 */
export function authAlternate(path: string): AuthAlternate {
	const normalise = (value: string) =>
		value.length > 1 && value.endsWith('/') ? value.slice(0, -1) : value;
	const onLogin = normalise(path) === '/auth/login';
	return onLogin
		? { label: 'Daftar', href: '/auth/register' }
		: { label: 'Masuk', href: '/auth/login' };
}
