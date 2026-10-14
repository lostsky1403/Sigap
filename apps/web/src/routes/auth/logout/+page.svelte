<script lang="ts">
	import Button from '$lib/ui/Button.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { LogOut } from 'lucide-svelte';
</script>

<svelte:head>
	<title>Keluar — Sigap</title>
</svelte:head>

<div class="sigap-logout">
	<section class="sigap-logout__panel" style:border-radius={RADIUS.panel}>
		<h1 class="sigap-logout__title">Keluar dari akun</h1>
		<!--
			The old page said "Sedang keluar..." and nothing else, which is a
			literal misstatement: this route's load function does nothing and the
			action only runs on POST, so loading the page signed nobody out. It
			said it was doing something it was not doing.

			So the page now says what is true and offers the action. The form
			posts to this same route, which is the frozen action - no new
			endpoint, no client-side session handling, and the server remains the
			only thing that can end a session.
		-->
		<p class="sigap-logout__body">
			Anda akan keluar dari akun ini pada perangkat ini. Janji temu dan nomor antrean yang
			sudah Anda buat tidak akan hilang.
		</p>

		<form method="POST" class="sigap-logout__actions">
			<Button type="submit" icon={LogOut}>Keluar</Button>
			<a class="sigap-logout__cancel" href="/">Batal</a>
		</form>
	</section>
</div>

<style>
	.sigap-logout {
		max-width: 440px;
		margin: 0 auto;
		padding: 24px 16px 8px;
	}

	@media (min-width: 768px) {
		.sigap-logout {
			padding: 40px 24px 8px;
		}
	}

	.sigap-logout__panel {
		padding: 20px 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-logout__title {
		margin: 0;
		font-size: 20px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-logout__body {
		margin: 8px 0 0;
		font-size: 14px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-logout__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
		margin-top: 20px;
	}

	.sigap-logout__cancel {
		display: inline-flex;
		align-items: center;
		/* Citizen touch floor, matching the primary button beside it. */
		min-height: 44px;
		padding: 0 16px;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-logout__cancel:hover {
		text-decoration: underline;
	}

	.sigap-logout__cancel:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
