<script lang="ts">
	import AdminSidebar from './AdminSidebar.svelte';
	import { ADMIN_ACCOUNT_HREF } from './navigation';

	/**
	 * The admin application shell.
	 *
	 * A sidebar and a content column, and nothing else. The shell deliberately
	 * does NOT decide authorization: it renders for whoever reaches `/admin`, and
	 * each page resolves its own scoped reads. A shell that gated navigation on a
	 * client-side role check would hide destinations from a legitimately
	 * permitted operator and show them to someone whose role the client guessed
	 * wrong — and the server would refuse the data either way, so the guess buys
	 * nothing except a wrong UI.
	 *
	 * `min-width: 0` on the content column is load-bearing rather than tidy. It is
	 * what lets a wide table shrink and hand its overflow to its own scroll
	 * region instead of pushing the whole page sideways, which is the difference
	 * between "the table scrolls" and "the admin panel has a horizontal scrollbar
	 * at 1024px".
	 */
	export let path: string = '/admin';
	export let accountHref: string = ADMIN_ACCOUNT_HREF;
</script>

<div class="sigap-admin-shell">
	<AdminSidebar {path} {accountHref} />
	<div class="sigap-admin-shell__content">
		<slot />
	</div>
</div>

<style>
	.sigap-admin-shell {
		display: flex;
		min-height: 100vh;
		background-color: var(--sigap-canvas);
	}

	.sigap-admin-shell__content {
		flex: 1;
		/* Without this a wide table widens the flex item past the viewport. */
		min-width: 0;
	}
</style>
