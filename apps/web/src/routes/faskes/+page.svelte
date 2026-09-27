<script lang="ts">
	import { onDestroy } from 'svelte';
	import FacilitySearch from '$lib/citizen/FacilitySearch.svelte';
	import FacilityResultRow from '$lib/citizen/FacilityResultRow.svelte';
	import {
		loadPublicFacilities,
		type CatalogSnapshot
	} from '$lib/citizen/usePublicFacilities';
	import Skeleton from '$lib/ui/Skeleton.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { hasSession as readHasSession } from '$lib/stores/session';
	import { RADIUS } from '$lib/design/tokens';
	import { FolderSearch, SearchX, CloudOff, X } from 'lucide-svelte';

	/**
	 * /faskes — the public facility catalog.
	 *
	 * Five states, kept deliberately distinct because they mean different things
	 * to a citizen and need different responses:
	 *
	 *   1. loading      — skeletons, region marked busy
	 *   2. results      — the real catalog from the public endpoint
	 *   3. empty        — the catalog itself has no facilities (an operational
	 *                     state the operator has to fix)
	 *   4. no results   — the catalog is fine, the citizen's search excluded
	 *                     everything (their input, their reset button)
	 *   5. error        — the request failed, with retry
	 *
	 * Collapsing 3 and 4 would be actively misleading: a citizen whose search
	 * returned nothing must not be told the town has no health facilities.
	 *
	 * Search runs over already-loaded data. The public catalog endpoint has no
	 * server-side search or filter contract, so filtering happens here rather
	 * than inventing a query parameter the backend would silently ignore.
	 */

	const catalogLoader = loadPublicFacilities();

	/**
	 * Subscribed rather than read through a getter. The loader's state is
	 * mutated by an async fetch, and Svelte re-runs a `$:` block only when a
	 * tracked dependency changes — reading `catalog.state` inside one does not
	 * make it one, so the page would load successfully and then render its
	 * loading state forever. The teardown releases the listener on unmount.
	 */
	let catalog: CatalogSnapshot = { state: 'loading', facilities: [], error: null };
	const unsubscribe = catalogLoader.unsubscribe((next) => (catalog = next));

	onDestroy(unsubscribe);

	/**
	 * The retry action comes from the loader, not from the snapshot.
	 *
	 * `catalog` is the data — state, rows, error — and a snapshot is a value,
	 * so it carries no behaviour. Reading `catalog.reload` therefore yields
	 * `undefined`, which ErrorState treats as "no retry available" and silently
	 * drops the button. The affordance disappears and nothing looks wrong.
	 */
	const reload = catalogLoader.reload;

	let query: string = '';

	/**
	 * Type filtering is not offered, and that is a contract decision rather than
	 * an omission.
	 *
	 * The public endpoint returns exactly `id`, `name`, `short_code`, and
	 * `is_active`. There is no `type` on the wire, so a "Puskesmas / Rumah
	 * Sakit" filter would have to infer the type from the facility name — and a
	 * name that happens to contain "RS" is not a verified classification.
	 * Presenting a guess as a filter would let a citizen rule out their own
	 * clinic because of a naming convention.
	 *
	 * Guessing from the name is precisely the fabrication this phase forbids.
	 * The filter therefore stays absent until the catalog endpoint exposes
	 * `type` as a real field, at which point it becomes real filtering over
	 * real data. Search over names and short codes works today and needs no
	 * invented field.
	 */

	$: normalisedQuery = query.trim().toLowerCase();

	// The store exposes a getter function, not a value. Calling it keeps the
	// prop a plain boolean, which is all ErrorState is allowed to know about.
	$: hasSession = readHasSession();
	$: filtered = normalisedQuery
		? catalog.facilities.filter(
				(facility) =>
					facility.name.toLowerCase().includes(normalisedQuery) ||
					facility.short_code.toLowerCase().includes(normalisedQuery)
			)
		: catalog.facilities;

	/**
	 * Distinguishes "your search matched nothing" from "there is nothing".
	 * Only meaningful once the catalog itself has loaded with rows.
	 */
	$: isNoResult =
		catalog.state === 'ready' &&
		catalog.facilities.length > 0 &&
		filtered.length === 0;

	function clearFilters() {
		query = '';
	}
</script>

<svelte:head>
	<title>Cari Faskes — Sigap</title>
</svelte:head>

