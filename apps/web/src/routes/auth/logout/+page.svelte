<script lang="ts">
	import Button from '$lib/ui/Button.svelte';
	import AuthPanel from '$lib/citizen/AuthPanel.svelte';
	import { LogOut } from 'lucide-svelte';
</script>

<svelte:head>
	<title>Keluar — Sigap</title>
</svelte:head>

<AuthPanel title="Keluar dari akun">
	<!--
		The old page said "Sedang keluar..." and nothing else, which is a
		literal misstatement: this route's load function does nothing and the
		action only runs on POST, so loading the page signed nobody out. It
		said it was doing something it was not doing.

		So the page now says what is true and offers the action. The form posts
		to this same route, which is the frozen action — no new endpoint, no
		client-side session handling, and the server remains the only thing that
		can end a session.
	-->
	<p class="sigap-logout__body">
		Anda akan keluar dari akun ini pada perangkat ini. Janji temu dan nomor antrean yang sudah
		Anda buat tidak akan hilang.
	</p>

	<form method="POST" class="sigap-logout__actions">
		<Button type="submit" icon={LogOut}>Keluar</Button>
		<a class="sigap-logout__cancel" href="/">Batal</a>
	</form>
</AuthPanel>

<style>
	.sigap-logout__body {
		margin: 0;
		font-size: var(--sigap-type-body);
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-logout__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sigap-space-3);
		margin-top: var(--sigap-space-5);
	}

	.sigap-logout__cancel {
		display: inline-flex;
		align-items: center;
		/* Citizen touch floor, matching the primary button beside it. */
		min-height: 44px;
		padding: 0 var(--sigap-space-4);
		font-size: var(--sigap-type-body);
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
