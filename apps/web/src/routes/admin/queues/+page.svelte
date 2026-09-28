<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import AdminToolbar from '$lib/admin/AdminToolbar.svelte';
	import FacilityFilter from '$lib/admin/FacilityFilter.svelte';
	import QueueBoard from '$lib/admin/QueueBoard.svelte';
	import {
		boardUpdatedLabel,
		createQueueBoard,
		showsStaleWarning,
		type QueueBoardState
	} from '$lib/admin/queueBoard';
	import { listFacilities, listQueueTickets } from '$lib/api/endpoints/admin';
	import { isEmptyScope } from '$lib/admin/readState';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from '$lib/ui/Icon.svelte';
	import { AlertTriangle } from 'lucide-svelte';
	import { subscribeToSession, getSession } from '$lib/stores/session';
	import type { AdminFacility } from '$lib/api/types/api';

	/**
	 * T-3B4-04: the queue read board.
	 *
	 * Polling is delegated wholesale to `createQueueBoard`, which wraps the
	 * Phase 3B1 `createPolling` helper. What this page owns is the wiring and the
	 * two things the helper cannot know: which facilities the operator has, and
	 * that a failed refresh must not blank the board.
	 *
	 * Facilities are loaded ONCE and never polled. Facility scope changes when an
	 * administrator changes a grant, which is rare, and the ticket rows carry
	 * their own facility references — polling a second list every thirty seconds
	 * would add a request per board for data that is already in hand.
	 */
	const board = createQueueBoard({ read: (signal) => listQueueTickets(signal) });

	let state: QueueBoardState = board.state;
	let facilities: AdminFacility[] = [];
	let facilitiesLoaded = false;
	let facilityFilter = '';
	let search = '';
	let unsubscribeSession: (() => void) | undefined;
	let unsubscribeBoard: (() => void) | undefined;

	/**
	 * `hasSession` is read from the store rather than from layout data. It is the
	 * ONLY session fact the client holds — no role, no permission list, no scope —
	 * and it exists solely to choose between the sign-in and refusal panels for a
	 * 403. Reading it from the store means every admin page consults the same
	 * source, so two pages can never disagree about whether anyone is signed in.
	 */
	let hasSession = getSession().hasSession;

	onMount(async () => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});
		unsubscribeBoard = board.subscribe((next) => {
			state = next;
		});
		board.start();

		// The scoped-facility read is what decides the class-1 empty state, so it
		// must resolve before the page claims anything about emptiness. Until it
		// does, `facilitiesLoaded` is false and no empty verdict is rendered.
		const result = await listFacilities();
		if (result.ok) {
			facilities = result.data;
			facilitiesLoaded = true;
		} else {
			// A failed scope read must not masquerade as an empty scope. Leaving
			// `facilitiesLoaded` false keeps the board authoritative and avoids
			// telling an operator they have no facilities.
			facilitiesLoaded = false;
		}
	});

	onDestroy(() => {
		// Required, not optional: createPolling deliberately does not tear itself
		// down, because a store and a component have different teardown contracts.
		// Without this the timer and the visibilitychange listener outlive the page.
		board.stop();
		unsubscribeBoard?.();
		unsubscribeSession?.();
	});

	/** Client-side narrowing over already-scoped, already-loaded rows. */
	$: scopedRows = state.load.rows;
	$: filtered = scopedRows.filter((ticket) => {
		if (facilityFilter && ticket.facility_id !== facilityFilter) return false;
		if (search) {
			const needle = search.trim().toLowerCase();
			if (!ticket.formatted_number.toLowerCase().includes(needle)) return false;
		}
		return true;
	});

	$: updatedLabel = boardUpdatedLabel(state);
	$: stale = showsStaleWarning(state);
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Antrean"
		subtitle="Pantau antrean dalam lingkup akses Anda. Pembaruan otomatis sekitar 30 detik, dijeda saat tab tidak aktif."
	>
		<svelte:fragment slot="aside">
			{#if updatedLabel}
				<!--
					Always present once data has loaded, INCLUDING in the stale state. An
					operator looking at possibly-out-of-date rows is exactly who needs to
					be told when those rows were true.
				-->
				<span class="sigap-admin-updated" aria-live="polite">{updatedLabel}</span>
			{/if}
		</svelte:fragment>
	</AdminPageHeader>

	<AdminToolbar
		refreshing={state.refreshing}
		onRefresh={() => void board.refresh()}
		note="Daftar antrean sudah dibatasi fasilitas dalam lingkup akses Anda. Filter di bawah hanya mempersempit tampilan data yang sudah dimuat."
	>
		<div class="sigap-admin-search">
			<label class="sigap-admin-search__label" for="queue-search">Cari nomor</label>
			<input
				class="sigap-admin-search__input"
				style:border-radius={RADIUS.control}
				id="queue-search"
				type="search"
				placeholder="Nomor antrean"
				bind:value={search}
			/>
		</div>

		<FacilityFilter
			{facilities}
			selected={facilityFilter}
			visible={filtered.length}
			total={scopedRows.length}
			onSelect={(value) => (facilityFilter = value)}
		/>
	</AdminToolbar>

	{#if stale}
		<!--
			The stale banner, not a replacement. The rows below stay on screen because
			they are the last thing that was true, and an operator mid-shift needs
			them far more than they need a tidy error page.
		-->
		<div class="sigap-admin-stale" style:border-radius={RADIUS.panel} role="alert">
			<Icon icon={AlertTriangle} size={18} />
			<div>
				<p class="sigap-admin-stale__title">Gagal memperbarui antrean.</p>
				<p class="sigap-admin-stale__body">
					Menampilkan data terakhir yang berhasil dimuat ({updatedLabel}). Periksa
					koneksi Anda, lalu coba lagi.
				</p>
			</div>
		</div>
	{/if}

	<AdminReadState
		loading={state.load.loading}
		error={state.load.failed ? state.load.error : null}
		{hasSession}
		rowCount={scopedRows.length}
		scopeEmpty={facilitiesLoaded && isEmptyScope(facilities)}
		emptyTitle="Belum ada antrean"
		emptyDescription="Belum ada antrean dalam lingkup operator saat ini."
		onRetry={() => void board.refresh()}
	>
		{#if filtered.length === 0}
			<div class="sigap-admin-nomatch" style:border-radius={RADIUS.panel} role="status">
				<p class="sigap-admin-nomatch__title">Tidak ada antrean yang cocok</p>
				<p class="sigap-admin-nomatch__body">
					Ubah atau hapus pencarian dan filter fasilitas Anda.
				</p>
				<button
					type="button"
					class="sigap-admin-nomatch__reset"
					style:border-radius={RADIUS.control}
					on:click={() => {
						search = '';
						facilityFilter = '';
					}}
				>
					Hapus filter
				</button>
			</div>
		{:else}
			<QueueBoard tickets={filtered} {facilities} />
		{/if}
	</AdminReadState>
</div>

<style>
	.sigap-admin-page {
		min-width: 0;
	}

	.sigap-admin-updated {
		font-size: 12px;
		color: var(--sigap-muted);
		/* Tabular figures so the label does not jitter as the minutes tick over. */
		font-variant-numeric: tabular-nums;
	}

	.sigap-admin-search {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.sigap-admin-search__label {
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-admin-search__input {
		height: 36px;
		width: 180px;
		padding: 0 10px;
		border: 1px solid var(--sigap-border);
		background-color: var(--sigap-surface);
		color: var(--sigap-foreground);
		font: inherit;
		font-size: 13px;
	}

	.sigap-admin-search__input:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-admin-stale {
		display: flex;
		gap: 10px;
		align-items: flex-start;
		margin: 12px 0;
		padding: 12px;
		font-size: 13px;
		line-height: 1.45;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-left: 3px solid var(--sigap-danger);
		color: var(--sigap-danger);
	}

	.sigap-admin-stale__title {
		margin: 0;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-admin-stale__body {
		margin: 0;
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

	.sigap-admin-nomatch__reset:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-admin-nomatch__reset:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
