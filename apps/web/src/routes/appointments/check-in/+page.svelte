<script lang="ts">
	// /appointments/check-in — Public check-in page
	// Validates check-in code, shows queue ticket from existing check-in response fields.

	import { onMount } from 'svelte';

	type CheckInResult = {
		appointment_id?: string;
		queue_ticket_id?: string;
		formatted_number?: string;
		status?: string;
		estimated_wait_minutes?: number;
		processing_time?: string;
	};

	let loading = $state(false);
	let error = $state('');
	let success = $state(false);
	let copyHint = $state('');

	let appointmentId = $state('');
	let checkinCode = $state('');

	let ticket: CheckInResult | null = $state(null);

	onMount(() => {
		// Demo convenience: allow deep-link from booking success page.
		const params = new URLSearchParams(window.location.search);
		const aid = params.get('appointment_id') || params.get('id') || '';
		const code = params.get('checkin_code') || params.get('code') || '';
		if (aid) appointmentId = aid;
		if (code) checkinCode = code;
	});

	async function submit() {
		loading = true;
		error = '';
		success = false;
		ticket = null;
		copyHint = '';
		try {
			const res = await fetch(`/api/v1/appointments/${appointmentId}/check-in`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ checkin_code: checkinCode })
			});
			const json = await res.json();
			if (json.success && json.data) {
				success = true;
				ticket = json.data as CheckInResult;
			} else {
				error = json.error || 'Gagal check-in. Periksa kode dan ID janji temu.';
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

	function reset() {
		success = false;
		ticket = null;
		error = '';
		copyHint = '';
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
		<div class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 p-5 dark:border-emerald-900 dark:bg-emerald-950">
			<div class="font-medium text-emerald-800 dark:text-emerald-300 mb-1">Check-in berhasil!</div>
			<div class="text-xs text-slate-600 dark:text-slate-400">Nomor antrean Anda</div>
			<div class="mt-2 inline-flex items-center gap-2 rounded-lg bg-white dark:bg-slate-900 border border-emerald-200 dark:border-emerald-800 px-6 py-3">
				<span class="font-mono text-3xl tracking-widest font-semibold text-slate-900 dark:text-white">
					{ticket.formatted_number || '—'}
				</span>
				{#if ticket.formatted_number}
					<button
						type="button"
						onclick={() => copyText('Nomor antrean', ticket?.formatted_number || '')}
						class="text-[10px] uppercase tracking-wide text-emerald-700 dark:text-emerald-400 hover:underline"
					>
						Salin
					</button>
				{/if}
			</div>

			{#if ticket.estimated_wait_minutes != null}
				<div class="mt-3 text-xs text-slate-600 dark:text-slate-400">
					Estimasi tunggu: <span class="font-medium text-slate-800 dark:text-slate-200">~{ticket.estimated_wait_minutes} menit</span>
				</div>
			{/if}
			{#if ticket.status}
				<div class="mt-1 text-xs text-slate-500 dark:text-slate-400">
					Status: <span class="font-medium">{ticket.status}</span>
				</div>
			{/if}

			<!-- Demo/debug refs from additive check-in response fields (UUIDs, not patient PII) -->
			<div class="mt-4 rounded-lg border border-emerald-200/80 dark:border-emerald-800/80 bg-white/70 dark:bg-slate-900/60 p-3 space-y-2">
				<div class="text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">Referensi demo</div>
				{#if ticket.appointment_id}
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<div class="text-[10px] text-slate-500">appointment_id</div>
							<div class="font-mono text-[11px] break-all text-slate-700 dark:text-slate-300">{ticket.appointment_id}</div>
						</div>
						<button type="button" onclick={() => copyText('appointment_id', ticket?.appointment_id || '')} class="shrink-0 text-[10px] text-emerald-700 dark:text-emerald-400 hover:underline">Salin</button>
					</div>
				{/if}
				{#if ticket.queue_ticket_id}
					<div class="flex items-start justify-between gap-2">
						<div class="min-w-0">
							<div class="text-[10px] text-slate-500">queue_ticket_id</div>
							<div class="font-mono text-[11px] break-all text-slate-700 dark:text-slate-300">{ticket.queue_ticket_id}</div>
						</div>
						<button type="button" onclick={() => copyText('queue_ticket_id', ticket?.queue_ticket_id || '')} class="shrink-0 text-[10px] text-emerald-700 dark:text-emerald-400 hover:underline">Salin</button>
					</div>
				{/if}
				{#if copyHint}
					<div class="text-[10px] text-emerald-700 dark:text-emerald-400">{copyHint}</div>
				{/if}
			</div>

			<div class="mt-4 flex flex-wrap gap-3">
				<a href="/patient/status" class="text-xs text-emerald-700 dark:text-emerald-400 hover:underline">→ Cek status kunjungan</a>
				<button type="button" onclick={reset} class="text-xs text-slate-500 hover:underline">Check-in lain</button>
			</div>
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); submit(); }} class="space-y-4">
			<div>
				<label for="aid" class="block text-xs font-medium text-slate-500 mb-1">ID Janji Temu <span class="text-red-500">*</span></label>
				<input id="aid" bind:value={appointmentId} required class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-mono dark:border-slate-700 dark:bg-slate-800" placeholder="UUID janji temu" />
			</div>
			<div>
				<label for="code" class="block text-xs font-medium text-slate-500 mb-1">Kode Check-In <span class="text-red-500">*</span></label>
				<input id="code" bind:value={checkinCode} required maxlength="6" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-mono dark:border-slate-700 dark:bg-slate-800" placeholder="6 karakter" />
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
