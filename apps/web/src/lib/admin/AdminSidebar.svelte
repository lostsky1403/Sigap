<script lang="ts">
	import { ADMIN_DESTINATIONS, isActiveDestination } from './navigation';
	import { RADIUS } from '$lib/design/tokens';
	import { LayoutDashboard, ListOrdered, CalendarCheck, CalendarClock, Building2, Bell } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import type { ComponentType } from 'svelte';

	/**
	 * The persistent admin sidebar: brand, six destinations, account.
	 *
	 * A real `<aside>` wrapping a real `<nav>` with an accessible name, so
	 * assistive tech can reach "Menu operasi" as a distinct landmark. The
	 * companion header nav in the citizen shell carries a different name, and
	 * two unlabelled navs on one page would be indistinguishable.
	 *
	 * The responsive behaviour is the frozen one and is deliberately two-state,
	 * not three: full sidebar at 1440px, and a 56px icon rail at ~1024px. There
	 * is no third mobile state. An operator's work is a desktop monitor; building
	 * a phone IA for admin would invent a design the phase was not asked for.
	 *
	 * `aria-label` on every link is what makes the collapsed rail usable. With
	 * the label hidden by CSS at 1024px, the visible text is gone, and an
	 * icon-only link with no accessible name is announced as just "link" — six
	 * identical links and no way to tell them apart. The label is therefore on
	 * every link at every width, not only in the collapsed state.
	 */
	export let path: string = '/admin';
	export let accountHref: string = '/';
	export let accountLabel: string = 'Beranda';

	const ICONS: Record<string, ComponentType> = {
		LayoutDashboard,
		ListOrdered,
		CalendarCheck,
		CalendarClock,
		Building2,
		Bell
	};
</script>

