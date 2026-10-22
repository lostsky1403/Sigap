<script lang="ts">
	import { tick } from 'svelte';
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { validateRequired } from '$lib/citizen/citizenFlow';

	/**
	 * The check-in form.
	 *
	 * Two fields, both required, and one of them is prefilled from the deep
	 * link. The appointment id is asked for rather than hidden because the
	 * public check-in route is legitimately code-keyed, not session-keyed: a
	 * citizen who bookmarks the page or opens it on another device still has a
	 * way in, and the deep link is a convenience rather than the only door.
	 *
	 * Bound with `bind:value` rather than a callback prop. The parent needs to
	 * seed these from the query string and read them on submit, and two-way
	 * binding expresses that without a pair of one-way callbacks that can drift
	 * out of sync with the inputs they are supposed to mirror.
	 *
	 * Both fields are `autocomplete="off"`. The code in particular: a browser
	 * autofilling a check-in code from an unrelated stored value would turn a
	 * typo into a wrong-code rejection with no visible cause, which is the most
	 * confusing way this page can fail.
	 */
	export let appointmentId: string = '';
	export let checkinCode: string = '';
	export let submitting: boolean = false;
	export let errors: Record<string, string> = {};
	export let onSubmit: () => void = () => {};

	// Shown only after a submit attempt, so a pristine form is not a wall of
	// red. `validateRequired` still gates the request; this decides whether the
	// message is visible yet.
	let attempted = false;

	$: appointmentError =
		errors.appointment_id ?? (attempted ? validateRequired(appointmentId, 'ID janji temu') : '');
	$: codeError = errors.checkin_code ?? (attempted ? validateRequired(checkinCode, 'Kode check-in') : '');

	async function handleSubmit() {
		attempted = true;

		// Focus the first invalid control once its error has rendered, so a
		// keyboard user is taken to the problem instead of being left on the
		// submit button with two messages somewhere above them.
		const firstInvalidId = validateRequired(appointmentId, 'ID janji temu')
			? 'sigap-checkin-appointment_id'
			: validateRequired(checkinCode, 'Kode check-in')
				? 'sigap-checkin-checkin_code'
				: null;

		if (firstInvalidId) {
			await tick();
			document.getElementById(firstInvalidId)?.focus();
			return;
		}

		onSubmit();
	}
</script>

<form class="sigap-checkin-form" novalidate on:submit|preventDefault={handleSubmit}>
	<!--
		The helpers are the approved copy and they carry real information: where
		the appointment id comes from, and the exact shape of the code. Without
		them a citizen who cannot find the id has nothing to go on, and one who
		types a five-character code learns nothing until it fails.

		The label names the source because "ID janji temu" alone does not tell a
		citizen holding only a six-character code that a long identifier exists
		at all, and the helper repeats the exact screen and section: a generic
		"check your booking" would leave them searching the confirmation page
		for something it calls appointment_id. The previous helper said "dari
		email", which is false here — notification_outbox rows stay at
		status=pending on the dev channel, so nothing is ever delivered.
	-->
	<Field
		id="sigap-checkin-appointment_id"
		label="ID janji temu (dari halaman konfirmasi)"
		helper="Ada di halaman konfirmasi janji temu Anda, di bagian 'Detail teknis' — baris appointment_id. Salin seluruh nilainya."
		required
		error={appointmentError}
	>
		<Input
			id="sigap-checkin-appointment_id"
			bind:value={appointmentId}
			describedBy={appointmentError ? 'sigap-checkin-appointment_id-error' : ''}
			invalid={Boolean(appointmentError)}
			placeholder="Tempel ID dari halaman konfirmasi"
			autocomplete="off"
		/>
	</Field>

	<Field
		id="sigap-checkin-checkin_code"
		label="Kode check-in"
		helper="6 karakter huruf dan angka, misalnya 4F8K2Q."
		required
		error={codeError}
	>
		<Input
			id="sigap-checkin-checkin_code"
			bind:value={checkinCode}
			describedBy={codeError ? 'sigap-checkin-checkin_code-error' : ''}
			invalid={Boolean(codeError)}
			placeholder="Contoh: 4F8K2Q"
			autocomplete="off"
		/>
	</Field>

	<Button type="submit" disabled={submitting} fullWidth>
		{submitting ? 'Memproses...' : 'Check-In'}
	</Button>
</form>

<style>
	.sigap-checkin-form {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-4);
	}

	/*
		Monospaced so a six-character code reads as a code. Scoped to this form
		rather than global: only these fields hold codes, and monospacing every
		text input on a civic page would be noise.
	*/
	.sigap-checkin-form :global(#sigap-checkin-checkin_code) {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		letter-spacing: 0.08em;
	}
</style>
