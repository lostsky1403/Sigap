<script lang="ts">
	import { onMount } from 'svelte';

	// Sigap Bed Availability Dashboard
	// Strict minimalist design: generous whitespace, single emerald accent,
	// clean typography, high contrast, no decorative noise.
	// Svelte 5 runes for reactivity.
	// Now with real-time updates via SSE from Go API (triggered by Rust engine queue creation).

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

	let samples: Facility[] = $state([
		{ id: 'f1', name: 'RSUD Kota Sehat', type: 'rumah_sakit', kecamatan: 'Sukamaju', kabupatenKota: 'Kota Bandung', totalBeds: 180, availableBeds: 42, lastUpdated: '2026-06-12T08:15:00Z', shortCode: 'RSK' },
		{ id: 'f2', name: 'Puskesmas Sukajaya', type: 'puskesmas', kecamatan: 'Sukajaya', kabupatenKota: 'Kab. Bandung', totalBeds: 28, availableBeds: 19, lastUpdated: '2026-06-12T07:50:00Z', shortCode: 'PKM' },
		{ id: 'f3', name: 'RS Mitra Sehat', type: 'rumah_sakit', kecamatan: 'Menteng', kabupatenKota: 'Jakarta Pusat', totalBeds: 95, availableBeds: 11, lastUpdated: '2026-06-12T09:05:00Z', shortCode: 'RSM' },
		{ id: 'f4', name: 'Puskesmas Melati Indah', type: 'puskesmas', kecamatan: 'Cilandak', kabupatenKota: 'Jakarta Selatan', totalBeds: 35, availableBeds: 27, lastUpdated: '2026-06-12T06:40:00Z', shortCode: 'PMI' },
		{ id: 'f5', name: 'RSUD Sejahtera', type: 'rumah_sakit', kecamatan: 'Cibadak', kabupatenKota: 'Kab. Sukabumi', totalBeds: 120, availableBeds: 68, lastUpdated: '2026-06-12T08:55:00Z', shortCode: 'RSJ' },
		{ id: 'f6', name: 'Puskesmas Harapan Baru', type: 'puskesmas', kecamatan: 'Parung', kabupatenKota: 'Kab. Bogor', totalBeds: 22, availableBeds: 5, lastUpdated: '2026-06-12T07:10:00Z', shortCode: 'PHB' }
	]);

	let search = $state('');
	let typeFilter = $state<'all' | 'rumah_sakit' | 'puskesmas'>('all');
	let sortMode = $state<'availability' | 'name'>('availability');

	// Queue form state (wired to real backend)
	let selectedFacility = $state<Facility | null>(null);
	let phone = $state('');
	let fullNameForForm = $state('Pengunjung');
	let submitting = $state(false);
	let ticket = $state<null | { nomorAntrean: string; processingTime: string; facilityName: string; phone: string }>(null);
	let error = $state('');

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

	// Submit to Go API (POST /api/v1/queues/generate) -> Rust engine for ticket + µs latency
	async function submitQueue() {
		if (!selectedFacility || !phone.trim()) return;
		submitting = true;
		error = '';
		ticket = null;
		try {
			const payload = {
				facilityId: selectedFacility.id,
				patient: {
					fullName: fullNameForForm.trim() || 'Pengunjung',
					phone: phone.trim()
				}
			};
			// Use relative URL via SvelteKit proxy (+server.ts) — same origin, works in Docker and avoids CORS
			const res = await fetch('/api/v1/queues/generate', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			});
			const json = await res.json().catch(() => ({} as any));
			if (!res.ok || json.success === false) {
				error = json.error || `Gagal mengambil antrean (HTTP ${res.status}).`;
			} else if (json.success && json.data) {
				const d = json.data;
				ticket = {
					nomorAntrean: d.formatted_number || d.FormattedNumber || d.ticket_id || 'ANTREAN-XXX',
					processingTime: d.processing_time || d.ProcessingTime || '42µs',
					facilityName: selectedFacility.name,
					phone: phone.trim()
				};
				// clear selection after success (ticket stays visible)
				selectedFacility = null;
				phone = '';
			} else {
				error = 'Respons server tidak valid.';
			}
		} catch (e) {
			error = 'Tidak dapat menghubungi API. Pastikan Go backend + Rust engine berjalan (docker compose).';
		} finally {
			submitting = false;
		}
	}

	// Live update from SSE (real-time magic)
	function applyLiveBedUpdate(facilityId: string) {
		const idx = samples.findIndex((f) => f.id === facilityId);
		if (idx === -1) return;
		const f = samples[idx];
		if (f.availableBeds > 0) {
			// Reassign array slice to trigger $derived + UI reactivity in runes
			samples = [
				...samples.slice(0, idx),
				{ ...f, availableBeds: f.availableBeds - 1, lastUpdated: new Date().toISOString() },
				...samples.slice(idx + 1)
			];
		}
	}

	// Connect to Go SSE via SvelteKit proxy (relative path). Proxy handles internal Docker routing to api container.
	onMount(() => {
		const es = new EventSource('/api/v1/events/beds');
		es.addEventListener('bed_updated', (ev) => {
			try {
				const data = JSON.parse(ev.data || '{}');
				if (data.facility_id) {
					applyLiveBedUpdate(data.facility_id);
				}
			} catch {
				// ignore bad event
			}
		});
		es.onerror = () => {
			// connection lost or api not running; UI stays usable
		};
		return () => es.close();
	});
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
			<div class="rounded-xl border border-slate-200 bg-white p-6 dark:border-slate-800 dark:bg-slate-950 {selectedFacility?.id === facility.id ? 'ring-2 ring-emerald-600 dark:ring-emerald-500' : ''}">
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
						onclick={() => {
							selectedFacility = facility;
							phone = '';
							fullNameForForm = 'Pengunjung';
							error = '';
							ticket = null;
						}}
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

	<!-- Queue form (replaces placeholder) + ticket with Rust latency -->
	{#if selectedFacility}
		<div class="mt-8 rounded-2xl border border-emerald-200 bg-white p-6 dark:border-emerald-800 dark:bg-slate-950">
			<div class="flex items-center justify-between">
				<div>
					<div class="text-[10px] uppercase tracking-[1px] text-emerald-600 dark:text-emerald-400">Ambil Antrean</div>
					<div class="text-xl font-semibold tracking-[-0.01em]">{selectedFacility.name}</div>
				</div>
				<button onclick={() => (selectedFacility = null)} class="text-xs text-slate-500 hover:text-slate-700 dark:text-slate-400">Batal</button>
			</div>

			<form onsubmit={(e) => { e.preventDefault(); submitQueue(); }} class="mt-4 flex flex-col sm:flex-row gap-3">
				<input
					type="text"
					bind:value={fullNameForForm}
					placeholder="Nama lengkap"
					class="rounded-lg border border-slate-200 bg-white px-4 py-2.5 text-sm focus:border-emerald-600 dark:border-slate-700 dark:bg-slate-900"
				/>
				<input
					type="tel"
					bind:value={phone}
					placeholder="Nomor HP (wajib, contoh 081234567890)"
					required
					class="flex-1 rounded-lg border border-slate-200 bg-white px-4 py-2.5 text-sm placeholder:text-slate-400 focus:outline-none focus:border-emerald-600 dark:border-slate-700 dark:bg-slate-900"
				/>
				<button
					type="submit"
					disabled={submitting || !phone}
					class="rounded-lg bg-emerald-600 px-8 py-2.5 text-sm font-medium text-white transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-60"
				>
					{submitting ? 'Memproses via Rust…' : 'Ambil Antrean'}
				</button>
			</form>

			{#if error}
				<div class="mt-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">
					{error}
				</div>
			{/if}
		</div>
	{/if}

	{#if ticket}
		<div class="mt-6 rounded-2xl border-2 border-emerald-600 bg-emerald-50 p-6 dark:bg-emerald-950">
			<div class="text-[10px] font-medium uppercase tracking-[2px] text-emerald-600 dark:text-emerald-400">Tiket Antrean Digital • Rust Engine</div>
			<div class="mt-1 font-mono text-4xl font-semibold tracking-[-0.03em] text-emerald-900 dark:text-white">{ticket.nomorAntrean}</div>
			<div class="mt-3 text-sm text-emerald-800 dark:text-emerald-200 space-y-0.5">
				<div>Fasilitas: <span class="font-medium">{ticket.facilityName}</span></div>
				<div>Nomor HP: <span class="font-medium">{ticket.phone}</span></div>
				<div class="pt-2 font-semibold text-emerald-700 dark:text-emerald-300">Diproses dalam {ticket.processingTime}</div>
			</div>
			<button onclick={() => (ticket = null)} class="mt-4 text-xs font-medium text-emerald-700 underline hover:no-underline dark:text-emerald-400">Tutup tiket</button>
		</div>
	{/if}

	<p class="mt-8 text-center text-[10px] text-slate-400 dark:text-slate-600">
		Contoh data untuk scaffolding awal Sigap. Integrasikan dengan Go API + Rust engine untuk produksi.
	</p>
</section>
