<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import AdminToolbar from '$lib/admin/AdminToolbar.svelte';
	import FacilityFilter from '$lib/admin/FacilityFilter.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { listFacilities, listSchedules, listServiceUnits } from '$lib/api/endpoints/admin';
	import { indexBy, facilityName, serviceUnitName } from '$lib/domain/joins';
	import { formatDate, formatTime } from '$lib/domain/format';
	import { isEmptyScope, ORDINARY_EMPTY_DESCRIPTION } from '$lib/admin/readState';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type { ApiError } from '$lib/api/errors';
	import type { AdminFacility, AdminSchedule, AdminServiceUnit } from '$lib/api/types/api';

	/**
	 * T-3B4-06: the schedule read table.
	 *
	 * READ-ONLY, and the reason is a security requirement rather than a scoping
	 * convenience.
	 *
	 * The pre-3B4 page had a create/edit dialog with a free-text "ID Dokter"
	 * field. That is the exact thing the phase forbids: there is no practitioner
	 * catalog in the API to validate the id against, no practitioner name to
	 * resolve it to, and a schedule whose practitioner is a typed-in string is a
	 * record the system cannot reason about. This page therefore displays NO
	 * practitioner field at all — not a name (invented), not an id (a raw UUID),
	 * not a free-text box (fabrication waiting to happen).
	 *
	 * Facility and service resolve through the join helpers, so a reference that
	 * is null, missing, or out of scope renders as "-".
	 */
	let schedules: AdminSchedule[] = [];
	let facilities: AdminFacility[] = [];
	let units: AdminServiceUnit[] = [];
	let loading = true;
	let error: ApiError | null = null;
	let facilitiesLoaded = false;
	let facilityFilter = '';
	let activeOnly = false;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	/**
	 * No `manageDenied` flag, and its absence is deliberate.
	 *
	 * The pre-3B4 page decided whether to offer schedule editing from a
	 * client-side guess. That guess had to be wrong: there is no `schedule.manage`
	 * fact in this client — no permission list, no role, no facility scope, only
	 * `hasSession` — so any check would have been inference from an email address
	 * or a token claim, which is exactly what the Phase 3B0 authorization model
	 * forbids.
	 *
	 * Worse, the obvious implementation was subtly incorrect. A 403 on the READ
	 * list means "you may not read schedules"; it says nothing about whether you
	 * may change them. Treating one as evidence of the other would refuse an
	 * editor to an operator who may legitimately hold `schedule.manage`.
	 *
	 * So this phase ships no editor and asserts no permission it cannot verify.
	 * Phase 3B5 owns the editor, and the only honest way to decide whether to
	 * show it is for the server to say so — not for the client to reason about a
	 * status code.
	 */

	onMount(async () => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		const [scheduleResult, facilityResult, unitResult] = await Promise.all([
			listSchedules(),
			listFacilities(),
			listServiceUnits()
		]);

		if (scheduleResult.ok) {
			schedules = scheduleResult.data;
		} else {
			error = scheduleResult.error;
		}
		if (facilityResult.ok) {
			facilities = facilityResult.data;
			facilitiesLoaded = true;
		}
		if (unitResult.ok) {
			units = unitResult.data;
		}
		loading = false;
	});

	$: facilityIndex = indexBy(facilities, (facility) => facility.id);
	$: unitIndex = indexBy(units, (unit) => unit.id);

	$: filtered = schedules.filter((schedule) => {
		if (facilityFilter && schedule.facility_id !== facilityFilter) return false;
		if (activeOnly && !schedule.is_active) return false;
		return true;
	});

	/**
	 * A schedule's service unit. The join is best-effort by design: the frozen
	 * reference itself renders "-" here and states that the schedule data carries
	 * only the unit code, so a "-" is a truthful rendering rather than a gap.
	 */
	function serviceLabel(schedule: AdminSchedule): string {
		return serviceUnitName(unitIndex, schedule.service_unit_id);
	}
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Jadwal"
		subtitle="Jadwal praktik dalam lingkup akses Anda. Halaman ini hanya menampilkan data."
	/>

	<AdminToolbar
		showRefresh={false}
		note="Pilihan fasilitas memantulkan fasilitas yang ada dalam data yang sudah dimuat. Nama unit layanan ditampilkan hanya bila unitnya dikenal; selain itu tampil sebagai “-”."
	>
		<FacilityFilter
			{facilities}
			selected={facilityFilter}
			visible={filtered.length}
			total={schedules.length}
			onSelect={(value) => (facilityFilter = value)}
		/>

		<div class="sigap-admin-checkfield">
			<input
				type="checkbox"
				id="schedule-active-only"
				bind:checked={activeOnly}
				class="sigap-admin-checkfield__input"
			/>
			<label class="sigap-admin-checkfield__label" for="schedule-active-only">
				Hanya jadwal aktif
			</label>
		</div>
	</AdminToolbar>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={schedules.length}
		scopeEmpty={facilitiesLoaded && isEmptyScope(facilities)}
		emptyTitle="Belum ada jadwal"
		emptyDescription={ORDINARY_EMPTY_DESCRIPTION}
	>
		{#if filtered.length === 0}
			<div class="sigap-admin-nomatch" style:border-radius={RADIUS.panel} role="status">
				<p class="sigap-admin-nomatch__title">Tidak ada jadwal yang cocok</p>
				<p class="sigap-admin-nomatch__body">Ubah atau hapus filter Anda.</p>
				<button
					type="button"
					class="sigap-admin-nomatch__reset"
					style:border-radius={RADIUS.control}
					on:click={() => {
						facilityFilter = '';
						activeOnly = false;
					}}
				>
					Hapus filter
				</button>
			</div>
		{:else}
			<DataTable
				caption="Jadwal praktik dalam lingkup akses"
				columns={[
					{ label: 'Tanggal' },
					{ label: 'Waktu' },
					{ label: 'Fasilitas', secondary: true },
					{ label: 'Layanan', secondary: true },
					{ label: 'Slot' },
					{ label: 'Kapasitas' },
					{ label: 'Status' }
				]}
			>
				{#each filtered as schedule (schedule.id)}
					<tr>
						<td class="sigap-schedule-row__date">{formatDate(schedule.schedule_date)}</td>
						<td class="sigap-schedule-row__time">
							{formatTime(schedule.start_time)}–{formatTime(schedule.end_time)}
						</td>
						<td class="sigap-table-col-secondary">
							{facilityName(facilityIndex, schedule.facility_id)}
						</td>
						<td class="sigap-table-col-secondary">{serviceLabel(schedule)}</td>
						<td class="sigap-schedule-row__num">{schedule.slot_minutes} mnt</td>
						<td class="sigap-schedule-row__num">{schedule.capacity_per_slot}</td>
						<td>
							<StatusBadge
								label={schedule.is_active ? 'Aktif' : 'Nonaktif'}
								tone={schedule.is_active ? 'success' : 'neutral'}
								size="sm"
							/>
						</td>
					</tr>
				{/each}
			</DataTable>
		{/if}
	</AdminReadState>
</div>

<style>
	.sigap-admin-page {
		min-width: 0;
	}

	.sigap-admin-checkfield {
		display: flex;
		align-items: center;
		gap: 6px;
		/* Aligns the checkbox with the 36px control row beside it. */
		height: 36px;
	}

	.sigap-admin-checkfield__input {
		width: 16px;
		height: 16px;
	}

	.sigap-admin-checkfield__label {
		font-size: 13px;
		color: var(--sigap-foreground);
	}

	.sigap-admin-checkfield__input:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-schedule-row__date {
		white-space: nowrap;
	}

	.sigap-schedule-row__time {
		color: var(--sigap-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sigap-schedule-row__num {
		font-variant-numeric: tabular-nums;
	}

	.sigap-admin-nomatch {
		padding: 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px dashed var(--sigap-border);
	}

	.sigap-admin-nomatch__title {
		margin: 0 0 4px;
		font-size: 15px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-admin-nomatch__body {
		margin: 0 0 12px;
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-admin-nomatch__reset {
		height: 36px;
		padding: 0 14px;
		font: inherit;
		font-size: 13px;
		font-weight: 500;
		color: var(--sigap-primary);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		cursor: pointer;
	}

	.sigap-admin-nomatch__reset:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
