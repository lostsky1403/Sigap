<script lang="ts">
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Select from '$lib/ui/Select.svelte';
	import { DENSITY } from '$lib/ui/density';
	import { RADIUS } from '$lib/design/tokens';
	import {
		isScheduleFormValid,
		slotCountFor,
		toScheduleRequest,
		validateScheduleForm,
		type ScheduleFormErrors,
		type ScheduleFormValues
	} from './scheduleForm';
	import type { AdminSchedule, ScheduleMutationOptions } from '$lib/api/types/api';

	/**
	 * T-3B5-03: the schedule create/edit form.
	 *
	 * THE OPTIONS ARE THE SERVER'S ANSWER, AND THE ONLY ONE THAT COUNTS.
	 *
	 * The facility list comes from `ScheduleMutationOptions`, not from the
	 * schedule read table. Those two look similar and are not: the read table is
	 * filtered by `schedule.read`, which an operator can hold at a facility
	 * without being able to change anything there. Deriving editable facilities
	 * from it would offer exactly the wrong set — most visibly for the mixed
	 * actor, who may READ facility A and may only MANAGE facility B. Offering A
	 * would produce a form that submits and is then refused with a 403, which is
	 * the worst possible outcome: the operator filled in a form the system
	 * invited them to fill in.
	 *
	 * There is deliberately no fallback to the read list. If the options call
	 * fails, the editor shows the failure; it does not substitute a guess.
	 *
	 * NO PRACTITIONER FIELD OF ANY KIND.
	 *
	 * Not a select (there is no practitioner catalog to select from), not a
	 * name (there is none to display), not a UUID (a machine identifier where a
	 * person belongs), and not free text (fabrication waiting to happen). The
	 * absence is the requirement, and `scheduleForm.toScheduleRequest` makes it
	 * structural: `practitioner_id` is not a property of the body it returns, so
	 * no edit to this component can put one on the wire.
	 */
	export let options: ScheduleMutationOptions | null = null;
	export let optionsLoading: boolean = false;
	export let editing: AdminSchedule | null = null;
	export let busy: boolean = false;
	export let submitError: string = '';
	export let onSubmit: (body: ReturnType<typeof toScheduleRequest>, id: string | null) => void =
		() => {};
	export let onCancel: () => void = () => {};
	/**
	 * Hide the form's own action row.
	 *
	 * T-3B5-03, for the same reason as `FacilityEditor.hideActions`: this form is
	 * mounted inside `AdminConfirmDialog`, which supplies its own Simpan/Batal
	 * pair in the footer. Two of each inside one modal is ambiguous — the form's
	 * submit does not close the dialog, the footer's confirm does — and it is
	 * invisible until you count the buttons.
	 *
	 * The footer confirm calls this component's `submit()` through `bind:this`,
	 * so validation still runs on the single remaining path. Keyboard submission
	 * of the form itself is unchanged.
	 */
	export let hideActions = false;

	function blank(): ScheduleFormValues {
		return {
			facility_id: '',
			service_unit_id: '',
			schedule_date: '',
			start_time: '',
			end_time: '',
			slot_minutes: '30',
			capacity_per_slot: '1'
		};
	}

	/**
	 * Seeded from the row being edited, so an update form starts from what the
	 * server actually holds rather than from what the operator remembers. The
	 * read list is the only place those persisted values come from — this is
	 * data display, not an authorization question.
	 */
	function fromSchedule(schedule: AdminSchedule): ScheduleFormValues {
		return {
			facility_id: schedule.facility_id,
			service_unit_id: schedule.service_unit_id,
			schedule_date: schedule.schedule_date,
			// `start_time` arrives as `HH:MM:SS` from Postgres `::text`. Trimming
			// the seconds keeps the value round-trippable: Go's strict check
			// accepts "09:00:00", but a native time input cannot hold it, so
			// sending the untrimmed form back would silently change nothing while
			// looking like a failed edit.
			start_time: schedule.start_time.slice(0, 5),
			end_time: schedule.end_time.slice(0, 5),
			slot_minutes: String(schedule.slot_minutes),
			capacity_per_slot: String(schedule.capacity_per_slot)
		};
	}

	let values: ScheduleFormValues = editing ? fromSchedule(editing) : blank();
	let errors: ScheduleFormErrors = {};
	let submitted = false;

	/** The id the form was last seeded from, so seeding is not re-entrant. */
	let seededFor: string | null = editing?.id ?? null;

	// Re-seed when the target changes, so opening a different row does not
	// leave the previous row's values in the form.
	//
	// KEYED ON A STRING, not on the object. Reacting to `editing` directly and
	// assigning `values` inside would be a cycle: the assignment invalidates
	// `values`, which re-runs the service-unit scoping below, which reads
	// `values`. Svelte rejects that outright, and it is right to — the result is
	// order-dependent and re-seeds the form whenever the cycle settles. Keying
	// on the id means this block can only re-run when the TARGET changed, so no
	// cycle exists.
	$: if (editing?.id !== undefined && editing?.id !== seededFor) {
		seededFor = editing?.id ?? null;
		values = editing ? fromSchedule(editing) : blank();
		errors = {};
		submitted = false;
	}

	/**
	 * Re-validating on every keystroke would put a red message under a field the
	 * operator is still typing into, which reads as an accusation. Messages
	 * appear once a submit has been attempted, and then track the field live.
	 */
	$: liveErrors = validateScheduleForm(values);
	$: shownErrors = submitted ? liveErrors : {};

	/**
	 * The service units offered are the ones nested under the SELECTED
	 * facility, and they come from the same server response. Choosing a
	 * facility therefore re-scopes the units automatically — a unit from another
	 * facility is never reachable, which is the same facility-scoped guarantee
	 * the facility list itself provides.
	 */
	$: facilities = options?.facilities ?? [];
	$: selectedFacility = facilities.find((facility) => facility.id === values.facility_id);
	$: serviceUnits = (selectedFacility?.service_units ?? []).map((unit) => ({
		value: unit.id,
		label: unit.name
	}));

	/**
	 * Changing facility invalidates any unit chosen under the previous one.
	 *
	 * Done in the event handler rather than as a reactive statement for the same
	 * reason as the seeding above: writing `values` from a `$:` that READS
	 * `values` is the cycle Svelte refuses. Here the write happens once, at the
	 * moment of the change, which is also when it is true — not on every re-render,
	 * where a unit belonging to another facility would be cleared again and again.
	 */
	function handleFacilityChange() {
		const stillOffered = (facilities.find((facility) => facility.id === values.facility_id)
			?.service_units ?? []).some((unit) => unit.id === values.service_unit_id);
		if (!stillOffered) {
			values = { ...values, service_unit_id: '' };
		}
	}

	/**
	 * A live preview of the slot count.
	 *
	 * Shown only when the arithmetic actually works, so it never displays a
	 * number for an impossible schedule. Its absence is itself the signal that
	 * something is wrong, and the submit button being disabled says so too.
	 */
	$: slotCount = slotCountFor(
		values.start_time,
		values.end_time,
		Number(values.slot_minutes)
	);

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		submit();
	}

	/**
	 * The imperative submit entry point, for a confirm button mounted OUTSIDE
	 * this form — which is the case here, because the dialog's footer lives in
	 * `AdminConfirmDialog` and this form is its body.
	 *
	 * Exposed as a component method rather than reached with
	 * `document.querySelector('.sigap-schedule-editor')`. A DOM query would
	 * couple the dialog to this component's class name: rename the class and
	 * the confirm button silently stops submitting, which is the kind of failure
	 * that only shows up as a dialog that does nothing. A typed method fails at
	 * the call site instead.
	 */
	export function submit() {
		submitted = true;
		if (!isScheduleFormValid(values)) return;
		onSubmit(toScheduleRequest(values), editing?.id ?? null);
	}
