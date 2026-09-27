<script lang="ts">
	import AuthForms from '$lib/citizen/AuthForms.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import type { ActionData } from './$types';

	/**
	 * Presentation only.
	 *
	 * The `+page.server.ts` action this page posts to is frozen. It validates
	 * that both fields are filled, that the password is at least eight
	 * characters, and that the confirmation matches, then signs up and
	 * redirects to `/auth/login?registered=1`. This file renders whatever that
	 * action decided and decides nothing itself.
	 */

	export let form: ActionData;

	// SvelteKit passes the page only the `fail(status, data)` payload, so the
	// message is the whole signal available. See the login page for the same
	// reasoning; AuthForms owns the classification.
	$: message = form?.message ?? '';
</script>

<svelte:head>
	<title>Daftar — Sigap</title>
</svelte:head>

<div class="sigap-auth">
	<section class="sigap-auth__panel" style:border-radius={RADIUS.panel}>
		<h1 class="sigap-auth__title">Daftar</h1>
		<p class="sigap-auth__subtitle">Buat akun untuk masuk.</p>

		<div class="sigap-auth__body">
			<AuthForms mode="register" {message} />
		</div>
	</section>

	<p class="sigap-auth__alt">
		Sudah punya akun?
		<a class="sigap-auth__alt-link" href="/auth/login">Masuk</a>
	</p>

	<!--
		Set expectations before the field is filled, not after. The action
		redirects to the login page the moment sign-up succeeds, so the person
		arriving at an inbox for a verification email needs to have been told
		to expect it.
	-->
	<p class="sigap-auth__note">
		Setelah mendaftar, Anda kembali ke halaman masuk; periksa email bila verifikasi diaktifkan.
	</p>
</div>

<style>
	.sigap-auth {
		max-width: 440px;
		margin: 0 auto;
		padding: 24px 16px 8px;
	}

	@media (min-width: 768px) {
		.sigap-auth {
			padding: 40px 24px 8px;
		}
	}

	.sigap-auth__panel {
		padding: 20px 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-auth__title {
		margin: 0;
		font-size: 20px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-auth__subtitle {
		margin: 4px 0 0;
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-auth__body {
		margin-top: 20px;
	}

	.sigap-auth__alt {
		margin: 16px 0 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-auth__alt-link {
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-auth__alt-link:hover {
		text-decoration: underline;
	}

	.sigap-auth__alt-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-auth__note {
		margin: 12px 0 0;
		font-size: 12px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}
</style>
