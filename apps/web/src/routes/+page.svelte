<script lang="ts">
	import { onDestroy } from 'svelte';
	import QuickActions from '$lib/citizen/QuickActions.svelte';
	import {
		loadPublicFacilities,
		type CatalogSnapshot
	} from '$lib/citizen/usePublicFacilities';
	import Skeleton from '$lib/ui/Skeleton.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import { facilityTypeLabel } from '$lib/citizen/facilityType';
	import { hasSession as readHasSession } from '$lib/stores/session';
	import { RADIUS } from '$lib/design/tokens';
	import { FolderSearch, CloudOff } from 'lucide-svelte';

	/**
	 * Beranda — the citizen landing page.
	 *
	 * Composition follows the frozen reference: a compact orientation header
	 * (not a hero), the primary action, secondary services, a short honest
	 * preview of the real facility catalog, and a three-step explainer.
	 *
	 * What is deliberately absent is the previous demo experience. There are no
	 * bed counts, no fabricated facilities, no distance or wait-time claims, no
	 * operational KPIs, and no mock map. A demo number on a civic health page
	 * is not a placeholder — it is a false claim about someone's local
	 * hospital, and a citizen has no way to tell it from a real one.
	 *
	 * The facility preview is a preview, labelled as such and capped, with a
	 * link to the full catalog. It shows only what the public API actually
	 * returns: name, short code, and the facility type enum.
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

	$: preview = catalog.facilities.slice(0, PREVIEW_LIMIT);
	$: hasMore = catalog.facilities.length > PREVIEW_LIMIT;

	// The store exposes a getter function, not a value. Calling it keeps the
	// prop a plain boolean, which is all ErrorState is allowed to know about.
	$: hasSession = readHasSession();

	onDestroy(unsubscribe);

	/** The three steps from the frozen "cara menggunakan Sigap" content. */
	const STEPS = [
		{
			title: 'Pilih faskes',
			body: 'Cari fasilitas kesehatan yang sesuai untuk Anda.'
		},
		{
			title: 'Buat janji atau ambil antrean',
			body: 'Simpan kode check-in atau nomor antrean Anda.'
		},
		{
			title: 'Pantau status',
			body: 'Lihat posisi antrean sampai Anda dilayani.'
		}
	];
</script>

<svelte:head>
	<title>Beranda — Sigap</title>
</svelte:head>