</script>

<form class="sigap-schedule-editor" on:submit={handleSubmit} novalidate>
	<Field
		id="schedule-facility"
		label="Fasilitas"
		required
		error={shownErrors.facility_id ?? ''}
		helper="Hanya fasilitas tempat Anda berhak mengelola jadwal."
		let:controlId
		let:describedBy
		let:invalid
	>
		<Select
			id={controlId}
			{describedBy}
			{invalid}
			size={DENSITY.adminComfortable}
			bind:value={values.facility_id}
			placeholder={optionsLoading ? 'Memuat fasilitas...' : 'Pilih fasilitas'}
			options={facilities.map((facility) => ({ value: facility.id, label: facility.name }))}
			on:change={handleFacilityChange}
		/>
	</Field>

	<Field
		id="schedule-service-unit"
		label="Unit layanan"
		required
		error={shownErrors.service_unit_id ?? ''}
		helper={
			values.facility_id === ''
				? 'Pilih fasilitas terlebih dahulu.'
				: 'Hanya unit layanan aktif di fasilitas terpilih.'
		}
		let:controlId
		let:describedBy
		let:invalid
	>
		<Select
			id={controlId}
			{describedBy}
			{invalid}
			size={DENSITY.adminComfortable}
			bind:value={values.service_unit_id}
			placeholder="Pilih unit layanan"
			disabled={selectedFacility === undefined}
			options={serviceUnits}
		/>
	</Field>

	<Field
		id="schedule-date"
		label="Tanggal"
		required
		error={shownErrors.schedule_date ?? ''}
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
			placeholder="YYYY-MM-DD"
			bind:value={values.schedule_date}
		/>
	</Field>

	<div class="sigap-schedule-editor__pair">
		<Field
			id="schedule-start"
			label="Jam mulai"
			required
			error={shownErrors.start_time ?? ''}
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
				placeholder="HH:MM"
				bind:value={values.start_time}
			/>
		</Field>

		<Field
			id="schedule-end"
			label="Jam selesai"
			required
			error={shownErrors.end_time ?? ''}
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
				placeholder="HH:MM"
				bind:value={values.end_time}
			/>
		</Field>
	</div>

	<div class="sigap-schedule-editor__pair">
		<Field
			id="schedule-slot"
			label="Durasi slot (menit)"
			required
			error={shownErrors.slot_minutes ?? ''}
			helper="5–180 menit, harus membagi habis rentang waktu."
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
				bind:value={values.slot_minutes}
			/>
		</Field>

		<Field
			id="schedule-capacity"
			label="Kapasitas per slot"
			required
			error={shownErrors.capacity_per_slot ?? ''}
			helper="1–100 kunjungan per slot."
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
				bind:value={values.capacity_per_slot}
			/>
		</Field>
	</div>

	<!--
		The slot preview, and what it is for.

		Without it, "the slot must divide the range exactly" is a rule the operator
		can only discover by being refused. With it, choosing 45 minutes over a
		three-hour range shows zero slots before submitting. The copy says "tidak
		terbagi habis" rather than showing "0", because a zero reads like a bug
		and naming the arithmetic is what lets someone fix it.
	-->
	<p class="sigap-schedule-editor__preview" role="status">
		{#if slotCount === null}
			Rentang jam dan durasi slot belum menghasilkan slot yang valid.
		{:else}
			{slotCount} slot per hari, kapasitas {values.capacity_per_slot || '-'} kunjungan per slot.
		{/if}
	</p>

	{#if submitError}
		<!--
			The server's own rejection, verbatim, in an alert region. Same rule as
			every other admin mutation: the backend names the rule that was broken,
			and substituting our own wording would hide which one it was.
		-->
		<p class="sigap-schedule-editor__error" role="alert">{submitError}</p>
	{/if}

	{#if !hideActions}
		<div class="sigap-schedule-editor__actions">
			<button
				type="button"
				class="sigap-schedule-editor__cancel"
				style:border-radius={RADIUS.control}
				on:click={onCancel}
			>
				Batal
			</button>
			<button
				type="submit"
				class="sigap-schedule-editor__submit"
				style:border-radius={RADIUS.control}
				disabled={busy || optionsLoading}
			>
				{busy ? 'Menyimpan...' : editing ? 'Simpan perubahan' : 'Simpan jadwal'}
			</button>
		</div>
	{/if}
</form>

<style>
	.sigap-schedule-editor {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.sigap-schedule-editor__pair {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 12px;
	}

	.sigap-schedule-editor__preview {
		margin: 0;
		padding: 8px 10px;
		font-size: 13px;
		color: var(--sigap-muted);
		background-color: var(--sigap-canvas);
		border-left: 3px solid var(--sigap-border);
	}

	.sigap-schedule-editor__error {
		margin: 0;
		font-size: 13px;
		line-height: 1.45;
		color: var(--sigap-danger);
	}

	.sigap-schedule-editor__actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		justify-content: flex-end;
	}

	.sigap-schedule-editor__cancel,
	.sigap-schedule-editor__submit {
		height: 40px;
		padding: 0 16px;
		font: inherit;
		font-size: 14px;
		font-weight: 500;
		cursor: pointer;
	}

	.sigap-schedule-editor__cancel {
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-schedule-editor__submit {
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		border: 1px solid var(--sigap-primary);
	}

	.sigap-schedule-editor__submit:disabled {
		background-color: var(--sigap-muted);
		border-color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-schedule-editor__cancel:focus-visible,
	.sigap-schedule-editor__submit:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
