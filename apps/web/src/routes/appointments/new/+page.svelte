<script lang="ts">
	// /appointments/new — Public booking page
	// Simple form to book an appointment. No auth required.
	import { onMount } from 'svelte';

	type BookingResult = {
		id?: string;
		checkin_code?: string;
		status?: string;
		appointment_time?: string;
	};

	type FacilityOption = { id: string; name: string; short_code?: string | null };
	type ServiceUnitOption = { id: string; facility_id: string; name: string; code?: string | null };

	let loading = $state(false);
	let error = $state('');
	let success = $state(false);
	let copyHint = $state('');
	let result: BookingResult | null = $state(null);

	let facilityId = $state('');
	let serviceUnitId = $state('');
	let practitionerId = $state('');
	let appointmentDate = $state('');
	let appointmentTime = $state('');
	let patientPhone = $state('');
	let patientName = $state('');
	let notes = $state('');

	let facilities = $state<FacilityOption[]>([]);
	let serviceUnits = $state<ServiceUnitOption[]>([]);

	const unitsForFacility = $derived(
		facilityId
			? serviceUnits.filter((u) => u.facility_id === facilityId)
			: serviceUnits
	);

	onMount(() => {
		loadOptions();
	});

	async function readApi(res: Response): Promise<{ ok: boolean; data: any }> {
		const contentType = res.headers.get('content-type') || '';
		if (!contentType.includes('application/json')) {
			return { ok: false, data: null };
		}
		try {
			return { ok: true, data: await res.json() };
		} catch {
			return { ok: false, data: null };
		}
	}

	async function loadOptions() {
		try {
		const [fRes, sRes] = await Promise.all([
			fetch('/api/v1/public/facilities'),
			fetch('/api/v1/public/service-units')
		]);
			const fParsed = await readApi(fRes);
			if (fParsed.ok && fParsed.data?.success && Array.isArray(fParsed.data.data)) {
				const sortedFacilities = [...fParsed.data.data].sort(
					(a: FacilityOption, b: FacilityOption) =>
						(a.name || '').localeCompare(b.name || '') || (a.id || '').localeCompare(b.id || '')
				);
				const seenNames = new Set<string>();
				facilities = sortedFacilities.filter((f: FacilityOption) => {
					if (!f?.id || !f.name || seenNames.has(f.name)) return false;
					seenNames.add(f.name);
					return true;
				});
			}
			const sParsed = await readApi(sRes);
			if (sParsed.ok && sParsed.data?.success && Array.isArray(sParsed.data.data)) {
				const seenUnits = new Set<string>();
				serviceUnits = sParsed.data.data
					.filter((u: ServiceUnitOption) => {
						if (!u?.id || !u.facility_id || seenUnits.has(u.id)) return false;
						seenUnits.add(u.id);
						return true;
					})
					.sort((a: ServiceUnitOption, b: ServiceUnitOption) => a.name.localeCompare(b.name));
			}
		} catch {
			// Lists unavailable — fall back to manual ID entry below.
		}
	}

	function onFacilityChange() {
		if (facilityId && serviceUnits.length > 0) {
			const matched = serviceUnits.some((u) => u.facility_id === facilityId);
			if (!matched) {
				// No units for this facility in the loaded list — fall back to manual entry.
				serviceUnitId = '';
			}
		}
	}

	async function submit() {
		loading = true;
		error = '';
		success = false;
		result = null;
		copyHint = '';
		try {
			const body: Record<string, string> = {
				facility_id: facilityId,
				service_unit_id: serviceUnitId,
				appointment_time: `${appointmentDate}T${appointmentTime}:00Z`,
				patient_phone: patientPhone,
				patient_display_name: patientName
			};
			if (practitionerId) body.practitioner_id = practitionerId;
			if (notes) body.notes = notes;
			const res = await fetch('/api/v1/appointments', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body)
			});
			const parsed = await readApi(res);
			if (!parsed.ok) {
				error = 'Gagal menghubungi layanan. Silakan coba lagi.';
				return;
			}
			const json = parsed.data;
			if (json.success && json.data) {
				success = true;
				result = json.data as BookingResult;
			} else {
				error = json.error || 'Gagal membuat janji temu.';
			}
		} catch (e: unknown) {
			error = e instanceof Error ? e.message : 'Tidak dapat menghubungi server.';
		} finally {
			loading = false;
		}
	}

	async function copyText(label: string, value: string) {
		if (!value) return;
		try {
			await navigator.clipboard.writeText(value);
			copyHint = `${label} disalin`;
			setTimeout(() => {
				if (copyHint === `${label} disalin`) copyHint = '';
			}, 2000);
		} catch {
			copyHint = 'Gagal menyalin';
		}
	}

	function checkInHref(): string {
		if (!result?.id) return '/appointments/check-in';
		const params = new URLSearchParams({ appointment_id: result.id });
		if (result.checkin_code) params.set('checkin_code', result.checkin_code);
		return `/appointments/check-in?${params.toString()}`;
	}

	function reset() {
		success = false;
		result = null;
		error = '';
		copyHint = '';
		facilityId = '';
		serviceUnitId = '';
		practitionerId = '';
		appointmentDate = '';
		appointmentTime = '';
		patientPhone = '';
		patientName = '';
		notes = '';
	}
