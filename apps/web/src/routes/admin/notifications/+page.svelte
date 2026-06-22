<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';

	// Matches apps/api/internal/notification/types.go and the JSON
	// projection in handler/notifications.go. We deliberately do NOT
	// have a `recipient_contact` or `recipient_contact_hash` field —
	// the server never returns either.
	type NotificationStatus = 'pending' | 'processing' | 'delivered' | 'failed' | 'cancelled';
	type NotificationChannel = 'dev' | 'sms' | 'whatsapp' | 'email';
	type NotificationRecipientType = 'patient' | 'staff' | 'facility_admin';

	interface NotificationRow {
		id: string;
		facility_id?: string;
		channel: NotificationChannel;
		template_key: string;
		subject: string;
		body_template: string;
		recipient_type: NotificationRecipientType;
		recipient_contact_masked: string;
		status: NotificationStatus;
		attempt_count: number;
		next_attempt_at: string;
		last_error_code?: string;
		created_at: string;
		updated_at: string;
	}

	interface SummaryCounts {
		pending: number;
		processing: number;
		delivered: number;
		failed: number;
		cancelled: number;
	}

	const STATUSES: NotificationStatus[] = ['pending', 'processing', 'delivered', 'failed', 'cancelled'];
	const CHANNELS: NotificationChannel[] = ['dev', 'sms', 'whatsapp', 'email'];

	let rows = $state<NotificationRow[]>([]);
	let summary = $state<SummaryCounts | null>(null);
	let listError = $state('');
	let summaryError = $state('');
	let listLoaded = $state(false);
	let summaryLoaded = $state(false);
	let busyId = $state<string | null>(null);

	// Filter state — mirrors URL search params for shareable links.
	let filterStatus = $state<string>('');
	let filterChannel = $state<string>('');
	let filterTemplate = $state<string>('');
	let filterFrom = $state<string>('');
	let filterTo = $state<string>('');

	const hasActiveFilters = $derived(
		filterStatus !== '' ||
			filterChannel !== '' ||
			filterTemplate !== '' ||
			filterFrom !== '' ||
			filterTo !== ''
	);

	function buildQueryString(): string {
		const p = new URLSearchParams();
		p.set('limit', '200');
		if (filterStatus) p.set('status', filterStatus);
		if (filterChannel) p.set('channel', filterChannel);
		if (filterTemplate) p.set('template_key', filterTemplate);
		if (filterFrom) p.set('created_from', `${filterFrom}T00:00:00Z`);
		if (filterTo) p.set('created_to', `${filterTo}T23:59:59Z`);
		return p.toString();
	}

	function syncFromUrl() {
		const params = $page.url.searchParams;
		filterStatus = params.get('status') ?? '';
		filterChannel = params.get('channel') ?? '';
		filterTemplate = params.get('template_key') ?? '';
		filterFrom = params.get('created_from')?.slice(0, 10) ?? '';
		filterTo = params.get('created_to')?.slice(0, 10) ?? '';
	}

	function pushFiltersToUrl() {
		const url = new URL($page.url);
		const set = (k: string, v: string) => {
			if (v) url.searchParams.set(k, v);
			else url.searchParams.delete(k);
		};
		set('status', filterStatus);
		set('channel', filterChannel);
		set('template_key', filterTemplate);
		set('created_from', filterFrom ? `${filterFrom}T00:00:00Z` : '');
		set('created_to', filterTo ? `${filterTo}T23:59:59Z` : '');
		history.replaceState(history.state, '', url.toString());
	}

	function relativeTime(iso: string): string {
		try {
			const t = new Date(iso).getTime();
			const diff = Math.max(0, Date.now() - t);
			const sec = Math.floor(diff / 1000);
			if (sec < 60) return `${sec}d lalu`;
			const min = Math.floor(sec / 60);
			if (min < 60) return `${min}m lalu`;
			const hr = Math.floor(min / 60);
			if (hr < 24) return `${hr}j lalu`;
			const d = Math.floor(hr / 24);
			return `${d}h lalu`;
		} catch {
			return iso;
		}
	}

	function statusClasses(s: NotificationStatus): string {
		switch (s) {
			case 'delivered':
				return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400';
			case 'pending':
			case 'processing':
				return 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-400';
			case 'failed':
				return 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-400';
			case 'cancelled':
				return 'bg-slate-100 text-slate-700 dark:bg-slate-900 dark:text-slate-400';
			default:
				return 'bg-slate-100 text-slate-700 dark:bg-slate-900 dark:text-slate-400';
		}
	}

	function summaryCardClasses(s: NotificationStatus): string {
		switch (s) {
			case 'pending':
				return 'border-blue-200 dark:border-blue-900 bg-blue-50 dark:bg-blue-950/40';
			case 'processing':
				return 'border-amber-200 dark:border-amber-900 bg-amber-50 dark:bg-amber-950/40';
			case 'delivered':
				return 'border-emerald-200 dark:border-emerald-900 bg-emerald-50 dark:bg-emerald-950/40';
			case 'failed':
				return 'border-red-200 dark:border-red-900 bg-red-50 dark:bg-red-950/40';
			case 'cancelled':
				return 'border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900/40';
		}
	}

	function summaryDotClasses(s: NotificationStatus): string {
		switch (s) {
			case 'pending':
				return 'bg-blue-500';
			case 'processing':
				return 'bg-amber-500';
			case 'delivered':
				return 'bg-emerald-500';
			case 'failed':
				return 'bg-red-500';
			case 'cancelled':
				return 'bg-slate-400';
		}
	}

	async function loadNotifications() {
		listError = '';
		try {
			const qs = buildQueryString();
			const res = await fetch(`/api/v1/admin/notifications?${qs}`, {
				headers: { Accept: 'application/json' }
			});
			if (!res.ok) {
				listError = `Gagal memuat outbox (HTTP ${res.status}).`;
				rows = [];
				return;
			}
			const body = await res.json();
			rows = (body?.data ?? []) as NotificationRow[];
		} catch (e) {
			listError = `Kesalahan jaringan: ${(e as Error).message}`;
			rows = [];
		} finally {
			listLoaded = true;
		}
	}

	async function loadSummary() {
		summaryError = '';
		try {
			const qs = buildQueryString();
			const res = await fetch(`/api/v1/admin/notifications/summary?${qs}`, {
				headers: { Accept: 'application/json' }
			});
			if (!res.ok) {
				summaryError = `Gagal memuat ringkasan (HTTP ${res.status}).`;
				summary = null;
				return;
			}
			const body = await res.json();
			summary = (body?.data ?? null) as SummaryCounts | null;
		} catch (e) {
			summaryError = `Kesalahan jaringan: ${(e as Error).message}`;
			summary = null;
		} finally {
			summaryLoaded = true;
		}
	}

	async function loadAll() {
		listLoaded = false;
		summaryLoaded = false;
		await Promise.all([loadNotifications(), loadSummary()]);
	}

	function applyFilters() {
		pushFiltersToUrl();
		loadAll();
	}

	function resetFilters() {
		filterStatus = '';
		filterChannel = '';
		filterTemplate = '';
		filterFrom = '';
		filterTo = '';
		pushFiltersToUrl();
		loadAll();
	}

	async function callAction(id: string, op: 'retry' | 'cancel') {
		busyId = `${op}:${id}`;
		try {
			const res = await fetch(`/api/v1/admin/notifications/${id}/${op}`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' }
			});
			if (!res.ok) {
				listError = `Aksi ${op} gagal (HTTP ${res.status}).`;
				return;
			}
			listError = '';
			await loadAll();
		} catch (e) {
			listError = `Kesalahan jaringan: ${(e as Error).message}`;
		} finally {
			busyId = null;
		}
	}

	onMount(() => {
		syncFromUrl();
		loadAll();
	});
