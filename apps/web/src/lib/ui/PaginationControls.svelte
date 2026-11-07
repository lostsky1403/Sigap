<script lang="ts">
	import Button from './Button.svelte';
	import { DENSITY } from './density';

	/**
	 * Bounded pagination for the admin list surfaces.
	 *
	 * WHY CLIENT-SIDE: the admin endpoints take no query parameters (admin.go
	 * parses none), so there is no server paging contract to honour. This pages
	 * the list the page has ALREADY loaded. It does not pretend otherwise —
	 * there is no "server-side" wording here, because a control that implies the
	 * server is returning page 4 of a larger set would be a lie about where the
	 * data came from.
	 *
	 * The whole point is the tab order. An unpaginated appointments table is
	 * ~254 rows x 3 action buttons ≈ 691 tab stops, which is unusable by
	 * keyboard. Bounding the rendered rows bounds the tab stops with it.
	 *
	 * Announcement, not focus theft: the range line is a polite live region, so
	 * a page change is spoken without yanking focus off the button the operator
	 * just pressed. Focus staying put is what lets them press "Berikutnya"
	 * repeatedly without hunting for it again.
	 */
	export let page: number = 1;
	export let pageSize: number = 25;
	export let total: number = 0;
	export let onPageChange: (page: number) => void = () => {};

	/** Accessible name for the nav region, e.g. "Navigasi halaman janji temu". */
	export let label: string = 'Navigasi halaman';
	/** The noun counted, so the range reads "dari 254 janji temu". */
	export let itemLabel: string = 'baris';

	$: pageCount = Math.max(1, Math.ceil(total / pageSize));
	$: from = total === 0 ? 0 : (page - 1) * pageSize + 1;
	$: to = Math.min(page * pageSize, total);
	$: canPrev = page > 1;
	$: canNext = page < pageCount;

	function go(next: number) {
		if (next < 1 || next > pageCount || next === page) return;
		onPageChange(next);
	}
</script>

<!--
	Rendered only when there is more than one page. A single page of results
	does not need a control that can only be pressed to no effect.
-->
{#if pageCount > 1}
	<nav class="sigap-pagination" aria-label={label}>
		<p class="sigap-pagination__range" role="status" aria-live="polite">
			Menampilkan {from}–{to} dari {total}
			{itemLabel}
		</p>
		<div class="sigap-pagination__controls">
			<Button
				variant="secondary"
				size={DENSITY.adminCompact}
				disabled={!canPrev}
				onClick={() => go(page - 1)}>Sebelumnya</Button
			>
			<span class="sigap-pagination__page">Halaman {page} dari {pageCount}</span>
			<Button
				variant="secondary"
				size={DENSITY.adminCompact}
				disabled={!canNext}
				onClick={() => go(page + 1)}>Berikutnya</Button
			>
		</div>
	</nav>
{/if}

<style>
	.sigap-pagination {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sigap-space-4);
		flex-wrap: wrap;
		margin-top: var(--sigap-space-4);
		padding-top: var(--sigap-space-3);
		border-top: 1px solid var(--sigap-border-subtle);
	}

	.sigap-pagination__range {
		margin: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		/* Tabular figures so the range does not jitter as the page changes. */
		font-variant-numeric: tabular-nums;
	}

	.sigap-pagination__controls {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-3);
	}

	.sigap-pagination__page {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-foreground);
		font-variant-numeric: tabular-nums;
	}

	/*
		Below the tablet breakpoint the controls take the full width and the
		range sits above them, so neither wraps into an unreadable pair of
		half-buttons on a narrow screen.
	*/
	@media (max-width: 767px) {
		.sigap-pagination {
			flex-direction: column;
			align-items: stretch;
			gap: var(--sigap-space-2);
		}

		.sigap-pagination__controls {
			justify-content: space-between;
		}
	}
</style>
