<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { DENSITY } from '$lib/ui/density';
	import Button from '$lib/ui/Button.svelte';
	import { QUEUE_STATUS_LABEL, queueTone } from '$lib/domain/status';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import { formatDateTime } from '$lib/domain/format';
	import type { QueueStatus } from '$lib/api/types/api';

	/**
	 * One row of the queue board.
	 *
	 * The transition buttons are DISABLED and clearly marked as such, and they
	 * reflect only the verified state machine in `domain/status.ts`, which
	 * mirrors the Go map exactly. Two reasons they are present at all in a
	 * read-only phase:
	 *
	 *  1. An operator needs to see what the system considers possible next. A row
	 *     with no affordances tells them nothing about why they cannot complete a
	 *     ticket that is already in service.
	 *  2. Offering an action the server would reject with 409 is worse than
	 *     offering none, so the visible set is derived from the same table the
	 *     server enforces rather than from a page's own guess.
	 *
	 * Phase 3B5 wires the execution. Until then each button carries a disabled
	 * reason, so an operator is told the transition exists AND that it is not
	 * available here — rather than being shown a control that silently does
	 * nothing.
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
	export let actionsEnabled: boolean = false;
	export let onTransition: (ticketId: string, status: QueueStatus) => void = () => {};
</script>

<tr class="sigap-queue-row">
	<td class="sigap-queue-row__number">{formattedNumber}</td>
	<td class="sigap-queue-row__status">
		<StatusBadge label={QUEUE_STATUS_LABEL[status]} tone={queueTone(status)} size="sm" />
	</td>
	<td class="sigap-queue-row__facility">{facilityName}</td>
	<td class="sigap-queue-row__time">{formatDateTime(registeredAt)}</td>
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
					label={QUEUE_STATUS_LABEL[next]}
					disabled={!actionsEnabled}
					disabledReason={actionsEnabled
						? undefined
						: `Perubahan status ke ${QUEUE_STATUS_LABEL[next]} belum diaktifkan pada tahap ini.`}
					onClick={() => onTransition(ticketId, next)}
				/>
			{/each}
		{/if}
	</td>
</tr>

<style>
	/*
		`tr` carries no styles of its own. Row presentation belongs to the
		`td` rules in DataTable, which own the 44px row height and the hover
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
		font-size: 12px;
		color: var(--sigap-muted);
	}

	/*
		A visually hidden caption for the actions cell would be redundant with the
		header; instead the header column is labelled "Aksi" in the page, so the
		buttons need no per-row naming.
	*/
	.sigap-queue-row__actions :global(.sigap-button) {
		margin-right: 6px;
	}
</style>
