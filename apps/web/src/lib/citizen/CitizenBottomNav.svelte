<script lang="ts">
	import { CITIZEN_DESTINATIONS, isActiveDestination } from './navigation';
	import { RADIUS } from '$lib/design/tokens';
	import { Home, MapPin, ClipboardCheck, Activity } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import type { ComponentType } from 'svelte';

	/**
	 * The mobile tab bar: exactly the four frozen destinations.
	 *
	 * Structural decisions worth stating:
	 *
	 * 1. It is a real `<nav>` with an accessible name. Two navs on one page
	 *    (this and the desktop one) are only distinguishable to assistive tech
	 *    by name, so `aria-label` is what makes "Navigasi bawah" reachable as
	 *    a landmark distinct from the header nav.
	 *
	 * 2. `aria-current="page"` is applied by exact-path comparison, so exactly
	 *    one tab is ever current. A prefix match would mark `/` current on every
	 *    route, which is both wrong and confusing when announced.
	 *
	 * 3. There is no centre action button. The frozen design explicitly keeps
	 *    the booking CTA out of the tab bar, and a raised middle item would
	 *    make the tab count ambiguous.
	 *
	 * The bar is fixed to the viewport bottom and the page reserves matching
	 * bottom padding, so the last row of content can never sit underneath it.
	 */
	export let path: string = '/';

	/**
	 * Icon lookup.
	 *
	 * `navigation.ts` stays data-only and holds Lucide names as strings, so the
	 * contract can be imported by tests without pulling in a component library.
	 * This map is the only place those names become components.
	 */
	const ICONS: Record<string, ComponentType> = {
		Home,
		MapPin,
		ClipboardCheck,
		Activity
	};
</script>

<nav class="sigap-bottom-nav" aria-label="Navigasi bawah">
	<ul class="sigap-bottom-nav__list" style:border-radius={RADIUS.panel}>
		{#each CITIZEN_DESTINATIONS as destination (destination.href)}
			{@const IconComponent = ICONS[destination.icon]}
			<li class="sigap-bottom-nav__item">
				<a
					class="sigap-bottom-nav__link"
					style:border-radius={RADIUS.control}
					href={destination.href}
					aria-label={destination.ariaLabel}
					aria-current={isActiveDestination(path, destination.href) ? 'page' : undefined}
				>
					{#if IconComponent}
						<Icon icon={IconComponent} size={22} />
					{/if}
					<span class="sigap-bottom-nav__label">{destination.label}</span>
				</a>
			</li>
		{/each}
	</ul>
</nav>

<style>
	/*
		Hidden from the bottom at desktop widths, where the single-line desktop
		nav takes over. `display: none` rather than visually-hidden: an
		off-screen nav landmark is still reachable by screen reader and by
		keyboard, so leaving it in the tree would produce two competing sets of
		navigation links on one page.
	*/
	.sigap-bottom-nav {
		display: none;
	}

	@media (max-width: 1023px) {
		.sigap-bottom-nav {
			display: block;
			position: fixed;
			bottom: 0;
			left: 0;
			right: 0;
			z-index: 40;
			background-color: var(--sigap-surface);
			border-top: 1px solid var(--sigap-border);
			/*
				Reserves the home-indicator inset on devices that have one, so the
				tab bar never sits under the system gesture area.
			*/
			padding-bottom: env(safe-area-inset-bottom, 0px);
		}
	}

	.sigap-bottom-nav__list {
		display: flex;
		align-items: stretch;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sigap-bottom-nav__item {
		flex: 1;
		min-width: 0;
	}

	.sigap-bottom-nav__link {
		position: relative;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		/* 12px padding plus 22px icon and label clears the 44px touch floor. */
		min-height: 56px;
		padding: 8px 4px;
		gap: 2px;
		font-size: 11px;
		line-height: 1.2;
		color: var(--sigap-muted);
		text-align: center;
		text-decoration: none;
	}

	.sigap-bottom-nav__link:hover {
		color: var(--sigap-foreground);
	}

	.sigap-bottom-nav__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-bottom-nav__label {
		/* Two words must not wrap and push the bar taller than the reserved space. */
		white-space: nowrap;
	}

	/*
		The active tab is marked by colour, weight, and a top rule — never by
		colour alone, so the state survives a monochrome or high-contrast
		display and is not a colour-only signal.
	*/
	.sigap-bottom-nav__link[aria-current='page'] {
		color: var(--sigap-primary);
		font-weight: 500;
	}

	.sigap-bottom-nav__link[aria-current='page']::before {
		content: '';
		position: absolute;
		top: 0;
		left: 50%;
		width: 32px;
		height: 2px;
		margin-left: -16px;
		background-color: var(--sigap-primary);
	}
</style>
