<script lang="ts">
	import { page } from '$app/stores';
	import AuthForms from '$lib/citizen/AuthForms.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import type { ActionData } from './$types';

	/**
	 * Presentation only.
	 *
	 * The `+page.server.ts` action this page posts to is frozen: it owns every
	 * credential decision, the 503 when Supabase is unconfigured, the 401 on a
	 * rejected password, and the 303 to `/` on success. Nothing here may
	 * pre-empt any of that, so this file does no validation and no fetching. It
	 * reads the action's own outcome and hands it to AuthForms.
	 */

	export let form: ActionData;

	/**
	 * The action's outcome.
	 *
	 * SvelteKit hands the page only the payload of `fail(status, data)`, so
	 * `form.message` is all that is available here. The status code exists on
	 * the response and not on `form`, which is why AuthForms classifies from
	 * the wording rather than from a number. That is a constraint of the
	 * frozen action contract, not a shortcut.
	 */
	$: message = form?.message ?? '';

	/**
	 * The redirect target after a successful registration.
	 *
	 * The action sends people to `/auth/login?registered=1`, so the login page
	 * has to recognise it. `has` rather than an equality check on `1`, because
	 * the query is a flag and `?registered` and `?registered=0` are the same
	 * statement.
	 */
	$: registered = $page.url.searchParams.has('registered');
</script>

<svelte:head>
	<title>Masuk — Sigap</title>
</svelte:head>

<div class="sigap-auth">
	<section class="sigap-auth__panel" style:border-radius={RADIUS.panel}>
		<h1 class="sigap-auth__title">Masuk</h1>
		<p class="sigap-auth__subtitle">Kembali ke Sigap.</p>

		<div class="sigap-auth__body">
			<AuthForms mode="login" {message} {registered} />
		</div>
	</section>

	<p class="sigap-auth__alt">
		Belum punya akun?
		<a class="sigap-auth__alt-link" href="/auth/register">Daftar</a>
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
</style>
