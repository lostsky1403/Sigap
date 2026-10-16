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
		description="Riwayat kunjungan Anda akan tampil di sini setelah fitur ini tersedia. Saat ini kami belum bisa menampilkan daftar kunjungan sebelumnya."
	/>

	<div class="sigap-wallet__notice" role="status">
		<p class="sigap-wallet__notice-text">
			Kami belum bisa menampilkan riwayat kunjungan. Simpan nomor antrean dan kode check-in Anda;
			status kunjungan yang sedang berjalan tetap bisa dilihat kapan saja.
		</p>
	</div>

	<div class="sigap-wallet__panel">
		<h2 class="sigap-wallet__panel-title">Riwayat kunjungan belum tersedia</h2>
		<p class="sigap-wallet__panel-body">
			Belum ada riwayat yang bisa ditampilkan. Kami sedang menyiapkannya.
		</p>

		<ul class="sigap-wallet__facts">
			<li>Status kunjungan yang sedang berjalan tetap bisa Anda lihat kapan saja.</li>
			<li>Simpan nomor antrean dan kode check-in Anda sampai fitur riwayat tersedia.</li>
		</ul>

		<div class="sigap-wallet__actions">
			<a class="sigap-wallet__action sigap-wallet__action--primary" href="/patient/status">
				Buka status kunjungan
			</a>
			<a class="sigap-wallet__action" href="/">Kembali ke beranda</a>
		</div>
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
		the full content column, so the page is not capped; the notice and the
		panel below keep their own reading measures.
	*/

	.sigap-wallet__notice {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-warning-tint);
		border: 1px solid var(--sigap-warning);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-wallet__notice-text {
		margin: 0;
		font-size: 13px;
		font-weight: 600;
		line-height: 1.5;
		color: var(--sigap-foreground);
	}

	.sigap-wallet__panel {
		padding: var(--sigap-space-6) var(--sigap-space-5);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
		text-align: center;
	}

	.sigap-wallet__panel-title {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		line-height: 1.4;
		color: var(--sigap-foreground);
	}

	.sigap-wallet__panel-body {
		margin: var(--sigap-space-2) 0 0;
		font-size: 13px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-wallet__facts {
		margin: var(--sigap-space-4) 0 0;
		padding: var(--sigap-space-3) var(--sigap-space-4);
		list-style: none;
		text-align: left;
		background-color: var(--sigap-surface-sunken);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-wallet__facts li {
		font-size: 12px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-wallet__facts li + li {
		margin-top: var(--sigap-space-2);
	}

	.sigap-wallet__actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--sigap-space-3);
		margin-top: var(--sigap-space-5);
	}

	.sigap-wallet__action {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-4);
		font-size: 13px;
		font-weight: 600;
		text-decoration: none;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-wallet__action--primary {
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-wallet__action:hover {
		background-color: var(--sigap-surface-sunken);
	}

	.sigap-wallet__action--primary:hover {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-wallet__action:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
