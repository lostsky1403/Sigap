<script lang="ts">
	import DataTable from '$lib/ui/DataTable.svelte';
	import QueueBoardRow from './QueueBoardRow.svelte';
	import { facilityName } from '$lib/domain/joins';
	import { allowedQueueTransitions } from '$lib/domain/status';
	import type { AdminFacility, AdminQueueTicket, QueueStatus } from '$lib/api/types/api';

	/**
	 * One band of the queue board: its heading, its table, and — when the band is
	 * date-scoped — the collapsed group of rows from earlier days.
	 *
	 * WHY THE OLDER GROUP IS A SIBLING OF THE `<section>`, NOT A CHILD.
	 *
	 * The section is named by its heading (`aria-labelledby`), and a heading that
	 * says "Selesai hari ini" must not name a container that also holds last
	 * week's rows. A screen reader entering the section hears the heading and
	 * then every row under it, with no boundary between today and the archive. So
	 * the archive is a separate `<details>` element next to the section: it is
	 * labelled by its own summary ("Selesai sebelum hari ini — 12 antrean"),
	 * which states its scope before any row is read out.
	 *
	 * Closed by default, and not rendered at all when empty — an empty
	 * disclosure is a control that does nothing.
	 */
	export let bandKey: string = '';
	export let label: string = '';
	export let hint: string = '';
	/** Rows inside the band's day scope, in board order. */
	export let rows: readonly AdminQueueTicket[] = [];
	/** Rows provably outside that scope. Collapsed, never mixed into `rows`. */
	export let olderRows: readonly AdminQueueTicket[] = [];
	/** Summary text for the collapsed group, e.g. "Selesai sebelum hari ini". */
	export let olderLabel: string = '';
	export let facilityIndex: ReadonlyMap<string, AdminFacility> = new Map();
	export let onTransition: (ticketId: string, status: QueueStatus) => void = () => {};
	export let pendingTicketId: string = '';
	export let pendingStatus: QueueStatus | '' = '';
	export let errorTicketId: string = '';
	export let errorMessage: string = '';

	/**
	 * The columns, shared by every band so the operator reads one table shape.
	 *
	 * `secondary` is hidden below 1280px, per the Pen's own note on the 768
	 * frame — "Kolom sekunder disembunyikan di bawah 1280px. Status dan Aksi
	 * tidak pernah disembunyikan." `pin` keeps Aksi inside the visible area when
	 * the remaining columns are wider than a phone: without it the only action
	 * on the board sits past the right edge of a scroll box with no affordance.
	 */
	const COLUMNS = [
		{ label: 'Nomor' },
		{ label: 'Status' },
		{ label: 'Fasilitas' },
		{ label: 'Terdaftar', secondary: true },
		{ label: 'Dipanggil', secondary: true },
		{ label: 'Selesai', secondary: true },
		{ label: 'Aksi', pin: true }
	] as const;
</script>

