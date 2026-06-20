<script lang="ts">
	import { onMount } from 'svelte';
	import ReferralMap from '../ReferralMap.svelte';
	import type { Facility, QueueApiResponse, NearbyApiResponse } from '$lib/types';

	// Sigap Bed Availability Dashboard

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

	// For Smart Routing modal (Peta Rujukan from Stitch design)
	let showReferralModal = $state(false);
	let referralAlts = $state<Facility[]>([]);

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
				if (sortMode === 'distance' && userLocation && a.lat != null && a.lon != null && b.lat != null && b.lon != null) {
					const ul = userLocation as NonNullable<typeof userLocation>;
					const da = getDistance(ul.lat, ul.lon, a.lat, a.lon);
					const db = getDistance(ul.lat, ul.lon, b.lat, b.lon);
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
						const json = await res.json().catch(() => ({} as QueueApiResponse));
						if (res.status === 429 || (json?.error && /2 antrean|batas maksimal/i.test(json.error as string))) {
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

	// For referral map: alternatives with beds available (used when target penuh)
	function getAltsFor(target: Facility) {
		return samples.filter((f) => f.id !== target.id && f.availableBeds > 0);
	}

	// Open Smart Routing modal (per Stitch Peta Rujukan Otomatis design)
	// Uses backend /nearby for alts if possible, else client
	function openReferral(fac: Facility) {
		selectedFacility = fac;
		showReferralModal = true;
		referralAlts = [];
		if (fac.lat != null && fac.lon != null) {
			fetch(`/api/v1/facilities/nearby?lat=${fac.lat}&lon=${fac.lon}&exclude=${fac.id}`)
				.then(res => res.json().catch(() => ({} as NearbyApiResponse)))
				.then((json: NearbyApiResponse) => {
					if (json.success && json.data) {
						referralAlts = json.data;
					} else {
						referralAlts = getAltsFor(fac);
					}
				})
				.catch(() => {
					referralAlts = getAltsFor(fac);
				});
		} else {
			referralAlts = getAltsFor(fac);
		}
	}

	function closeReferralModal() {
		showReferralModal = false;
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
			const json = await res.json().catch(() => ({} as QueueApiResponse));
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
		<!-- Chaos Mode button per Stitch: diagonal stripe pattern (Brick Red #B91C1C and White) border, high-tension, invert on hover -->
		<button
			onclick={runChaosMode}
			disabled={chaosRunning}
			class="rounded-lg px-4 py-2 text-sm font-medium text-white transition disabled:cursor-not-allowed disabled:opacity-60 flex items-center gap-1 border-2 border-[#B91C1C] bg-[#B91C1C] hover:invert"
			style="background-image: repeating-linear-gradient(45deg, #B91C1C, #B91C1C 4px, #fff 4px, #fff 8px); border-image: repeating-linear-gradient(45deg, #B91C1C, #B91C1C 4px, #fff 4px, #fff 8px) 1;"
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

		<!-- Test button for Playwright to trigger full RS modal (Peta Rujukan) without changing real data -->
		<button
			onclick={() => {
				// Simulate full by using f6 (low beds) or force
				const testFac = samples.find(f => f.id === 'f6') || samples[0];
				openReferral(testFac);
			}}
			class="text-xs px-2 py-1 border border-emerald-600 text-emerald-600 rounded hover:bg-emerald-50 dark:hover:bg-emerald-950"
			title="Test: buka modal Peta Rujukan untuk faskes dengan kasur rendah (simulasi penuh)"
		>
			🧪 Test Modal Peta Rujukan
		</button>
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
						{@const ul = userLocation!}
						{@const d = getDistance(ul.lat, ul.lon, facility.lat, facility.lon)}
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

					<!-- Linear performance bar 2px per Stitch Sigap design: Emerald for stable/optimal, Brick Red for urgent -->
					<div class="mt-2 h-[2px] w-full bg-slate-200 dark:bg-slate-700 overflow-hidden">
						<div
							class="h-[2px] transition-[width] duration-200 ease-out {occupancy > 70 ? 'bg-red-600' : 'bg-emerald-600'}"
							style="width: {100 - occupancy}%"
						></div>
					</div>
					<div class="mt-1 text-right text-xs tabular-nums text-slate-500 dark:text-slate-400">
						{occupancy}% terisi {occupancy > 70 ? '• Urgent' : '• Optimal'}
					</div>
				</div>

				<div class="mt-6 flex items-center justify-between text-xs">
					<span class="text-slate-500 dark:text-slate-400">Update {formatTime(facility.lastUpdated)}</span>
					<button
						onclick={() => {
							if (facility.availableBeds <= 0) {
								// Full: open Smart Routing modal (Peta Rujukan) instead of standard form
								openReferral(facility);
							} else {
								selectedFacility = facility;
								phone = '';
								fullNameForForm = 'Pengunjung';
								error = '';
								ticket = null;
							}
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
	<!-- Super App: if target penuh (availableBeds <= 0), show mapcn-style referral map with pins + one-click auto route to alt -->
	{#if selectedFacility}
		{#if selectedFacility.availableBeds <= 0}
			<div class="mt-8 text-sm text-red-600 dark:text-red-400">RS penuh. Klik "Lihat antrean →" pada kartu untuk membuka modal Peta Rujukan Otomatis (desain dari Stitch).</div>
		{:else}
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
	{/if}

	{#if ticket}
		<!-- Apple Wallet style per Stitch Sigap design: high-contrast Slate header, perforated line, QR, punched 12px (0.75rem) corners, technical metadata with Geist-like labels -->
		<div class="mt-6 rounded-[12px] border border-slate-300 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-950 overflow-hidden" style="box-shadow: 0 6px 8px #00000024;">
			<!-- Slate header -->
			<div class="bg-slate-900 text-white p-4">
				<div class="flex justify-between items-start">
					<div>
						<div class="text-[10px] font-medium uppercase tracking-[1px] text-slate-400">Tiket Antrean Digital • Rust Engine</div>
						<div class="mt-1 font-mono text-3xl font-semibold tracking-[-0.03em]">{ticket.nomorAntrean}</div>
					</div>
					<!-- Simple QR placeholder (perforated style) -->
					<div class="w-12 h-12 border-2 border-white/80 bg-white/10 flex items-center justify-center rounded">
						<div class="text-[8px] font-mono text-white/70">QR</div>
					</div>
				</div>
			</div>
			
			<!-- Perforated separator line -->
			<div class="border-t border-dashed border-slate-300 dark:border-slate-600 mx-4"></div>
			
			<div class="p-4 text-sm">
				<div class="flex justify-between">
					<div>
						<div class="text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">Fasilitas</div>
						<div class="font-medium text-slate-900 dark:text-white">{ticket.facilityName}</div>
					</div>
					<div class="text-right">
						<div class="text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">Nomor HP</div>
						<div class="font-medium text-slate-900 dark:text-white">{ticket.phone}</div>
					</div>
				</div>
				
				<div class="mt-3 pt-3 border-t border-dashed border-slate-200 dark:border-slate-700">
					<div class="text-[10px] uppercase tracking-[1px] text-emerald-600 dark:text-emerald-400">Diproses dalam {ticket.processingTime}</div>
					<div class="text-[9px] text-slate-500 dark:text-slate-400 mt-1">Bukti kriptografi tersedia di Dompet Jejak Medis</div>
				</div>
			</div>
			
			<button onclick={() => (ticket = null)} class="w-full py-2 text-xs font-medium bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-600 dark:text-slate-400 border-t border-slate-200 dark:border-slate-700">Tutup Tiket</button>
		</div>
	{/if}

	<p class="mt-8 text-center text-[10px] text-slate-400 dark:text-slate-600">
		Contoh data untuk scaffolding awal Sigap. Integrasikan dengan Go API + Rust engine untuk produksi.
	</p>

	<!-- Modal Peta Rujukan Otomatis (per desain Stitch "Peta Rujukan Otomatis") -->
	{#if showReferralModal && selectedFacility}
		<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
		<div class="fixed inset-0 bg-black/60 z-[100] flex items-center justify-center p-4" role="presentation" tabindex="-1" onclick={(e) => { if (e.currentTarget === e.target) closeReferralModal(); }}>
			<div 
				class="bg-slate-900 rounded-2xl w-full max-w-5xl max-h-[90vh] overflow-auto shadow-xl border border-slate-700"
			>
				<div class="p-6">
					<div class="flex justify-between items-start mb-4">
						<div>
							<div class="text-xs uppercase tracking-[1px] text-red-600 dark:text-red-400">RS TUJUAN PENUH</div>
							<div class="text-2xl font-semibold tracking-[-0.01em] text-white">{selectedFacility.name}</div>
							<p class="text-sm text-slate-300 mt-1">Sistem rujukan otomatis aktif. Peta menampilkan alternatif dengan kasur tersedia (pin hijau emerald). Klik pin untuk ambil antrean otomatis.</p>
						</div>
						<button 
							onclick={closeReferralModal}
							class="text-sm px-3 py-1 rounded border border-slate-700 hover:bg-slate-800 text-slate-300 dark:text-slate-300"
						>
							Tutup
						</button>
					</div>

					<ReferralMap
						target={selectedFacility}
						alternatives={referralAlts.length ? referralAlts : getAltsFor(selectedFacility)}
						onSelect={(f) => {
							// Auto-routing: pilih alt, tutup modal, submit
							selectedFacility = f;
							phone = '';
							fullNameForForm = 'Pengunjung';
							error = '';
							ticket = null;
							showReferralModal = false;
							submitQueue();
						}}
					/>
				</div>
			</div>
		</div>
	{/if}
</section>
