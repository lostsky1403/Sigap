<script lang="ts">
	import { onDestroy } from 'svelte';
	import FacilitySearch from '$lib/citizen/FacilitySearch.svelte';
	import FacilityResultRow from '$lib/citizen/FacilityResultRow.svelte';
	import {
		loadPublicFacilities,
		type CatalogSnapshot
	} from '$lib/citizen/usePublicFacilities';
	import {
		FACILITY_TYPE_LABELS,
		PUBLIC_FACILITY_TYPES,
		isPublicFacilityType
	} from '$lib/citizen/facilityType';
	import type { PublicFacilityType } from '$lib/api/types/api';
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
	 * Facility type filter — now real filtering, not inference.
	 *
	 * This used to be absent, and the reason was correct at the time: the
	 * public endpoint returned only `id`, `name`, `short_code`, and
	 * `is_active`. A "Puskesmas / Rumah Sakit" filter would then have had to
	 * guess the type from the facility name, and a name that happens to start
	 * with "RS" is not a verified classification. Presenting a guess as a
	 * filter would let a citizen rule out their own clinic over a naming
	 * convention.
	 *
	 * The catalog endpoint now projects the existing `facilities.type` enum
	 * read-only, so the filter compares one real field against another. Nothing
	 * here inspects `name` or `short_code`, and no `?type=` query parameter
	 * exists on the backend — filtering the loaded catalog here is honest about
	 * being a view over data already fetched, and adding a server-side filter
	 * would mean a second request shape and a second thing to keep in step.
	 *
	 * `null` rather than the string "semua" is the unfiltered state. A
	 * sentinel string would collide with the wire values, which are the
	 * database's, not the filter's.
	 */
	let selectedType: PublicFacilityType | null = null;

	/**
	 * Filter options, in frozen order: Semua, Puskesmas, Rumah Sakit.
	 *
	 * Built from the shared label map rather than hand-written, so a control
	 * can never render an empty or mismatched label. "Semua" is prepended as
	 * the null case — it is a view over both categories, not a third category.
	 */
	$: typeFilters = [
		{ value: null, label: 'Semua' },
		...PUBLIC_FACILITY_TYPES.map((value) => ({ value, label: FACILITY_TYPE_LABELS[value] }))
	];

	$: normalisedQuery = query.trim().toLowerCase();

	// The store exposes a getter function, not a value. Calling it keeps the
	// prop a plain boolean, which is all ErrorState is allowed to know about.
	$: hasSession = readHasSession();

	/**
	 * Search and type filter compose, by intersection rather than by precedence.
	 *
	 * Each predicate is applied to the same collection, so a result satisfies
	 * both when both are set. Writing it as two sequential filters would also
	 * compose, but the single pass makes the contract obvious: no code path
	 * can apply the query and skip the type, or the reverse.
	 *
	 * The type predicate uses the runtime guard rather than a bare comparison.
	 * TypeScript types describe what the backend promises, not what it sends,
	 * and this is fetched JSON — so an unrecognised value is a real possibility
	 * after a backend deploy lands ahead of a frontend release. Such a facility
	 * matches no specific filter, but still appears under "Semua" instead of
	 * being hidden, which would understate the catalog.
	 */
	$: filtered = catalog.facilities.filter((facility) => {
		if (selectedType !== null) {
			// The guard runs first so an unrecognised value is compared as
			// "not that type" rather than being coerced into a match.
			if (!isPublicFacilityType(facility.type)) return false;
			if (facility.type !== selectedType) return false;
		}
		if (!normalisedQuery) return true;
		return (
			facility.name.toLowerCase().includes(normalisedQuery) ||
			facility.short_code.toLowerCase().includes(normalisedQuery)
		);
	});

	/**
	 * Distinguishes "your filters matched nothing" from "there is nothing".
	 * Only meaningful once the catalog itself has loaded with rows.
	 */
	$: isNoResult =
		catalog.state === 'ready' &&
		catalog.facilities.length > 0 &&
		filtered.length === 0;

	/**
	 * Reset clears both inputs, not just the one that is visible.
	 *
	 * Leaving the type chip on "Rumah Sakit" while clearing the search would
	 * show a full catalog and call it filtered — the citizen would see results
	 * and have no visible way to understand why they are not everything.
	 * Restoring `null` puts the group back to its "Semua" default as well as
	 * clearing the field.
	 */
	function clearFilters() {
		query = '';
		selectedType = null;
	}

	/**
	 * True when anything is narrowing the catalog, which is what decides
	 * whether offering a reset makes sense. Checked against the real state
	 * rather than tracking a separate flag, so the affordance cannot drift out
	 * of step with the filters it is supposed to clear.
	 */
	$: hasActiveFilters = normalisedQuery.length > 0 || selectedType !== null;

	/**
	 * The selected type's label, for the no-result message.
	 *
	 * Read through the same shared map the chips render from, so the sentence
	 * "tidak ada faskes bertipe X" can never disagree with the chip the
	 * citizen actually pressed.
	 *
	 * Deliberately *not* named with a `$` prefix. In a Svelte template, `$foo`
	 * is the store subscription operator: `$activeTypeLabel` would be read as
	 * "subscribe to the store called `activeTypeLabel`", and a plain object has
	 * no `subscribe` method, so the page throws `store_invalid_shape` the
	 * moment the no-result branch renders. The `$` is not cosmetic here; it
	 * changes what the compiler emits.
	 */
	$: selectedTypeLabel = selectedType ? FACILITY_TYPE_LABELS[selectedType] : '';
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
		Search and type filter are hidden while loading and on error. Offering a
		filter over data that has not arrived would imply results exist, and
		offering it on an error screen would let a citizen filter an empty list
		and conclude the search is broken.
	-->
	{#if catalog.state === 'ready' && catalog.facilities.length > 0}
		<section class="sigap-faskes__search" aria-label="Cari dan saring faskes">
			<FacilitySearch value={query} onInput={(next) => (query = next)} />

			<!--
				The type chips, matching the frozen reference exactly: Semua,
				Puskesmas, Rumah Sakit, with "Semua" selected on arrival.

				`role="group"` with a name is what makes the set legible as one
				control rather than three loose buttons — a screen reader
				announces the group name, then each option, so "Puskesmas" is
				heard in the context of "saring berdasarkan tipe".

				`aria-pressed` is the right state here rather than
				`aria-selected`, because these are toggle buttons over one list
				and not tabs navigating between panels. It also states the
				selected state to assistive tech without depending on colour:
				the pressed attribute is the only thing a screen reader reports,
				so relying on the accent border alone would leave a sighted
				user as the only one who can tell what is active.
			-->
			<div
				class="sigap-faskes__type-group"
				role="group"
				aria-label="Saring berdasarkan tipe"
			>
				{#each typeFilters as filter (filter.label)}
					<button
						type="button"
						class="sigap-faskes__chip"
						style:border-radius={RADIUS.control}
						aria-pressed={selectedType === filter.value ? 'true' : 'false'}
						on:click={() => (selectedType = filter.value)}
					>
						{filter.label}
					</button>
				{/each}
			</div>
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
			<!--
				State 4: the filters excluded everything. Offer the way back.

				The line above names whichever filters are actually active, because
				the cause differs. Quoting an empty string ("Tidak ada hasil untuk
				""") when only the type chip is set would blame a search the
				citizen never typed, and send them off to shorten a query that does
				not exist. Each case names itself.
			-->
			<p class="sigap-faskes__count" role="status" aria-live="polite">
				{#if selectedType && normalisedQuery}
					Tidak ada hasil untuk “{query.trim()}” pada tipe {selectedTypeLabel}
				{:else if selectedType}
					Tidak ada faskes bertipe {selectedTypeLabel}
				{:else}
					Tidak ada hasil untuk “{query.trim()}”
				{/if}
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
				{#if hasActiveFilters}
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

			<!--
				Offered whenever anything is narrowed, not only when a query is
				typed. A citizen who tapped "Rumah Sakit" and got one result has
				no other way back to the other five, so the affordance has to
				appear for a type filter too. `hasActiveFilters` is the single
				source of truth for "is anything narrowed", shared with the
				count line, so the two cannot disagree.
			-->
			{#if hasActiveFilters}
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

	/*
		The chip row, transcribed from the frozen reference's `.sw-m-chip`
		container: a horizontal, single-line scroller. `overflow-x: auto` rather
		than a wrap, because at 390px three chips plus gaps are close to the
		viewport edge and wrapping would push the result count down a line for
		every citizen. It scrolls instead.
	*/
	.sigap-faskes__type-group {
		display: flex;
		gap: 8px;
		margin-top: 12px;
		overflow-x: auto;
		/* No visible scrollbar on a three-item control that always fits. */
		scrollbar-width: none;
	}

	.sigap-faskes__type-group::-webkit-scrollbar {
		display: none;
	}

	.sigap-faskes__chip {
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		/* Citizen touch floor, same as the search field. */
		height: 44px;
		padding: 0 14px;
		font-family: inherit;
		font-size: 13px;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		cursor: pointer;
	}

	/*
		Selected state.

		Weight and colour change together, not colour alone. The accent border
		is a 1px difference on a small control and is the first thing lost on a
		low-contrast display or a bright outdoor screen; the weight shift
		survives both, and `aria-pressed` carries the same information to
		assistive tech. Three redundant channels for one state is the point —
		none of them is sufficient on its own.
	*/
	.sigap-faskes__chip[aria-pressed='true'] {
		font-weight: 500;
		color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-faskes__chip:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-faskes__chip:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
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