<div class="sigap-beranda">
	<!-- A. Compact orientation. A heading and one line, not a hero. -->
	<section class="sigap-beranda__intro" aria-labelledby="sigap-beranda-title">
		<h1 id="sigap-beranda-title" class="sigap-page-title">
			Apa yang bisa Anda lakukan di Sigap?
		</h1>
		<p class="sigap-page-subtitle">
			Pilih layanan untuk memulai: buat janji, datang langsung, atau pantau kunjungan Anda.
		</p>
	</section>

	<!-- B + C. Primary and secondary actions. -->
	<QuickActions />

	<!-- D. Real catalog preview. -->
	<section class="sigap-beranda__section" aria-labelledby="sigap-beranda-facilities">
		<div class="sigap-beranda__section-head">
			<h2 id="sigap-beranda-facilities" class="sigap-section-title">Faskes aktif</h2>
			<a class="sigap-inline-link" style:border-radius={RADIUS.control} href="/faskes">
				Lihat semua
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
				style:border-radius={RADIUS.panel}
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
		{:else if catalog.facilities.length === 0}
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
			<ul class="sigap-beranda__facility-list" style:border-radius={RADIUS.panel}>
				{#each preview as facility (facility.id)}
					<li class="sigap-beranda__facility">
						<div class="sigap-beranda__facility-head">
							<span class="sigap-beranda__facility-name">{facility.name}</span>
							{#if facility.short_code}
								<span class="sigap-code-badge" style:border-radius={RADIUS.control}>
									{facility.short_code}
								</span>
							{/if}
						</div>
						<!--
							The frozen reference puts the classification on its own
							line under the identity here too, so this restores the
							designed composition rather than adding a new element to
							it. The value is the real enum through the shared label map,
							so a facility named "RS Foo" that is registered as a
							Puskesmas reads as "Puskesmas", consistently with /faskes.
						-->
						<p class="sigap-beranda__facility-type">
							{facilityTypeLabel(facility.type)}
						</p>
					</li>
				{/each}
			</ul>

			{#if hasMore}
				<p class="sigap-beranda__more">
					Menampilkan {PREVIEW_LIMIT} dari {catalog.facilities.length} faskes.
				</p>
			{/if}
		{/if}
	</section>

	<!-- E. How it works. -->
	<section class="sigap-beranda__section" aria-labelledby="sigap-beranda-steps">
		<h2 id="sigap-beranda-steps" class="sigap-section-title">Cara menggunakan Sigap</h2>
		<ol class="sigap-steps" style:border-radius={RADIUS.panel}>
			{#each STEPS as step, index (step.title)}
				<li class="sigap-steps__item">
					<span class="sigap-steps__number" style:border-radius="9999px" aria-hidden="true">
						{index + 1}
					</span>
					<div class="sigap-steps__body">
						<h3 class="sigap-steps__title">{step.title}</h3>
						<p class="sigap-steps__text">{step.body}</p>
					</div>
				</li>
			{/each}
		</ol>
	</section>

	<!-- F. Support. -->
	<section class="sigap-beranda__section sigap-beranda__section--last">
		<p class="sigap-beranda__support">
			Butuh bantuan? Ikuti langkah panduan di atas, atau hubungi langsung faskes Anda.
		</p>
	</section>
</div>

<style>
	.sigap-beranda {
		max-width: 1024px;
		margin: 0 auto;
		padding: 20px 16px 8px;
	}

	@media (min-width: 1024px) {
		.sigap-beranda {
			padding: 32px 24px 8px;
		}
	}

	.sigap-beranda__section {
		margin-top: 24px;
	}

	.sigap-beranda__section--last {
		margin-top: 24px;
	}

	.sigap-beranda__section-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 12px;
	}

	.sigap-inline-link {
		display: inline-flex;
		align-items: center;
		height: 44px;
		padding: 0 4px;
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
		margin-top: 8px;
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-beranda__skeleton-row {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.sigap-beranda__skeleton-row + .sigap-beranda__skeleton-row {
		margin-top: 16px;
	}

	.sigap-beranda__facility-list {
		margin: 8px 0 0;
		padding: 0;
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-beranda__facility {
		padding: 12px 16px;
	}

	.sigap-beranda__facility + .sigap-beranda__facility {
		border-top: 1px solid var(--sigap-border);
	}

	.sigap-beranda__facility-head {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.sigap-beranda__facility-name {
		min-width: 0;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	/* Mirrors the catalog row: classification under the identity, muted. */
	.sigap-beranda__facility-type {
		margin: 4px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-beranda__more {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-code-badge {
		flex: none;
		display: inline-flex;
		align-items: center;
		height: 20px;
		padding: 0 6px;
		font-size: 11px;
		font-weight: 500;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-steps {
		margin: 12px 0 0;
		padding: 16px;
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-steps__item {
		position: relative;
		display: flex;
		gap: 12px;
	}

	.sigap-steps__item:not(:last-child) {
		padding-bottom: 16px;
	}

	/*
		The connector between step markers. Drawn with a pseudo-element rather
	 *than a border so it can start at the marker edge without affecting the
	 *flex layout.
	*/
	.sigap-steps__item:not(:last-child)::after {
		content: '';
		position: absolute;
		left: 13px;
		top: 36px;
		width: 1px;
		height: calc(100% - 36px);
		background-color: var(--sigap-border);
	}

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
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-steps__body {
		min-width: 0;
	}

	.sigap-steps__title {
		margin: 0;
		font-size: 13px;
		font-weight: 700;
		line-height: 1rem;
		color: var(--sigap-foreground);
	}

	.sigap-steps__text {
		margin: 2px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-beranda__support {
		margin: 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}
</style>
