<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminFreshness from '$lib/admin/AdminFreshness.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import AdminStatusBadge, {
		QUEUE_STATUS_ICON,
		activeStateIcon
	} from '$lib/admin/AdminStatusBadge.svelte';
	import {
		listAppointments,
		listFacilities,
		listQueueTickets,
		listSchedules
	} from '$lib/api/endpoints/admin';
	import { indexBy, facilityName } from '$lib/domain/joins';
	import { formatClockTime, formatDate, formatNumber } from '$lib/domain/format';
	import { QUEUE_STATUS_LABEL, queueTone } from '$lib/domain/status';
	import { isEmptyScope } from '$lib/admin/readState';
	import {
		activeQueueCount,
		attentionItems,
		headlineMetrics,
		nextSchedule,
		scopeLabel
	} from '$lib/admin/ringkasan';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type { ApiError } from '$lib/api/errors';
	import type {
		AdminAppointment,
		AdminFacility,
		AdminQueueTicket,
		AdminSchedule,
		QueueStatus
	} from '$lib/api/types/api';

	/**
	 * T-3B4-03: Ringkasan.
	 *
	 * A derived-counts overview and nothing more. Every figure comes from one of
	 * four already-loaded, backend-scoped lists, and each is labelled with the
	 * source it came from so an operator can check it.
	 *
	 * What is deliberately absent is as important as what is present: no
	 * percentage, no trend arrow, no comparison to yesterday, no SLA, no
	 * occupancy rate, no chart, no global total, and no notification fallback.
	 * Every one of those needs a denominator or a history these four reads do not
	 * carry, and an invented number on a clinic dashboard is one an operator will
	 * act on. `ringkasan.ts` computes the counts and this page renders them.
	 *
	 * This page does NOT poll. It is a snapshot of the four reads taken together,
	 * and its freshness label says when. Polling an overview an operator glances
	 * at would spend requests to change numbers nobody is acting on.
	 */
	let queues: AdminQueueTicket[] = [];
	let appointments: AdminAppointment[] = [];
	let schedules: AdminSchedule[] = [];
	let facilities: AdminFacility[] = [];
	let loading = true;
	let error: ApiError | null = null;
	let loadedAt: Date | null = null;
	/*
		Separate from `loading`: that drives `AdminReadState`, which replaces the
		whole overview with a loading panel. Right for a first read, wrong for a
		refresh — the operator asked for fresh numbers and should keep the ones
		they are reading until the new ones arrive.
	*/
	let refreshing = false;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	onMount(() => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});
		void loadOverview();
	});

	/**
	 * The four scoped reads, as one reloadable unit.
	 *
	 * Named rather than inlined in `onMount` because `AdminReadState`'s retry has
	 * to be able to run it again. An inline `Promise.all` inside the mount hook
	 * leaves the error panel with nothing to call, which is how a failed overview
	 * becomes a dead end: the operator's only way back is a manual page reload.
	 *
	 * `error` is cleared on entry so a retry cannot render the previous failure
	 * next to the request that is replacing it, and `loading` is set so the retry
	 * shows the loading state rather than a frozen error panel.
	 */
	async function loadOverview() {
		loading = true;
		error = null;

		// All four are issued together. The overview cannot show a count until every
		// source has answered, so there is nothing to gain from sequencing them.
		await Promise.all([
			listQueueTickets(),
			listAppointments(),
			listSchedules(),
			listFacilities()
		])
			.then(([queueResult, appointmentResult, scheduleResult, facilityResult]) => {
				// A partial failure is still a failure. Showing a headline summary with
				// three of four numbers would present an incomplete overview as a
				// complete one, and the operator cannot tell which figure is missing.
				if (!queueResult.ok) return (error = queueResult.error);
				if (!appointmentResult.ok) return (error = appointmentResult.error);
				if (!scheduleResult.ok) return (error = scheduleResult.error);
				if (!facilityResult.ok) return (error = facilityResult.error);

				queues = queueResult.data;
				appointments = appointmentResult.data;
				schedules = scheduleResult.data;
				facilities = facilityResult.data;
				// Stamped only after ALL four succeed, so the label can never claim a
				// freshness for an overview that is missing a source.
				loadedAt = new Date();
			})
			.finally(() => (loading = false));
	}

	/**
	 * The header's `Perbarui`. Re-reads without blanking the overview.
	 *
	 * It cannot call `loadOverview` directly: that sets `loading`, which swaps
	 * the summary for a loading panel, so every figure would flicker on each
	 * refresh. The tradeoff is explicit — a refresh that fails leaves the
	 * previous figures on screen with a stale `loadedAt`, which is the honest
	 * state, rather than showing an error over numbers that are still the last
	 * ones the server confirmed.
	 */
	async function refreshOverview() {
		refreshing = true;
		try {
			await loadOverview();
		} finally {
			refreshing = false;
		}
	}

	$: metrics = headlineMetrics({ queues, appointments, schedules, facilities });
	$: attention = attentionItems({ queues, appointments, schedules, facilities });
	$: upcoming = nextSchedule(schedules);
	$: scope = scopeLabel(facilities);
	$: facilityIndex = indexBy(facilities, (facility) => facility.id);

	const QUEUE_BANDS: readonly QueueStatus[] = ['called', 'waiting', 'in_service'];
