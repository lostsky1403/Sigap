<script lang="ts">
	import { page } from '$app/stores';
	import AdminShell from '$lib/admin/AdminShell.svelte';
</script>

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

	:global(.sigap-admin-page) {
		min-width: 0;
		padding: 20px 24px;
	}
</style>