<div class="sigap-faskes">
	<section class="sigap-faskes__intro" aria-labelledby="sigap-faskes-title">
		<h1 id="sigap-faskes-title" class="sigap-page-title">Cari Faskes</h1>
		<p class="sigap-page-subtitle">Telusuri fasilitas kesehatan yang aktif di Sigap.</p>
	</section>

	<!--
		Search is hidden while loading and on error. Offering a filter over data
		that has not arrived would imply results exist, and offering it on an
		error screen would let a citizen filter an empty list and conclude the
		search is broken.
	-->
	{#if catalog.state === 'ready' && catalog.facilities.length > 0}
		<section class="sigap-faskes__search" aria-label="Cari dan saring faskes">
			<FacilitySearch value={query} onInput={(next) => (query = next)} />
		</section>
	{/if}

	<section class="sigap-faskes__results" aria-labelledby="sigap-faskes-results-title">
		{#if catalog.state === 'loading'}
			<!--
				`aria-busy` on the region is what distinguishes "still loading" from
				"nothing here". The skeletons themselves are decorative and hidden
				from assistive tech by the Skeleton primitive.
			-->
			<div
				class="sigap-faskes__status"
				role="status"
				aria-busy="true"
				aria-live="polite"
			>
				<p class="sigap-faskes__status-text">Memuat daftar faskes…</p>
				<div class="sigap-faskes__skeletons" style:border-radius={RADIUS.panel}>
					{#each Array.from({ length: 4 }) as _, index (index)}
						<div class="sigap-faskes__skeleton-row">
							<Skeleton width="50%" height="15px" />
							<Skeleton width="20%" height="12px" />
						</div>
					{/each}
				</div>
			</div>
		{:else if catalog.state === 'error' && catalog.error}
			<h2 id="sigap-faskes-results-title" class="sigap-visually-hidden">Hasil pencarian</h2>
			<ErrorState
				error={catalog.error}
				{hasSession}
				icon={CloudOff}
				onRetry={reload}
				retryLabel="Coba lagi"
			/>
		{:else if catalog.facilities.length === 0}
			<h2 id="sigap-faskes-results-title" class="sigap-visually-hidden">Hasil pencarian</h2>
			<!-- State 3: empty catalog. Nothing is wrong with the citizen's search. -->
			<EmptyState
				title="Belum ada data faskes"
				description="Daftar fasilitas kesehatan masih kosong. Faskes akan tampil di sini setelah diaktifkan oleh pengelola."
				icon={FolderSearch}
				actionLabel="Muat ulang"
				onAction={reload}
			/>
		{:else if isNoResult}
			<!-- State 4: the search excluded everything. Offer the way back. -->
			<p class="sigap-faskes__count" role="status" aria-live="polite">
				Tidak ada hasil untuk “{query.trim()}”
			</p>
			<EmptyState
				title="Tidak ada faskes yang cocok"
				description="Coba kata kunci lain yang lebih pendek, atau hapus semua filter untuk melihat seluruh faskes."
				icon={SearchX}
				actionLabel="Hapus semua filter"
				onAction={clearFilters}
			/>
		{:else}
			<!-- State 2: results. -->
			<p class="sigap-faskes__count" role="status" aria-live="polite">
				{#if normalisedQuery}
					Menampilkan {filtered.length} dari {catalog.facilities.length} faskes
				{:else}
					Menampilkan {catalog.facilities.length} faskes
				{/if}
			</p>

			<ul
				id="sigap-facility-results"
				class="sigap-faskes__list"
				style:border-radius={RADIUS.panel}
			>
				{#each filtered as facility (facility.id)}
					<li>
						<FacilityResultRow {facility} />
					</li>
				{/each}
			</ul>

			{#if normalisedQuery}
				<div class="sigap-faskes__clear">
					<Button variant="secondary" icon={X} onClick={clearFilters}>
						Hapus semua filter
					</Button>
				</div>
			{/if}
		{/if}
	</section>
</div>

<style>
	.sigap-faskes {
		max-width: 1024px;
		margin: 0 auto;
		padding: 20px 16px 8px;
	}

	@media (min-width: 1024px) {
		.sigap-faskes {
			padding: 32px 24px 8px;
		}
	}

	.sigap-faskes__search {
		margin-top: 16px;
	}

	.sigap-faskes__results {
		margin-top: 16px;
	}

	.sigap-faskes__status-text {
		margin: 0 0 8px;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-faskes__count {
		margin: 0 0 8px;
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-muted);
	}

	.sigap-faskes__skeletons {
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-faskes__skeleton-row {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.sigap-faskes__skeleton-row + .sigap-faskes__skeleton-row {
		margin-top: 16px;
	}

	.sigap-faskes__list {
		margin: 0;
		padding: 0;
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	/*
		Rows are separate list items, so the divider is drawn between items
	 *rather than inside the row, which keeps the border off the rounded outer
	 *edges of the panel.
	*/
	.sigap-faskes__list > li + li {
		border-top: 1px solid var(--sigap-border);
	}

	.sigap-faskes__clear {
		display: flex;
		justify-content: center;
		margin-top: 16px;
	}
</style>
