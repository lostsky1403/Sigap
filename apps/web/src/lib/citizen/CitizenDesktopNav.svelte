<script lang="ts">
	import { CITIZEN_DESTINATIONS, isActiveDestination } from './navigation';

	/**
	 * The desktop single-line navigation.
	 *
	 * Four destinations, no more. The booking CTA is *not* here: it is chrome
	 * that belongs to the masthead, and the frozen reference places it in the
	 * right-hand cluster beside the auth control rather than inside the
	 * destination list. Keeping it out of this component is also what makes
	 * "exactly four destinations" true of the rendered nav, not just of the
	 * `CITIZEN_DESTINATIONS` array.
	 *
	 * The list is centred in the space the brand and the account cluster leave
	 * behind, which is how the reference lays it out: the nav's optical centre
	 * sits between the wordmark and the buttons, not at the viewport's centre.
	 *
	 * It is a separate `<nav>` landmark with its own name so the two
	 * navigations are distinguishable rather than merged, and it is removed
	 * from the tree below 1024px where the bottom bar takes over.
	 */
	export let path: string = '/';
</script>

<nav class="sigap-desktop-nav" aria-label="Navigasi utama">
	<ul class="sigap-desktop-nav__list">
		{#each CITIZEN_DESTINATIONS as destination (destination.href)}
			<li>
				<a
					class="sigap-desktop-nav__link"
					href={destination.href}
					aria-current={isActiveDestination(path, destination.href) ? 'page' : undefined}
				>
					{destination.label}
				</a>
			</li>
		{/each}
	</ul>
</nav>

<style>
	.sigap-desktop-nav {
		display: none;
	}

	@media (min-width: 1024px) {
		.sigap-desktop-nav {
			/* Takes the slack between the wordmark and the account cluster, so
			   the list centres in that gap rather than in the viewport. */
			flex: 1 1 auto;
			display: flex;
			justify-content: center;
			min-width: 0;
		}
	}

	.sigap-desktop-nav__list {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-4);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	/*
		The current page is signalled by colour and weight together, never by
		colour alone. The reference draws no underline under the active item —
		the teal label is the whole mark — and adding one would put a second
		2px teal rule in the masthead beside the brand mark.
	*/
	.sigap-desktop-nav__link {
		display: inline-flex;
		align-items: center;
		height: 44px;
		/* "Status" is only ~39px wide; a 44px min-width makes the whole label
		   a touch target without widening the visible text. */
		min-width: var(--sigap-control-citizen);
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-muted);
		text-decoration: none;
	}

	.sigap-desktop-nav__link:hover {
		color: var(--sigap-primary);
	}

	.sigap-desktop-nav__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-desktop-nav__link[aria-current='page'] {
		color: var(--sigap-primary);
		font-weight: 600;
	}
</style>
