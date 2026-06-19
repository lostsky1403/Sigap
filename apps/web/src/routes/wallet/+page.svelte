<script lang="ts">
	// /wallet — Dompet Jejak Medis (Immutable Health Records)
	// Cards with visit details + cryptographic SHA-256 signature (from Rust engine) as anti-tamper proof.
	// Uses emerald-600 accents, clean Tailwind, monospace for sig.

	let phone = $state('081234567890');
	let records = $state<MedicalRecord[]>([]);
	let loading = $state(false);
	let err = $state('');

	async function loadWallet() {
		loading = true;
		err = '';
		try {
			const res = await fetch(`/api/v1/medical-records?phone=${encodeURIComponent(phone)}`);
			const json = await res.json();
			if (json.success && json.data) {
				records = json.data;
			} else {
				err = json.error || 'Gagal memuat dompet.';
			}
		} catch (e) {
			err = 'Tidak dapat menghubungi API.';
		} finally {
			loading = false;
		}
	}

	// auto load demo on mount
	import { onMount } from 'svelte';
	import type { MedicalRecord } from '$lib/types';
	onMount(() => {
		loadWallet();
	});
</script>

<div class="px-6 py-8 max-w-3xl mx-auto">
	<div class="mb-8">
		<h1 class="text-3xl font-semibold tracking-[-0.02em]">Dompet Jejak Medis</h1>
		<p class="mt-2 text-slate-600 dark:text-slate-400">Riwayat kunjungan & antrean Anda. Setiap entri dilindungi signature kriptografi SHA-256 (dihasilkan di Rust Engine) sebagai bukti immutable / anti-ubah.</p>
	</div>

	<div class="mb-4 flex gap-2">
		<input
			type="tel"
			bind:value={phone}
			placeholder="Nomor HP pasien"
			class="flex-1 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm focus:border-emerald-600 dark:bg-slate-950 dark:border-slate-700"
		/>
		<button
			onclick={loadWallet}
			disabled={loading}
			class="rounded-lg bg-emerald-600 px-5 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60"
		>
			{loading ? 'Memuat...' : 'Muat Dompet'}
		</button>
	</div>

	{#if err}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{err}</div>
	{/if}

	{#if records.length === 0 && !loading}
		<p class="text-slate-500 dark:text-slate-400 text-sm">Belum ada riwayat. Lakukan antrean untuk mengisi dompet (signature dibuat otomatis di Rust).</p>
	{/if}

	<div class="space-y-4">
		{#each records as rec}
			<!-- Apple Wallet style per Stitch Sigap design: high-contrast Slate header, perforated line, QR, punched 12px corners, technical metadata -->
			<div class="rounded-[12px] border border-slate-300 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-950 overflow-hidden" style="box-shadow: 0 6px 8px #00000024;">
				<!-- Slate header -->
				<div class="bg-slate-900 text-white p-3">
					<div class="flex justify-between items-center">
						<div>
							<div class="text-[10px] font-medium uppercase tracking-[1px] text-slate-400">{rec.facility_name}</div>
							<div class="font-mono text-2xl font-semibold tracking-[-0.02em]">{rec.formatted_number}</div>
						</div>
						<!-- QR placeholder -->
						<div class="w-10 h-10 border border-white/70 bg-white/10 flex items-center justify-center rounded text-[7px] font-mono text-white/60">QR</div>
					</div>
				</div>
				
				<!-- Perforated separator -->
				<div class="border-t border-dashed border-slate-300 dark:border-slate-600 mx-3"></div>
				
				<div class="p-3 text-xs">
					<div class="text-slate-500 dark:text-slate-400">{new Date(rec.visit_time).toLocaleString('id-ID')}</div>
					
					<div class="mt-2 pt-2 border-t border-dashed border-slate-200 dark:border-slate-700">
						<div class="text-[9px] uppercase tracking-[1px] text-emerald-600 dark:text-emerald-400">Hash Signature (SHA-256 — Immutable Proof)</div>
						<div class="mt-0.5 font-mono text-[10px] break-all text-slate-700 dark:text-slate-300 bg-emerald-50 dark:bg-emerald-950/40 p-1.5 rounded">
							{rec.signature}
						</div>
						<div class="mt-0.5 text-[8px] text-emerald-500/70">Bukti kriptografi dari Rust Engine — rekam medis anti-ubah.</div>
					</div>
				</div>
			</div>
		{/each}
	</div>

	<p class="mt-8 text-center text-[10px] text-slate-400 dark:text-slate-600">Sigap Super App • Health Wallet • Powered by Rust SHA-256 + Postgres</p>
</div>