</script>

<div class="sigap-admin-page">
	<AdminPageHeader title="Ringkasan" subtitle="Ringkasan kegiatan dalam lingkup akses Anda.">
		<svelte:fragment slot="aside">
			<AdminFreshness {loadedAt} refreshing={refreshing} onRefresh={() => void refreshOverview()} />
		</svelte:fragment>
	</AdminPageHeader>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={facilities.length}
		scopeEmpty={loading === false && error === null && isEmptyScope(facilities)}
		emptyTitle="Belum ada fasilitas"
		onRetry={() => void loadOverview()}
	>
		<!--
			The scope, stated before any figure. Every number below is bounded by
			WHICH facilities the operator holds in scope (one named facility, or the
			unscoped all-facilities view), so a glance cannot mistake a scoped
			snapshot for a whole-clinic total. Freshness stays in the page header
			(AdminFreshness) — stating the timestamp twice would make the two
			disagree on a re-read.
		-->
		<p class="sigap-ringkasan-scope">
			<span class="sigap-ringkasan-scope__name">{scope}</span>
		</p>

		<!--
			The attention list comes FIRST: it is the actionable half. Each row is
			a count, the source it was counted from, and where to act on it — and
			nothing else. A row an operator cannot follow is noise sitting above
			the numbers that actually matter.
		-->
		<section class="sigap-ringkasan-section" aria-labelledby="ringkasan-attention">
			<h2 class="sigap-ringkasan-section__title" id="ringkasan-attention">Perlu perhatian</h2>
			{#if attention.length === 0}
				<p class="sigap-ringkasan-section__empty">
					Tidak ada hal yang perlu perhatian dari data yang dimuat.
				</p>
			{:else}
				<ul class="sigap-ringkasan-attention">
					{#each attention as item (item.label)}
						<li class="sigap-ringkasan-attention__row">
							<span class="sigap-ringkasan-attention__count">{formatNumber(item.value)}</span>
							<span class="sigap-ringkasan-attention__label">{item.label}</span>
							<span class="sigap-ringkasan-attention__source">{item.source}</span>
							{#if item.href}
								<a class="sigap-ringkasan-link" href={item.href}>Buka</a>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<!--
			The compact summary, AFTER the attention list. Labelled lines rather
			than a KPI-card grid: each figure is one traceable line, and a row of
			shadowed tiles would imply comparisons and trends these four scoped
			reads cannot support.
		-->
		<section class="sigap-ringkasan-section" aria-label="Ringkasan angka utama">
			<ul class="sigap-ringkasan-summary">
				{#each metrics as metric (metric.label)}
					<li class="sigap-ringkasan-summary__row">
						<span class="sigap-ringkasan-summary__label">{metric.label}</span>
						<span class="sigap-ringkasan-summary__value">{formatNumber(metric.value)}</span>
						<span class="sigap-ringkasan-summary__source">
							{#if metric.href}
								<a class="sigap-ringkasan-link" href={metric.href}>{metric.source}</a>
							{:else}
								{metric.source}
							{/if}
						</span>
					</li>
				{/each}
			</ul>
		</section>

		<!--
			The provenance disclaimer, stated plainly rather than implied.

			"Bukan data real-time" is the important half. This page does not poll, so
			an operator glancing at it is looking at a snapshot, and a figure that
			looks live would send someone to a counter on a number that has since
			moved. The second half matters just as much: these counts are scoped, so
			they describe the operator's facilities and not the whole clinic.
		-->
		<p class="sigap-ringkasan-note">
			Ringkasan dihitung dari data yang dimuat untuk fasilitas dalam lingkup operator.
			Bukan data real-time, dan bukan angka untuk seluruh klinik.
		</p>

		<section class="sigap-ringkasan-section" aria-labelledby="ringkasan-queue">
			<h2 class="sigap-ringkasan-section__title" id="ringkasan-queue">Antrean saat ini</h2>
			<div class="sigap-ringkasan-bands">
				{#each QUEUE_BANDS as status (status)}
					<div class="sigap-ringkasan-bands__item">
						<AdminStatusBadge
							label={QUEUE_STATUS_LABEL[status]}
							tone={queueTone(status)}
							size="sm"
							icon={QUEUE_STATUS_ICON[status]}
						/>
						<!--
							Counted from the scoped queue list, not from a dedicated
							endpoint. The total beside it is the derived active count, so
							the two are always from the same read and cannot disagree.
						-->
						<p class="sigap-ringkasan-bands__count">
							{formatNumber(queues.filter((ticket) => ticket.status === status).length)}
						</p>
					</div>
				{/each}
			</div>
			<a class="sigap-ringkasan-link" href="/admin/queues">Buka Antrean</a>
		</section>

		<section class="sigap-ringkasan-section" aria-labelledby="ringkasan-schedule">
			<h2 class="sigap-ringkasan-section__title" id="ringkasan-schedule">Jadwal berikutnya</h2>
			{#if !upcoming}
				<p class="sigap-ringkasan-section__empty">
					Tidak ada jadwal aktif berikutnya dalam lingkup operator.
				</p>
			{:else}
				<!--
					The scroll region is required, not decorative. This table has five
					columns and is wider than a 390px viewport, and without a scroller
					of its own it widens the PAGE — which on the admin shell means the
					sidebar scrolls away and the operator loses their navigation. The
					shared `DataTable` wraps its own table for exactly this reason;
					this bespoke table has to do the same.
				-->
				<div class="sigap-ringkasan-schedule__region">
					<table class="sigap-ringkasan-schedule">
						<caption class="sigap-ringkasan-schedule__caption">
							Jadwal aktif terdekat dalam lingkup operator
						</caption>
						<thead>
							<tr>
								<th scope="col">Tanggal</th>
								<th scope="col">Waktu</th>
								<th scope="col">Fasilitas</th>
								<th scope="col">Kapasitas per slot</th>
								<th scope="col">Status</th>
							</tr>
						</thead>
						<tbody>
							<tr>
								<td>{formatDate(upcoming.schedule_date)}</td>
								<td>
									{formatClockTime(upcoming.start_time)}–{formatClockTime(upcoming.end_time)}
								</td>
								<td>{facilityName(facilityIndex, upcoming.facility_id)}</td>
								<td>{formatNumber(upcoming.capacity_per_slot)}</td>
								<td>
									<AdminStatusBadge
										label="Aktif"
										tone="success"
										size="sm"
										icon={activeStateIcon(true)}
									/>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
				<p class="sigap-ringkasan-note">
					Nama unit layanan tidak ditampilkan — data jadwal hanya berisi kode unit.
				</p>
			{/if}
		</section>
	</AdminReadState>
</div>

<style>
	.sigap-admin-page {
		min-width: 0;
	}


	/*
		The scope + freshness line. Stated before any figure, it names the TWO
		horizons every number below is bounded by — the facility scope (derived
		from the loaded facility list) and the read time (the same `loadedAt`
		the sticky header shows). It is a sentence, not a tile, so it cannot be
		mistaken for a metric.
	*/
	.sigap-ringkasan-scope {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--sigap-space-2);
		margin: 0 0 var(--sigap-space-4);
		font-size: var(--sigap-type-label);
	}

	.sigap-ringkasan-scope__name {
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	/*
		The compact summary: labelled lines, NOT a KPI-card grid. A shadowed,
		tinted, big-number-in-a-box tile is the visual grammar of a generic SaaS
		dashboard and implies comparisons and trends these four scoped reads
		cannot support. Each line is one figure plus its provenance, on the page
		ground with hairline separators. No shadow, no tint, no box per figure.
	*/
	.sigap-ringkasan-summary {
		margin: 0;
		padding: 0;
		list-style: none;
		border-top: 1px solid var(--sigap-border-subtle);
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ringkasan-summary__row {
		display: flex;
		align-items: baseline;
		gap: var(--sigap-space-3);
		min-height: var(--sigap-control-admin-comfortable);
		font-size: var(--sigap-type-label);
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ringkasan-summary__row:last-child {
		border-bottom: none;
	}

	.sigap-ringkasan-summary__label {
		flex: 1;
		min-width: 0;
		color: var(--sigap-foreground);
	}

	.sigap-ringkasan-summary__value {
		/* Two-digit alignment so the sources line up as counts change width. */
		min-width: 2ch;
		text-align: right;
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: var(--sigap-foreground);
	}

	.sigap-ringkasan-summary__source {
		text-align: right;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		white-space: nowrap;
	}

	.sigap-ringkasan-link {
		/*
			An inline follow-through ("Buka", "Daftar antrean", "Buka Antrean"),
			not a word: an invisible 36px compact floor so the link is a target
			without restyling the row it sits in.
		*/
		display: inline-flex;
		align-items: center;
		min-height: var(--sigap-control-admin-compact);
		font-size: var(--sigap-type-label);
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-ringkasan-link:hover {
		text-decoration: underline;
	}

	.sigap-ringkasan-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-ringkasan-note {
		margin: 0 0 var(--sigap-space-5);
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-section {
		margin-bottom: var(--sigap-space-5);
	}

	.sigap-ringkasan-section__title {
		margin: 0 0 var(--sigap-space-2);
		font-size: var(--sigap-type-label);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-ringkasan-section__empty {
		margin: 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-attention {
		margin: 0;
		padding: 0;
		list-style: none;
		/* List hairlines, not control boundaries. */
		border-top: 1px solid var(--sigap-border-subtle);
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ringkasan-attention__row {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-3);
		min-height: var(--sigap-control-admin-comfortable);
		font-size: var(--sigap-type-label);
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ringkasan-attention__row:last-child {
		border-bottom: none;
	}

	.sigap-ringkasan-attention__count {
		font-weight: 600;
		/* Two-digit alignment so the labels line up as counts change width. */
		min-width: 2ch;
		text-align: right;
		font-variant-numeric: tabular-nums;
	}

	.sigap-ringkasan-attention__label {
		flex: 1;
		min-width: 0;
	}

	.sigap-ringkasan-attention__source {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		white-space: nowrap;
	}

	.sigap-ringkasan-bands {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sigap-space-3);
		margin-bottom: var(--sigap-space-2);
	}

	.sigap-ringkasan-bands__item {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-1);
		padding: var(--sigap-space-2) var(--sigap-space-3);
		background-color: var(--sigap-surface);
		/* A panel outline is decorative; the badge inside carries the state. */
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-ringkasan-bands__count {
		margin: 0;
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
		font-variant-numeric: tabular-nums;
	}

	/*
		The table's own scroll container. A five-column table is wider than a
		narrow viewport, and letting it widen the page would take the sidebar with
		it — the operator would lose navigation while reading the table. Same
		contract as the shared `DataTable` region.
	*/
	.sigap-ringkasan-schedule__region {
		overflow-x: auto;
		max-width: 100%;
	}

	.sigap-ringkasan-schedule {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--sigap-type-label);
	}

	.sigap-ringkasan-schedule__caption {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	.sigap-ringkasan-schedule th {
		padding: var(--sigap-space-3);
		text-align: left;
		/* 11px uppercase muted header on the sunken ground, per the admin register. */
		font-size: var(--sigap-type-micro);
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface-sunken);
		/* The header's own bottom edge: a rule, not a control boundary. */
		border-bottom: 1px solid var(--sigap-border-subtle);
		white-space: nowrap;
	}

	.sigap-ringkasan-schedule td {
		height: var(--sigap-control-admin-comfortable);
		padding: 0 var(--sigap-space-3);
		/* A row separator, so the subtle token. */
		border-bottom: 1px solid var(--sigap-border-subtle);
		background-color: var(--sigap-surface);
	}
</style>
