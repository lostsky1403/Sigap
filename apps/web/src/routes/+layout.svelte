<script lang="ts">
	import '../app.css';
	import type { LayoutData } from './$types';

	export let data: LayoutData;
</script>

<div class="min-h-screen bg-white text-slate-900 dark:bg-slate-950 dark:text-slate-100">
	<header class="border-b border-slate-200 dark:border-slate-800">
		<div class="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between gap-4">
			<div class="flex items-center gap-3 shrink-0">
				<a href="/" class="font-semibold tracking-tight text-xl">Sigap</a>
				<div class="text-xs px-2 py-0.5 rounded bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400">MVP</div>
			</div>
			<nav class="hidden md:flex items-center gap-1 text-sm" aria-label="Navigasi utama">
				<a href="/" class="px-3 py-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-900">Beranda</a>
				<a href="/appointments/new" class="px-3 py-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-900">Buat Janji Temu</a>
				<a href="/appointments/check-in" class="px-3 py-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-900">Check-In</a>
				<a href="/patient/status" class="px-3 py-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-900">Status Kunjungan</a>
				<a href="/admin/queues" class="px-3 py-2 rounded-lg text-emerald-700 dark:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-950">Admin</a>
			</nav>
			<div class="flex items-center gap-3 text-sm shrink-0">
				{#if data.userEmail}
					<span class="hidden sm:block text-slate-500 truncate max-w-[16rem]" title={data.userEmail}>
						{data.userEmail}
					</span>
					<form method="POST" action="/auth/logout">
						<button
							type="submit"
							class="px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-900"
						>
							Keluar
						</button>
					</form>
				{:else}
					<a href="/auth/login" class="px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-900">
						Masuk
					</a>
					<a
						href="/auth/register"
						class="px-3 py-1.5 rounded-lg bg-emerald-600 text-white hover:bg-emerald-700"
					>
						Daftar
					</a>
				{/if}
			</div>
		</div>
	</header>

	<main class="max-w-6xl mx-auto">
		<slot />
	</main>

	<footer class="mt-16 border-t border-slate-200 dark:border-slate-800 py-6 text-center text-xs text-slate-500">
		Sigap — Open Source Civic-Tech • Data contoh untuk scaffolding
	</footer>
</div>
