<script lang="ts">
	import { afterUpdate } from 'svelte';
	import QueueBand from './QueueBand.svelte';
	import { indexBy } from '$lib/domain/joins';
	import { latestQueueChange, partitionQueueDay } from './queueSource';
	import { formatDateTime } from '$lib/domain/format';
	import type { AdminFacility, AdminQueueTicket, QueueStatus } from '$lib/api/types/api';

	/**
	 * The queue read board.
	 *
	 * BANDS, IN THE ORDER A CLERK WORKS THEM. Menunggu is first because the
	 * recurring task on this screen is "call the next waiting patient": with
	 * sixty waiting rows, putting that band second meant no waiting patient was
	 * on screen at 1440x900 at all — the operator had to scroll past fifteen
	 * tickets nobody was working. Dipanggil follows (the ones already in hand),
	 * Selesai last.
	 *
	 * DAY SCOPE. Dipanggil and Selesai are scoped to the LOCAL calendar day of
	 * their own stamp (`called_at` for the first, `completed_at` for the second,
	 * with the `skipped` exception documented in `queueSource.queueBandStamp`).
	 * Rows provably from an earlier day are folded into a closed `<details>`
	 * group under their band, so the live band means what its heading says while
	 * the archive stays one click away. Menunggu is deliberately NOT scoped: a
	 * patient registered yesterday is still waiting today.
	 *
	 * A row with no readable stamp is UNKNOWN, not old, and stays in the live
	 * band. The heading of the third band says so when that happens, because
	 * "Selesai hari ini" over a row with no completion time would be a claim the
	 * data cannot support.
	 *
	 * The board is READ-ONLY in this phase. The transition buttons are rendered
	 * from the same verified state machine the server enforces, so an operator
	 * can see what is possible next without being offered an action that would
	 * 409.
	 */
	export let tickets: readonly AdminQueueTicket[] = [];
	export let facilities: readonly AdminFacility[] = [];
	export let onTransition: (ticketId: string, status: QueueStatus) => void = () => {};
	/** Ticket id whose transition is in flight, or ''. */
	export let pendingTicketId: string = '';
	/** The status that transition is moving to, for the in-flight row's label. */
	export let pendingStatus: QueueStatus | '' = '';
	/** Ticket id whose transition was rejected, or ''. */
	export let errorTicketId: string = '';
	/** The backend's verbatim message for the rejected transition. */
	export let errorMessage: string = '';

	$: facilityIndex = indexBy(facilities, (facility) => facility.id);

	/**
	 * Membership by status alone — the server's answer, not a re-derivation.
	 * Only the DAY SCOPE below is computed here, and only for the two bands that
	 * claim a day.
	 */
	$: waiting = tickets.filter((t) => t.status === 'waiting');
	$: called = tickets.filter((t) => t.status === 'called' || t.status === 'in_service');
	$: finished = tickets.filter(
		(t) => t.status === 'completed' || t.status === 'cancelled' || t.status === 'skipped'
	);

	/*
		`partitionQueueDay` defaults to `new Date()` on every evaluation rather
		than a clock captured once at mount. The board polls, so each poll
		re-evaluates these statements and the day boundary moves with the wall
		clock; a `now` captured at init would keep filing a 00.05 visit under
		yesterday until the page was reloaded.
	*/
	$: calledScope = partitionQueueDay(called, 'called');
	$: finishedScope = partitionQueueDay(finished, 'finished');

	/**
	 * The third band's heading has to be true of its rows.
	 *
	 * With every row stamped today, "Selesai hari ini" is exact. When some rows
	 * carry no readable stamp they stay in the band (a missing time is not
	 * evidence of an old row), so the heading names them instead of claiming a
	 * scope it cannot prove.
	 */
	$: finishedLabel =
		finishedScope.unknownCount === 0
			? 'Selesai hari ini'
			: finishedScope.todayCount === 0
				? 'Selesai — waktu tidak diketahui'
				: 'Selesai hari ini dan waktu tidak diketahui';

	$: finishedHint =
		finishedScope.unknownCount === 0
			? 'Sudah diselesaikan atau ditutup'
			: 'Sudah diselesaikan atau ditutup; sebagian baris tidak membawa cap waktu yang bisa dibaca';

	/**
	 * The bands in work order. Kept as one derived array so the order is stated
	 * once, in one place, rather than implied by the order of three markup
	 * blocks that a later edit could silently swap.
	 */
	$: bands = [
		{
			key: 'waiting',
			label: 'Menunggu',
			hint: 'Sudah terdaftar, belum dipanggil',
			rows: waiting,
			olderRows: [] as AdminQueueTicket[],
			olderLabel: ''
		},
		{
			key: 'called',
			label: 'Dipanggil',
			hint: 'Sedang dipanggil atau dilayani',
			rows: calledScope.live,
			olderRows: calledScope.older,
			olderLabel: 'Dipanggil sebelum hari ini'
		},
		{
			key: 'finished',
			label: finishedLabel,
			hint: finishedHint,
			rows: finishedScope.live,
			olderRows: finishedScope.older,
			olderLabel: 'Selesai sebelum hari ini'
		}
	];

	/**
	 * The board-level source marker. Delegates to `latestQueueChange`, which owns
	 * the schema question documented there: `queue_tickets` carries no
	 * `updated_at`, so the marker is derived from the three timestamps a status
	 * transition actually stamps.
	 */
	$: lastRowChange = latestQueueChange(tickets);

	/**
	 * FOCUS AFTER A MUTATION.
	 *
	 * The queue is worked in a tight loop — Panggil, Panggil, Layani, Panggil —
	 * and every SUCCESSFUL transition moves the clicked row out of its band. That
	 * DESTROYS the button the operator just pressed, so focus falls to <body> and
	 * they must re-find their place before the next call: the single most
	 * expensive interruption repeated across a shift. So on success, focus moves
	 * to the next waiting patient's primary (Panggil) action instead.
	 *
	 * On FAILURE the row does not move: its (re-enabled) button stays exactly
	 * where it was and focus is already on it, so nothing is done there and focus
	 * never collapses to <body>.
	 *
	 * Detection reuses the page's single pending slot. The page drives
	 * `pendingTicketId` from '' to the ticket id while a request is in flight and
	 * back to '' once it settles — and by then the reload has already resolved,
	 * so the bands reflect the server's new truth. An idle poll refresh never
	 * changes `pendingTicketId`, so it can never yank focus from the operator.
	 */
	let boardEl: HTMLDivElement;
	let previousPendingId = '';

	afterUpdate(() => {
		if (previousPendingId !== '' && pendingTicketId === '') {
			const finishedId = previousPendingId;
			previousPendingId = pendingTicketId;
			if (errorTicketId !== finishedId) {
				focusNextPatient();
			}
			return;
		}
		previousPendingId = pendingTicketId;
	});

	/**
	 * Moves focus to the next waiting patient's primary action, falling back to
	 * the first action left on the board, then to the board itself — so focus
	 * never collapses to <body> even when the queue is empty.
	 */
	function focusNextPatient() {
		if (typeof document === 'undefined') return;
		const button = boardEl?.querySelector<HTMLButtonElement>(
			'.sigap-queue-band[data-band-key="waiting"] .sigap-queue-row__actions button'
		);
		if (button) {
			button.focus();
			return;
		}
		const any = boardEl?.querySelector<HTMLButtonElement>(
			'.sigap-queue-row__actions button'
		);
		if (any) {
			any.focus();
			return;
		}
		boardEl?.focus();
	}
