<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import {
		listAppointments,
		listFacilities,
		listQueueTickets,
		listSchedules
	} from '$lib/api/endpoints/admin';
	import { indexBy, facilityName } from '$lib/domain/joins';
	import { formatDate, formatTime, formatNumber } from '$lib/domain/format';
	import { QUEUE_STATUS_LABEL, queueTone } from '$lib/domain/status';
	import { formatUpdatedAt, isEmptyScope } from '$lib/admin/readState';
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
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	onMount(() => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		// All four are issued together. The overview cannot show a count until every
		// source has answered, so there is nothing to gain from sequencing them.
		Promise.all([
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
	});

	$: metrics = headlineMetrics({ queues, appointments, schedules, facilities });
	$: attention = attentionItems({ queues, appointments, schedules, facilities });
	$: upcoming = nextSchedule(schedules);
	$: facilityIndex = indexBy(facilities, (facility) => facility.id);

	const QUEUE_BANDS: readonly QueueStatus[] = ['called', 'waiting', 'in_service'];
</script>

<div class="sigap-admin-page">
	<AdminPageHeader title="Ringkasan" subtitle="Ringkasan kegiatan dalam lingkup akses Anda.">
		<svelte:fragment slot="aside">
			{#if loadedAt}
				<span class="sigap-admin-updated" aria-live="polite">
					{formatUpdatedAt(loadedAt)}
				</span>
			{/if}
		</svelte:fragment>
	</AdminPageHeader>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={facilities.length}
		scopeEmpty={loading === false && error === null && isEmptyScope(facilities)}
		emptyTitle="Belum ada fasilitas"
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
						<StatusBadge
							label={QUEUE_STATUS_LABEL[status]}
							tone={queueTone(status)}
							size="sm"
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
								{formatTime(upcoming.start_time)}–{formatTime(upcoming.end_time)}
							</td>
							<td>{facilityName(facilityIndex, upcoming.facility_id)}</td>
							<td>{formatNumber(upcoming.capacity_per_slot)}</td>
							<td>
								<StatusBadge label="Aktif" tone="success" size="sm" />
							</td>
						</tr>
					</tbody>
				</table>
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

	.sigap-admin-updated {
		font-size: 12px;
		color: var(--sigap-muted);
		font-variant-numeric: tabular-nums;
	}

	.sigap-ringkasan-strip {
		display: flex;
		flex-wrap: wrap;
		gap: 32px;
		padding: 14px 0;
	}

	.sigap-ringkasan-strip__item {
		min-width: 160px;
	}

	.sigap-ringkasan-strip__label {
		margin: 0 0 2px;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-strip__value {
		margin: 0;
		font-size: 20px;
		font-weight: 600;
		color: var(--sigap-foreground);
		font-variant-numeric: tabular-nums;
	}

	.sigap-ringkasan-strip__source {
		margin: 2px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-strip__link {
		font-size: 13px;
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
		margin: 0 0 20px;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-section {
		margin-bottom: 24px;
	}

	.sigap-ringkasan-section__title {
		margin: 0 0 8px;
		font-size: 13px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-ringkasan-section__empty {
		margin: 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-ringkasan-attention {
		margin: 0;
		padding: 0;
		list-style: none;
		border-top: 1px solid var(--sigap-border);
		border-bottom: 1px solid var(--sigap-border);
	}

	.sigap-ringkasan-attention__row {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 40px;
		font-size: 13px;
		border-bottom: 1px solid var(--sigap-border);
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
		gap: 12px;
		margin-bottom: 8px;
	}

	.sigap-ringkasan-bands__item {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px 12px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-radius: 8px;
	}

	.sigap-ringkasan-bands__count {
		margin: 0;
		font-size: 18px;
		font-weight: 600;
		color: var(--sigap-foreground);
		font-variant-numeric: tabular-nums;
	}

	.sigap-ringkasan-schedule {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
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
		padding: 10px 12px;
		text-align: left;
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--sigap-muted);
		border-bottom: 1px solid var(--sigap-border);
		white-space: nowrap;
	}

	.sigap-ringkasan-schedule td {
		height: 44px;
		padding: 0 12px;
		border-bottom: 1px solid var(--sigap-border);
		background-color: var(--sigap-surface);
	}
</style>
