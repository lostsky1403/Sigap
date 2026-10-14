<script lang="ts">
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Select from '$lib/ui/Select.svelte';
	import { DENSITY } from '$lib/ui/density';
	import { RADIUS } from '$lib/design/tokens';
	import {
		blankFacility,
		isFacilityFormValid,
		validateFacilityForm,
		type FacilityFormErrors,
		type FacilityFormValues
	} from './facilityForm';
	import type { AdminFacility } from '$lib/api/types/api';

	/**
	 * T-3B5-04: the facility create/edit form.
	 *
	 * The fields are exactly the ones `CreateFacilityRequest` declares. None are
	 * invented and none are dropped: a form with a field the API ignores is
	 * worse than a missing one, because the operator believes it was saved.
	 *
	 * `available_beds` is included because the API's create request carries it.
	 * The previous client `FacilityInput` type omitted it, which would have made
	 * every created facility fall back to the column default for a value the
	 * operator had just typed.
	 */
	export let facility: AdminFacility | null = null;
	export let busy: boolean = false;
	export let submitError: string = '';
	export let onSubmit: (values: FacilityFormValues) => void = () => {};
	export let onCancel: () => void = () => {};
	/**
	 * Hide the form's own action row.
	 *
	 * T-3B5-04. The editor is mounted as the BODY of `AdminConfirmDialog`, which
	 * already renders a Batal/Simpan pair in its footer. Keeping both gives the
	 * operator two Save buttons and two Cancel buttons inside one modal, and the
	 * two do not even mean the same thing: the form's submit does not close the
	 * dialog, while the footer's confirm both submits and lets the page close it
	 * on success. That ambiguity is a defect regardless of how the buttons are
	 * clicked, and it is invisible in a screenshot — it needs two of each
	 * control to notice.
	 *
	 * The action row is kept for the standalone case (`hideActions=false`), so
	 * the form is still usable outside a dialog without becoming a second
	 * implementation of submission.
	 */
	export let hideActions = false;

	/**
	 * Seeded from the row, so an edit starts from what the server holds.
	 *
	 * This is data display, not an authorization question: the row is in the
	 * operator's read set, and its contents are facts about the facility.
	 */
	function fromFacility(row: AdminFacility): FacilityFormValues {
		return {
			name: row.name ?? '',
			type: (row.type ?? '') as FacilityFormValues['type'],
			address: row.address ?? '',
			kecamatan: row.kecamatan ?? '',
			kabupaten_kota: row.kabupaten_kota ?? '',
			provinsi: row.provinsi ?? '',
			phone: row.phone ?? '',
			total_beds: String(row.total_beds ?? 0),
			available_beds: String(row.available_beds ?? 0),
			short_code: row.short_code ?? ''
		};
	}

	let values: FacilityFormValues = facility ? fromFacility(facility) : blankFacility();
	let errors: FacilityFormErrors = {};
	let submitted = false;

	// Keyed on the id, for the same reason as ScheduleEditor: reacting to the
	// object and writing `values` inside would be a reactive cycle.
	let seededFor: string | null = facility?.id ?? null;
	$: if ((facility?.id ?? null) !== seededFor) {
		seededFor = facility?.id ?? null;
		values = facility ? fromFacility(facility) : blankFacility();
		errors = {};
		submitted = false;
	}

	// Create validates the full required set; update only what the PATCH
	// handler enforces, so a rename is not blocked by four untouched address
	// fields the server never asked for.
	$: mode = facility ? ('update' as const) : ('create' as const);
	$: liveErrors = validateFacilityForm(values, mode);
	$: shownErrors = submitted ? liveErrors : {};

	export function submit() {
		submitted = true;
		if (!isFacilityFormValid(values, mode)) return;
		onSubmit(values);
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		submit();
	}
</script>