<div class="sigap-queue-band" data-band-key={bandKey}>
	<section aria-labelledby="queue-group-{bandKey}">
		<div class="sigap-queue-band__head">
			<h2 class="sigap-queue-band__title" id="queue-group-{bandKey}">{label}</h2>
			<!--
				The count is in the heading rather than a badge beside it, so a
				screen reader announcing the section hears the number as part of the
				group name instead of as an unlabelled number floating nearby.
			-->
			<span class="sigap-queue-band__count">{rows.length}</span>
			<p class="sigap-queue-band__hint">{hint}</p>
		</div>

		{#if rows.length === 0}
			<p class="sigap-queue-band__empty">Tidak ada antrean pada kelompok ini.</p>
		{:else}
			<DataTable
				caption="{label} — {rows.length} antrean dalam lingkup akses"
				columns={COLUMNS}
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

	{#if olderRows.length > 0}
		<details class="sigap-queue-band__older">
			<summary class="sigap-queue-band__older-summary">
				{olderLabel} — {olderRows.length} antrean
			</summary>
			<div class="sigap-queue-band__older-body">
				<DataTable
					caption="{olderLabel} — {olderRows.length} antrean dalam lingkup akses"
					columns={COLUMNS}
				>
					{#each olderRows as ticket (ticket.id)}
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
			</div>
		</details>
	{/if}
</div>

<style>
	/*
		The wrapper exists so the archive can sit close to its band's table. The
		board's own gap between bands is 24px; using that here would push the
		collapsed summary so far down that it reads as part of the NEXT band.
	*/
	.sigap-queue-band {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-2);
	}

	/*
		QUEUE BOARD ROW DENSITY: 36px, the bottom of the 36-40px admin band.

		The shared `DataTable` pins every data row at 40px — the comfortable
		admin row. The queue board is different from the appointment/schedule
		tables: a clerk works it for a whole shift, advancing ticket after
		ticket, so it benefits from more rows per screen. The action buttons here
		are already the 36px ADMIN-COMPACT density (`DENSITY.adminCompact` in
		QueueBoardRow), so a 36px row holds them exactly — no control is shrunk
		to hit the number — while ~10% more waiting patients fit in a viewport.

		The override is scoped to `.sigap-queue-band`, so it reaches only this
		board and never the shared tables whose 40px contract is measured by
		e2e/admin-table-density.spec.ts (whose TABLES list deliberately omits
		/admin/queues).

		`:global` is load-bearing for the same reason `DataTable` documents: the
		`<td>` cells live in `QueueBoardRow`, a different Svelte component, so a
		scoped selector here would match nothing and the board would silently
		stay at the shared 40px. `height` on a table cell is a minimum under
		auto layout, so the long outcome message on a failed mutation still
		expands its row rather than clipping.
	*/
	/*
		The 36px band applies ONLY while the board is a real `<table>` (≥768px).
		Below 768px `QueueBoard` reflows `<table>`/`<tbody>`/`<tr>` into block
		boxes and `QueueBoardRow` turns each row into a stacked CARD whose height
		is its content. A fixed 36px cell there pins the actions cell shorter than
		its stacked buttons, so the controls paint past the card into the next row
		(measured 31px of overlap at 390/360). Scoping the density to the table
		breakpoint removes the conflicting height and lets QueueBoardRow's
		`height: auto` govern the card.
	*/
	@media (min-width: 768px) {
		.sigap-queue-band :global(.sigap-data-table tbody td) {
			height: 36px;
		}
	}

	.sigap-queue-band__head {
		display: flex;
		align-items: baseline;
		gap: var(--sigap-space-2);
		flex-wrap: wrap;
		margin-bottom: var(--sigap-space-2);
	}

	.sigap-queue-band__title {
		margin: 0;
		font-size: var(--sigap-type-label);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-queue-band__count {
		/* `min-width: 2ch` keeps a single-digit count from shifting the hint. */
		min-width: 2ch;
		font-size: var(--sigap-type-label);
		font-weight: 600;
		text-align: right;
		color: var(--sigap-primary);
		font-variant-numeric: tabular-nums;
	}

	.sigap-queue-band__hint {
		margin: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-queue-band__empty {
		margin: 0;
		padding: var(--sigap-space-4) var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
		background-color: var(--sigap-surface);
		/* A panel outline is decorative: it is a placeholder ground, not a control. */
		border: 1px dashed var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	/*
		The collapsed archive. `summary` is the only interactive element in it, so
		it carries the target size and the pointer: an operator looking for an old
		ticket needs to see that there is something to open.
	*/
	.sigap-queue-band__older-summary {
		/*
			No `display` override: the default `list-item` is what draws the
			disclosure triangle. Setting flex here centres the text but silently
			removes the only visual signal that the group opens.
		*/
		min-height: 40px;
		padding: 10px var(--sigap-space-3);
		line-height: 1.4;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
		cursor: pointer;
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-queue-band__older-summary:hover {
		color: var(--sigap-foreground);
	}

	.sigap-queue-band__older-body {
		margin-top: var(--sigap-space-2);
	}
</style>
