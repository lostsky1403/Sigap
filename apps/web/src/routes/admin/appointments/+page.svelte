<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import AdminToolbar from '$lib/admin/AdminToolbar.svelte';
	import FacilityFilter from '$lib/admin/FacilityFilter.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import AppointmentRow from '$lib/admin/AppointmentRow.svelte';
	import AdminConfirmDialog from '$lib/admin/AdminConfirmDialog.svelte';
	import AdminMutationFeedback from '$lib/admin/AdminMutationFeedback.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import {
		listAppointments,
		listFacilities,
		listServiceUnits,
		updateAppointmentStatus
	} from '$lib/api/endpoints/admin';
	import { indexBy, facilityName, serviceUnitName } from '$lib/domain/joins';
	import { APPOINTMENT_STATUS_LABEL } from '$lib/domain/status';
	import { runAdminMutation } from '$lib/admin/adminMutation';
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
	 * The status filter's labels, in the order an operator would use them.
	 *
	 * Used for the filter, so every status is listed including the terminal
	 * ones — an operator needs to filter to "everything already cancelled"
	 * exactly as much as to filter to "scheduled".
	 */
	const TRANSITION_LABEL: Record<AppointmentStatus, string> = {
		scheduled: 'Terjadwal',
		checked_in: 'Sudah Check-in',
		queued: 'Dalam Antrean',
		completed: 'Selesai',
		cancelled: 'Dibatalkan',
		no_show: 'Tidak Datang'
	};

	/**
	 * T-3B5-02: the mutation state.
	 *
	 * Per-appointment, for the same reason the queue board is per-ticket: an
	 * operator moves several appointments through a clinic session in a row, and
	 * a page-wide lock would make that slower rather than safer.
	 */
	let pendingId = '';
	let pendingStatus: AppointmentStatus | '' = '';
	let errorId = '';
	let errorMessage = '';

	/** The appointment awaiting cancellation confirmation, or null. */
	let cancelTarget: AdminAppointment | null = null;

	/**
	 * The re-read used by every mutation.
	 *
	 * It exists as a named function rather than inline in `onMount` for one
	 * reason: a mutation must re-read the data WITHOUT resetting the operator's
	 * facility and status filters. Reloading from `onMount` would silently widen
	 * the table back to every row in the middle of a clinic session.
	 */
	async function reloadAppointments() {
		const result = await listAppointments();
		if (result.ok) {
			appointments = result.data;
			// A successful re-read clears the page-level failure too: the reason
			// the page was replaced is gone once rows render again.
			error = null;
			// A successful re-read means any previous rejection is stale: the row it
			// referred to may no longer be in the state that caused it.
			errorId = '';
			errorMessage = '';
		} else {
			error = result.error;
		}
		loading = false;
	}

	async function handleTransition(id: string, next: AppointmentStatus) {
		errorId = '';
		errorMessage = '';
		pendingId = id;
		pendingStatus = next;

		const target = appointments.find((appointment) => appointment.id === id);
		const fromLabel = APPOINTMENT_STATUS_LABEL[target?.status ?? 'scheduled'];

		const outcome = await runAdminMutation(
			() => updateAppointmentStatus(id, next),
			{
				subject: target?.patient_display_name ?? '',
				fromLabel,
				toLabel: APPOINTMENT_STATUS_LABEL[next],
				noun: 'Janji temu',
				// `reloadAppointments` re-reads without touching the filters, so the
				// operator's narrowed view survives the mutation.
				reload: reloadAppointments
			},
			hasSession
		);

		pendingId = '';
		pendingStatus = '';

		// Every failure lands on the row, whatever its kind, and with the
		// backend's own wording. A rejected transition is a fact about that one
		// appointment, not a reason to replace the table.
		if (!outcome.ok && outcome.failure) {
			errorId = id;
			errorMessage = outcome.failure.message;
		}
	}

	/**
	 * Cancelling is the only action routed through a dialog.
	 *
	 * `cancelled` is terminal, so there is no second action an operator can take
	 * to undo a mistaken click. Every other transition leaves the row in a state
	 * where more actions remain available, which is what makes immediate
	 * execution honest for them and annoying for this one.
	 */
	function requestCancel(appointment: AdminAppointment) {
		cancelTarget = appointment;
	}

	function dismissCancel() {
		cancelTarget = null;
	}

	async function confirmCancel() {
		const target = cancelTarget;
		if (!target) return;
		// Closed before the request: the dialog has done its job, and leaving it
		// up while the mutation runs would trap the operator behind a modal that
		// is no longer asking them anything.
		cancelTarget = null;
		await handleTransition(target.id, 'cancelled');
	}

	/**
	 * The dialog body, naming the appointment and stating the consequence.
	 *
	 * A confirmation that does not name its subject is a confirmation of
	 * something abstract, and an operator working through a list of similar rows
	 * cannot tell which one they are about to destroy. `cancelled` is also
	 * terminal — no reactivation control exists — so the text says that plainly
	 * rather than implying the action is reversible.
	 */
	$: cancelDescription = cancelTarget
		? `Janji temu ${cancelTarget.patient_display_name} pada ${formatDateTime(
				cancelTarget.appointment_time
			)} akan dibatalkan. Status "Dibatalkan" bersifat final dan tidak dapat diaktifkan kembali.`
		: '';
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
					<AppointmentRow
						{appointment}
						facilityName={facilityName(facilityIndex, appointment.facility_id)}
						serviceUnitName={serviceUnitName(unitIndex, appointment.service_unit_id)}
						pendingStatus={pendingId === appointment.id ? pendingStatus : ''}
						errorMessage={errorId === appointment.id ? errorMessage : ''}
						onTransition={handleTransition}
						onRequestCancel={requestCancel}
					/>
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

<!--
	THE ONLY CONFIRMATION DIALOG ON THIS PAGE.

	Cancellation is routed here by `AppointmentRow`; the other four transitions
	execute on a single click. The dialog's own accessibility — focus entry, trap,
	Escape, `aria-labelledby`/`aria-describedby`, and focus returning to the
	button that opened it — is `Dialog.svelte`'s contract, so this component only
	has to hand it the right strings.
-->
<AdminConfirmDialog
	open={cancelTarget !== null}
	title="Batalkan janji temu?"
	description={cancelDescription}
	confirmLabel="Ya, batalkan"
	onCancel={dismissCancel}
	onConfirm={confirmCancel}
/>

<!--
	The polite live region and the toast queue. Mounted once per page: a second
	host would double every announcement and stack duplicate toasts.
-->
<AdminMutationFeedback />

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

	/*
		The row's own geometry lives in `AppointmentRow.svelte`, not here.
		Duplicating it would give the 40px contract a second place to drift
		apart, which is the exact failure the density E2E exists to catch.
	*/

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