<form class="sigap-facility-editor" on:submit={handleSubmit} novalidate>
	<Field
		id="facility-name"
		label="Nama fasilitas"
		required
		error={shownErrors.name ?? ''}
		let:controlId
		let:describedBy
		let:invalid
	>
		<Input
			id={controlId}
			{describedBy}
			{invalid}
			size={DENSITY.adminComfortable}
			bind:value={values.name}
		/>
	</Field>

	<Field
		id="facility-type"
		label="Tipe"
		required
		error={shownErrors.type ?? ''}
		let:controlId
		let:describedBy
		let:invalid
	>
		<Select
			id={controlId}
			{describedBy}
			{invalid}
			size={DENSITY.adminComfortable}
			bind:value={values.type}
			placeholder="Pilih tipe"
			options={[
				{ value: 'rumah_sakit', label: 'Rumah sakit' },
				{ value: 'puskesmas', label: 'Puskesmas' }
			]}
		/>
	</Field>

	<Field
		id="facility-address"
		label="Jalan"
		required={mode === 'create'}
		error={shownErrors.address ?? ''}
		let:controlId
		let:describedBy
		let:invalid
	>
		<Input
			id={controlId}
			{describedBy}
			{invalid}
			size={DENSITY.adminComfortable}
			bind:value={values.address}
		/>
	</Field>

	<div class="sigap-facility-editor__pair">
		<Field
			id="facility-kecamatan"
			label="Kecamatan"
			required={mode === 'create'}
			let:controlId
			let:describedBy
			let:invalid
		>
			<Input
				id={controlId}
				{describedBy}
				{invalid}
				size={DENSITY.adminComfortable}
				bind:value={values.kecamatan}
			/>
		</Field>

		<Field
			id="facility-kabupaten"
			label="Kabupaten/Kota"
			required={mode === 'create'}
			let:controlId
			let:describedBy
			let:invalid
		>
			<Input
				id={controlId}
				{describedBy}
				{invalid}
				size={DENSITY.adminComfortable}
				bind:value={values.kabupaten_kota}
			/>
		</Field>
	</div>

	<Field
		id="facility-provinsi"
		label="Provinsi"
		required={mode === 'create'}
		let:controlId
		let:describedBy
		let:invalid
	>
		<Input
			id={controlId}
			{describedBy}
			{invalid}
			size={DENSITY.adminComfortable}
			bind:value={values.provinsi}
		/>
	</Field>

	<Field
		id="facility-phone"
		label="Nomor telepon"
		required={mode === 'create'}
		error={shownErrors.phone ?? ''}
		helper="Karakter < > &quot; ' &amp; tidak diperbolehkan."
		let:controlId
		let:describedBy
		let:invalid
	>
		<Input
			id={controlId}
			{describedBy}
			{invalid}
			type="tel"
			size={DENSITY.adminComfortable}
			bind:value={values.phone}
		/>
	</Field>

	<div class="sigap-facility-editor__pair">
		<Field
			id="facility-total-beds"
			label="Total tempat tidur"
			error={shownErrors.total_beds ?? ''}
			let:controlId
			let:describedBy
			let:invalid
		>
			<Input
				id={controlId}
				{describedBy}
				{invalid}
				type="text"
				size={DENSITY.adminComfortable}
				inputmode="numeric"
				bind:value={values.total_beds}
			/>
		</Field>

		<Field
			id="facility-available-beds"
			label="Tempat tidur tersedia"
			error={shownErrors.available_beds ?? ''}
			let:controlId
			let:describedBy
			let:invalid
		>
			<Input
				id={controlId}
				{describedBy}
				{invalid}
				type="text"
				size={DENSITY.adminComfortable}
				inputmode="numeric"
				bind:value={values.available_beds}
			/>
		</Field>
	</div>

	<Field
		id="facility-short-code"
		label="Kode singkat"
		required={mode === 'create'}
		error={shownErrors.short_code ?? ''}
		helper="Dipakai pada nomor antrean, misalnya PMK-0001."
		let:controlId
		let:describedBy
		let:invalid
	>
		<Input
			id={controlId}
			{describedBy}
			{invalid}
			type="text"
			size={DENSITY.adminComfortable}
			bind:value={values.short_code}
		/>
	</Field>

	{#if submitError}
		<p class="sigap-facility-editor__error" role="alert">{submitError}</p>
	{/if}

	{#if !hideActions}
		<div class="sigap-facility-editor__actions">
			<button
				type="button"
				class="sigap-facility-editor__cancel"
				style:border-radius={RADIUS.control}
				on:click={onCancel}
			>
				Batal
			</button>
			<button
				type="submit"
				class="sigap-facility-editor__submit"
				style:border-radius={RADIUS.control}
				disabled={busy}
			>
				{busy ? 'Menyimpan...' : 'Simpan'}
			</button>
		</div>
	{/if}
</form>

<style>
	.sigap-facility-editor {
		display: flex;
		flex-direction: column;
		gap: 14px;
	}

	.sigap-facility-editor__pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 12px;
	}

	.sigap-facility-editor__error {
		margin: 0;
		font-size: 13px;
		line-height: 1.45;
		color: var(--sigap-danger);
	}

	.sigap-facility-editor__actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		justify-content: flex-end;
	}

	.sigap-facility-editor__cancel,
	.sigap-facility-editor__submit {
		height: 40px;
		padding: 0 16px;
		font: inherit;
		font-size: 14px;
		font-weight: 500;
		cursor: pointer;
	}

	.sigap-facility-editor__cancel {
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-facility-editor__submit {
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		border: 1px solid var(--sigap-primary);
	}

	.sigap-facility-editor__submit:disabled {
		background-color: var(--sigap-muted);
		border-color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-facility-editor__cancel:focus-visible,
	.sigap-facility-editor__submit:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
