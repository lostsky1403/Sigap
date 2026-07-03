<script lang="ts">
	// /patient/status — Public status lookup page
	// Accepts a check-in code and returns appointment/queue status info.

	let loading = $state(false);
	let error = $state('');
	let code = $state('');

	type StatusData = {
		found_by: string;
		facility_name: string;
		appointment_status: string;
		appointment_time: string;
		checkin_status: string;
		queue_number: number | null;
		queue_status: string | null;
		queue_formatted_number: string | null;
	};

	let result: StatusData | null = $state(null);

	const checkinLabels: Record<string, string> = {
		not_checked_in: 'Belum check-in',
		checked_in: 'Sudah check-in',
		in_queue: 'Sedang dalam antrean',
		selesai: 'Selesai',
		dibatalkan: 'Dibatalkan',
		tidak_hadir: 'Tidak hadir'
	};

	const queueStatusLabels: Record<string, string> = {
		waiting: 'Menunggu',
		called: 'Dipanggil',
		in_service: 'Sedang dilayani',
		completed: 'Selesai'
	};

	function formatTime(iso: string): string {
		try {
			const d = new Date(iso);
			return d.toLocaleDateString('id-ID', {
				day: 'numeric',
				month: 'long',
				year: 'numeric',
				hour: '2-digit',
				minute: '2-digit'
			});
		} catch {
			return iso;
		}
	}

	function checkinColor(status: string): string {
		switch (status) {
			case 'not_checked_in':
				return 'text-amber-700 dark:text-amber-400';
			case 'checked_in':
			case 'in_queue':
				return 'text-blue-700 dark:text-blue-400';
			case 'selesai':
				return 'text-emerald-700 dark:text-emerald-400';
			case 'dibatalkan':
			case 'tidak_hadir':
				return 'text-red-700 dark:text-red-400';
			default:
				return 'text-slate-700 dark:text-slate-300';
		}
	}

	async function lookup() {
		loading = true;
		error = '';
		result = null;
		try {
			const res = await fetch(`/api/v1/patient/status?code=${encodeURIComponent(code)}`);
			const json = await res.json();
			if (json.success && json.data) {
				result = json.data;
			} else {
				error = json.error || 'Kode tidak ditemukan. Periksa kembali kode Anda.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi server.';
		} finally {
			loading = false;
		}
	}

	function reset() {
		result = null;
		error = '';
		code = '';
	}
</script>

<div class="px-6 py-8 max-w-lg mx-auto">
	<div class="mb-6">
		<h1 class="text-2xl font-semibold tracking-[-0.01em]">Cek Status Kunjungan</h1>
		<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Masukkan kode check-in untuk melihat status janji temu Anda.</p>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>
	{/if}

	{#if result}
		<div class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm dark:border-emerald-900 dark:bg-emerald-950">
			<div class="font-medium text-emerald-700 dark:text-emerald-400 mb-3">Informasi Kunjungan</div>

			<div class="space-y-3">
				<div>
					<div class="text-xs text-slate-500 dark:text-slate-400">Fasilitas</div>
					<div class="font-medium text-slate-800 dark:text-slate-200">{result.facility_name}</div>
				</div>

				<div>
					<div class="text-xs text-slate-500 dark:text-slate-400">Waktu Janji Temu</div>
					<div class="font-medium text-slate-800 dark:text-slate-200">{formatTime(result.appointment_time)}</div>
				</div>

				<div>
					<div class="text-xs text-slate-500 dark:text-slate-400">Status Check-In</div>
					<div class="font-medium {checkinColor(result.checkin_status)}">
						{checkinLabels[result.checkin_status] || result.checkin_status}
					</div>
				</div>

				{#if result.queue_number !== null}
					<div>
						<div class="text-xs text-slate-500 dark:text-slate-400">Nomor Antrean</div>
						<div class="mt-1 inline-block rounded bg-white dark:bg-slate-900 border border-emerald-200 dark:border-emerald-800 px-4 py-2 font-mono text-lg tracking-wider font-semibold text-slate-800 dark:text-slate-200">
							{result.queue_formatted_number || `#${result.queue_number}`}
						</div>
					</div>
				{/if}

				{#if result.queue_status}
					<div>
						<div class="text-xs text-slate-500 dark:text-slate-400">Status Antrean</div>
						<div class="font-medium text-slate-800 dark:text-slate-200">
							{queueStatusLabels[result.queue_status] || result.queue_status}
						</div>
					</div>
				{/if}
			</div>

			<div class="mt-4">
				<button onclick={reset} class="text-xs text-emerald-700 dark:text-emerald-400 hover:underline">Cek kode lain</button>
			</div>
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); lookup(); }} class="space-y-4">
			<div>
				<label for="code" class="block text-xs font-medium text-slate-500 mb-1">Kode Check-In <span class="text-red-500">*</span></label>
				<input id="code" bind:value={code} required maxlength="6" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-mono dark:border-slate-700 dark:bg-slate-800" placeholder="Masukkan kode 6 karakter" />
			</div>
			<button
				type="submit"
				disabled={loading}
				class="w-full rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60"
			>
				{loading ? 'Mencari…' : 'Cari Status'}
			</button>
		</form>
	{/if}
</div>
