<script lang="ts">
	// /appointments/check-in — Public check-in page
	// Validates check-in code, calls gRPC GenerateQueueNumber, shows queue ticket.

	let loading = $state(false);
	let error = $state('');
	let success = $state(false);

	let appointmentId = $state('');
	let checkinCode = $state('');

	let ticket: { id: string; queue_number: number; formatted_number: string; facility_id: string; status: string } | null = $state(null);

	async function submit() {
		loading = true;
		error = '';
		success = false;
		ticket = null;
		try {
			const res = await fetch(`/api/v1/appointments/${appointmentId}/check-in`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ checkin_code: checkinCode })
			});
			const json = await res.json();
			if (json.success && json.data) {
				success = true;
				ticket = json.data;
			} else {
				error = json.error || 'Gagal check-in. Periksa kode dan ID janji temu.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi server.';
		} finally {
			loading = false;
		}
	}

	function reset() {
		success = false;
		ticket = null;
		error = '';
		appointmentId = '';
		checkinCode = '';
	}
</script>

<div class="px-6 py-8 max-w-lg mx-auto">
	<div class="mb-6">
		<h1 class="text-2xl font-semibold tracking-[-0.01em]">Check-In</h1>
		<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Masukkan kode check-in untuk mendapat nomor antrean.</p>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>
	{/if}

	{#if success && ticket}
		<div class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-400">
			<div class="font-medium mb-1">Check-in berhasil!</div>
			<div class="text-xs text-slate-600 dark:text-slate-400">Nomor antrean Anda:</div>
			<div class="mt-2 inline-block rounded bg-white dark:bg-slate-900 border border-emerald-200 dark:border-emerald-800 px-6 py-3 font-mono text-2xl tracking-widest font-semibold">{ticket.formatted_number}</div>
			<div class="mt-2 text-xs text-slate-500 dark:text-slate-400">Queue #{ticket.queue_number} • Fasilitas {ticket.facility_id.slice(0,8)}…</div>
			<div class="mt-3">
				<button onclick={reset} class="text-xs text-emerald-700 dark:text-emerald-400 hover:underline">Check-in lain</button>
			</div>
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); submit(); }} class="space-y-4">
			<div>
				<label for="aid" class="block text-xs font-medium text-slate-500 mb-1">ID Janji Temu <span class="text-red-500">*</span></label>
				<input id="aid" bind:value={appointmentId} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
			</div>
			<div>
				<label for="code" class="block text-xs font-medium text-slate-500 mb-1">Kode Check-In <span class="text-red-500">*</span></label>
				<input id="code" bind:value={checkinCode} required maxlength="6" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-mono dark:border-slate-700 dark:bg-slate-800" />
			</div>
			<button
				type="submit"
				disabled={loading}
				class="w-full rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60"
			>
				{loading ? 'Memproses…' : 'Check-In'}
			</button>
		</form>
	{/if}
</div>
