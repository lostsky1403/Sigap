<script lang="ts">
	// /admin/facilities — Facility Administration Console
	// Minimal CRUD UI with loading, error, empty states. Dark/light compatible.

	import { onMount } from 'svelte';

	type Facility = {
		id: string;
		name: string;
		type: string;
		address?: string;
		kecamatan: string;
		kabupaten_kota: string;
		provinsi?: string;
		phone?: string;
		total_beds: number;
		available_beds: number;
		is_active: boolean;
		short_code?: string;
	};

	let facilities = $state<Facility[]>([]);
	let loading = $state(false);
	let error = $state('');
	let showCreate = $state(false);
	let editing = $state<Facility | null>(null);
	let busy = $state(false);

	// Create / Edit form state
	let form = $state<Partial<Facility>>({});

	function resetForm(f?: Partial<Facility>) {
		form = f ? { ...f } : {
			name: '',
			type: 'rumah_sakit',
			address: '',
			kecamatan: '',
			kabupaten_kota: '',
			provinsi: '',
			phone: '',
			total_beds: 0,
			available_beds: 0,
			short_code: ''
		};
	}

	async function apiFetch(path: string, opts?: RequestInit) {
		const url = `/api/v1${path}`;
		const res = await fetch(url, {
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

	async function loadFacilities() {
		loading = true;
		error = '';
		try {
			const res = await apiFetch('/admin/facilities');
			const json = await res.json();
			if (json.success && json.data) {
				facilities = json.data;
			} else {
				error = json.error || 'Gagal memuat fasilitas.';
			}
		} catch (e: any) {
			error = e.message || 'Tidak dapat menghubungi API.';
		} finally {
			loading = false;
		}
	}

	async function createFacility() {
		busy = true;
		error = '';
		try {
			const res = await apiFetch('/admin/facilities', {
				method: 'POST',
				body: JSON.stringify(form)
			});
			const json = await res.json();
			if (json.success) {
				showCreate = false;
				resetForm();
				await loadFacilities();
			} else {
				error = json.error || 'Gagal membuat fasilitas.';
			}
		} catch (e: any) {
			error = e.message || 'Gagal membuat fasilitas.';
		} finally {
			busy = false;
		}
	}

	async function updateFacility() {
		if (!editing) return;
		busy = true;
		error = '';
		try {
			const body: Record<string, any> = {};
			(['name', 'type', 'address', 'kecamatan', 'kabupaten_kota', 'provinsi', 'phone', 'total_beds', 'available_beds', 'short_code'] as const).forEach((k) => {
				if (form[k] !== undefined && form[k] !== editing![k]) {
					body[k] = form[k];
				}
			});
			const res = await apiFetch(`/admin/facilities/${editing.id}`, {
				method: 'PATCH',
				body: JSON.stringify(body)
			});
			const json = await res.json();
			if (json.success) {
				editing = null;
				resetForm();
				await loadFacilities();
			} else {
				error = json.error || 'Gagal mengupdate fasilitas.';
			}
		} catch (e: any) {
			error = e.message || 'Gagal mengupdate fasilitas.';
		} finally {
			busy = false;
		}
	}

	async function deactivate(facility: Facility) {
		if (!confirm(`Nonaktifkan fasilitas "${facility.name}"?`)) return;
		busy = true;
		error = '';
		try {
			const res = await apiFetch(`/admin/facilities/${facility.id}/deactivate`, {
				method: 'PATCH'
			});
			const json = await res.json();
			if (json.success) {
				await loadFacilities();
			} else {
				error = json.error || 'Gagal menonaktifkan fasilitas.';
			}
		} catch (e: any) {
			error = e.message || 'Gagal menonaktifkan fasilitas.';
		} finally {
			busy = false;
		}
	}

	function typeLabel(type: string) {
		return type === 'rumah_sakit' ? 'Rumah Sakit' : 'Puskesmas';
	}

	onMount(() => {
		loadFacilities();
	});
</script>

<div class="px-6 py-8">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold tracking-[-0.01em]">Manajemen Fasilitas</h1>
			<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">Kelola data fasilitas kesehatan daerah.</p>
		</div>
		<button
			onclick={() => { showCreate = true; resetForm(); }}
			class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60"
			disabled={busy}
		>
			+ Tambah Fasilitas
		</button>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300">{error}</div>
	{/if}

	{#if loading}
		<div class="py-12 text-center text-sm text-slate-500">Memuat fasilitas…</div>
	{:else if facilities.length === 0}
		<div class="py-12 text-center text-sm text-slate-500">
			<div class="mb-2">Tidak ada fasilitas.</div>
			{#if !showCreate}
				<button onclick={() => { showCreate = true; resetForm(); }} class="text-emerald-600 hover:underline text-sm">Tambah fasilitas pertama.</button>
			{/if}
		</div>
	{:else}
		<div class="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
			<table class="w-full text-sm">
				<thead class="bg-slate-50 dark:bg-slate-950 border-b border-slate-200 dark:border-slate-800">
					<tr class="text-left text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">
						<th class="px-4 py-3 font-medium">Nama</th>
						<th class="px-4 py-3 font-medium">Tipe</th>
						<th class="px-4 py-3 font-medium">Lokasi</th>
						<th class="px-4 py-3 font-medium">Kasur</th>
						<th class="px-4 py-3 font-medium">Status</th>
						<th class="px-4 py-3 font-medium text-right">Aksi</th>
					</tr>
				</thead>
				<tbody>
					{#each facilities as f (f.id)}
						<tr class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-900/50">
							<td class="px-4 py-3">
								<div class="font-medium text-slate-900 dark:text-white">{f.name}</div>
								{#if f.short_code}
									<div class="text-[10px] text-slate-400 font-mono">{f.short_code}</div>
								{/if}
							</td>
							<td class="px-4 py-3 text-slate-600 dark:text-slate-300">{typeLabel(f.type)}</td>
							<td class="px-4 py-3 text-slate-600 dark:text-slate-300">{f.kecamatan}, {f.kabupaten_kota}</td>
							<td class="px-4 py-3 text-slate-600 dark:text-slate-300 tabular-nums">{f.available_beds} / {f.total_beds}</td>
							<td class="px-4 py-3">
								{#if f.is_active}
									<span class="inline-block rounded-full bg-emerald-100 text-emerald-700 text-[10px] px-2 py-0.5 font-medium dark:bg-emerald-950 dark:text-emerald-400">Aktif</span>
								{:else}
									<span class="inline-block rounded-full bg-red-100 text-red-700 text-[10px] px-2 py-0.5 font-medium dark:bg-red-950 dark:text-red-400">Nonaktif</span>
								{/if}
							</td>
							<td class="px-4 py-3 text-right">
								<button
									onclick={() => { editing = { ...f }; resetForm(f); }}
									class="text-xs text-emerald-600 hover:underline dark:text-emerald-500 mr-3"
								>
									Edit
								</button>
								{#if f.is_active}
									<button
										onclick={() => deactivate(f)}
										class="text-xs text-red-600 hover:underline dark:text-red-500"
									>
										Nonaktifkan
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<!-- Create / Edit Modal -->
{#if showCreate || editing}
	<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-start justify-center bg-black/50 p-4 pt-16"
		role="presentation"
		tabindex="-1"
		onclick={(e) => { if (e.currentTarget === e.target) { showCreate = false; editing = null; } }}
	>
		<div class="w-full max-w-lg rounded-2xl border border-slate-200 bg-white p-6 shadow-xl dark:border-slate-700 dark:bg-slate-950">
			<h2 class="text-lg font-semibold mb-4">{showCreate ? 'Tambah Fasilitas' : 'Edit Fasilitas'}</h2>
			<div class="space-y-3 text-sm">
				<div>
					<label for="fac-name" class="block text-xs text-slate-500 mb-1">Nama <span class="text-red-600">*</span></label>
					<input id="fac-name" bind:value={form.name} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 focus:border-emerald-600 dark:border-slate-700 dark:bg-slate-900" />
				</div>
				<div>
					<label for="fac-type" class="block text-xs text-slate-500 mb-1">Tipe <span class="text-red-600">*</span></label>
					<select id="fac-type" bind:value={form.type} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900">
						<option value="rumah_sakit">Rumah Sakit</option>
						<option value="puskesmas">Puskesmas</option>
					</select>
				</div>
				<div>
					<label for="fac-address" class="block text-xs text-slate-500 mb-1">Alamat</label>
					<input id="fac-address" bind:value={form.address} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
				</div>
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="fac-kec" class="block text-xs text-slate-500 mb-1">Kecamatan <span class="text-red-600">*</span></label>
						<input id="fac-kec" bind:value={form.kecamatan} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
					</div>
					<div>
						<label for="fac-kab" class="block text-xs text-slate-500 mb-1">Kab/Kota <span class="text-red-600">*</span></label>
						<input id="fac-kab" bind:value={form.kabupaten_kota} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
					</div>
				</div>
				<div>
					<label for="fac-prov" class="block text-xs text-slate-500 mb-1">Provinsi</label>
					<input id="fac-prov" bind:value={form.provinsi} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
				</div>
				<div>
					<label for="fac-phone" class="block text-xs text-slate-500 mb-1">Telepon <span class="text-red-600">*</span></label>
					<input id="fac-phone" bind:value={form.phone} type="tel" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
				</div>
				<div class="grid grid-cols-3 gap-3">
					<div>
						<label for="fac-total" class="block text-xs text-slate-500 mb-1">Total Kasur</label>
						<input id="fac-total" bind:value={form.total_beds} type="number" min="0" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
					</div>
					<div>
						<label for="fac-avail" class="block text-xs text-slate-500 mb-1">Tersedia</label>
						<input id="fac-avail" bind:value={form.available_beds} type="number" min="0" class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
					</div>
					<div>
						<label for="fac-code" class="block text-xs text-slate-500 mb-1">Kode</label>
						<input id="fac-code" bind:value={form.short_code} class="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 dark:border-slate-700 dark:bg-slate-900" />
					</div>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					onclick={() => { showCreate = false; editing = null; }}
					class="rounded-lg border border-slate-200 px-4 py-2 text-sm text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-900"
				>
					Batal
				</button>
				<button
					onclick={() => { showCreate ? createFacility() : updateFacility(); }}
					disabled={busy || !form.name || !form.kecamatan || !form.kabupaten_kota}
					class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-60"
				>
					{busy ? 'Memproses…' : (showCreate ? 'Buat' : 'Simpan')}
				</button>
			</div>
		</div>
	</div>
{/if}
