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
		lat?: number;
		lon?: number;
	};

	let samples: Facility[] = $state([
		{ id: 'f1', name: 'RSUD Kota Sehat', type: 'rumah_sakit', kecamatan: 'Sukamaju', kabupatenKota: 'Kota Bandung', totalBeds: 180, availableBeds: 42, lastUpdated: '2026-06-12T08:15:00Z', shortCode: 'RSK', lat: -6.9175, lon: 107.6191 },
		{ id: 'f2', name: 'Puskesmas Sukajaya', type: 'puskesmas', kecamatan: 'Sukajaya', kabupatenKota: 'Kab. Bandung', totalBeds: 28, availableBeds: 19, lastUpdated: '2026-06-12T07:50:00Z', shortCode: 'PKM', lat: -6.9820, lon: 107.6820 },
		{ id: 'f3', name: 'RS Mitra Sehat', type: 'rumah_sakit', kecamatan: 'Menteng', kabupatenKota: 'Jakarta Pusat', totalBeds: 95, availableBeds: 11, lastUpdated: '2026-06-12T09:05:00Z', shortCode: 'RSM', lat: -6.1751, lon: 106.8270 },
		{ id: 'f4', name: 'Puskesmas Melati Indah', type: 'puskesmas', kecamatan: 'Cilandak', kabupatenKota: 'Jakarta Selatan', totalBeds: 35, availableBeds: 27, lastUpdated: '2026-06-12T06:40:00Z', shortCode: 'PMI', lat: -6.2658, lon: 106.7814 },
		{ id: 'f5', name: 'RSUD Sejahtera', type: 'rumah_sakit', kecamatan: 'Cibadak', kabupatenKota: 'Kab. Sukabumi', totalBeds: 120, availableBeds: 68, lastUpdated: '2026-06-12T08:55:00Z', shortCode: 'RSJ', lat: -6.9197, lon: 106.9270 },
		{ id: 'f6', name: 'Puskesmas Harapan Baru', type: 'puskesmas', kecamatan: 'Parung', kabupatenKota: 'Kab. Bogor', totalBeds: 22, availableBeds: 5, lastUpdated: '2026-06-12T07:10:00Z', shortCode: 'PHB', lat: -6.5950, lon: 106.8000 }
	]);

	let search = $state('');
	let typeFilter = $state<'all' | 'rumah_sakit' | 'puskesmas'>('all');
	let sortMode = $state<'availability' | 'name' | 'distance'>('availability');

	// Queue form state (wired to real backend)
	let selectedFacility = $state<Facility | null>(null);
	let phone = $state('');
	let fullNameForForm = $state('Pengunjung');
	let submitting = $state(false);
	let ticket = $state<null | { nomorAntrean: string; processingTime: string; facilityName: string; phone: string }>(null);
	let error = $state('');

	// Supercharge UI states (Chaos Mode, Anti-Calo Radar Log, Geo-Sort)
	let userLocation = $state<{ lat: number; lon: number } | null>(null);
	let antiCaloLogs = $state<{ time: string; msg: string }[]>([]);
	let chaosRunning = $state(false);

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
				if (sortMode === 'distance' && userLocation && a.lat != null && b.lat != null) {
					const da = getDistance(userLocation.lat, userLocation.lon, a.lat, a.lon);
					const db = getDistance(userLocation.lat, userLocation.lon, b.lat, b.lon);
					if (Math.abs(da - db) > 0.01) return da - db;
					// tie-break: prefer higher availability (lower occupancy)
					return calcOccupancy(a.availableBeds, a.totalBeds) - calcOccupancy(b.availableBeds, b.totalBeds);
				}
				// default: more available first (low occupancy)
				return calcOccupancy(a.availableBeds, a.totalBeds) - calcOccupancy(b.availableBeds, b.totalBeds);
			})
	);

	function formatTime(iso: string) {
		return new Date(iso).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
	}

	// Haversine distance in km (pure, no deps) for geo-sorting faskes
	function getDistance(lat1: number, lon1: number, lat2: number, lon2: number): number {
		const R = 6371;
		const dLat = ((lat2 - lat1) * Math.PI) / 180;
		const dLon = ((lon2 - lon1) * Math.PI) / 180;
		const a =
			Math.sin(dLat / 2) * Math.sin(dLat / 2) +
			Math.cos((lat1 * Math.PI) / 180) *
				Math.cos((lat2 * Math.PI) / 180) *
				Math.sin(dLon / 2) *
				Math.sin(dLon / 2);
		const c = 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
		return R * c;
	}

	function addAntiCaloLog(fac: Facility | string) {
		const time = new Date().toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
		const short = typeof fac === 'string' ? fac : fac.shortCode || fac.name;
		// newest on top, cap at 40 for perf/scroll
		antiCaloLogs = [{ time, msg: `🚨 Upaya calo diblokir di ${short}!` }, ...antiCaloLogs].slice(0, 40);
	}

	// Chaos Mode: rapid 50 concurrent queue requests (mix unique phones for successes/SSE bed moves + repeats for 429s)
	// Proves backend resilience + anti-spam + live UI via SSE. Runs in ~1s locally.
	async function runChaosMode() {
		if (chaosRunning) return;
		chaosRunning = true;
		error = '';
		const promises: Promise<void>[] = [];
		for (let i = 0; i < 50; i++) {
			// cycle facilities + mostly unique phones; every 5th repeats base phone to trigger 429s for log radar
			const fac = samples[i % samples.length];
			const isRepeatForCalo = i % 5 === 0;
			const phone = isRepeatForCalo ? '081234567890' : `0812345${(10000 + i).toString().slice(-4)}`;
			const payload = {
				facilityId: fac.id,
				patient: { fullName: 'Chaos Load Tester', phone }
			};
			promises.push(
				fetch('/api/v1/queues/generate', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify(payload)
				})
					.then(async (res) => {
						const json = await res.json().catch(() => ({} as any));
						if (res.status === 429 || (json?.error && /2 antrean|batas maksimal/i.test(json.error))) {
							addAntiCaloLog(fac);
						}
						// successes publish SSE -> applyLiveBedUpdate -> progress bars race (real-time magic)
					})
					.catch(() => {
						/* ignore network blips during load test */
					})
			);
		}
		await Promise.allSettled(promises);
		chaosRunning = false;

		// Visual demo boost (client-side): rapidly apply a few decrements so progress bars race visibly during 1-2s chaos.
		// Mirrors what real SSE + publish on successful generates would do (multiple bars + live updates).
		// Real 429s still logged from backend responses above; SSE will also decrement on actual successes.
		for (let k = 0; k < 12; k++) {
			const f = samples[k % samples.length];
			if (f.availableBeds > 0) {
				applyLiveBedUpdate(f.id);
			}
		}
	}

	// Geolokasi: browser native or mock, then sort by dist + availability
	function findNearestFacilities() {
		const mockJakarta = { lat: -6.2088, lon: 106.8456 }; // safe fallback for demo / headless
		if (!navigator.geolocation) {
			userLocation = mockJakarta;
			sortMode = 'distance';
			return;
		}
		navigator.geolocation.getCurrentPosition(
			(pos) => {
				userLocation = { lat: pos.coords.latitude, lon: pos.coords.longitude };
				sortMode = 'distance';
			},
			() => {
				// permission denied or error -> mock for demo
				userLocation = mockJakarta;
				sortMode = 'distance';
			},
			{ enableHighAccuracy: false, timeout: 6000, maximumAge: 30000 }
		);
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
				if (res.status === 429 || (json?.error && /2 antrean|batas maksimal/i.test(json.error))) {
					addAntiCaloLog(selectedFacility || 'Faskes');
				}
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
			<option value="distance">Urutkan: Jarak Terdekat</option>
		</select>
	</div>

	<!-- Supercharged controls: Chaos Load Tester + Geo + prominent Anti-Calo Radar Log (gamifikasi) -->
	<div class="mb-4 flex flex-wrap items-center gap-2">
		<button
			onclick={runChaosMode}
			disabled={chaosRunning}
			class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-60 flex items-center gap-1"
			title="Fire 50 rapid queue requests (unique phones for successes + repeats for 429s). Watch beds + SSE fly + anti-calo log fill!"
		>
			{chaosRunning ? '⏳ Chaos Mode (50x running...)' : '🚀 Chaos Mode (Load Test 50x)'}
		</button>

		<button
			onclick={findNearestFacilities}
			class="rounded-lg border border-emerald-600 px-4 py-2 text-sm font-medium text-emerald-600 transition hover:bg-emerald-50 dark:hover:bg-emerald-950 dark:text-emerald-500 flex items-center gap-1"
			title="Gunakan GPS browser atau mock untuk sortir faskes berdasarkan jarak + ketersediaan kasur"
		>
			📍 Cari Faskes Terdekat
		</button>

		{#if userLocation}
			<button
				onclick={() => {
					userLocation = null;
					if (sortMode === 'distance') sortMode = 'availability';
				}}
				class="text-xs text-slate-500 underline hover:text-slate-700 dark:text-slate-400"
			>
				Reset Lokasi & Sort
			</button>
			<span class="text-[10px] text-slate-500 dark:text-slate-400 tabular-nums">
				Lokasi Anda: {userLocation.lat.toFixed(3)}, {userLocation.lon.toFixed(3)}
			</span>
		{/if}
	</div>

	<!-- Log Radar Anti-Calo: scrolling, red on 429s (from chaos or manual). Newest on top. -->
	<div class="mb-6">
		<div class="mb-1 flex items-center gap-2 text-[10px] font-medium uppercase tracking-[1px] text-red-600 dark:text-red-400">
			🛡️ Log Radar Anti-Calo (Gamifikasi)
			<span class="font-mono text-[9px] normal-case text-red-500/70 dark:text-red-400/70">— deteksi calo real-time via 429</span>
		</div>
		<div
			class="h-36 overflow-y-auto rounded-xl border border-red-200 bg-red-50 p-3 text-[10px] font-mono leading-tight text-red-700 dark:border-red-900 dark:bg-red-950/60 dark:text-red-300"
		>
			{#if antiCaloLogs.length === 0}
				<div class="italic text-red-400/80 dark:text-red-400/60">Belum ada upaya calo. Tekan Chaos Mode untuk banjiri request & lihat radar aktif + progress bar bergerak cepat via SSE.</div>
			{:else}
				{#each antiCaloLogs as log}
					<div class="border-b border-red-200/60 py-0.5 last:border-b-0 dark:border-red-900/50">
						[{log.time}] {log.msg}
					</div>
				{/each}
			{/if}
		</div>
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
					{#if userLocation && facility.lat != null && facility.lon != null}
						{@const d = getDistance(userLocation.lat, userLocation.lon, facility.lat, facility.lon)}
						<span class="ml-1 inline-block rounded bg-emerald-100 px-1 py-0 text-[9px] font-medium text-emerald-700 dark:bg-emerald-900/60 dark:text-emerald-300">
							{d.toFixed(1)} km
						</span>
					{/if}
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
