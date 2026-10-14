<script lang="ts">
	import DataTable from '$lib/ui/DataTable.svelte';
	import QueueBoardRow from './QueueBoardRow.svelte';
	import { indexBy, facilityName } from '$lib/domain/joins';
	import { allowedQueueTransitions } from '$lib/domain/status';
	import { latestQueueChange } from './queueSource';
	import { formatDateTime } from '$lib/domain/format';
	import type { AdminFacility, AdminQueueTicket, QueueStatus } from '$lib/api/types/api';

	/**
	 * The queue read board.
	 *
	 * Groups the scoped tickets into the three frozen bands — Dipanggil, Menunggu,
	 * Selesai hari ini — and renders each as its own table so an operator can
	 * scan one band without sorting. The grouping is DERIVED from the current
	 * status field, not from a date comparison: a ticket's status is the server's
	 * authoritative answer, and re-deriving "is it done today" from a timestamp
	 * would contradict the backend the moment the two disagreed.
	 *
	 * The board is READ-ONLY in this phase. The transition buttons are rendered
	 * disabled, from the same verified state machine the server enforces, so an
	 * operator can see what is possible next without being offered an action that
	 * would 409. Phase 3B5 wires execution; `actionsEnabled` is the single switch
	 * that turns them on, so nothing else has to change to make them live.
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
	 * The three bands. Membership is by status alone:
	 *   Dipanggil  — called, or in service. These are the tickets an operator acts on now.
	 *   Menunggu   — waiting.
	 *   Selesai    — completed today, plus the closed states (cancelled, skipped).
	 */
	$: called = tickets.filter((t) => t.status === 'called' || t.status === 'in_service');
	$: waiting = tickets.filter((t) => t.status === 'waiting');
	$: finished = tickets.filter(
		(t) => t.status === 'completed' || t.status === 'cancelled' || t.status === 'skipped'
	);

	/**
	 * The board-level source marker. Delegates to `latestQueueChange`, which owns
	 * the schema question documented there: `queue_tickets` carries no
	 * `updated_at`, so the marker is derived from the three timestamps a status
	 * transition actually stamps.
	 */
	$: lastRowChange = latestQueueChange(tickets);

	const GROUPS = [
		{ key: 'called', label: 'Dipanggil', hint: 'Sedang dipanggil atau dilayani' },
		{ key: 'waiting', label: 'Menunggu', hint: 'Sudah terdaftar, belum dipanggil' },
		{ key: 'finished', label: 'Selesai hari ini', hint: 'Sudah diselesaikan atau ditutup' }
	] as const;
</script>

<div class="sigap-queue-board">
	{#each GROUPS as group (group.key)}
		{@const rows = group.key === 'called' ? called : group.key === 'waiting' ? waiting : finished}
		<section class="sigap-queue-board__group" aria-labelledby="queue-group-{group.key}">
			<div class="sigap-queue-board__group-head">
				<h2 class="sigap-queue-board__group-title" id="queue-group-{group.key}">
					{group.label}
				</h2>
				<!--
					The count is in the heading rather than a badge beside it, so a
					screen reader announcing the section hears the number as part of the
					group name instead of as an unlabelled number floating nearby.
				-->
				<span class="sigap-queue-board__group-count">{rows.length}</span>
				<p class="sigap-queue-board__group-hint">{group.hint}</p>
			</div>

			{#if rows.length === 0}
				<p class="sigap-queue-board__group-empty">Tidak ada antrean pada kelompok ini.</p>
			{:else}
				<DataTable
					caption="{group.label} — {rows.length} antrean dalam lingkup akses"
					columns={[
						{ label: 'Nomor' },
						{ label: 'Status' },
						{ label: 'Fasilitas' },
						{ label: 'Terdaftar' },
						{ label: 'Dipanggil', secondary: true },
						{ label: 'Selesai', secondary: true },
						{ label: 'Aksi' }
					]}
				>
					{#each rows as ticket (ticket.id)}
						<QueueBoardRow
							ticketId={ticket.id}
							formattedNumber={ticket.formatted_number}
							status={ticket.status}
							facilityName={facilityName(facilityIndex, ticket.facility_id)}
							registeredAt={ticket.registered_at}
							calledAt={ticket.called_at}
							completedAt={ticket.completed_at}
							transitions={allowedQueueTransitions(ticket.status)}
							{onTransition}
							pendingStatus={pendingTicketId === ticket.id ? pendingStatus : ''}
							errorMessage={errorTicketId === ticket.id ? errorMessage : ''}
						/>
					{/each}
				</DataTable>
			{/if}
		</section>
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
		gap: 24px;
	}

	.sigap-queue-board__group-head {
		display: flex;
		align-items: baseline;
		gap: 8px;
		flex-wrap: wrap;
		margin-bottom: 8px;
	}

	.sigap-queue-board__group-title {
		margin: 0;
		font-size: 13px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-queue-board__group-count {
		/* `min-width: 2ch` keeps a single-digit count from shifting the hint. */
		min-width: 2ch;
		font-size: 13px;
		font-weight: 600;
		text-align: right;
		color: var(--sigap-primary);
		font-variant-numeric: tabular-nums;
	}

	.sigap-queue-board__group-hint {
		margin: 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-queue-board__group-empty {
		margin: 0;
		padding: 16px 12px;
		font-size: 13px;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface);
		border: 1px dashed var(--sigap-border);
		border-radius: 8px;
	}

	.sigap-queue-board__source {
		margin: 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
