<script lang="ts">
	// /admin/appointments — Appointment Admin Console
	// List appointments with status update controls.

	import { onMount } from 'svelte';

	type Appointment = {
		id: string;
		facility_id: string;
		service_unit_id: string;
		practitioner_id?: string;
		practitioner_schedule_id?: string;
		appointment_time?: string;
		status: 'scheduled' | 'checked_in' | 'queued' | 'completed' | 'cancelled' | 'no_show';
		patient_display_name: string;
		checkin_code?: string;
		queue_ticket_id?: string;
		created_at?: string;
		updated_at?: string;
	};

	let appointments = $state<Appointment[]>([]);
	let loading = $state(false);
	let error = $state('');
	let busyId = $state<string | null>(null);
	let filterStatus = $state('');

	const statusLabels: Record<string, string> = {
		scheduled: 'Terjadwal',
		checked_in: 'Check-In',
		queued: 'Antre',
		completed: 'Selesai',
		cancelled: 'Dibatalkan',
		no_show: 'Tidak Hadir'
	};

	const statusColors: Record<string, string> = {
		scheduled: 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-400',
		checked_in: 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-400',
		queued: 'bg-purple-100 text-purple-700 dark:bg-purple-950 dark:text-purple-400',
		completed: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400',
		cancelled: 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-400',
		no_show: 'bg-slate-100 text-slate-700 dark:bg-slate-900 dark:text-slate-400'
	};

	const allStatuses = ['scheduled','checked_in','queued','completed','cancelled','no_show'];

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

	async function loadAppointments() {
		loading = true;
		error = '';
		try {
			const res = await apiFetch('/admin/appointments');
			const json = await res.json();
			if (json.success && json.data) {
				appointments = json.data;
			} else {
				error = json.error || 'Gagal memuat janji temu.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi API.';
		} finally {
			loading = false;
		}
	}

	async function updateStatus(a: Appointment, newStatus: string) {
		busyId = a.id;
		error = '';
		try {
			const res = await apiFetch(`/admin/appointments/${a.id}/status`, {
				method: 'PATCH',
				body: JSON.stringify({ status: newStatus })
			});
			const json = await res.json();
			if (json.success) {
				await loadAppointments();
			} else {
				error = json.error || 'Gagal memperbarui status.';
			}
		} catch (e: any) {
			error = e.message || 'Gagal memperbarui status.';
		} finally {
			busyId = null;
		}
	}

	function filtered(): Appointment[] {
		if (!filterStatus) return appointments;
		return appointments.filter(a => a.status === filterStatus);
	}

	onMount(() => {
		loadAppointments();
	});
</script>

<div class="px-6 py-8">
	<div class="flex flex-col sm:flex-row items-start sm:items-center justify-between mb-6 gap-3">
		<div>
			<h1 class="text-2xl font-semibold tracking-[-0.01em]">Janji Temu</h1>
			<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Kelola status janji temu pasien.</p>
		</div>
		<div class="flex gap-2">
			<select bind:value={filterStatus} class="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
				<option value="">Semua Status</option>
				{#each allStatuses as s}
					<option value={s}>{statusLabels[s]}</option>
				{/each}
			</select>
			<button
				onclick={loadAppointments}
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
		<div class="py-12 text-center text-sm text-slate-500">Memuat janji temu…</div>
	{:else if filtered().length === 0}
		<div class="py-12 text-center text-sm text-slate-500">
			<div class="mb-2">Tidak ada janji temu.</div>
			{#if filterStatus}
				<button onclick={() => filterStatus = ''} class="text-emerald-600 hover:underline text-sm">Reset filter.</button>
			{:else}
				<button onclick={loadAppointments} class="text-emerald-600 hover:underline text-sm">Muat ulang.</button>
			{/if}
		</div>
	{:else}
		<div class="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
			<table class="w-full text-sm">
				<thead class="bg-slate-50 dark:bg-slate-950 border-b border-slate-200 dark:border-slate-800">
					<tr class="text-left text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">
						<th class="px-4 py-3 font-medium">Pasien</th>
						<th class="px-4 py-3 font-medium">Waktu</th>
						<th class="px-4 py-3 font-medium">Kode</th>
						<th class="px-4 py-3 font-medium">Status</th>
						<th class="px-4 py-3 font-medium text-right">Aksi</th>
					</tr>
				</thead>
				<tbody>
					{#each filtered() as a (a.id)}
						<tr class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-900/50">
							<td class="px-4 py-3">
								<div class="font-medium">{a.patient_display_name}</div>
								<div class="text-[10px] text-slate-400 font-mono">{a.facility_id.slice(0,8)}…/{a.service_unit_id.slice(0,8)}…</div>
							</td>
							<td class="px-4 py-3">
								{#if a.appointment_time}
									{new Date(a.appointment_time).toLocaleString('id-ID')}
								{:else}
									<span class="text-slate-400">—</span>
								{/if}
							</td>
							<td class="px-4 py-3 font-mono text-xs">
								{#if a.checkin_code}
									{a.checkin_code}
								{:else}
									<span class="text-slate-400">—</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								<span class="inline-block rounded-full text-[10px] px-2 py-0.5 font-medium {statusColors[a.status] || 'bg-slate-100 text-slate-700'}">
									{statusLabels[a.status] || a.status}
								</span>
								{#if a.queue_ticket_id}
									<div class="text-[10px] text-slate-400 mt-0.5">Ticket: {a.queue_ticket_id.slice(0,8)}…</div>
								{/if}
							</td>
							<td class="px-4 py-3 text-right">
								{#if busyId === a.id}
									<span class="text-xs text-slate-400">Memperbarui…</span>
								{:else}
									<div class="flex flex-col gap-1 items-end">
										{#if a.status === 'scheduled'}
											<button onclick={() => updateStatus(a, 'completed')} class="text-xs rounded px-2 py-0.5 text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950 border border-slate-200 dark:border-slate-700">Selesai</button>
											<button onclick={() => updateStatus(a, 'cancelled')} class="text-xs rounded px-2 py-0.5 text-red-600 hover:bg-red-50 dark:hover:bg-red-950 border border-slate-200 dark:border-slate-700">Batal</button>
											<button onclick={() => updateStatus(a, 'no_show')} class="text-xs rounded px-2 py-0.5 text-slate-600 hover:bg-slate-50 dark:hover:bg-slate-900 border border-slate-200 dark:border-slate-700">Tidak Hadir</button>
										{:else if a.status === 'checked_in' || a.status === 'queued'}
											<button onclick={() => updateStatus(a, 'completed')} class="text-xs rounded px-2 py-0.5 text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950 border border-slate-200 dark:border-slate-700">Selesai</button>
											<button onclick={() => updateStatus(a, 'cancelled')} class="text-xs rounded px-2 py-0.5 text-red-600 hover:bg-red-50 dark:hover:bg-red-950 border border-slate-200 dark:border-slate-700">Batal</button>
										{:else}
											<span class="text-xs text-slate-400">—</span>
										{/if}
									</div>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