</script>

<div class="sigap-queue-board" bind:this={boardEl} tabindex="-1">
	{#each bands as band (band.key)}
		<QueueBand
			bandKey={band.key}
			label={band.label}
			hint={band.hint}
			rows={band.rows}
			olderRows={band.olderRows}
			olderLabel={band.olderLabel}
			{facilityIndex}
			{onTransition}
			{pendingTicketId}
			{pendingStatus}
			{errorTicketId}
			{errorMessage}
		/>
	{/each}

	<!--
		The board-level source marker. Paired with the page's "Diperbarui pukul"
		label, which answers WHEN THE PAGE LOADED; this answers WHEN THE DATA LAST
		CHANGED. Those are different questions and an operator comparing them is how
		a stalled feed gets noticed.
	-->
	<p class="sigap-queue-board__source">
		Diperbarui pukul {formatDateTime(lastRowChange)} — waktu perubahan antrean
		terakhir yang tercatat pada data ini.
	</p>
</div>

<style>
	.sigap-queue-board {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-5);
	}

	.sigap-queue-board__source {
		margin: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	/*
		MOBILE (≤767px): stop the table scaffold from imposing table-row-group
		geometry, so QueueBoardRow's cards can lay out as blocks.

		The reflow is scoped to `.sigap-queue-board` (this board only) rather
		than written as a bare `:global(.sigap-data-table)`, which would hide
		every admin table's header on a phone. The card layout itself lives in
		QueueBoardRow; this is only the structural unlock: the `<table>` and
		`<tbody>` become block boxes and the desktop column header is hidden,
		because each card carries its own field labels.
	*/
	@media (max-width: 767px) {
		.sigap-queue-board :global(.sigap-data-table) {
			display: block;
		}

		.sigap-queue-board :global(.sigap-data-table tbody) {
			display: block;
		}

		.sigap-queue-board :global(.sigap-data-table thead) {
			display: none;
		}
	}
</style>
