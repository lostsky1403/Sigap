<script lang="ts">
	import { onDestroy } from 'svelte';
	import QuickActions from '$lib/citizen/QuickActions.svelte';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import { loadPublicFacilities, type CatalogSnapshot } from '$lib/citizen/usePublicFacilities';
	import Skeleton from '$lib/ui/Skeleton.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import {
		facilityTypeLabel,
		FACILITY_TYPE_LABELS,
		PUBLIC_FACILITY_TYPES,
		isPublicFacilityType
	} from '$lib/citizen/facilityType';
	import type { PublicFacilityType } from '$lib/api/types/api';
	import { hasSession as readHasSession } from '$lib/stores/session';
	import { ChevronRight, CloudOff, FolderSearch } from 'lucide-svelte';

	/**
	 * Beranda — the citizen landing page.
	 *
	 * Composition follows the approved V2.2 frame, top to bottom:
	 *
	 *   eyebrow / 2px rule / page title / lede   (the page device, `PageHeader`)
	 *   MULAI DARI SINI + "Pilih tugas Anda"     (the task picker, `QuickActions`)
	 *   FASKES AKTIF + "Fasilitas dalam katalog" (a real preview of the catalog)
	 *   CARA MENGGUNAKAN SIGAP + "Tiga langkah"  (the explainer)
	 *
	 * The 2px teal rule belongs to the page device only. Sections deliberately
	 * carry no rule: three more teal rules down one page turns the design's
	 * signature mark into wallpaper, and the reference has exactly one.
	 *
	 * What is deliberately absent is the previous demo experience. There are no
	 * bed counts, no fabricated facilities, no distance or wait-time claims, no
	 * operational KPIs, and no mock map. A demo number on a civic health page
	 * is not a placeholder — it is a false claim about someone's local
	 * hospital, and a citizen has no way to tell it from a real one.
	 *
	 * The preview shows only what the public API actually returns: name, short
	 * code, and the facility type enum.
	 */

	const catalogLoader = loadPublicFacilities();

	/**
	 * Subscribe explicitly rather than trusting a getter to be reactive.
	 *
	 * The loader mutates its state from an async fetch. Svelte only re-runs a
	 * `$:` block when a tracked dependency changes, and reading `catalog.state`
	 * inside one does not create that dependency — the page would request
	 * successfully and then render its loading state forever. Subscribing makes
	 * the snapshot an actual reactive input, and the teardown is what stops the
	 * component holding a listener after it unmounts.
	 */
	let catalog: CatalogSnapshot = { state: 'loading', facilities: [], error: null };
	const unsubscribe = catalogLoader.unsubscribe((next) => (catalog = next));

	/**
	 * The retry action comes from the loader, not from the snapshot.
	 *
	 * A snapshot is data — state, rows, error — and carries no behaviour, so
	 * `catalog.reload` is `undefined`. ErrorState reads a missing callback as
	 * "nothing to retry" and drops the button without complaint.
	 */
	const reload = catalogLoader.reload;

	/**
	 * Four rows is a preview, not a truncated catalog. The full list lives at
	 * /faskes, and the count line is honest about which one you are looking at.
	 */
	const PREVIEW_LIMIT = 4;

	/**
	 * The preview's own type filter — null is "Semua", mirroring /faskes. A
	 * sentinel string would collide with the wire enum values.
	 */
	let selectedPreviewType: PublicFacilityType | null = null;

	/**
	 * Sorted here, not trusted from the wire.
	 *
	 * The public endpoint does `ORDER BY name`, so this is a no-op against the
	 * real API — but the page's own copy claims the order ("menurut nama"), and
	 * a claim the page cannot enforce is a claim that goes stale the moment the
	 * query changes. Sorting four rows costs nothing and makes the sentence
	 * true by construction.
	 *
	 * `localeCompare` with the Indonesian collation, so "Puskesmas Melati
	 * Indah" sorts before "Puskesmas Sukajaya" the way a reader expects rather
	 * than by code point.
	 */
	$: sorted = [...catalog.facilities].sort((a, b) => a.name.localeCompare(b.name, 'id'));
	$: filteredSorted =
		selectedPreviewType === null
			? sorted
			: sorted.filter(
					(facility) =>
						isPublicFacilityType(facility.type) && facility.type === selectedPreviewType
				);
	$: preview = filteredSorted.slice(0, PREVIEW_LIMIT);
	$: hasMore = filteredSorted.length > PREVIEW_LIMIT;

	/** Filter chips in frozen order: Semua, Puskesmas, Rumah Sakit. */
	$: previewTypeFilters = [
		{ value: null, label: 'Semua' },
		...PUBLIC_FACILITY_TYPES.map((value) => ({ value, label: FACILITY_TYPE_LABELS[value] }))
	];
	$: selectedPreviewTypeLabel = selectedPreviewType ? FACILITY_TYPE_LABELS[selectedPreviewType] : '';

	/**
	 * Two strings per slot, because one of them is a claim about the list.
	 *
	 * "Empat faskes pertama" is only true when the catalog is longer than the
	 * preview. On a three-facility catalog it is a small lie about how much of
	 * the list you are looking at, which is exactly the class of statement this
	 * page is not allowed to make.
	 */
	$: previewDescription =
		selectedPreviewType !== null
			? `Faskes bertipe ${selectedPreviewTypeLabel} dari katalog publik yang aktif.`
			: hasMore
				? `Empat faskes pertama menurut nama, dari katalog publik yang aktif.`
				: `Seluruh faskes aktif, dari katalog publik.`;
	$: previewNote =
		selectedPreviewType !== null
			? `Menampilkan ${preview.length} faskes bertipe ${selectedPreviewTypeLabel}`
			: hasMore
				? `Menampilkan ${PREVIEW_LIMIT} faskes pertama (diurutkan menurut nama)`
				: `Menampilkan ${preview.length} faskes (diurutkan menurut nama)`;

	// The store exposes a getter function, not a value. Calling it keeps the
	// prop a plain boolean, which is all ErrorState is allowed to know about.
	$: hasSession = readHasSession();

	onDestroy(unsubscribe);

	/** The three steps, in the approved copy. */
	const STEPS = [
		{
			title: 'Pilih fasilitas',
			body: 'Cari faskes berdasarkan nama atau jenisnya.'
		},
		{
			title: 'Buat janji atau ambil antrean',
			body: 'Pilih layanan dan waktu, atau datang langsung.'
		},
		{
			title: 'Pantau status kunjungan',
			body: 'Simpan kode check-in Anda untuk melihat status kunjungan.'
		}
	];