<aside class="sigap-admin-sidebar" aria-label="Navigasi panel operasi">
	<div class="sigap-admin-sidebar__brand">
		<span class="sigap-admin-sidebar__mark" aria-hidden="true">S</span>
		<span class="sigap-admin-sidebar__brand-text">
			<span class="sigap-admin-sidebar__brand-name">Sigap</span>
			<span class="sigap-admin-sidebar__brand-sub">Panel Operasi</span>
		</span>
	</div>

	<nav class="sigap-admin-sidebar__nav" aria-label="Menu operasi">
		<ul class="sigap-admin-sidebar__list">
			{#each ADMIN_DESTINATIONS as destination (destination.href)}
				{@const IconComponent = ICONS[destination.icon]}
				<li class="sigap-admin-sidebar__item">
					<a
						class="sigap-admin-sidebar__link"
						style:border-radius={RADIUS.control}
						href={destination.href}
						aria-label={destination.label}
						aria-current={isActiveDestination(path, destination.href) ? 'page' : undefined}
					>
						{#if IconComponent}
							<Icon icon={IconComponent} size={16} />
						{/if}
						<span class="sigap-admin-sidebar__label">{destination.label}</span>
					</a>
				</li>
			{/each}
		</ul>
	</nav>

	<div class="sigap-admin-sidebar__account">
		<!--
			A static link to Beranda, never a role-conditional one. The shell does
			not know whether the visitor is an operator, and guessing from a token
			would be exactly the client-side authorization inference Phase 3B0
			forbids. One link, always present, no branching.
		-->
		<a
			class="sigap-admin-sidebar__account-link"
			style:border-radius={RADIUS.control}
			href={accountHref}
			aria-label={accountLabel}
		>
			<Icon icon={Building2} size={16} />
			<span class="sigap-admin-sidebar__account-label">{accountLabel}</span>
		</a>
	</div>
</aside>

<style>
	/*
		Layout, not control, so it is held to no control scale. The two widths are
		the frozen ones: 224px expanded, 56px collapsed. The collapse breakpoint
		is 1179px, which is where a 224px rail plus a readable table stops fitting
		on a 1024px monitor.
	*/
	.sigap-admin-sidebar {
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		position: sticky;
		top: 0;
		width: 224px;
		height: 100vh;
		background-color: var(--sigap-surface);
		border-right: 1px solid var(--sigap-border);
	}

	.sigap-admin-sidebar__brand {
		display: flex;
		align-items: center;
		gap: 8px;
		height: 56px;
		padding: 0 16px;
		border-bottom: 1px solid var(--sigap-border);
	}

	.sigap-admin-sidebar__mark {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 24px;
		height: 24px;
		border-radius: 6px;
		background-color: var(--sigap-primary);
		color: var(--sigap-surface);
		font-size: 12px;
		font-weight: 600;
		line-height: 1;
	}

	.sigap-admin-sidebar__brand-text {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.sigap-admin-sidebar__brand-name {
		font-size: 14px;
		font-weight: 600;
		line-height: 16px;
		color: var(--sigap-foreground);
	}

	.sigap-admin-sidebar__brand-sub {
		font-size: 11px;
		line-height: 16px;
		color: var(--sigap-muted);
	}

	.sigap-admin-sidebar__nav {
		flex: 1;
		padding: 12px 10px;
		overflow-y: auto;
	}

	.sigap-admin-sidebar__list {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sigap-admin-sidebar__item + .sigap-admin-sidebar__item {
		margin-top: 4px;
	}

	.sigap-admin-sidebar__link {
		display: flex;
		align-items: center;
		gap: 10px;
		/*
			`var(--sigap-control-admin-comfortable)` = 40px, not a literal.

			The frozen HTML reference draws its nav rows at 38px, but 38 is not a
			design token: CONTROL_HEIGHT is exactly { citizen: 44, adminCompact: 36,
			adminComfortable: 40 }. Copying the mockup's number would introduce a
			third admin density that no token names, and the visual audit rejects
			any control height that is neither a token nor a citizen target. The
			comfortable density is the right of the two anyway — it is the size
			meant for an operator working a queue all day — and 40 > 38, so the row
			is the more comfortable of the two candidates.
		*/
		height: var(--sigap-control-admin-comfortable);
		padding: 0 12px;
		font-size: 14px;
		font-weight: 450;
		color: var(--sigap-foreground);
		text-decoration: none;
		border: 1px solid transparent;
	}

	.sigap-admin-sidebar__link:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-admin-sidebar__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	/*
		The active destination is marked by colour AND weight AND a border, so the
		state survives a monochrome display and is never colour-only. Exactly one
		link carries `aria-current`, decided by exact-path comparison in
		navigation.ts.
	*/
	.sigap-admin-sidebar__link[aria-current='page'] {
		background-color: var(--sigap-surface);
		border-color: var(--sigap-border);
		color: var(--sigap-primary);
		font-weight: 500;
	}

	.sigap-admin-sidebar__label {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sigap-admin-sidebar__account {
		padding: 12px 10px;
		border-top: 1px solid var(--sigap-border);
	}

	.sigap-admin-sidebar__account-link {
		display: flex;
		align-items: center;
		gap: 10px;
		/* Same frozen density as the nav rows above, for the same reason. */
		height: var(--sigap-control-admin-comfortable);
		padding: 0 12px;
		font-size: 13px;
		color: var(--sigap-foreground);
		text-decoration: none;
		border: 1px solid var(--sigap-border);
	}

	.sigap-admin-sidebar__account-link:hover {
		background-color: var(--sigap-canvas);
		color: var(--sigap-primary);
	}

	.sigap-admin-sidebar__account-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	/*
		~1024px: the 56px icon rail.

		The visible label is hidden, but `aria-label` on the anchor is not — the
		accessible name is what keeps six icon-only links distinguishable. Icons
		are `aria-hidden` in the Icon wrapper, so hiding the text does not
		double-announce anything.
	*/
	@media (max-width: 1179px) {
		.sigap-admin-sidebar {
			width: 56px;
		}

		.sigap-admin-sidebar__brand,
		.sigap-admin-sidebar__link,
		.sigap-admin-sidebar__account-link {
			justify-content: center;
		}

		.sigap-admin-sidebar__brand {
			padding: 0;
		}

		.sigap-admin-sidebar__link,
		.sigap-admin-sidebar__account-link {
			padding: 0;
		}

		.sigap-admin-sidebar__brand-text,
		.sigap-admin-sidebar__label,
		.sigap-admin-sidebar__account-label {
			display: none;
		}
	}
</style>
