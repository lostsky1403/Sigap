<script lang="ts">
	// /admin/schedules — Practitioner Schedule Admin
	// List, create, and edit practitioner schedules.

	import { onMount } from 'svelte';

	type Schedule = {
		id: string;
		facility_id: string;
		practitioner_id?: string;
		service_unit_id: string;
		schedule_date: string;
		start_time: string;
		end_time: string;
		slot_minutes: number;
		capacity_per_slot: number;
		is_active: boolean;
		created_at?: string;
		updated_at?: string;
	};

	let schedules = $state<Schedule[]>([]);
	let loading = $state(false);
	let error = $state('');
	let busyId = $state<string | null>(null);
	let showForm = $state(false);
	let editing = $state<Schedule | null>(null);

	let formFacilityId = $state('');
	let formServiceUnitId = $state('');
	let formPractitionerId = $state('');
	let formScheduleDate = $state('');
	let formStartTime = $state('');
	let formEndTime = $state('');
	let formSlotMinutes = $state(30);
	let formCapacity = $state(1);
	let formIsActive = $state(true);

	const dayNames = ['Minggu','Senin','Selasa','Rabu','Kamis','Jumat','Sabtu'];

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

	async function loadSchedules() {
		loading = true;
		error = '';
		try {
			const res = await apiFetch('/admin/schedules');
			const json = await res.json();
			if (json.success && json.data) {
				schedules = json.data;
			} else {
				error = json.error || 'Gagal memuat jadwal.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi API.';
		} finally {
			loading = false;
		}
	}

	function resetForm() {
		formFacilityId = '';
		formServiceUnitId = '';
		formPractitionerId = '';
		formScheduleDate = '';
		formStartTime = '';
		formEndTime = '';
		formSlotMinutes = 30;
		formCapacity = 1;
		formIsActive = true;
		editing = null;
	}

	function openCreate() {
		resetForm();
		showForm = true;
	}

	function openEdit(s: Schedule) {
		editing = s;
		formFacilityId = s.facility_id;
		formServiceUnitId = s.service_unit_id;
		formPractitionerId = s.practitioner_id || '';
		formScheduleDate = s.schedule_date;
		formStartTime = s.start_time.slice(0, 5);
		formEndTime = s.end_time.slice(0, 5);
		formSlotMinutes = s.slot_minutes;
		formCapacity = s.capacity_per_slot;
		formIsActive = s.is_active;
		showForm = true;
	}

	async function saveSchedule() {
		busyId = 'form';
		error = '';
		try {
			const body = {
				facility_id: formFacilityId,
				service_unit_id: formServiceUnitId,
				practitioner_id: formPractitionerId || undefined,
				schedule_date: formScheduleDate,
				start_time: formStartTime,
				end_time: formEndTime,
				slot_minutes: formSlotMinutes,
				capacity_per_slot: formCapacity,
				is_active: formIsActive
			};
			const path = editing ? `/admin/schedules/${editing.id}` : '/admin/schedules';
			const method = editing ? 'PATCH' : 'POST';
			const res = await apiFetch(path, { method, body: JSON.stringify(body) });
			const json = await res.json();
			if (json.success) {
				showForm = false;
				resetForm();
				await loadSchedules();
			} else {
				error = json.error || 'Gagal menyimpan jadwal.';
			}
		} catch (e: any) {
			error = e.message || 'Gagal menyimpan jadwal.';
		} finally {
			busyId = null;
		}
	}

	function dayLabel(dateStr: string): string {
		const d = new Date(dateStr);
		return `${dayNames[d.getDay()]}, ${dateStr}`;
	}

	onMount(() => {
		loadSchedules();
	});
</script>

<div class="px-6 py-8">
	<div class="flex flex-col sm:flex-row items-start sm:items-center justify-between mb-6 gap-3">
		<div>
			<h1 class="text-2xl font-semibold tracking-[-0.01em]">Jadwal Praktik</h1>
			<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Kelola jadwal praktik dokter per layanan.</p>
		</div>
		<button
			onclick={openCreate}
			class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700"
		>
			+ Jadwal Baru
		</button>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>
	{/if}

	{#if loading}
		<div class="py-12 text-center text-sm text-slate-500">Memuat jadwal…</div>
	{:else if schedules.length === 0}
		<div class="py-12 text-center text-sm text-slate-500">
			<div class="mb-2">Tidak ada jadwal praktik.</div>
			<button onclick={loadSchedules} class="text-emerald-600 hover:underline text-sm">Muat ulang.</button>
		</div>
	{:else}
		<div class="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
			<table class="w-full text-sm">
				<thead class="bg-slate-50 dark:bg-slate-950 border-b border-slate-200 dark:border-slate-800">
					<tr class="text-left text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">
						<th class="px-4 py-3 font-medium">Tanggal</th>
						<th class="px-4 py-3 font-medium">Waktu</th>
						<th class="px-4 py-3 font-medium">Kapasitas</th>
						<th class="px-4 py-3 font-medium">Status</th>
						<th class="px-4 py-3 font-medium text-right">Aksi</th>
					</tr>
				</thead>
				<tbody>
					{#each schedules as s (s.id)}
						<tr class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-900/50">
							<td class="px-4 py-3">
								<div class="font-medium">{dayLabel(s.schedule_date)}</div>
								<div class="text-[10px] text-slate-400 font-mono">{s.facility_id.slice(0,8)}…/{s.service_unit_id.slice(0,8)}…</div>
							</td>
							<td class="px-4 py-3">{s.start_time.slice(0,5)} – {s.end_time.slice(0,5)}</td>
							<td class="px-4 py-3">{s.capacity_per_slot} / {s.slot_minutes}m</td>
							<td class="px-4 py-3">
								{#if s.is_active}
									<span class="inline-block rounded-full text-[10px] px-2 py-0.5 font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400">Aktif</span>
								{:else}
									<span class="inline-block rounded-full text-[10px] px-2 py-0.5 font-medium bg-slate-100 text-slate-700 dark:bg-slate-900 dark:text-slate-400">Nonaktif</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-right">
								<button
									onclick={() => openEdit(s)}
									class="text-xs rounded px-2 py-0.5 text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950 border border-slate-200 dark:border-slate-700"
								>
									Edit
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

{#if showForm}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4" onclick={() => showForm = false}>
		<div class="w-full max-w-lg rounded-xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-700 shadow-xl p-6" onclick={(e) => e.stopPropagation()}>
			<h2 class="text-lg font-semibold mb-4">{editing ? 'Edit Jadwal' : 'Jadwal Baru'}</h2>
			<div class="space-y-3">
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">ID Fasilitas</label>
						<input bind:value={formFacilityId} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">ID Layanan</label>
						<input bind:value={formServiceUnitId} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
				</div>
				<div>
					<label class="block text-xs font-medium text-slate-500 mb-1">ID Dokter (opsional)</label>
					<input bind:value={formPractitionerId} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">Tanggal</label>
						<input type="date" bind:value={formScheduleDate} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">Mulai</label>
						<input type="time" bind:value={formStartTime} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">Selesai</label>
						<input type="time" bind:value={formEndTime} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">Slot (menit)</label>
						<input type="number" min="5" bind:value={formSlotMinutes} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
					<div>
						<label class="block text-xs font-medium text-slate-500 mb-1">Kapasitas/slot</label>
						<input type="number" min="1" bind:value={formCapacity} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-800" />
					</div>
				</div>
				<label class="flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={formIsActive} class="rounded" />
					<span class="text-slate-700 dark:text-slate-300">Aktif</span>
				</label>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button onclick={() => { showForm = false; resetForm(); }} class="rounded-lg border border-slate-200 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">Batal</button>
				<button onclick={saveSchedule} disabled={busyId === 'form'} class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60">
					{busyId === 'form' ? 'Menyimpan…' : 'Simpan'}
				</button>
			</div>
		</div>
	</div>
{/if}
