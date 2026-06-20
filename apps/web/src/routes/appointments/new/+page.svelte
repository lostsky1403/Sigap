<script lang="ts">
	// /appointments/new — Public booking page
	// Simple form to book an appointment. No auth required.

	let loading = $state(false);
	let error = $state('');
	let success = $state(false);
	let checkinCode = $state('');

	let facilityId = $state('');
	let serviceUnitId = $state('');
	let practitionerId = $state('');
	let appointmentDate = $state('');
	let appointmentTime = $state('');
	let patientPhone = $state('');
	let patientName = $state('');
	let notes = $state('');

	async function submit() {
		loading = true;
		error = '';
		success = false;
		checkinCode = '';
		try {
			const body: Record<string, any> = {
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
			const json = await res.json();
			if (json.success && json.data) {
				success = true;
				checkinCode = json.data.checkin_code;
			} else {
				error = json.error || 'Gagal membuat janji temu.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi server.';
		} finally {
			loading = false;
		}
	}

	function reset() {
		success = false;
		checkinCode = '';
		error = '';
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

	{#if success}
		<div class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-400">
			<div class="font-medium mb-1">Janji temu berhasil dibuat!</div>
			<div class="text-xs">Simpan kode check-in Anda:</div>
			<div class="mt-2 inline-block rounded bg-white dark:bg-slate-900 border border-emerald-200 dark:border-emerald-800 px-4 py-2 font-mono text-lg tracking-widest">{checkinCode}</div>
			<div class="mt-3 flex gap-2">
				<a href="/appointments/check-in" class="text-xs text-emerald-700 dark:text-emerald-400 hover:underline">→ Check-In sekarang</a>
				<button onclick={reset} class="text-xs text-slate-500 hover:underline">Buat janji lain</button>
			</div>
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); submit(); }} class="space-y-4">
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="fac" class="block text-xs font-medium text-slate-500 mb-1">ID Fasilitas <span class="text-red-500">*</span></label>
					<input id="fac" bind:value={facilityId} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
				</div>
				<div>
					<label for="su" class="block text-xs font-medium text-slate-500 mb-1">ID Layanan <span class="text-red-500">*</span></label>
					<input id="su" bind:value={serviceUnitId} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
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
