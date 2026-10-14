<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { MapPin, CalendarPlus, ClipboardCheck, Ticket, Activity } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import type { ComponentType } from 'svelte';

	/**
	 * Beranda's primary actions.
	 *
	 * Every entry links to a route that exists. Nothing here is aspirational:
	 * a quick action pointing at a page that 404s is worse than no action at
	 * all, because it teaches a citizen that the app is broken.
	 *
	 * Check-in and walk-in are separate entries, not one merged "queue" action.
	 * They are genuinely different journeys: check-in redeems a code for an
	 * appointment you already booked, walk-in takes a number on arrival. Folding
	 * them together is how someone ends up in a walk-in queue holding an
	 * appointment they never used.
	 *
	 * Walk-in targets `/queues/new`, which Phase 3B3 implements. The link is
	 * correct now and the destination arrives with that phase; this phase owns
	 * the shell and the catalog, not the transactional flow.
	 */
	interface QuickAction {
		label: string;
		description: string;
		href: string;
		icon: ComponentType;
	}

	/**
	 * `primary` is the one action the frozen design promotes visually. It is a
	 * single button, not a tile, so the page has one unambiguous first step.
	 */
	const PRIMARY: QuickAction = {
		label: 'Buat Janji Temu',
		description: 'Pesan kunjungan lebih awal agar tidak antre panjang.',
		href: '/appointments/new',
		icon: CalendarPlus
	};

	const SECONDARY: QuickAction[] = [
		{
			label: 'Cari Faskes',
			description: 'Lihat faskes yang tersedia',
			href: '/faskes',
			icon: MapPin
		},
		{
			label: 'Check-In Janji Temu',
			description: 'Untuk yang sudah punya janji',
			href: '/appointments/check-in',
			icon: ClipboardCheck
		},
		{
			label: 'Ambil Antrean Tanpa Janji',
			description: 'Datang langsung hari ini',
			href: '/queues/new',
			icon: Ticket
		},
		{
			label: 'Cek Status Kunjungan',
			description: 'Lihat posisi antrean Anda',
			href: '/patient/status',
			icon: Activity
		}
	];
</script>

<section class="sigap-quick-actions" aria-labelledby="sigap-quick-actions-title">
	<h2 id="sigap-quick-actions-title" class="sigap-section-title">Mulai dari sini</h2>

	<a
		class="sigap-quick-actions__primary"
		style:border-radius={RADIUS.control}
		href={PRIMARY.href}
	>
		<Icon icon={PRIMARY.icon} size={20} />
		<span>{PRIMARY.label}</span>
	</a>

	<h3 class="sigap-section-title sigap-quick-actions__subhead">Layanan lain</h3>

	<ul class="sigap-quick-actions__list" style:border-radius={RADIUS.panel}>
		{#each SECONDARY as action (action.href)}
			<li>
				<a
					class="sigap-quick-actions__item"
					style:border-radius={RADIUS.control}
					href={action.href}
				>
					<span class="sigap-quick-actions__icon">
						<Icon icon={action.icon} size={18} />
					</span>
					<span class="sigap-quick-actions__text">
						<span class="sigap-quick-actions__label">{action.label}</span>
						<span class="sigap-quick-actions__description">{action.description}</span>
					</span>
				</a>
			</li>
		{/each}
	</ul>

	<p class="sigap-quick-actions__note">
		Check-in dan antrean tanpa janji adalah dua jalur berbeda.
	</p>
</section>

<style>
	.sigap-quick-actions__primary {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		/* Citizen touch floor. */
		min-height: 48px;
		margin-top: 8px;
		padding: 0 16px;
		font-size: 16px;
		font-weight: 500;
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-quick-actions__primary:hover {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-quick-actions__primary:active {
		background-color: var(--sigap-primary-active);
	}

	.sigap-quick-actions__primary:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-quick-actions__subhead {
		margin-top: 24px;
	}

	.sigap-quick-actions__list {
		margin: 8px 0 0;
		padding: 0;
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-quick-actions__list > li + li {
		border-top: 1px solid var(--sigap-border);
	}

	.sigap-quick-actions__item {
		display: flex;
		align-items: center;
		gap: 12px;
		/* 56px row: comfortably past the 44px citizen touch target. */
		min-height: 56px;
		padding: 12px 16px;
		text-decoration: none;
	}

	.sigap-quick-actions__item:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-quick-actions__item:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-quick-actions__icon {
		flex: none;
		display: flex;
		color: var(--sigap-muted);
	}

	.sigap-quick-actions__text {
		min-width: 0;
	}

	.sigap-quick-actions__label {
		display: block;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-quick-actions__description {
		display: block;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-quick-actions__note {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
