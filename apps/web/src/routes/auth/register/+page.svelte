<script lang="ts">
	import AuthForms from '$lib/citizen/AuthForms.svelte';
	import AuthPanel from '$lib/citizen/AuthPanel.svelte';
	import type { ActionData } from './$types';

	/**
	 * Presentation only.
	 *
	 * The `+page.server.ts` action this page posts to is frozen. It validates
	 * that both fields are filled, that the password is at least eight
	 * characters, and that the confirmation matches, then signs up and
	 * redirects to `/auth/login?registered=1`. This file renders whatever that
	 * action decided and decides nothing itself.
	 *
	 * The approved frame draws two more controls than this form renders: a
	 * `Nama lengkap` field and a health-data consent checkbox. Neither can be
	 * honoured by the frozen action, which reads `email`, `password`, and
	 * `confirm` and nothing else — so both are withheld rather than rendered
	 * as decoration. A name input the action discards is silent data loss, and
	 * a consent checkbox nothing validates or stores is worse than no
	 * checkbox: it presents itself as a recorded consent for health data and
	 * records nothing. Both need a backend change, which is out of scope here.
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

<AuthPanel
	title="Daftar"
	description="Buat akun untuk memantau status kunjungan dan janji temu Anda."
>
	<AuthForms mode="register" {message} />

	<div slot="footer">
		Sudah punya akun?
		<a href="/auth/login">Masuk</a>
	</div>

	<!--
		Set expectations before the field is filled, not after. The action
		redirects to the login page the moment sign-up succeeds, so the person
		arriving at an inbox for a verification email needs to have been told
		to expect it. It sits outside the card because it is a caveat about the
		flow rather than a field of the form.
	-->
	<p slot="note">
		Setelah mendaftar, Anda kembali ke halaman masuk; periksa email bila verifikasi diaktifkan.
	</p>
</AuthPanel>
