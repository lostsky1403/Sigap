<script lang="ts">
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Select, { type Option } from '$lib/ui/Select.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { validateName, validatePhone, validateRequired } from '$lib/citizen/citizenFlow';

	/**
	 * The walk-in form.
	 *
	 * Three fields, no appointment and no code. That is the whole difference
	 * from the check-in form, and it is a real difference in the backend too:
	 * this one registers a new queue ticket, while check-in redeems a code
	 * against a booking that already exists.
	 *
	 * The facility list is passed in rather than fetched here, so the page owns
	 * the loading and error states and this component stays a form. That also
	 * means the facility options are the same verified public catalog rows the
	 * rest of the citizen UI uses, with the same type labels.
	 */
	export let facilityOptions: Option[] = [];
	export let facilityId: string = '';
	export let fullName: string = '';
	export let phone: string = '';
	export let submitting: boolean = false;
	export let disabled: boolean = false;
	export let errors: Record<string, string> = {};
	export let onSubmit: () => void = () => {};

	/**
	 * Validation messages appear only after a submit attempt, so the form is
	 * not a wall of red before the citizen has done anything wrong. The
	 * validation still gates the request; this only decides visibility.
	 */
	let attempted = false;

	$: facilityError = errors.facility_id ?? (attempted ? validateRequired(facilityId, 'Fasilitas') : '');
	$: nameError = errors.fullName ?? (attempted ? validateName(fullName) : '');
	$: phoneError = errors.phone ?? (attempted ? validatePhone(phone) : '');

	function handleSubmit() {
		attempted = true;
		if (validateRequired(facilityId, 'Fasilitas')) return;
		if (validateName(fullName)) return;
		if (validatePhone(phone)) return;
		onSubmit();
	}
</script>

<form class="sigap-walkin-form" novalidate on:submit|preventDefault={handleSubmit}>
	<Field
		id="sigap-walkin-facilityId"
		label="Fasilitas"
		required
		error={facilityError}
		helper={disabled ? 'Memuat daftar fasilitas...' : ''}
	>
		<Select
			id="sigap-walkin-facilityId"
			bind:value={facilityId}
			options={facilityOptions}
			placeholder="Pilih fasilitas..."
			describedBy={facilityError ? 'sigap-walkin-facilityId-error' : ''}
			invalid={Boolean(facilityError)}
			required
			{disabled}
		/>
	</Field>

	<Field id="sigap-walkin-fullName" label="Nama lengkap" required error={nameError}>
		<Input
			id="sigap-walkin-fullName"
			autocomplete="name"
			bind:value={fullName}
			describedBy={nameError ? 'sigap-walkin-fullName-error' : ''}
			invalid={Boolean(nameError)}
			{disabled}
			required
		/>
	</Field>

	<Field
		id="sigap-walkin-phone"
		label="Nomor telepon"
		required
		error={phoneError}
		helper="10-15 digit angka."
	>
		<Input
			id="sigap-walkin-phone"
			type="tel"
			autocomplete="tel"
			inputmode="tel"
			bind:value={phone}
			describedBy={phoneError ? 'sigap-walkin-phone-error' : ''}
			invalid={Boolean(phoneError)}
			{disabled}
			required
		/>
	</Field>

	<Button type="submit" disabled={submitting || disabled} fullWidth>
		{submitting ? 'Memproses...' : 'Ambil Nomor Antrean'}
	</Button>
</form>

<style>
	.sigap-walkin-form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
</style>
