<script lang="ts">
	import { page } from '$app/stores';
	import AdminShell from '$lib/admin/AdminShell.svelte';
	import { ADMIN_DESTINATIONS, isActiveDestination } from '$lib/admin/navigation';

	/**
	 * The document title, derived from the same nav table the sidebar renders.
	 *
	 * All six admin routes shipped an empty `<title>`. That is not cosmetic: an
	 * empty title gives every admin tab the same label, so six open pages are
	 * indistinguishable in the tab strip and in browser history, and a screen
	 * reader announces the page without naming it.
	 *
	 * Deriving it from ADMIN_DESTINATIONS rather than hardcoding six strings
	 * means a renamed destination renames its title too, and a seventh admin
	 * page gets one for free.
	 */
	$: currentAdminTitle =
		ADMIN_DESTINATIONS.find((d) => isActiveDestination($page.url.pathname, d.href))?.label ?? '';
</script>

<svelte:head>
	<title>{currentAdminTitle ? `${currentAdminTitle} — Sigap` : 'Sigap'}</title>
</svelte:head>

<!--
	The admin layout.

	Separate from the citizen shell on purpose. The two products have different
	audiences, different destinations, and different densities, and a shared
	shell is how a bottom tab bar ends up on a page designed for a 1440px
	monitor. Phase 3B4 replaced the root layout's documented admin passthrough —
	which existed only to keep /admin reachable before this shell existed — so
	this is the first real admin navigation.

	The shell renders unconditionally. It carries no session check, no role
	check, and no permission check: whether the visitor may read any given page
	is answered by the server on that page's own request, and this layout has no
	data with which to answer it early. Hiding the sidebar on a 403 would also
	leave an operator with no way to reach a page they do have access to.

	`data` is intentionally not declared. SvelteKit passes layout data to every
	layout, and an undeclared prop is simply ignored, so an admin page that later
	needs `hasSession` will declare it in its own file. The root layout above
	already reads it, and `hasSession` remains the only session fact the client
	ever sees.

	The layout slot belongs INSIDE the shell, not beside it. `AdminShell` renders
	its own `<slot />` in the content column, so passing it children is what puts
	a page there; a self-closing `<AdminShell />` type-checks and renders a
	perfectly valid sidebar around nothing at all.
-->
<AdminShell path={$page.url.pathname}>
	<!--
		A visible statement that this is a development preview acting as a seeded
		identity.

		The admin screens are reachable here without signing in because the web
		tier injects `SIGAP_LOCAL_E2E_ACTOR`, a documented local-only affordance:
		the Go API resolves that subject's roles from the database on every
		request and refuses to arm the selector outside SIGAP_ENV=local. That is
		correct for a local preview and would be a serious problem if it were
		ever mistaken for the shipped product — so it says so, on the page, in
		the operator's own view.

		Deliberately not dismissible. A banner an operator can close is a banner
		that is gone by the time they screenshot a page for someone else.
	-->
	<div class="sigap-admin-devbar" role="note" data-testid="admin-dev-banner">
		Pratinjau pengembangan — layar admin berjalan sebagai identitas uji coba
		(<code>local-global-super-admin</code>), bukan sesi pengguna sungguhan.
	</div>
	<slot />
</AdminShell>

<style>
	/*
		Scoped to the admin main region. `min-width: 0` is what allows a wide table
		to shrink and hand its overflow to its own scroll region rather than
		widening the page; without it the flex item grows to the table's width and
		the whole admin panel scrolls sideways at 1024px.
	*/
	:global(.sigap-admin-shell__content) {
		min-width: 0;
	}

	/*
		The Pen's admin `Content` frame is `[20,24]`: 20px vertical, 24px
		horizontal. 20 is not a step on the 4px ladder, so it is written as the
		literal the frame specifies rather than rounded to 16 or 24 — a 4px
		difference on every admin page is visible as the header sitting closer
		to or further from the toolbar than the reference.
	*/
	:global(.sigap-admin-page) {
		min-width: 0;
		padding: 20px var(--sigap-space-5);
	}

	/*
		The preview-identity strip. Warning-toned rather than neutral: it is
		describing a condition that must not be mistaken for normal operation.
		Full width of the content column, directly under the header, so it is
		read before the page it qualifies.
	*/
	.sigap-admin-devbar {
		padding: var(--sigap-space-2) var(--sigap-space-5);
		font-size: var(--sigap-type-caption);
		color: var(--sigap-warning);
		background-color: var(--sigap-warning-tint);
		border-bottom: 1px solid var(--sigap-warning);
	}

	.sigap-admin-devbar code {
		font-family: var(--sigap-font-mono, ui-monospace, monospace);
		font-size: inherit;
	}
</style>
