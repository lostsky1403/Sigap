/**
 * The frozen Admin navigation contract.
 *
 * The single place the six admin destinations are declared, mirroring
 * `citizen/navigation.ts` for the same reason: "exactly six destinations" and
 * "exactly one `aria-current`" then become structural facts rather than
 * something the shell has to remember. Adding a seventh entry here fails the
 * shell tests.
 *
 * Deliberately data-only — Lucide names are strings resolved by the sidebar, so
 * a contract test can import this without pulling in a component library.
 *
 * WHY THIS IS NOT THE CITIZEN NAVIGATION
 *
 * The admin product is a different audience on different hardware. An operator
 * works a queue all day on a desktop monitor and needs six dense destinations
 * and no bottom tab bar, so there is no admin mobile design: the shell collapses
 * to a 56px icon rail rather than becoming a different IA. That is the frozen
 * reference behaviour, and inventing a phone IA for admin would be a decision
 * Phase 3B4 was not asked to make.
 */

/** A destination in the admin shell. */
export interface AdminDestination {
	/** Visible label. Also the accessible name on the collapsed rail, so it must stand alone. */
	label: string;
	/** Canonical in-app route. Never a full URL; the shell stays same-origin. */
	href: string;
	/** Lucide component name, resolved by the sidebar to keep this module data-only. */
	icon: string;
}

/**
 * Exactly six, in the frozen order.
 *
 * `Ringkasan` is `/admin` itself rather than a nested path, so the overview has
 * the shortest possible URL and the sidebar entry is the one an operator
 * reaches first.
 */
export const ADMIN_DESTINATIONS: readonly AdminDestination[] = [
	{ label: 'Ringkasan', href: '/admin', icon: 'LayoutDashboard' },
	{ label: 'Antrean', href: '/admin/queues', icon: 'ListOrdered' },
	{ label: 'Janji Temu', href: '/admin/appointments', icon: 'CalendarCheck' },
	{ label: 'Jadwal', href: '/admin/schedules', icon: 'CalendarClock' },
	{ label: 'Fasilitas', href: '/admin/facilities', icon: 'Building2' },
	{ label: 'Notifikasi', href: '/admin/notifications', icon: 'Bell' }
] as const;

/**
 * True when `path` is the destination currently being viewed.
 *
 * Exact match, for the same reason the citizen helper uses one: a prefix match
 * marks `/admin` current on every admin route, which would put several
 * `aria-current="page"` markers on one screen. The active destination is the
 * single most important accessibility fact in the shell, and a prefix match
 * breaks it silently.
 *
 * Trailing slashes normalise, because SvelteKit may add one and it names the
 * same page.
 */
export function isActiveDestination(path: string, href: string): boolean {
	const normalise = (value: string) =>
		value.length > 1 && value.endsWith('/') ? value.slice(0, -1) : value;
	return normalise(path) === normalise(href);
}

/**
 * The account area's destination.
 *
 * A plain static link to Beranda. It is NOT a role check: the shell has no idea
 * whether the visitor is an operator, and inventing a "return to admin home"
 * variant based on a client-side role guess is exactly the inference the
 * Phase 3B0 authorization model forbids. One static link, always.
 */
export const ADMIN_ACCOUNT_HREF = '/';
