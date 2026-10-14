<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import CitizenHeader from '$lib/citizen/CitizenHeader.svelte';
	import type { LayoutData } from './$types';

	export let data: LayoutData;

	/**
	 * Hydration marker.
	 *
	 * The page is interactive only after SvelteKit has hydrated it, and the
	 * window between "the server HTML is painted" and "the client took over" is
	 * the one an automated browser can fall into: a `fill()` lands before the
	 * component has attached its binding, the input event is lost, and the
	 * value the test typed never reaches the app. Under parallel load that
	 * window widens from milliseconds to seconds, which is what made two
	 * citizen specs flake intermittently.
	 *
	 * This is the ROOT layout, and Svelte runs a child's `onMount` before its
	 * parent's. So by the time this fires, every page component below it has
	 * already mounted and run its own `onMount` — which is exactly the
	 * guarantee the E2E fixture needs: the marker appearing means the whole
	 * tree is interactive, including a page that reads its deep-link query in
	 * `onMount`.
	 *
	 * The attribute is inert in production. It exists so a browser test can
	 * wait for a fact rather than guess a duration.
	 */
	onMount(() => {
		document.documentElement.dataset.sigapHydrated = 'true';
	});

	/**
	 * The citizen shell.
	 *
	 * One root layout serves the citizen routes, so this is what they all
	 * inherit. That is intentional: the header, the two navigations, and the
	 * session presentation are the shell, and rebuilding them per page is how
	 * two pages end up disagreeing about where "Status" lives.
	 *
	 * `/admin` is excluded. The admin product is a separate shell with its own
	 * destinations and density, and `routes/admin/+layout.svelte` owns it. This
	 * file used to render a documented admin passthrough here — a single link to
	 * /admin/queues, there only so admin pages stayed reachable before Phase 3B4
	 * built a real shell. That shell now exists, so the passthrough is removed
	 * rather than left to render alongside it: two shells on one page would give
	 * the admin product a citizen header and tab bar it should not have, and two
	 * competing navigation landmarks.
	 */
	$: isAdminArea = $page.url.pathname.startsWith('/admin');
	$: hasSession = data.hasSession ?? false;
	$: userEmail = data.userEmail ?? '';
</script>

{#if isAdminArea}
	<!--
		Rendered as a bare fragment on purpose. The admin layout owns the shell,
		and this branch exists only to withhold the citizen chrome; adding a
		wrapper element here would put the admin pages inside a second landmark
		nesting level for no structural gain.
	-->
	<slot />
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
</style>
