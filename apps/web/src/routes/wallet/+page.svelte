<script lang="ts">
	/**
	 * /wallet — Jejak Kunjungan.
	 *
	 * This route has no working data source. `GET /api/v1/medical-records` is
	 * not registered in the Go server and has no SvelteKit proxy, so the page
	 * cannot load a record and must not pretend otherwise.
	 *
	 * The previous implementation called that endpoint with a hardcoded demo
	 * phone number, rendered an Apple-Wallet-style card with a fabricated QR
	 * block, and asserted that a SHA-256 signature was "anti-tamper proof from
	 * the Rust Engine". None of that was true, and it put a patient lookup
	 * behind a phone number typed into a public page.
	 *
	 * What ships instead is the approved V2.2 frame: an honest empty state that
	 * tells the citizen the feature is not ready and routes them to the thing
	 * that does work — the live visit status. No fetch, no phone field, no
	 * fabricated record, no cryptographic claim.
	 */
	import PageHeader from '$lib/ui/PageHeader.svelte';
</script>

<svelte:head>
	<title>Jejak Kunjungan — Sigap</title>
	<meta
		name="description"
		content="Riwayat kunjungan Anda di Sigap. Fitur ini belum tersedia."
	/>
</svelte:head>

<div class="sigap-page sigap-wallet">
	<PageHeader
		title="Riwayat kunjungan"
		description="Riwayat kunjungan Anda di fasilitas kesehatan."
	/>

	<div class="sigap-wallet__unavailable" role="status">
		<h2 class="sigap-wallet__title">Riwayat kunjungan belum tersedia</h2>
		<p class="sigap-wallet__body">
			Fitur ini akan menampilkan riwayat kunjungan Anda nanti. Untuk kunjungan yang sedang
			berjalan, simpan <strong>kode check-in</strong> Anda — kode itulah yang membuka
			halaman status. <strong>Nomor antrean</strong> hanya menunjukkan posisi Anda di
			layar tunggu fasilitas, bukan kunci untuk memeriksa status.
		</p>
		<a class="sigap-wallet__link" href="/patient/status">Buka status kunjungan</a>
	</div>
</div>

<style>
	/*
	 * Every value below resolves from the frozen token source. The wallet page
	 * carries no palette of its own: the earlier version was the only route in
	 * the app with a dark theme, and a second theme is a second design system.
	 */
	/*
		Layout comes from `.sigap-page`. The Jejak Kunjungan frame runs across
		the full content column; the single unavailable block below keeps a
		bounded reading measure.
	*/

	.sigap-wallet__unavailable {
		max-width: 560px;
		padding: var(--sigap-space-6) var(--sigap-space-5);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
		text-align: center;
	}

	.sigap-wallet__title {
		margin: 0;
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		line-height: 1.4;
		color: var(--sigap-foreground);
	}

	.sigap-wallet__body {
		margin: var(--sigap-space-2) 0 0;
		font-size: var(--sigap-type-body);
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-wallet__link {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-4);
		margin-top: var(--sigap-space-5);
		font-size: var(--sigap-type-label);
		font-weight: 600;
		text-decoration: none;
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		border: 1px solid var(--sigap-primary);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-wallet__link:hover {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-wallet__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