</script>

<div class="px-6 py-8 max-w-lg mx-auto">
	<div class="mb-6">
		<h1 class="text-2xl font-semibold tracking-[-0.01em]">Buat Janji Temu</h1>
		<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Isi formulir di bawah untuk mendaftar janji temu.</p>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>
	{/if}

	{#if success && result}
		<div class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 p-5 text-sm dark:border-emerald-900 dark:bg-emerald-950">
			<div class="font-medium text-emerald-800 dark:text-emerald-300 mb-1">Janji temu berhasil dibuat!</div>
			<div class="text-xs text-slate-600 dark:text-slate-400">Simpan kode check-in Anda</div>
			<div class="mt-2 inline-flex items-center gap-2 rounded-lg bg-white dark:bg-slate-900 border border-emerald-200 dark:border-emerald-800 px-4 py-2">
				<span class="font-mono text-xl tracking-widest font-semibold text-slate-900 dark:text-white">{result.checkin_code || '—'}</span>
				{#if result.checkin_code}
					<button type="button" onclick={() => copyText('Kode check-in', result?.checkin_code || '')} class="text-[10px] uppercase tracking-wide text-emerald-700 dark:text-emerald-400 hover:underline">Salin</button>
				{/if}
			</div>

			<!-- Demo refs: booking response already returns id + checkin_code -->
			<div class="mt-4 rounded-lg border border-emerald-200/80 dark:border-emerald-800/80 bg-white/70 dark:bg-slate-900/60 p-3 space-y-2">
				<div class="text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">Referensi demo</div>
				{#if result.id}
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<div class="text-[10px] text-slate-500">appointment_id</div>
							<div class="font-mono text-[11px] break-all text-slate-700 dark:text-slate-300">{result.id}</div>
						</div>
						<button type="button" onclick={() => copyText('appointment_id', result?.id || '')} class="shrink-0 text-[10px] text-emerald-700 dark:text-emerald-400 hover:underline">Salin</button>
					</div>
				{/if}
				{#if result.status}
					<div class="text-[10px] text-slate-500">status: <span class="font-mono text-slate-700 dark:text-slate-300">{result.status}</span></div>
				{/if}
				{#if copyHint}
					<div class="text-[10px] text-emerald-700 dark:text-emerald-400">{copyHint}</div>
				{/if}
			</div>

			<div class="mt-4 flex flex-wrap gap-3">
				<a href={checkInHref()} class="text-xs text-emerald-700 dark:text-emerald-400 hover:underline">→ Check-In sekarang</a>
				<button type="button" onclick={reset} class="text-xs text-slate-500 hover:underline">Buat janji lain</button>
			</div>
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); submit(); }} class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="fac" class="block text-xs font-medium text-slate-500 mb-1">Fasilitas <span class="text-red-500">*</span></label>
					{#if facilities.length > 0}
						<select
							id="fac"
							bind:value={facilityId}
							onchange={onFacilityChange}
							required
							class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800"
						>
							<option value="" disabled>Pilih fasilitas…</option>
							{#each facilities as f (f.id)}
								<option value={f.id}>{f.name}</option>
							{/each}
						</select>
					{:else}
						<input id="fac" bind:value={facilityId} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					{/if}
				</div>
				<div>
					<label for="su" class="block text-xs font-medium text-slate-500 mb-1">Layanan <span class="text-red-500">*</span></label>
					{#if facilityId && unitsForFacility.length > 0}
						<select
							id="su"
							bind:value={serviceUnitId}
							required
							class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800"
						>
							<option value="" disabled>Pilih layanan…</option>
							{#each unitsForFacility as u (u.id)}
								<option value={u.id}>{u.name}</option>
							{/each}
						</select>
					{:else}
						<input id="su" bind:value={serviceUnitId} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					{/if}
				</div>
			</div>
			<div>
				<label for="prac" class="block text-xs font-medium text-slate-500 mb-1">ID Dokter (opsional)</label>
				<input id="prac" bind:value={practitionerId} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="date" class="block text-xs font-medium text-slate-500 mb-1">Tanggal <span class="text-red-500">*</span></label>
					<input id="date" type="date" bind:value={appointmentDate} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
				</div>
				<div>
					<label for="time" class="block text-xs font-medium text-slate-500 mb-1">Waktu <span class="text-red-500">*</span></label>
					<input id="time" type="time" bind:value={appointmentTime} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
				</div>
			</div>
			<div>
				<label for="phone" class="block text-xs font-medium text-slate-500 mb-1">Nomor HP <span class="text-red-500">*</span></label>
				<input id="phone" type="tel" bind:value={patientPhone} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
			</div>
			<div>
				<label for="name" class="block text-xs font-medium text-slate-500 mb-1">Nama Pasien <span class="text-red-500">*</span></label>
				<input id="name" bind:value={patientName} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
			</div>
			<div>
				<label for="notes" class="block text-xs font-medium text-slate-500 mb-1">Catatan (opsional)</label>
				<textarea id="notes" bind:value={notes} rows="2" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800"></textarea>
			</div>
			<button
				type="submit"
				disabled={loading}
				class="w-full rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60"
			>
				{loading ? 'Memproses…' : 'Daftar Janji Temu'}
			</button>
		</form>
	{/if}
</div>
