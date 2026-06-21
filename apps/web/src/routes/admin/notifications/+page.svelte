<script lang="ts">
	import { onMount } from 'svelte';

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

	let rows: NotificationRow[] = [];
	let error = '';
	let busyId: string | null = null;
	let mounted = false;

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

	async function loadNotifications() {
		try {
			const res = await fetch('/api/v1/admin/notifications?limit=200', {
				headers: { Accept: 'application/json' }
			});
			if (!res.ok) {
				error = `Gagal memuat outbox (HTTP ${res.status}).`;
				rows = [];
				return;
			}
			const body = await res.json();
			rows = (body?.data ?? []) as NotificationRow[];
			error = '';
		} catch (e) {
			error = `Kesalahan jaringan: ${(e as Error).message}`;
			rows = [];
		}
	}

	async function callAction(id: string, op: 'retry' | 'cancel') {
		busyId = `${op}:${id}`;
		try {
			const res = await fetch(`/api/v1/admin/notifications/${id}/${op}`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' }
			});
			if (!res.ok) {
				error = `Aksi ${op} gagal (HTTP ${res.status}).`;
				return;
			}
			error = '';
			await loadNotifications();
		} catch (e) {
			error = `Kesalahan jaringan: ${(e as Error).message}`;
		} finally {
			busyId = null;
		}
	}

	onMount(() => {
		mounted = true;
		loadNotifications();
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
			onclick={loadNotifications}
			class="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700"
		>
			Muat ulang
		</button>
	</div>

	{#if error}
		<div
			class="mb-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
		>
			{error}
		</div>
	{/if}

	{#if !mounted}
		<div class="py-12 text-center text-sm text-slate-500">Memuat…</div>
	{:else if rows.length === 0 && !error}
		<div class="py-12 text-center text-sm text-slate-500">
			<div class="mb-2">Tidak ada notifikasi dalam outbox.</div>
			<button onclick={loadNotifications} class="text-emerald-600 hover:underline text-sm">
				Muat ulang.
			</button>
		</div>
	{:else}
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
				</tbody>
			</table>
		</div>
	{/if}
</div>