<script lang="ts">
	import { DENSITY } from '$lib/ui/density';
	import Button from '$lib/ui/Button.svelte';
	import { QUEUE_STATUS_LABEL, QUEUE_TRANSITION_LABEL, queueTone } from '$lib/domain/status';
	import AdminStatusBadge, { QUEUE_STATUS_ICON } from './AdminStatusBadge.svelte';
	import { formatDateTime } from '$lib/domain/format';
	import type { QueueStatus } from '$lib/api/types/api';

	/**
	 * One row of the queue board.
	 *
	 * The transition buttons are derived from the verified state machine in
	 * `domain/status.ts`, which mirrors the Go map exactly. That derivation is
	 * what keeps the board honest: a button is shown only where the server would
	 * accept it, so an operator is never offered an action that fails.
	 *
	 * T-3B5-01 wires the execution. The visible set is unchanged from Phase 3B4 —
	 * what changed is that the buttons now do something, and a rejection from
	 * the server appears on this row with the backend's own wording.
	 */
	export let ticketId: string = '';
	export let formattedNumber: string = '';
	export let status: QueueStatus = 'waiting';
	export let facilityName: string = '-';
	export let registeredAt: string = '';
	export let calledAt: string | undefined = undefined;
	export let completedAt: string | undefined = undefined;
	/** Labels for the transitions this status allows. Empty for terminal states. */
	export let transitions: readonly QueueStatus[] = [];
	export let onTransition: (ticketId: string, status: QueueStatus) => void = () => {};
	/**
	 * The transition currently in flight for THIS row, or ''.
	 *
	 * Per-row rather than per-board on purpose. An operator advancing three
	 * tickets in a row must be able to keep working: disabling the whole board
	 * during one request would make the queue slower to work through, which is
	 * the opposite of what the control is for.
	 */
	export let pendingStatus: QueueStatus | '' = '';
	/**
	 * The backend's message for a rejected transition on THIS row, or ''.
	 *
	 * Rendered verbatim. The server names both statuses in the sentence
	 * ("Transisi status tidak valid: waiting → in_service."), which is more
	 * useful than anything a client could substitute, and rewriting it would
	 * hide which rule was actually broken.
	 */
	export let errorMessage: string = '';
</script>

<!--
	`data-ticket-id` is the stable key for this row.

	A formatted number is NOT unique across time: the daily counter resets, so
	"DEMO-0001" exists once per facility per day. Anything that needs to point at
	one specific ticket — a test, a future deep link from a notification — has to
	use the id, not the number the citizen reads.
-->
<tr class="sigap-queue-row" data-ticket-id={ticketId}>
	<td class="sigap-queue-row__number">{formattedNumber}</td>
	<td class="sigap-queue-row__status">
		<AdminStatusBadge
			label={QUEUE_STATUS_LABEL[status]}
			tone={queueTone(status)}
			size="sm"
			icon={QUEUE_STATUS_ICON[status]}
		/>
	</td>
	<td class="sigap-queue-row__facility">{facilityName}</td>
	<td class="sigap-queue-row__time sigap-table-col-secondary">{formatDateTime(registeredAt)}</td>
	<td class="sigap-queue-row__time sigap-table-col-secondary">{formatDateTime(calledAt)}</td>
	<td class="sigap-queue-row__time sigap-table-col-secondary">{formatDateTime(completedAt)}</td>
	<td class="sigap-queue-row__actions">
		{#if transitions.length === 0}
			<!--
				A terminal state says so in words. An empty cell would read as
				"still loading", which is a different and wrong claim.
			-->
			<span class="sigap-queue-row__terminal">Tidak ada aksi lanjutan</span>
		{:else}
			{#each transitions as next (next)}
				<Button
					variant={next === 'completed' ? 'primary' : 'secondary'}
					size={DENSITY.adminCompact}
					label={QUEUE_TRANSITION_LABEL[next]}
					disabled={pendingStatus !== ''}
					onClick={() => onTransition(ticketId, next)}
				/>
			{/each}
		{/if}
	</td>
</tr>

{#if pendingStatus !== '' || errorMessage !== ''}
	<!--
		A second row carrying the outcome of the mutation attempted on the row
		above. It is a separate <tr> rather than a cell inside the original row
		because the message can be long, and letting it reflow inside a
		40px-height cell would break the row-density contract the whole board is
		built on.

		`role="alert"` because a rejected mutation is a failure the operator
		did not expect and needs to hear about; the buttons themselves were
		polite.
	-->
	<tr class="sigap-queue-row__outcome">
		<td colspan="7" class="sigap-queue-row__outcome-cell">
			{#if pendingStatus !== ''}
				<span class="sigap-queue-row__pending" role="status">
					Menyimpan perubahan ke {QUEUE_STATUS_LABEL[pendingStatus]}…
				</span>
			{:else if errorMessage !== ''}
				<span class="sigap-queue-row__error" role="alert">{errorMessage}</span>
			{/if}
		</td>
	</tr>
{/if}

<style>
	/*
		`tr` carries no styles of its own. Row presentation belongs to the
		`td` rules in DataTable, which own the 40px row height and the hover
		state; duplicating them here would give the two densities a second place
		to drift apart.
	*/
	.sigap-queue-row__number {
		font-weight: 600;
		/* Tabular figures so the numbers align and a column of them is scannable. */
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.02em;
	}

	.sigap-queue-row__time {
		color: var(--sigap-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	/*
		These two columns are secondary: a second and third timestamp. DataTable's
		own global breakpoint rule hides BOTH the header and the cell, so the
		columns disappear together and every remaining value stays under its own
		label. A media query here would hide only the cell and shift the whole row.
	*/

	.sigap-queue-row__actions {
		white-space: nowrap;
	}

	.sigap-queue-row__terminal {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	/*
		The outcome row. It carries no density constraint of its own — the 40px
		contract belongs to a data row, and this is a message, not data. It is
		bordered and coloured on the leading edge so it reads as belonging to the
		row above rather than as a new band on the board.

		The leading edge is DECORATIVE: it is an accent on a message, not the
		outline of a control, so it uses `--sigap-border-subtle`. A 4.16:1 bar
		down the side of every outcome row would out-shout the danger colour that
		appears there when the mutation is actually refused.
	*/
	.sigap-queue-row__outcome-cell {
		padding: var(--sigap-space-2) var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		line-height: 1.45;
		background-color: var(--sigap-surface);
		border-left: 3px solid var(--sigap-border-subtle);
	}

	.sigap-queue-row__pending {
		color: var(--sigap-muted);
	}

	.sigap-queue-row__error {
		color: var(--sigap-danger);
	}

	/*
		A visually hidden caption for the actions cell would be redundant with the
		header; instead the header column is labelled "Aksi" in the page, so the
		buttons need no per-row naming.
	*/
	.sigap-queue-row__actions :global(.sigap-button) {
		margin-right: var(--sigap-space-2);
	}
</style>
