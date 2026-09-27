<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import CitizenHeader from '$lib/citizen/CitizenHeader.svelte';
	import AccountMenu from '$lib/citizen/AccountMenu.svelte';
	import type { LayoutData } from './$types';

	export let data: LayoutData;

	/**
	 * The citizen shell.
	 *
	 * One root layout serves the whole app, so this is what every citizen route
	 * inherits. That is intentional: the header, the two navigations, and the
	 * session presentation are the shell, and rebuilding them per page is how
	 * two pages end up disagreeing about where "Status" lives.
	 *
	 * The admin product is a separate shell, and Phase 3B4 owns it. The citizen
	 * bar is therefore not rendered under `/admin`, and the pre-existing admin
	 * affordance is preserved verbatim below so admin pages do not lose the
	 * navigation they have always had. Nothing here is an admin design: it is
	 * the current link, unchanged, so 3B4 can replace it deliberately.
	 */
	$: isAdminArea = $page.url.pathname.startsWith('/admin');
	$: hasSession = data.hasSession ?? false;
	$: userEmail = data.userEmail ?? '';
</script>

{#if isAdminArea}
	<!--
		Admin reachability, preserved and nothing more.

		Before Phase 3B2 the single shared header exposed admin through exactly
		one link, to /admin/queues, and the admin pages carry no cross-links
		between them. So restoring that one link restores the prior reachability
		exactly: no admin destination is newly reachable and none is lost.

		This is deliberately not an admin navigation. Choosing an admin IA —
		grouping, labelling, ordering, current-state treatment — is Phase 3B4's
		decision to make with the frozen admin reference in hand, not something
		to settle implicitly while working on the citizen shell.
	-->
	<header class="sigap-admin-passthrough">
		<div class="sigap-admin-passthrough__inner">
			<a class="sigap-admin-passthrough__brand" href="/">Sigap</a>
			<a
				class="sigap-admin-passthrough__admin"
				href="/admin/queues"
				aria-current={$page.url.pathname === '/admin/queues' ? 'page' : undefined}
			>
				Admin
			</a>
			<div class="sigap-admin-passthrough__account">
				<AccountMenu {hasSession} {userEmail} />
			</div>
		</div>
	</header>

	<main>
		<slot />
	</main>
{:else}
	<CitizenHeader path={$page.url.pathname} {hasSession} {userEmail} />

	<!--
		The bottom padding reserves the fixed tab bar plus the device safe-area
		inset, so the last element of a long page can never end up underneath it.
	-->
	<main class="sigap-citizen-main">
		<slot />
	</main>
{/if}

<style>
	.sigap-citizen-main {
		/*
		 * 96px clears the 56px tab bar with room to spare, and
		 * env(safe-area-inset-bottom) adds the home-indicator inset on devices
		 * that have one.
		 */
		padding-bottom: calc(96px + env(safe-area-inset-bottom, 0px));
	}

	.sigap-admin-passthrough {
		background-color: var(--sigap-surface);
		border-bottom: 1px solid var(--sigap-border);
	}

	.sigap-admin-passthrough__inner {
		display: flex;
		align-items: center;
		gap: 16px;
		height: 56px;
		padding: 0 16px;
	}

	.sigap-admin-passthrough__brand {
		font-size: 17px;
		font-weight: 600;
		color: var(--sigap-foreground);
		text-decoration: none;
	}

	.sigap-admin-passthrough__admin {
		display: inline-flex;
		align-items: center;
		height: 44px;
		padding: 0 8px;
		font-size: 14px;
		color: var(--sigap-foreground);
		text-decoration: none;
	}

	.sigap-admin-passthrough__admin:hover {
		color: var(--sigap-primary);
	}

	.sigap-admin-passthrough__admin:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-admin-passthrough__admin[aria-current='page'] {
		color: var(--sigap-primary);
		font-weight: 500;
	}

	.sigap-admin-passthrough__account {
		margin-left: auto;
	}
</style>