</script>

<svelte:head>
	<title>Beranda — Sigap</title>
</svelte:head>

<div class="sigap-page sigap-page--roomy sigap-beranda">
	<PageHeader
		eyebrow="Layanan kesehatan warga"
		title="Cari faskes, buat janji temu, dan pantau kunjungan Anda"
		description="Satu layanan untuk menemukan fasilitas kesehatan, membuat janji temu, check-in, dan memantau status kunjungan."
	/>

	<QuickActions />

	<section aria-labelledby="sigap-beranda-facilities">
		<div class="sigap-beranda__section-head">
			<div class="sigap-beranda__section-intro">
				<p class="sigap-eyebrow">Faskes aktif</p>
				<h2 id="sigap-beranda-facilities" class="sigap-section-title">
					Fasilitas dalam katalog
				</h2>
				<p class="sigap-section-desc">{previewDescription}</p>
			</div>

			<a class="sigap-inline-link" href="/faskes">
				Lihat semua fasilitas
				<Icon icon={ChevronRight} size={14} />
			</a>
		</div>

		{#if catalog.state === 'loading'}
			<!--
				Skeletons are decorative and hidden from assistive tech; the
				surrounding region carries the busy state so "still loading" is
				distinguishable from "no facilities".
			-->
			<div
				class="sigap-beranda__skeletons"
				role="status"
				aria-busy="true"
				aria-live="polite"
			>
				<span class="sigap-visually-hidden">Memuat daftar faskes</span>
				{#each Array.from({ length: PREVIEW_LIMIT }) as _, index (index)}
					<div class="sigap-beranda__skeleton-row">
						<Skeleton width="55%" height="14px" />
						<Skeleton width="30%" height="12px" />
					</div>
				{/each}
			</div>
		{:else if catalog.state === 'error' && catalog.error}
			<ErrorState
				error={catalog.error}
				{hasSession}
				icon={CloudOff}
				onRetry={reload}
				retryLabel="Muat ulang"
			/>
		{:else if sorted.length === 0}
			<!--
				Empty is a real operational state, not an error, and the frozen copy
				names the cause: faskes appear once an operator activates them.
				Saying "no results" here would blame the citizen's filters for an
				empty catalog.
			-->
			<EmptyState
				title="Belum ada data faskes"
				description="Daftar fasilitas kesehatan masih kosong. Faskes akan tampil di sini setelah diaktifkan oleh pengelola."
				icon={FolderSearch}
				actionLabel="Muat ulang"
				onAction={reload}
			/>
		{:else}
			<div
				class="sigap-beranda__filter"
				role="group"
				aria-label="Saring pratinjau berdasarkan tipe"
			>
				{#each previewTypeFilters as ft (ft.label)}
					<button
						type="button"
						class="sigap-beranda__filter-chip"
						aria-pressed={selectedPreviewType === ft.value ? 'true' : 'false'}
						on:click={() => (selectedPreviewType = ft.value)}
					>
						{ft.label}
					</button>
				{/each}
			</div>

			{#if filteredSorted.length === 0}
				<p class="sigap-beranda__filter-empty">
					Tidak ada faskes bertipe {selectedPreviewTypeLabel} dalam katalog.
				</p>
			{:else}
				<ul class="sigap-beranda__facility-list">
					{#each preview as facility (facility.id)}
						<!--
							The whole row is the target, as the reference draws it: a
							chevron, not a labelled sub-button. The hit area is the
							anchor, so it is 56px tall at every viewport rather than a
							small link inside a static row.
						-->
						<li>
							<a
								class="sigap-beranda__facility"
								href={`/appointments/new?facility_id=${encodeURIComponent(facility.id)}`}
							>
								<span class="sigap-beranda__facility-text">
									<span class="sigap-beranda__facility-name">{facility.name}</span>
									<!--
										The real enum through the shared label map, so a
										facility named "RS Foo" registered as a Puskesmas
										reads as "Puskesmas" here exactly as on /faskes.
									-->
									<span class="sigap-beranda__facility-type">
										{facilityTypeLabel(facility.type)}
									</span>
								</span>

								{#if facility.short_code}
									<span class="sigap-beranda__facility-code sigap-identifier">
										{facility.short_code}
									</span>
								{/if}

								<span class="sigap-beranda__facility-chevron" aria-hidden="true">
									<Icon icon={ChevronRight} size={16} />
								</span>
							</a>
						</li>
					{/each}
				</ul>

				<p class="sigap-beranda__more">{previewNote}</p>
			{/if}
		{/if}
	</section>

	<section aria-labelledby="sigap-beranda-steps">
		<div class="sigap-beranda__section-intro">
			<p class="sigap-eyebrow">Cara menggunakan Sigap</p>
			<h2 id="sigap-beranda-steps" class="sigap-section-title">Tiga langkah</h2>
		</div>

		<ol class="sigap-steps">
			{#each STEPS as step, index (step.title)}
				<li class="sigap-steps__item">
					<span class="sigap-steps__number" aria-hidden="true">{index + 1}</span>
					<div class="sigap-steps__body">
						<h3 class="sigap-steps__title">{step.title}</h3>
						<p class="sigap-steps__text">{step.body}</p>
					</div>
				</li>
			{/each}
		</ol>
	</section>
</div>

<style>
	/*
		Beranda is the one page whose `Body` gap differs: the Pen runs 64px
		between sections on desktop and 32px on a phone, against the 24px every
		other citizen page uses. Layout, width and the top offset all come from
		`.sigap-page--roomy`.
	*/
	.sigap-beranda {
		--sigap-page-gap: var(--sigap-space-6);
		/*
			Beranda's own type scale, and it is deliberately smaller than the
			other citizen pages. The Pen draws this hero at 22px/600 on a phone
			and 24px/700 on a desktop, where Faskes, Status, Check-In, booking and
			walk-in all open at 32px/600.

			That is not an inconsistency to normalise away: Beranda is a hub, and
			the frames treat it as a compact orientation header over a set of
			sections that each carry a 24px/700 title of their own. Matching the
			task pages here would make the hero compete with the section headings
			underneath it.
		*/
		--sigap-page-title-size: 22px;
		--sigap-page-title-weight: 600;
		--sigap-page-title-size-lg: 24px;
		--sigap-page-title-weight-lg: 700;
		/* The section headings take the same 24px/700 the hero does. */
		--sigap-section-title-size: 18px;
		--sigap-section-title-size-lg: 24px;
		--sigap-section-title-weight-lg: 700;
	}

	@media (min-width: 1024px) {
		.sigap-beranda {
			--sigap-page-gap: var(--sigap-space-8);
		}
	}

	/*
		The heading block and the "Lihat semua" link share a row, and the link
		sits on the heading's baseline rather than the description's. Aligning it
		to the block's top would float it beside the eyebrow, which reads as a
		second heading.
	*/
	.sigap-beranda__section-head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--sigap-space-3);
	}

	.sigap-beranda__section-intro {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-1);
		min-width: 0;
	}

	.sigap-inline-link {
		flex: none;
		display: inline-flex;
		align-items: center;
		gap: var(--sigap-space-1);
		/* Citizen touch floor, so the link is a target and not a word. */
		min-height: 44px;
		font-size: 13px;
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-inline-link:hover {
		text-decoration: underline;
	}

	.sigap-inline-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-beranda__skeletons {
		margin-top: var(--sigap-space-3);
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-beranda__skeleton-row {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-2);
	}

	.sigap-beranda__skeleton-row + .sigap-beranda__skeleton-row {
		margin-top: var(--sigap-space-4);
	}

	/*
		Rules, not a box. The rows are the controls and each draws its own
		hairline; a panel outline around them would add a second, competing
		edge at the same weight.
	*/
	.sigap-beranda__facility-list {
		margin: var(--sigap-space-3) 0 0;
		padding: 0;
		list-style: none;
		border-top: 1px solid var(--sigap-border-subtle);
	}

	.sigap-beranda__facility-list > li {
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-beranda__facility {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-3);
		/* 56px: comfortably past the 44px citizen touch target. */
		min-height: 56px;
		padding: var(--sigap-space-2) var(--sigap-space-1);
		text-decoration: none;
	}

	.sigap-beranda__facility:hover {
		background-color: var(--sigap-surface);
	}

	.sigap-beranda__facility:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-beranda__facility-text {
		flex: 1 1 auto;
		min-width: 0;
	}

	.sigap-beranda__facility-name {
		display: block;
		font-size: 15px;
		font-weight: 500;
		color: var(--sigap-foreground);
		/* Long facility names wrap instead of overflowing at 390px. */
		overflow-wrap: anywhere;
	}

	/* Mirrors the catalog row: classification under the identity, muted. */
	.sigap-beranda__facility-type {
		display: block;
		margin-top: 2px;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	/*
		Static, non-interactive chip. The row's own hit area is the anchor, so
		this is a label, not a control — hence the subtle edge, not the 4.16:1
		one. It is monospace and tabular so "PMI" and "RSK" align down the list.
	*/
	.sigap-beranda__facility-code {
		flex: none;
		display: inline-flex;
		align-items: center;
		height: 20px;
		padding: 0 var(--sigap-space-2);
		font-size: 11px;
		font-weight: 500;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-beranda__facility-chevron {
		flex: none;
		display: flex;
		color: var(--sigap-muted);
	}

	.sigap-beranda__more {
		margin: var(--sigap-space-2) 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-steps {
		margin: var(--sigap-space-3) 0 0;
		padding: var(--sigap-space-4);
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-steps__item {
		position: relative;
		display: flex;
		gap: var(--sigap-space-3);
	}

	.sigap-steps__item:not(:last-child) {
		padding-bottom: var(--sigap-space-4);
	}

	/*
		The connector between step markers. Drawn with a pseudo-element rather
		than a border so it can start at the marker's edge without taking part
		in the flex layout.
	*/
	.sigap-steps__item:not(:last-child)::after {
		content: '';
		position: absolute;
		left: 13px;
		top: 32px;
		width: 1px;
		height: calc(100% - 32px);
		background-color: var(--sigap-border-subtle);
	}

	/*
		Not a control: nothing here is operable. The ordinal survives on the
		number's own colour, so the square is decoration and stays subtle.
	*/
	.sigap-steps__number {
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		font-size: 13px;
		font-weight: 600;
		color: var(--sigap-primary);
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
		/* 50% keeps the marker a circle without an off-scale literal radius. */
		border-radius: 50%;
	}

	.sigap-steps__body {
		min-width: 0;
	}

	.sigap-steps__title {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		line-height: 1.25rem;
		color: var(--sigap-foreground);
	}

	.sigap-steps__text {
		margin: 2px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	/*
		The preview's type filter, the same three chips as /faskes, in the same
		full-height citizen control. They sit inside the section rather than
		above the results, but the control itself stays identical so a chip that
		means "Puskesmas" here taps the same as it does on the discovery page.
	*/
	.sigap-beranda__filter {
		display: flex;
		flex-wrap: wrap;
		gap: var(--sigap-space-2);
		margin-top: var(--sigap-space-3);
	}

	.sigap-beranda__filter-chip {
		flex: none;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-3);
		font-family: inherit;
		font-size: 12px;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-radius: var(--sigap-radius-control);
		cursor: pointer;
	}

	.sigap-beranda__filter-chip[aria-pressed='true'] {
		font-weight: 500;
		color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-beranda__filter-chip:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-beranda__filter-chip:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-beranda__filter-empty {
		margin: var(--sigap-space-3) 0 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}
</style>
