<script lang="ts">
	import BedAvailabilityDashboard from '$lib/components/dashboard/BedAvailabilityDashboard.svelte';
</script>

<div class="px-6 py-12">
	<div class="max-w-2xl">
		<h1 class="text-4xl font-semibold tracking-[-0.02em] text-balance">
			Selamat datang di Sigap
		</h1>
		<p class="mt-4 text-lg text-slate-600 dark:text-slate-400">
			Platform informasi dan antrean kesehatan daerah. Cepat, transparan, dan mudah diakses.
		</p>
	</div>

	<div class="mt-12">
		<BedAvailabilityDashboard />
	</div>

	<div class="mt-10 text-xs text-slate-500">
		Info bed dan antrean diperbarui secara langsung, sehingga pasien dan petugas selalu melihat status terkini.
	</div>
</div>
