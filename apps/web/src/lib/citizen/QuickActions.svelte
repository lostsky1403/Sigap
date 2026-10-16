<script lang="ts">
	import Icon from '$lib/ui/Icon.svelte';
	import { ChevronRight, ClipboardCheck, Ticket, Activity } from 'lucide-svelte';
	import type { ComponentType } from 'svelte';

	/**
	 * Beranda's task picker — the "Mulai dari sini" section.
	 *
	 * Composition follows the approved V2.2 frame: an uppercase eyebrow, a
	 * section title, ONE filled primary action, then a rule-separated list of
	 * the remaining journeys.
	 *
	 * The primary action reads "Cari faskes dan buat janji temu" and lands on
	 * /faskes, because that is where the journey actually starts: the catalog
	 * row is what carries the facility into the booking form. Pointing the
	 * button straight at /appointments/new would drop the citizen into a
	 * facility picker with no context, and would also remove the only
	 * prominent route into the catalog.
	 *
	 * The list is three rows, not four. "Cari Faskes" is no longer a row
	 * because the primary action now IS that journey; keeping both would give
	 * one destination two entries in the same section.
	 *
	 * Check-in and walk-in stay separate rows. They are genuinely different
	 * journeys — check-in redeems a code for an appointment you already
	 * booked, walk-in takes a number on arrival — and folding them together is
	 * how someone ends up in a walk-in queue holding an unused appointment.
	 */
	interface TaskRow {
		label: string;
		hint: string;
		href: string;
		icon: ComponentType;
	}

	const PRIMARY = {
		label: 'Cari faskes dan buat janji temu',
		href: '/faskes'
	} as const;

	const TASKS: TaskRow[] = [
		{
			label: 'Check-In Janji Temu',
			hint: 'Untuk yang sudah punya janji temu',
			href: '/appointments/check-in',
			icon: ClipboardCheck
		},
		{
			label: 'Ambil Antrean Tanpa Janji',
			hint: 'Datang langsung ke fasilitas',
			href: '/queues/new',
			icon: Ticket
		},
		{
			label: 'Cek Status Kunjungan',
			hint: 'Lihat status kunjungan Anda',
			href: '/patient/status',
			icon: Activity
		}
	];
</script>

<section class="sigap-tasks" aria-labelledby="sigap-tasks-title">
	<div class="sigap-tasks__head">
		<p class="sigap-eyebrow">Mulai dari sini</p>
		<h2 id="sigap-tasks-title" class="sigap-section-title">Pilih tugas Anda</h2>
	</div>

	<a class="sigap-tasks__primary" href={PRIMARY.href}>
		<span>{PRIMARY.label}</span>
		<Icon icon={ChevronRight} size={18} />
	</a>

	<ul class="sigap-tasks__list">
		{#each TASKS as task (task.href)}
			<li class="sigap-tasks__row">
				<a class="sigap-tasks__link" href={task.href}>
					<span class="sigap-tasks__icon">
						<Icon icon={task.icon} size={18} />
					</span>
					<span class="sigap-tasks__text">
						<span class="sigap-tasks__label">{task.label}</span>
						<span class="sigap-tasks__hint">{task.hint}</span>
					</span>
					<span class="sigap-tasks__chevron">
						<Icon icon={ChevronRight} size={18} />
					</span>
				</a>
			</li>
		{/each}
	</ul>
</section>

<style>
	.sigap-tasks__head {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-2);
	}

	/*
	 * The one filled primary action in this section, and the only filled
	 * primary on the page besides the masthead's outlined CTA. 480px is the
	 * approved measure — it is a deliberate size, not `width: 100%`, so the
	 * button reads as a control rather than a banner on a wide screen.
	 */
	.sigap-tasks__primary {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sigap-space-3);
		width: 100%;
		max-width: 480px;
		min-height: 52px;
		margin-top: var(--sigap-space-4);
		padding: 0 var(--sigap-space-4);
		font-size: 15px;
		font-weight: 600;
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		border-radius: var(--sigap-radius-control);
		text-decoration: none;
	}

	.sigap-tasks__primary:hover {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-tasks__primary:active {
		background-color: var(--sigap-primary-active);
	}

	.sigap-tasks__primary:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	/*
	 * Rules instead of boxes: the list has a top and bottom hairline and each
	 * row a separator, but no panel outline. A bordered card around three
	 * plain rows is exactly the box the design removes.
	 */
	.sigap-tasks__list {
		margin: var(--sigap-space-4) 0 0;
		padding: 0;
		list-style: none;
		border-top: 1px solid var(--sigap-border-subtle);
	}

	.sigap-tasks__row {
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-tasks__link {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-3);
		min-height: 56px;
		padding: var(--sigap-space-2) var(--sigap-space-1);
		text-decoration: none;
	}

	.sigap-tasks__link:hover {
		background-color: var(--sigap-surface);
	}

	.sigap-tasks__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-tasks__icon {
		flex: none;
		display: flex;
		color: var(--sigap-primary);
	}

	.sigap-tasks__text {
		min-width: 0;
		flex: 1 1 auto;
	}

	.sigap-tasks__label {
		display: block;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-tasks__hint {
		display: block;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-tasks__chevron {
		flex: none;
		display: flex;
		color: var(--sigap-muted);
	}
</style>
