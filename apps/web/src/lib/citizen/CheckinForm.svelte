<script lang="ts">
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

	function handleSubmit() {
		attempted = true;
		if (validateRequired(appointmentId, 'ID janji temu')) return;
		if (validateRequired(checkinCode, 'Kode check-in')) return;
		onSubmit();
	}
</script>

<form class="sigap-checkin-form" novalidate on:submit|preventDefault={handleSubmit}>
	<Field id="sigap-checkin-appointment_id" label="ID janji temu" required error={appointmentError}>
		<Input
			id="sigap-checkin-appointment_id"
			bind:value={appointmentId}
			describedBy={appointmentError ? 'sigap-checkin-appointment_id-error' : ''}
			invalid={Boolean(appointmentError)}
			placeholder="ID janji temu"
			autocomplete="off"
		/>
	</Field>

	<Field id="sigap-checkin-checkin_code" label="Kode check-in" required error={codeError}>
		<Input
			id="sigap-checkin-checkin_code"
			bind:value={checkinCode}
			describedBy={codeError ? 'sigap-checkin-checkin_code-error' : ''}
			invalid={Boolean(codeError)}
			placeholder="Kode check-in"
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
		gap: 16px;
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
