<script lang="ts">
	import Button from '$lib/ui/Button.svelte';
	import AdminStatusBadge, { APPOINTMENT_STATUS_ICON } from './AdminStatusBadge.svelte';
	import { DENSITY } from '$lib/ui/density';
	import { formatDateTime } from '$lib/domain/format';
	import {
		APPOINTMENT_STATUS_LABEL,
		APPOINTMENT_TRANSITION_LABEL,
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
	<td class="sigap-appointment-row__time" data-label="Waktu">
		{formatDateTime(appointment.appointment_time)}
	</td>
	<td class="sigap-appointment-row__patient" data-label="Pasien">
		{appointment.patient_display_name}
	</td>
	<td class="sigap-table-col-secondary">{facilityName}</td>
	<td class="sigap-table-col-secondary">{serviceUnitName}</td>
	<td class="sigap-appointment-row__status" data-label="Status">
		<AdminStatusBadge
			label={APPOINTMENT_STATUS_LABEL[appointment.status]}
			tone={appointmentTone(appointment.status)}
			size="sm"
			icon={APPOINTMENT_STATUS_ICON[appointment.status]}
		/>
	</td>
	<td class="sigap-appointment-row__actions sigap-table-col-pinned">
		{#if terminal}
			<span class="sigap-appointment-row__terminal">Tidak ada aksi lanjutan</span>
		{:else}
			{#each transitions as next (next)}
				{#if next === 'cancelled'}
					<!-- Routed through the confirm dialog rather than straight to the
					     mutation. See the note above: cancellation is the only terminal
					     move, so it is the only one worth interrupting for. -->
					<Button
						variant="danger"
						size={DENSITY.adminCompact}
						label={APPOINTMENT_TRANSITION_LABEL[next]}
						disabled={busy}
						onClick={() => onRequestCancel(appointment)}
					/>
				{:else}
					<Button
						variant={next === 'completed' ? 'primary' : 'secondary'}
						size={DENSITY.adminCompact}
						label={APPOINTMENT_TRANSITION_LABEL[next]}
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
		margin-right: var(--sigap-space-2);
	}

	.sigap-appointment-row__terminal {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	/*
		Decorative leading accent on a message row, not a control boundary, so it
		uses the subtle border token.
	*/
	.sigap-appointment-row__outcome-cell {
		padding: var(--sigap-space-2) var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		line-height: 1.45;
		background-color: var(--sigap-surface);
		border-left: 3px solid var(--sigap-border-subtle);
	}

	/*
		MOBILE (≤767px): the row reflows into a stacked card.

		The desktop presentation pins the Aksi column to the right edge (via
		DataTable's `pin`, which adds `sigap-table-col-pinned`) and lets the rest
		of the table scroll under it, which works down to the 1280px secondary
		breakpoint. Below that the visible columns are Waktu, Pasien, Status and
		the three inline buttons — and at 390/360 the three buttons alone are
		measured ~429px wide, more than the ~334px content area a phone leaves
		after the 56px rail. The pinned cell cannot compress three `nowrap`
		buttons, so they render past the right edge while remaining
		keyboard-focusable: a WCAG 2.4.11 violation.

		The card is option (1) of the directive: every authorized action stays
		VISIBLE and tappable on its own full-width row, with no horizontal
		scroll. The `<tr>`/`<td>` structure is preserved in the DOM — the desktop
		table and every `tr`-scoped e2e locator keep matching — only the paint
		changes, so the tests that count/locate the action buttons are untouched.
	*/
	@media (max-width: 767px) {
		/*
			The block reflow needs the table and its body to stop imposing
			table-row-group geometry on block children, and the header band
			hidden: each card carries its own field labels below.
		*/
		:global(.sigap-data-table) {
			display: block;
		}

		:global(.sigap-data-table tbody) {
			display: block;
		}

		:global(.sigap-data-table thead) {
			display: none;
		}

		.sigap-appointment-row {
			display: block;
			padding: var(--sigap-space-3);
			background-color: var(--sigap-surface);
			border-bottom: 1px solid var(--sigap-border-subtle);
		}

		.sigap-appointment-row__outcome {
			display: block;
		}

		/*
			Label lines. Each non-secondary column becomes a labelled, full-width
			line. (Fasilitas/Layanan are already hidden below 1280px.)
		*/
		.sigap-appointment-row__time,
		.sigap-appointment-row__patient,
		.sigap-appointment-row__status {
			display: block;
			width: auto;
			height: auto;
			padding: var(--sigap-space-1) 0;
			border-bottom: 0;
		}

		/*
			The frozen 11px uppercase field-label treatment, matching the admin
			header scale. `attr(data-label)` reads the label from the cell so the
			column heading and the card field stay in one place.
		*/
		.sigap-appointment-row__time::before,
		.sigap-appointment-row__patient::before,
		.sigap-appointment-row__status::before {
			content: attr(data-label);
			display: block;
			margin-bottom: var(--sigap-space-1);
			font-size: var(--sigap-type-micro);
			font-weight: 600;
			letter-spacing: 0.06em;
			text-transform: uppercase;
			color: var(--sigap-muted);
		}

		/*
			Actions lose the right-edge pin and stack full-width. The compound
			selector names the pinned class too, so this rule outranks
			DataTable's `td.sigap-table-col-pinned` sticky rule — which would
			otherwise keep the buttons glued to the right edge and off-screen.
		*/
		.sigap-appointment-row__actions.sigap-table-col-pinned {
			position: static;
			width: auto;
			/*
				`height: auto` is the actual fix for the crushed buttons. A table
				cell keeps the admin row's 40px band even after `display: flex`,
				so the flex-column has a definite 40px height; the three buttons
				— each defaulting to `flex: 0 1 auto` — then shrink to their
				`min-height: auto` floor (14px line-height + 2px border = 16px).
				Letting the cell size to its content removes the constraint the
				buttons were shrinking against.
			*/
			height: auto;
			margin-top: var(--sigap-space-2);
			padding: 0;
			white-space: normal;
			border-left: 0;
			background-color: transparent;
			display: flex;
			flex-direction: column;
			align-items: stretch;
			gap: var(--sigap-space-2);
		}

		.sigap-appointment-row__actions.sigap-table-col-pinned :global(.sigap-button) {
			width: 100%;
			margin-right: 0;
			/*
				Belt-and-suspenders floor against the collapse above: never shrink
				along the column's main axis, and never fall below the button's
				own admin-compact density height even if line-height were the only
				thing sizing it.
			*/
			flex: none;
			min-height: var(--sigap-control-admin-compact);
		}

		/*
			The outcome line keeps its leading accent but drops the table-row
			geometry so it sits flush under its card.
		*/
		.sigap-appointment-row__outcome-cell {
			display: block;
			width: auto;
			height: auto;
		}
	}
</style>
