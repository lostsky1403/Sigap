<script lang="ts">
	import { page } from '$app/stores';
	import AuthForms from '$lib/citizen/AuthForms.svelte';
	import AuthPanel from '$lib/citizen/AuthPanel.svelte';
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

<AuthPanel
	title="Masuk"
	description="Masuk untuk mengelola fasilitas dan antrean."
>
	<AuthForms mode="login" {message} {registered} />

	<div slot="footer">
		Belum punya akun?
		<a href="/auth/register">Daftar</a>
	</div>
</AuthPanel>
