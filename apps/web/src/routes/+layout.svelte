<script lang="ts">
	import '../app.css';
</script>

<div class="min-h-screen bg-white text-slate-900 dark:bg-slate-950 dark:text-slate-100">
	<header class="border-b border-slate-200 dark:border-slate-800">
		<div class="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
			<div class="flex items-center gap-3">
				<div class="font-semibold tracking-tight text-xl">Sigap</div>
				<div class="text-xs px-2 py-0.5 rounded bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-400">MVP</div>
			</div>
			<div class="text-sm text-slate-500">Layanan Kesehatan Daerah</div>
		</div>
	</header>

	<main class="max-w-6xl mx-auto">
		<slot />
	</main>

	<footer class="mt-16 border-t border-slate-200 dark:border-slate-800 py-6 text-center text-xs text-slate-500">
		Sigap — Open Source Civic-Tech • Data contoh untuk scaffolding
	</footer>
</div>
