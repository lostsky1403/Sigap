<script lang="ts">
	// Sigap Bed Availability Dashboard
	// Strict minimalist design: generous whitespace, single emerald accent,
	// clean typography, high contrast, no decorative noise.
	// Svelte 5 runes for reactivity.

	type Facility = {
		id: string;
		name: string;
		type: 'rumah_sakit' | 'puskesmas';
		kecamatan: string;
		kabupatenKota: string;
		totalBeds: number;
		availableBeds: number;
		lastUpdated: string;
		shortCode: string;
	};

	const samples: Facility[] = [
		{ id: 'f1', name: 'RSUD Kota Sehat', type: 'rumah_sakit', kecamatan: 'Sukamaju', kabupatenKota: 'Kota Bandung', totalBeds: 180, availableBeds: 42, lastUpdated: '2026-06-12T08:15:00Z', shortCode: 'RSK' },
		{ id: 'f2', name: 'Puskesmas Sukajaya', type: 'puskesmas', kecamatan: 'Sukajaya', kabupatenKota: 'Kab. Bandung', totalBeds: 28, availableBeds: 19, lastUpdated: '2026-06-12T07:50:00Z', shortCode: 'PKM' },
		{ id: 'f3', name: 'RS Mitra Sehat', type: 'rumah_sakit', kecamatan: 'Menteng', kabupatenKota: 'Jakarta Pusat', totalBeds: 95, availableBeds: 11, lastUpdated: '2026-06-12T09:05:00Z', shortCode: 'RSM' },
		{ id: 'f4', name: 'Puskesmas Melati Indah', type: 'puskesmas', kecamatan: 'Cilandak', kabupatenKota: 'Jakarta Selatan', totalBeds: 35, availableBeds: 27, lastUpdated: '2026-06-12T06:40:00Z', shortCode: 'PMI' },
		{ id: 'f5', name: 'RSUD Sejahtera', type: 'rumah_sakit', kecamatan: 'Cibadak', kabupatenKota: 'Kab. Sukabumi', totalBeds: 120, availableBeds: 68, lastUpdated: '2026-06-12T08:55:00Z', shortCode: 'RSJ' },
		{ id: 'f6', name: 'Puskesmas Harapan Baru', type: 'puskesmas', kecamatan: 'Parung', kabupatenKota: 'Kab. Bogor', totalBeds: 22, availableBeds: 5, lastUpdated: '2026-06-12T07:10:00Z', shortCode: 'PHB' }
	];

	let search = $state('');
	let typeFilter = $state<'all' | 'rumah_sakit' | 'puskesmas'>('all');
	let sortMode = $state<'availability' | 'name'>('availability');

	function calcOccupancy(available: number, total: number): number {
		if (total <= 0) return 0;
		return Math.round(((total - available) / total) * 100);
	}

	const filtered = $derived(
		samples
			.filter((f) => {
				const q = search.toLowerCase().trim();
				const matchesSearch = !q || f.name.toLowerCase().includes(q) || f.kecamatan.toLowerCase().includes(q);
				const matchesType = typeFilter === 'all' || f.type === typeFilter;
				return matchesSearch && matchesType;
			})
			.sort((a, b) => {
				if (sortMode === 'name') return a.name.localeCompare(b.name, 'id');
				// More available (lower occupancy) first for usefulness
				return calcOccupancy(a.availableBeds, a.totalBeds) - calcOccupancy(b.availableBeds, b.totalBeds);
			})
	);

	function formatTime(iso: string) {
		return new Date(iso).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
	}
</script>

