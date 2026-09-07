<script lang="ts">
	import type { ActionData } from './$types';

	export let form: ActionData;
</script>

<svelte:head>
	<title>Daftar — Sigap</title>
</svelte:head>

<div class="max-w-md mx-auto mt-12 px-6">
	<h1 class="text-2xl font-semibold tracking-tight">Daftar</h1>

	{#if form?.message}
		<p role="alert" class="mt-3 text-sm rounded-lg px-3 py-2 bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-400">
			{form.message}
		</p>
	{/if}

	<form method="POST" class="mt-6 space-y-4">
		<div>
			<label for="email" class="block text-sm font-medium text-slate-700 dark:text-slate-300">Email</label>
			<input
				id="email"
				name="email"
				type="email"
				autocomplete="email"
				required
				class="mt-1 w-full rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-emerald-500"
			/>
		</div>
		<div>
			<label for="password" class="block text-sm font-medium text-slate-700 dark:text-slate-300">Kata sandi</label>
			<input
				id="password"
				name="password"
				type="password"
				autocomplete="new-password"
				required
				minlength="8"
				class="mt-1 w-full rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-emerald-500"
			/>
		</div>
		<div>
			<label for="confirm" class="block text-sm font-medium text-slate-700 dark:text-slate-300">Konfirmasi kata sandi</label>
			<input
				id="confirm"
				name="confirm"
				type="password"
				autocomplete="new-password"
				required
				minlength="8"
				class="mt-1 w-full rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-emerald-500"
			/>
		</div>
		<button
			type="submit"
			class="w-full rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700"
		>
			Daftar
		</button>
	</form>

	<p class="mt-4 text-sm text-slate-500">
		Sudah punya akun?
		<a href="/auth/login" class="text-emerald-700 dark:text-emerald-400 hover:underline">Masuk</a>
	</p>
</div>
