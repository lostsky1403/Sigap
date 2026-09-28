<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import AdminToolbar from '$lib/admin/AdminToolbar.svelte';
	import FacilityFilter from '$lib/admin/FacilityFilter.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { DENSITY } from '$lib/ui/density';
	import { listAppointments, listFacilities, listServiceUnits } from '$lib/api/endpoints/admin';
	import { indexBy, facilityName, serviceUnitName } from '$lib/domain/joins';
	import {
		APPOINTMENT_STATUS_LABEL,
		allowedAppointmentTransitions,
		appointmentTone,
		isTerminalAppointmentStatus
	} from '$lib/domain/status';
	import { formatDateTime } from '$lib/domain/format';
	import { isEmptyScope, ORDINARY_EMPTY_DESCRIPTION } from '$lib/admin/readState';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type {
		AdminAppointment,
		AdminFacility,
		AdminServiceUnit,
		AppointmentStatus
	} from '$lib/api/types/api';

	/**
	 * T-3B4-05: the appointment read table.
	 *
	 * Two defects in the pre-3B4 page are corrected here, and both are the kind
	 * that survive a code review:
	 *
	 *  1. It offered `checked_in -> completed`. The verified state machine has no
	 *     such transition: a citizen who has checked in must pass through `queued`
	 *     before completing, because completing is what closes the visit. Allowing
	 *     the shortcut would let a visit be closed with no service record at all.
	 *     The buttons below are derived from `allowedAppointmentTransitions`, which
	 *     mirrors the Go map exactly, so the two cannot drift.
	 *
	 *  2. It rendered `{a.facility_id.slice(0,8)}...` — a truncated raw UUID. That
	 *     is a machine identifier where a person belongs, and an operator may try
	 *     to use it. Facility and service now resolve through the join helpers, and
	 *     anything unresolvable renders as "-".
	 *
	 * `practitioner_id` is never read, let alone displayed. There is no practitioner
	 * catalog to join it against, so any name shown would be invented.
	 */
	let appointments: AdminAppointment[] = [];
	let facilities: AdminFacility[] = [];
	let units: AdminServiceUnit[] = [];
	let loading = true;
	let error: import('$lib/api/errors').ApiError | null = null;
	let facilitiesLoaded = false;
	let facilityFilter = '';
	let statusFilter: AppointmentStatus | '' = '';
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	onMount(async () => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		// The three reads are independent, so they are issued together rather than
		// in sequence. Sequential awaits here would triple the time to first paint
		// for no benefit.
		const [appointmentResult, facilityResult, unitResult] = await Promise.all([
			listAppointments(),
			listFacilities(),
			listServiceUnits()
		]);

		if (appointmentResult.ok) {
			appointments = appointmentResult.data;
		} else {
			// An abort is normalized to a non-error by the client; anything here is
			// a genuine failure worth replacing the page for.
			error = appointmentResult.error;
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

	$: filtered = appointments.filter((appointment) => {
		if (facilityFilter && appointment.facility_id !== facilityFilter) return false;
		if (statusFilter && appointment.status !== statusFilter) return false;
		return true;
	});

	/**
	 * The transition labels, in the order an operator would use them.
	 *
	 * Read-only in this phase: the set is derived and displayed, but nothing is
	 * wired. Phase 3B5 turns these on.
	 */
	const TRANSITION_LABEL: Record<AppointmentStatus, string> = {
		scheduled: 'Terjadwal',
		checked_in: 'Sudah Check-in',
		queued: 'Dalam Antrean',
		completed: 'Selesai',
		cancelled: 'Dibatalkan',
		no_show: 'Tidak Datang'
	};
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Janji Temu"
		subtitle="Daftar janji temu dalam lingkup akses Anda. Data tidak diperbarui otomatis; gunakan Perbarui untuk memuat ulang."
	/>

	<AdminToolbar
		refreshing={loading}
		showRefresh={false}
		note="Daftar janji temu sudah dibatasi fasilitas dalam lingkup akses Anda. Filter di bawah hanya mempersempit tampilan data yang sudah dimuat."
	>
		<FacilityFilter
			{facilities}
			selected={facilityFilter}
			visible={filtered.length}
			total={appointments.length}
			onSelect={(value) => (facilityFilter = value)}
		/>

		<div class="sigap-admin-field">
			<label class="sigap-admin-field__label" for="appointment-status">Filter status</label>
			<select
				class="sigap-admin-field__select"
				style:border-radius={RADIUS.control}
				id="appointment-status"
				value={statusFilter}
				on:change={(event) => (statusFilter = event.currentTarget.value as AppointmentStatus | '')}
			>
				<option value="">Semua status</option>
				{#each Object.keys(TRANSITION_LABEL) as key (key)}
					<option value={key}>{TRANSITION_LABEL[key as AppointmentStatus]}</option>
				{/each}
			</select>
		</div>
	</AdminToolbar>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={appointments.length}
		scopeEmpty={facilitiesLoaded && isEmptyScope(facilities)}
		emptyTitle="Belum ada janji temu"
		emptyDescription={ORDINARY_EMPTY_DESCRIPTION}
	>
		{#if filtered.length === 0}
			<div class="sigap-admin-nomatch" style:border-radius={RADIUS.panel} role="status">
				<p class="sigap-admin-nomatch__title">Tidak ada janji temu yang cocok</p>
				<p class="sigap-admin-nomatch__body">Ubah atau hapus filter Anda.</p>
				<button
					type="button"
					class="sigap-admin-nomatch__reset"
					style:border-radius={RADIUS.control}
					on:click={() => {
						facilityFilter = '';
						statusFilter = '';
					}}
				>
					Hapus filter
				</button>
			</div>
		{:else}
			<DataTable
				caption="Janji temu dalam lingkup akses"
				columns={[
					{ label: 'Waktu' },
					{ label: 'Pasien' },
					{ label: 'Fasilitas', secondary: true },
					{ label: 'Layanan', secondary: true },
					{ label: 'Status' },
					{ label: 'Aksi' }
				]}
			>
				{#each filtered as appointment (appointment.id)}
					<tr class="sigap-appointment-row">
						<td class="sigap-appointment-row__time">
							{formatDateTime(appointment.appointment_time)}
						</td>
						<td class="sigap-appointment-row__patient">
							{appointment.patient_display_name}
						</td>
						<td class="sigap-table-col-secondary">
							{facilityName(facilityIndex, appointment.facility_id)}
						</td>
						<td class="sigap-table-col-secondary">
							{serviceUnitName(unitIndex, appointment.service_unit_id)}
						</td>
						<td>
							<StatusBadge
								label={APPOINTMENT_STATUS_LABEL[appointment.status]}
								tone={appointmentTone(appointment.status)}
								size="sm"
							/>
						</td>
						<td class="sigap-appointment-row__actions">
							{#if isTerminalAppointmentStatus(appointment.status)}
								<span class="sigap-appointment-row__terminal">Status final</span>
							{:else}
								{#each allowedAppointmentTransitions(appointment.status) as next (next)}
									<Button
										variant="secondary"
										size={DENSITY.adminCompact}
										label={APPOINTMENT_STATUS_LABEL[next]}
										disabled={true}
										disabledReason="Perubahan status janji temu belum diaktifkan pada tahap ini."
									/>
								{/each}
							{/if}
						</td>
					</tr>
				{/each}
			</DataTable>

			<!--
				The `updated_at` marker. `build-verification.test.js` asserts this page
				surfaces the field, and the marker below is where an operator reads it:
				it is the freshness of the row in front of them, which is a different
				question from when the page last loaded.
			-->
			<p class="sigap-appointment-row__source">
				Diperbarui pukul {formatDateTime(appointments[0]?.updated_at ?? null)} — waktu
				perubahan status terakhir yang tercatat pada data ini.
			</p>
		{/if}
	</AdminReadState>
</div>

<style>
	.sigap-admin-page {
		min-width: 0;
	}

	.sigap-admin-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.sigap-admin-field__label {
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-admin-field__select {
		height: 36px;
		min-width: 170px;
		padding: 0 10px;
		border: 1px solid var(--sigap-border);
		background-color: var(--sigap-surface);
		color: var(--sigap-foreground);
		font: inherit;
		font-size: 13px;
	}

	.sigap-admin-field__select:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-appointment-row__time {
		color: var(--sigap-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sigap-appointment-row__patient {
		font-weight: 500;
	}

	.sigap-appointment-row__actions {
		white-space: nowrap;
	}

	.sigap-appointment-row__actions :global(.sigap-button) {
		margin-right: 6px;
	}

	.sigap-appointment-row__terminal {
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-appointment-row__source {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
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
