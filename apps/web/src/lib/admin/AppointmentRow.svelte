<script lang="ts">
	import Button from '$lib/ui/Button.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import { DENSITY } from '$lib/ui/density';
	import { formatDateTime } from '$lib/domain/format';
	import {
		APPOINTMENT_STATUS_LABEL,
		allowedAppointmentTransitions,
		appointmentTone,
		isTerminalAppointmentStatus
	} from '$lib/domain/status';
	import type { AdminAppointment, AppointmentStatus } from '$lib/api/types/api';

	/**
	 * One appointment row, with its live transitions.
	 *
	 * The offered set comes from `allowedAppointmentTransitions`, which mirrors
	 * the Go map exactly. That is the single most important thing this component
	 * does, because the map contains one absence that is easy to reintroduce by
	 * accident: there is NO `checked_in -> completed`.
	 *
	 * Why it is absent is worth stating, because "add the shortcut" looks like a
	 * usability improvement. Completing an appointment is what closes the visit;
	 * a citizen who has checked in but is not yet in the queue has not been
	 * served. Offering the shortcut would let a visit be closed with no service
	 * record at all, and the server would reject it with a 400 anyway — so the
	 * button would be a control that exists only to fail.
	 *
	 * CANCELLATION IS THE ONLY CONFIRMED ACTION. The other four transitions move
	 * a visit forward or close it as a no-show, and each is reversible in the
	 * sense that an operator can act again from the resulting state. Cancelling
	 * is the one an operator cannot undo and cannot recover from by clicking
	 * something else, because `cancelled` is terminal. A dialog is warranted
	 * exactly there and nowhere else — confirming all five would train operators
	 * to dismiss dialogs without reading them, which defeats the one dialog that
	 * matters.
	 */
	export let appointment: AdminAppointment;
	export let facilityName: string = '-';
	export let serviceUnitName: string = '-';
	export let pendingStatus: AppointmentStatus | '' = '';
	export let errorMessage: string = '';
	export let onTransition: (id: string, status: AppointmentStatus) => void = () => {};
	export let onRequestCancel: (appointment: AdminAppointment) => void = () => {};

	$: transitions = allowedAppointmentTransitions(appointment.status);
	$: terminal = isTerminalAppointmentStatus(appointment.status);
	$: busy = pendingStatus !== '';

	/**
	 * The pending label, resolved once.
	 *
	 * Written as a derived string rather than an inline
	 * `APPOINTMENT_STATUS_LABEL[pendingStatus]` because `busy` is a separate
	 * boolean: TypeScript cannot narrow `pendingStatus` to a real
	 * `AppointmentStatus` from it, so the inline index would be typed against
	 * `'' | AppointmentStatus` and rejected. Resolving it here keeps the `''`
	 * case out of the record lookup entirely.
	 */
	$: pendingLabel = pendingStatus === '' ? '' : APPOINTMENT_STATUS_LABEL[pendingStatus];
</script>

<tr class="sigap-appointment-row">
	<td class="sigap-appointment-row__time">
		{formatDateTime(appointment.appointment_time)}
	</td>
	<td class="sigap-appointment-row__patient">
		{appointment.patient_display_name}
	</td>
	<td class="sigap-table-col-secondary">{facilityName}</td>
	<td class="sigap-table-col-secondary">{serviceUnitName}</td>
	<td>
		<StatusBadge
			label={APPOINTMENT_STATUS_LABEL[appointment.status]}
			tone={appointmentTone(appointment.status)}
			size="sm"
		/>
	</td>
	<td class="sigap-appointment-row__actions">
		{#if terminal}
			<span class="sigap-appointment-row__terminal">Status final</span>
		{:else}
			{#each transitions as next (next)}
				{#if next === 'cancelled'}
					<!-- Routed through the confirm dialog rather than straight to the
					     mutation. See the note above: cancellation is the only terminal
					     move, so it is the only one worth interrupting for. -->
					<Button
						variant="danger"
						size={DENSITY.adminCompact}
						label={APPOINTMENT_STATUS_LABEL[next]}
						disabled={busy}
						onClick={() => onRequestCancel(appointment)}
					/>
				{:else}
					<Button
						variant={next === 'completed' ? 'primary' : 'secondary'}
						size={DENSITY.adminCompact}
						label={APPOINTMENT_STATUS_LABEL[next]}
						disabled={busy}
						onClick={() => onTransition(appointment.id, next)}
					/>
				{/if}
			{/each}
		{/if}
	</td>
</tr>

{#if busy || errorMessage !== ''}
	<tr class="sigap-appointment-row__outcome">
		<td colspan="6" class="sigap-appointment-row__outcome-cell">
			{#if busy}
				<span role="status">Menyimpan perubahan ke {pendingLabel}...</span>
			{:else}
				<span role="alert">{errorMessage}</span>
			{/if}
		</td>
	</tr>
{/if}

<style>
	.sigap-appointment-row__time {
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

	.sigap-appointment-row__outcome-cell {
		padding: 8px 12px;
		font-size: 13px;
		line-height: 1.45;
		background-color: var(--sigap-surface);
		border-left: 3px solid var(--sigap-border);
	}
</style>