<section class="px-6 py-8">
	<div class="mb-8">
		<h2 class="text-2xl font-semibold tracking-[-0.015em]">Dashboard Ketersediaan Kasur</h2>
		<p class="mt-1 text-sm text-slate-600 dark:text-slate-400">Data contoh • Perbarui untuk data real-time</p>
	</div>

	<!-- Filters — clean, legible, generous -->
	<div class="mb-6 flex flex-col sm:flex-row gap-3">
		<input
			type="text"
			bind:value={search}
			placeholder="Cari nama fasilitas atau kecamatan..."
			class="flex-1 rounded-lg border border-slate-200 bg-white px-4 py-2.5 text-sm placeholder:text-slate-400 focus:outline-none focus:border-emerald-600 dark:border-slate-800 dark:bg-slate-950 dark:placeholder:text-slate-500"
		/>

		<select
			bind:value={typeFilter}
			class="rounded-lg border border-slate-200 bg-white px-3 py-2.5 text-sm dark:border-slate-800 dark:bg-slate-950"
		>
			<option value="all">Semua Fasilitas</option>
			<option value="rumah_sakit">Rumah Sakit</option>
			<option value="puskesmas">Puskesmas</option>
		</select>

		<select
			bind:value={sortMode}
			class="rounded-lg border border-slate-200 bg-white px-3 py-2.5 text-sm dark:border-slate-800 dark:bg-slate-950"
		>
			<option value="availability">Urutkan: Ketersediaan</option>
			<option value="name">Urutkan: Nama</option>
		</select>
	</div>

	<!-- Results — calm cards, excellent internal spacing -->
	<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
		{#each filtered as facility (facility.id)}
			{@const occupancy = calcOccupancy(facility.availableBeds, facility.totalBeds)}
			<div class="rounded-xl border border-slate-200 bg-white p-6 dark:border-slate-800 dark:bg-slate-950">
				<div class="flex items-start justify-between gap-4">
					<div>
						<div class="font-semibold text-[17px] tracking-[-0.01em] text-slate-900 dark:text-white leading-tight">
							{facility.name}
						</div>
						<div class="mt-0.5 text-[10px] font-medium uppercase tracking-[1px] text-emerald-600 dark:text-emerald-500">
							{facility.type === 'rumah_sakit' ? 'Rumah Sakit' : 'Puskesmas'}
						</div>
					</div>
					<div class="font-mono text-xs text-slate-400 dark:text-slate-600 pt-0.5">{facility.shortCode}</div>
				</div>

				<div class="mt-1 text-sm text-slate-600 dark:text-slate-400">
					{facility.kecamatan}, {facility.kabupatenKota}
				</div>

				<div class="mt-6">
					<div class="flex items-baseline justify-between text-sm">
						<span class="text-slate-600 dark:text-slate-400">Tersedia</span>
						<span class="font-semibold tabular-nums text-xl text-slate-950 dark:text-white">
							{facility.availableBeds}
							<span class="text-sm font-normal text-slate-400">/ {facility.totalBeds}</span>
						</span>
					</div>

					<!-- Thin, calm progress bar -->
					<div class="mt-2 h-[5px] w-full rounded-full bg-slate-100 dark:bg-slate-800 overflow-hidden">
						<div
							class="h-[5px] bg-emerald-600 transition-[width] duration-200 ease-out"
							style="width: {100 - occupancy}%"
						></div>
					</div>
					<div class="mt-1 text-right text-xs tabular-nums text-slate-500 dark:text-slate-400">
						{occupancy}% terisi
					</div>
				</div>

				<div class="mt-6 flex items-center justify-between text-xs">
					<span class="text-slate-500 dark:text-slate-400">Update {formatTime(facility.lastUpdated)}</span>
					<button
						onclick={() => alert('Fitur daftar antrean & integrasi API akan hadir di iterasi berikutnya')}
						class="font-medium text-emerald-600 hover:underline dark:text-emerald-500"
					>
						Lihat antrean →
					</button>
				</div>
			</div>
		{/each}

		{#if filtered.length === 0}
			<p class="col-span-full text-sm text-slate-500 py-8 text-center">Tidak ada fasilitas yang cocok dengan filter.</p>
		{/if}
	</div>

	<p class="mt-8 text-center text-[10px] text-slate-400 dark:text-slate-600">
		Contoh data untuk scaffolding awal Sigap. Integrasikan dengan Go API + Rust engine untuk produksi.
	</p>
</section>
