<script lang="ts">
	// /admin/queues — Queue Operator Console
	// List queue tickets with status badges, allow status updates.

	import { onMount } from 'svelte';

	type QueueTicket = {
		id: string;
		facility_id: string;
		queue_number: number;
		formatted_number: string;
		status: 'waiting' | 'called' | 'in_service' | 'completed' | 'cancelled' | 'skipped';
		registered_at: string;
		called_at?: string;
		completed_at?: string;
	};

	let tickets = $state<QueueTicket[]>([]);
	let loading = $state(false);
	let error = $state('');
	let facilityId = $state('');
	let busyId = $state<string | null>(null);

	const statusLabels: Record<string, string> = {
		waiting: 'Menunggu',
		called: 'Dipanggil',
		in_service: 'Dilayani',
		completed: 'Selesai',
		cancelled: 'Dibatalkan',
		skipped: 'Dilewati'
	};

	const statusColors: Record<string, string> = {
		waiting: 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-400',
		called: 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-400',
		in_service: 'bg-purple-100 text-purple-700 dark:bg-purple-950 dark:text-purple-400',
		completed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400',
		cancelled: 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-400',
		skipped: 'bg-slate-100 text-slate-700 dark:bg-slate-900 dark:text-slate-400'
	};

	const validTransitions: Record<string, string[]> = {
		waiting: ['called', 'cancelled'],
		called: ['in_service', 'cancelled', 'skipped'],
		in_service: ['completed']
	};

	async function apiFetch(path: string, opts?: RequestInit) {
		const res = await fetch(`/api/v1${path}`, {
			...opts,
			headers: {
				'Content-Type': 'application/json',
				...opts?.headers
			}
		});
		if (res.status === 401 || res.status === 403) {
			throw new Error('Akses ditolak. Pastikan Anda memiliki izin yang sesuai.');
		}
		return res;
	}

	async function loadTickets() {
		loading = true;
		error = '';
		try {
			const query = facilityId ? `?facility_id=${encodeURIComponent(facilityId)}` : '';
			const res = await apiFetch(`/admin/queues${query}`);
			const json = await res.json();
			if (json.success && json.data) {
				tickets = json.data;
			} else {
				error = json.error || 'Gagal memuat antrean.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi API.';
		} finally {
			loading = false;
		}
	}

	async function updateStatus(ticket: QueueTicket, newStatus: string) {
		busyId = ticket.id;
		error = '';
		try {
			const res = await apiFetch(`/admin/queues/${ticket.id}/status`, {
				method: 'PATCH',
				body: JSON.stringify({ status: newStatus })
			});
			const json = await res.json();
			if (json.success) {
				await loadTickets();
			} else {
				error = json.error || 'Gagal memperbarui status.';
			}
		} catch (e: any) {
			error = e.message || 'Gagal memperbarui status.';
		} finally {
			busyId = null;
		}
	}

	function availableTransitions(status: string): string[] {
		return validTransitions[status] || [];
	}

	onMount(() => {
		loadTickets();
	});
</script>

<div class="px-6 py-8">
	<div class="flex flex-col sm:flex-row items-start sm:items-center justify-between mb-6 gap-3">
		<div>
			<h1 class="text-2xl font-semibold tracking-[-0.01em]">Konsole Antrean</h1>
			<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Kelola status antrean pasien per fasilitas.</p>
		</div>
		<div class="flex gap-2">
			<input
				bind:value={facilityId}
				placeholder="Filter ID Fasilitas…"
				class="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm w-52 focus:border-emerald-600 dark:border-slate-700 dark:bg-slate-900"
			/>
			<button
				onclick={loadTickets}
				disabled={loading}
				class="rounded-lg border border-slate-200 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900 disabled:opacity-60"
			>
				{loading ? 'Memuat…' : 'Muat'}
			</button>
		</div>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>
	{/if}

	{#if loading}
		<div class="py-12 text-center text-sm text-slate-500">Memuat antrean…</div>
	{:else if tickets.length === 0}
		<div class="py-12 text-center text-sm text-slate-500">
			<div class="mb-2">Tidak ada antrean.</div>
			{#if facilityId}
				<button onclick={() => { facilityId = ''; loadTickets(); }} class="text-emerald-600 hover:underline text-sm">Reset filter.</button>
			{:else}
				<span class="text-slate-400">Masukkan ID fasilitas untuk memfilter.</span>
			{/if}
		</div>
	{:else}
		<div class="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
			<table class="w-full text-sm">
				<thead class="bg-slate-50 dark:bg-slate-950 border-b border-slate-200 dark:border-slate-800">
					<tr class="text-left text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">
						<th class="px-4 py-3 font-medium">Nomor</th>
						<th class="px-4 py-3 font-medium">Fasilitas ID</th>
						<th class="px-4 py-3 font-medium">Status</th>
						<th class="px-4 py-3 font-medium">Terdaftar</th>
						<th class="px-4 py-3 font-medium text-right">Aksi</th>
					</tr>
				</thead>
				<tbody>
					{#each tickets as t (t.id)}
						<tr class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-900/50">
							<td class="px-4 py-3">
								<div class="font-mono font-medium text-base">{t.formatted_number}</div>
								<div class="text-[10px] text-slate-400">#{t.queue_number}</div>
							</td>
							<td class="px-4 py-3 font-mono text-xs text-slate-500">{t.facility_id.slice(0, 8)}…</td>
							<td class="px-4 py-3">
								<span class="inline-block rounded-full text-[10px] px-2 py-0.5 font-medium {statusColors[t.status] || 'bg-slate-100 text-slate-700'}">
									{statusLabels[t.status] || t.status}
								</span>
							</td>
							<td class="px-4 py-3 text-slate-500 dark:text-slate-400 text-xs">
								{new Date(t.registered_at).toLocaleString('id-ID')}
							</td>
							<td class="px-4 py-3 text-right">
								{#if busyId === t.id}
									<span class="text-xs text-slate-400">Memperbarui…</span>
								{:else if availableTransitions(t.status).length > 0}
									{#each availableTransitions(t.status) as next}
										<button
											onclick={() => updateStatus(t, next)}
											class="ml-2 text-xs rounded px-2 py-0.5 {next === 'cancelled' ? 'text-red-600 hover:bg-red-50 dark:hover:bg-red-950' : 'text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950'} border border-slate-200 dark:border-slate-700"
										>
											→ {statusLabels[next] || next}
										</button>
									{/each}
								{:else}
									<span class="text-xs text-slate-400">—</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