</script>

<div class="px-6 py-8">
	<div class="flex flex-col sm:flex-row items-start sm:items-center justify-between mb-6 gap-3">
		<div>
			<h1 class="text-2xl font-semibold tracking-[-0.01em]">Outbox Notifikasi</h1>
			<p class="text-sm text-slate-500 dark:text-slate-400 mt-1">
				Antrean notifikasi appointment & check-in. Dev provider only — belum ada vendor SMS/WhatsApp/email.
			</p>
		</div>
		<button
			type="button"
			onclick={loadAll}
			class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700"
		>
			Muat ulang
		</button>
	</div>

	<!-- Summary cards -->
	<div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3 mb-6">
		{#each STATUSES as s (s)}
			<div class={`rounded-xl border p-4 ${summaryCardClasses(s)}`}>
				<div class="flex items-center gap-2 mb-1">
					<span class={`inline-block w-2 h-2 rounded-full ${summaryDotClasses(s)}`}></span>
					<span class="text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400 font-medium">
						{s}
					</span>
				</div>
				<div class="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-50">
					{#if !summaryLoaded}
						<span class="text-slate-400">…</span>
					{:else if summaryError}
						<span class="text-slate-400 text-sm">—</span>
					{:else}
						{summary?.[s] ?? 0}
					{/if}
				</div>
			</div>
		{/each}
	</div>
	{#if summaryError}
		<div
			class="mb-4 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300"
		>
			Ringkasan tidak dapat dimuat. Tabel di bawah masih menampilkan data terbaru.
		</div>
	{/if}

	<!-- Filter bar -->
	<div
		class="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-950 p-4 mb-4"
	>
		<div class="flex flex-col lg:flex-row gap-3 lg:items-end">
			<div class="flex-1 min-w-0">
				<label class="block text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400 mb-1" for="filter-status">
					Status
				</label>
				<select
					id="filter-status"
					bind:value={filterStatus}
					class="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
				>
					<option value="">Semua</option>
					{#each STATUSES as s (s)}
						<option value={s}>{s}</option>
					{/each}
				</select>
			</div>
			<div class="flex-1 min-w-0">
				<label class="block text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400 mb-1" for="filter-channel">
					Channel
				</label>
				<select
					id="filter-channel"
					bind:value={filterChannel}
					class="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
				>
					<option value="">Semua</option>
					{#each CHANNELS as c (c)}
						<option value={c}>{c}</option>
					{/each}
				</select>
			</div>
			<div class="flex-1 min-w-0">
				<label class="block text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400 mb-1" for="filter-template">
					Template key
				</label>
				<input
					id="filter-template"
					type="text"
					bind:value={filterTemplate}
					placeholder="contoh: appointment.booked"
					class="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm font-mono"
				/>
			</div>
			<div class="flex-1 min-w-0">
				<label class="block text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400 mb-1" for="filter-from">
					Dari
				</label>
				<input
					id="filter-from"
					type="date"
					bind:value={filterFrom}
					class="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
				/>
			</div>
			<div class="flex-1 min-w-0">
				<label class="block text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400 mb-1" for="filter-to">
					Sampai
				</label>
				<input
					id="filter-to"
					type="date"
					bind:value={filterTo}
					class="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm"
				/>
			</div>
			<div class="flex gap-2">
				<button
					type="button"
					onclick={applyFilters}
					class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700"
				>
					Terapkan
				</button>
				{#if hasActiveFilters}
					<button
						type="button"
						onclick={resetFilters}
						class="rounded-lg border border-slate-200 dark:border-slate-700 px-3 py-2 text-sm font-medium text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-900"
					>
						Reset
					</button>
				{/if}
			</div>
		</div>
	</div>

	{#if listError}
		<div
			class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
		>
			{listError}
			<button
				type="button"
				onclick={loadNotifications}
				class="ml-3 underline text-red-800 dark:text-red-200"
			>
				Coba lagi
			</button>
		</div>
	{/if}

	<!-- Table -->
	<div class="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
		<table class="w-full text-sm">
			<thead class="bg-slate-50 dark:bg-slate-950 border-b border-slate-200 dark:border-slate-800">
				<tr class="text-left text-[10px] uppercase tracking-[1px] text-slate-500 dark:text-slate-400">
					<th class="px-4 py-3 font-medium">Created</th>
					<th class="px-4 py-3 font-medium">Channel</th>
					<th class="px-4 py-3 font-medium">Template</th>
					<th class="px-4 py-3 font-medium">Penerima (masked)</th>
					<th class="px-4 py-3 font-medium">Status</th>
					<th class="px-4 py-3 font-medium">Attempts</th>
					<th class="px-4 py-3 font-medium text-right">Aksi</th>
				</tr>
			</thead>
			<tbody>
				{#if !listLoaded}
					{#each Array(5) as _, i (i)}
						<tr class="border-b border-slate-100 dark:border-slate-800">
							<td class="px-4 py-3">
								<div class="h-3 w-16 bg-slate-200 dark:bg-slate-800 rounded animate-pulse"></div>
								<div class="h-2 w-12 bg-slate-100 dark:bg-slate-900 rounded mt-2 animate-pulse"></div>
							</td>
							<td class="px-4 py-3"><div class="h-3 w-10 bg-slate-200 dark:bg-slate-800 rounded animate-pulse"></div></td>
							<td class="px-4 py-3">
								<div class="h-3 w-40 bg-slate-200 dark:bg-slate-800 rounded animate-pulse"></div>
								<div class="h-2 w-32 bg-slate-100 dark:bg-slate-900 rounded mt-2 animate-pulse"></div>
							</td>
							<td class="px-4 py-3"><div class="h-3 w-24 bg-slate-200 dark:bg-slate-800 rounded animate-pulse"></div></td>
							<td class="px-4 py-3"><div class="h-5 w-16 bg-slate-200 dark:bg-slate-800 rounded-full animate-pulse"></div></td>
							<td class="px-4 py-3"><div class="h-3 w-6 bg-slate-200 dark:bg-slate-800 rounded animate-pulse mx-auto"></div></td>
							<td class="px-4 py-3"></td>
						</tr>
					{/each}
				{:else if rows.length === 0 && !listError}
					<tr>
						<td colspan="7" class="px-4 py-12 text-center text-sm text-slate-500">
							{#if hasActiveFilters}
								Tidak ada notifikasi yang cocok dengan filter yang dipilih.
							{:else}
								Tidak ada notifikasi dalam outbox.
							{/if}
						</td>
					</tr>
				{:else}
					{#each rows as r (r.id)}
						<tr
							class="border-b border-slate-100 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-slate-900/50"
						>
							<td class="px-4 py-3">
								<div class="font-medium">{relativeTime(r.created_at)}</div>
								<div class="text-[10px] text-slate-400 font-mono">{r.id.slice(0, 8)}…</div>
							</td>
							<td class="px-4 py-3">
								<span class="font-mono text-xs uppercase">{r.channel}</span>
							</td>
							<td class="px-4 py-3">
								<div class="font-mono text-xs">{r.template_key}</div>
								<div class="text-[10px] text-slate-400 mt-0.5 max-w-md truncate" title={r.subject}>
									{r.subject}
								</div>
							</td>
							<td class="px-4 py-3 font-mono text-xs">
								<div>{r.recipient_contact_masked}</div>
								<div class="text-[10px] text-slate-400">{r.recipient_type}</div>
							</td>
							<td class="px-4 py-3">
								<span
									class={`inline-block rounded-full text-[10px] px-2 py-0.5 font-medium ${statusClasses(r.status)}`}
								>
									{r.status}
								</span>
								{#if r.last_error_code}
									<div class="text-[10px] text-red-600 dark:text-red-400 mt-0.5">
										{r.last_error_code}
									</div>
								{/if}
							</td>
							<td class="px-4 py-3 text-center">{r.attempt_count}</td>
							<td class="px-4 py-3 text-right">
								{#if r.status === 'failed' || r.status === 'pending'}
									<button
										type="button"
										onclick={() => callAction(r.id, 'retry')}
										disabled={busyId === `retry:${r.id}`}
										class="text-xs rounded px-2 py-0.5 text-emerald-600 hover:bg-emerald-50 dark:hover:bg-emerald-950 border border-slate-200 dark:border-slate-700 disabled:opacity-50"
									>
										Retry
									</button>
								{/if}
								{#if r.status === 'pending' || r.status === 'failed'}
									<button
										type="button"
										onclick={() => callAction(r.id, 'cancel')}
										disabled={busyId === `cancel:${r.id}`}
										class="ml-2 text-xs rounded px-2 py-0.5 text-red-600 hover:bg-red-50 dark:hover:bg-red-950 border border-slate-200 dark:border-slate-700 disabled:opacity-50"
									>
										Cancel
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
	</div>
</div>
