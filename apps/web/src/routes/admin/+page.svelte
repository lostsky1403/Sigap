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
		nextSchedule
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
				// A partial failure is still a failure. Showing a headline strip with
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
	 * the strip for a loading panel, so every figure would flicker on each
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
			The headline strip. Each figure names its source, because a count an
			operator cannot trace is a number they have to take on faith.
		-->
		<section class="sigap-ringkasan-strip" aria-label="Ringkasan angka utama">
			{#each metrics as metric (metric.label)}
				<div class="sigap-ringkasan-strip__item">
					<p class="sigap-ringkasan-strip__label">{metric.label}</p>
					<p class="sigap-ringkasan-strip__value">{formatNumber(metric.value)}</p>
					<p class="sigap-ringkasan-strip__source">
						{#if metric.href && metric.detail}
							<a class="sigap-ringkasan-strip__link" href={metric.href}>
								{metric.source}
							</a>
							<span class="sigap-ringkasan-strip__detail">{metric.detail}</span>
						{:else}
							{metric.source}
						{/if}
					</p>
				</div>
			{/each}
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
							{#if item.href}
								<a class="sigap-ringkasan-strip__link" href={item.href}>Buka</a>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</section>

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
			<a class="sigap-ringkasan-strip__link" href="/admin/queues">Buka Antrean</a>
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
		The count strip. A rule-separated strip on `--sigap-surface`, NOT a row of
		KPI cards.

		Why the distinction is load-bearing: a shadowed, tinted,
		big-number-in-a-box tile is the visual grammar of a generic SaaS dashboard,
		and it implies a comparison and a trend that these four scoped reads cannot
		support. The page carries integers and their provenance and nothing else, so
		the strip carries the numbers on the page ground, with a hairline between
		them and an 11px uppercase label above each one. No shadow, no tint, no box
		per figure.
	*/
	.sigap-ringkasan-strip {
		display: flex;
		flex-wrap: wrap;
		background-color: var(--sigap-surface);
		border-top: 1px solid var(--sigap-border-subtle);
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ringkasan-strip__item {
		flex: 1 1 160px;
		min-width: 160px;
		padding: var(--sigap-space-4);
		/* The rule between two figures. Decorative, so the subtle token. */
		border-right: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ringkasan-strip__item:last-child {
		border-right: none;
	}

	.sigap-ringkasan-strip__label {
		margin: 0 0 var(--sigap-space-1);
		/*
			Sentence case at 12px, which is what the Pen draws (`Antrean aktif`,
			`Janji temu terjadwal`, `Fasilitas aktif` at fs=12, fw=normal). It was
			an 11px uppercase micro-label, which reads as a column header rather
			than as the name of the figure beneath it — and it made this strip
			disagree with the Notifikasi summary cards, which use sentence-case
			badge labels for the same kind of datum.
		*/
		font-size: var(--sigap-type-caption);
		font-weight: 400;
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-strip__value {
		margin: 0;
		font-size: var(--sigap-type-page-title);
		font-weight: 600;
		color: var(--sigap-foreground);
		font-variant-numeric: tabular-nums;
	}

	.sigap-ringkasan-strip__source {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-strip__link {
		font-size: var(--sigap-type-label);
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-ringkasan-strip__link:hover {
		text-decoration: underline;
	}

	.sigap-ringkasan-strip__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-ringkasan-strip__detail {
		display: block;
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
