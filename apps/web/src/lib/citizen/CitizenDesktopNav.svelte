<script lang="ts">
	import { CITIZEN_DESTINATIONS, isActiveDestination } from './navigation';
	import { RADIUS, CONTROL_HEIGHT } from '$lib/design/tokens';
	import { CalendarPlus } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';

	/**
	 * The desktop single-line navigation.
	 *
	 * Same four destinations as the mobile bar, rendered as one horizontal row
	 * with the booking CTA at the end. It is a separate `<nav>` landmark with
	 * its own name so the two navigations are distinguishable rather than
	 * merged, and it is removed from the tree below 1024px where the bottom bar
	 * takes over.
	 *
	 * The active destination uses an underline plus the brand colour, so the
	 * current page is not signalled by colour alone.
	 */
	export let path: string = '/';
</script>

<nav class="sigap-desktop-nav" aria-label="Navigasi utama">
	<ul class="sigap-desktop-nav__list">
		{#each CITIZEN_DESTINATIONS as destination (destination.href)}
			<li>
				<a
					class="sigap-desktop-nav__link"
					style:border-radius={RADIUS.control}
					href={destination.href}
					aria-current={isActiveDestination(path, destination.href) ? 'page' : undefined}
				>
					{destination.label}
				</a>
			</li>
		{/each}
	</ul>

	<!--
		Booking is a primary action, not a destination, so it is a CTA rather
	than a tab. It is intentionally outside the list: keeping it out of
		`CITIZEN_DESTINATIONS` is what preserves the four-destination contract.
	-->
	<a
		class="sigap-desktop-nav__cta"
		style:border-radius={RADIUS.control}
		style:min-height="{CONTROL_HEIGHT.citizen}px"
		href="/appointments/new"
	>
		<Icon icon={CalendarPlus} size={18} />
		<span>Buat Janji Temu</span>
	</a>
</nav>

<style>
	.sigap-desktop-nav {
		display: none;
	}

	@media (min-width: 1024px) {
		.sigap-desktop-nav {
			display: flex;
			align-items: center;
			gap: 16px;
		}
	}

	.sigap-desktop-nav__list {
		display: flex;
		align-items: center;
		gap: 4px;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sigap-desktop-nav__link {
		display: inline-flex;
		align-items: center;
		height: 44px;
		padding: 0 8px;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
		text-decoration: none;
		/* The active underline, drawn as a border so it cannot shift layout. */
		border-bottom: 2px solid transparent;
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
		border-bottom-color: var(--sigap-primary);
	}

	.sigap-desktop-nav__cta {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 0 16px;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-desktop-nav__cta:hover {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-desktop-nav__cta:active {
		background-color: var(--sigap-primary-active);
	}

	.sigap-desktop-nav__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
