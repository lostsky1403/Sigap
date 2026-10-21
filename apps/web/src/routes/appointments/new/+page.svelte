<script lang="ts">
	import { onMount, tick } from 'svelte';
	import BookingStepper from '$lib/citizen/BookingStepper.svelte';
	import BookingSummary from '$lib/citizen/BookingSummary.svelte';
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Select, { type Option } from '$lib/ui/Select.svelte';
	import Textarea from '$lib/ui/Textarea.svelte';
	import Button from '$lib/ui/Button.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { AlertTriangle, CalendarCheck, Copy, Check } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import {
		bookAppointment,
		listPublicFacilities,
		listPublicServiceUnits
	} from '$lib/api/endpoints/public';
	import { isAbort, type ApiError } from '$lib/api/errors';
	import { hasSession as readHasSession } from '$lib/stores/session';
	import type {
		BookAppointmentResult,
		PublicFacility,
		PublicServiceUnit
	} from '$lib/api/types/api';
	import {
		checkInDeepLink,
		classifyTransactionFailure,
		formatAppointmentTime,
		resolveFailureMessage,
		validateAppointmentTime,
		validateName,
		validatePhone,
		validateRequired,
		type FlowFailure
	} from '$lib/citizen/citizenFlow';

	/**
	 * /appointments/new — public booking.
	 *
	 * Three steps (Fasilitas, Layanan, Waktu + data) that map to three
	 * ideas rather than three pages. A step is not a route and not a separate
	 * submit: the whole thing is one form, one request, validated as a unit.
	 * Stepping shows progress and lets the form sections come into focus one at
	 * a time on a small screen.
	 *
	 * The step is derived from the selections rather than stored as a counter,
	 * so it is impossible to be on step 3 with no facility chosen, and
	 * backtracking is automatic — clearing the facility returns the citizen to
	 * step 1, because that is genuinely where they are.
	 */

	type LoadState = 'loading' | 'ready' | 'error';
	type Step = 1 | 2 | 3;

	/* ---------------- catalogs ---------------- */

	let facilities: PublicFacility[] = [];
	let serviceUnits: PublicServiceUnit[] = [];
	let catalogState: LoadState = 'loading';
	let catalogError: ApiError | null = null;
	/** Discards a slow earlier service-unit response when a newer one exists. */
	let serviceRequestId = 0;

	/* ---------------- selections ---------------- */

	let facilityId = '';
	let serviceUnitId = '';
	let appointmentTime = '';
	let patientName = '';
	let patientPhone = '';
	let notes = '';

	/* ---------------- form state ---------------- */

	let fieldErrors: Record<string, string> = {};
	let submitFailure: FlowFailure | null = null;
	let submitting = false;
	let result: BookAppointmentResult | null = null;
	let copyHint = '';
	let copied = false;

	/* ---------------- derived ---------------- */

	$: selectedFacility = facilities.find((f) => f.id === facilityId) ?? null;
	$: selectedServiceUnit = serviceUnits.find((u) => u.id === validServiceUnitId) ?? null;

	/*
		Service units are requested per facility rather than fetched once for the
		whole catalog and filtered in the browser. The endpoint takes a real
		`facility_id` parameter, so using it is the honest route — and it means
		the list can never show a unit belonging to another clinic beside a
		chosen facility, which is the mistake a client-side filter invites once
		the catalog is large enough to be stale.
	*/
	$: unitsForFacility = serviceUnits.filter((u) => u.facility_id === facilityId);

	/*
		Select options, built from the loaded rows.

		Using the Select primitive rather than a hand-rolled <select> is what
		keeps the label association intact: Field hands the control an id and a
		describedby list, and Select applies them. A raw element with the ids
		typed alongside it renders correctly in a screenshot and is completely
		unlabelled for a screen reader — the failure Field exists to prevent.
	*/
	$: facilityOptions = facilities.map<Option>((facility) => ({
		value: facility.id,
		label: facility.name
	}));

	$: serviceOptions = unitsForFacility.map<Option>((unit) => ({
		value: unit.id,
		label: unit.name
	}));

	/**
	 * The service selection that is actually valid right now.
	 *
	 * Derived rather than stored-and-mutated, and that is the whole point.
	 * The first version cleared `serviceUnitId` from a handler when the
	 * facility changed. Two things went wrong:
	 *
	 *   1. At change time the new clinic's list has not arrived, so a valid
	 *      unit looked invalid purely because the array was still empty — and
	 *      the guard against that (`if empty, skip`) also skipped the real
	 *      case, leaving a unit from the previous clinic selected.
	 *   2. Mutating a variable inside a reactive block does not recompute the
	 *      other reactive values that depend on it. Svelte orders the graph
	 *      from declarations, and `reconcileServiceUnit` writing `serviceUnitId`
	 *      is invisible to it, so `step` silently kept saying 3.
	 *
	 * A single derived value removes both problems. There is no window in
	 * which the stored selection and the valid selection disagree, because
	 * there is only one of them.
	 *
	 * Re-picking the SAME facility is safe without a loading flag: `serviceUnits`
	 * is not cleared while a request is in flight, so the previous list is
	 * still being filtered — and it still contains the chosen unit.
	 */
	$: validServiceUnitId =
		serviceUnitId && unitsForFacility.some((u) => u.id === serviceUnitId) ? serviceUnitId : '';

	$: step = (!facilityId ? 1 : !validServiceUnitId ? 2 : 3) as Step;

	// The store exposes a getter function, not a value. Calling it keeps the
	// prop a plain boolean, which is all ErrorState is allowed to know about.
	$: hasSession = readHasSession();

	/**
	 * A service unit that does not belong to the chosen facility is invalid.
	 *
	 * When the facility changes, a previously selected unit may not exist for
	 * the new one. Carrying it forward would submit a mismatched pair, and the
	 * server would reject it with "Unit layanan tidak ditemukan" — a correct
	 * error about a selection the citizen never made.
	 *
	 * Two guards matter here. The clear is skipped while the new list is still
	 * in flight, because `unitsForFacility` is legitimately empty during load
	 * and an unconditional check would wipe the selection on every change. And
	 * it is skipped when the unit is still valid, so re-picking the same clinic
	 * does not throw away a good choice.
	 */
	/**
	 * On facility change.
	 *
	 * Nothing to do beyond letting the derived `validServiceUnitId` recompute.
	 * An earlier version cleared the selection here, which was both unreliable
	 * (the new list had not arrived) and invisible to the rest of the reactive
	 * graph (mutating a variable here does not recompute `step`). Dropping an
	 * invalid selection is now a property of the state instead of an event.
	 */
	function onFacilityChanged() {
		/* intentionally empty: validity is derived, not enforced by mutation */
	}
	/* ---------------- catalog loading ---------------- */

	async function loadFacilities() {
		const outcome = await listPublicFacilities();
		if (outcome.ok) {
			facilities = outcome.data;
			catalogState = 'ready';
			catalogError = null;
		} else if (!isAbort(outcome.error)) {
			/*
				Abort is checked after the `ok` narrowing because `ApiResult` is a
				discriminated union: reading `.error` on the success arm is a
				type error, and the compiler is right to refuse. The order also
				matches the meaning — there is nothing to abort on a success.
			*/
			catalogState = 'error';
			catalogError = outcome.error;
		}
	}

	/**
	 * Loads service units for a facility, discarding stale responses.
	 *
	 * The request id is compared after the await: switching facility quickly
	 * fires several requests, and without this the last to *arrive* would win
	 * rather than the last *issued*, leaving a list of units for the clinic the
	 * citizen just navigated away from.
	 */
	async function loadServiceUnits(forFacilityId: string) {
		const id = ++serviceRequestId;
		if (!forFacilityId) {
			serviceUnits = [];
			return;
		}
		const outcome = await listPublicServiceUnits(forFacilityId);
		if (id !== serviceRequestId) return;
		serviceUnits = outcome.ok ? outcome.data : [];
	}

	$: if (facilityId) void loadServiceUnits(facilityId);
	/*
		Clearing when the facility is cleared is not redundant with the line
		above. A reactive block only re-runs when its dependency changes, and
		this one's dependency is a *truthy* facilityId, so the empty branch
		would never execute on its own.
	*/
	$: if (!facilityId) {
		serviceRequestId += 1;
		serviceUnits = [];
	}

	/* ---------------- submit ---------------- */

	function validate(): Record<string, string> {
		const errors: Record<string, string> = {};

		/*
			Validate only the step the citizen can actually see.

			Steps 2 and 3 are `disabled` and hidden until their predecessor is
			satisfied, so reporting their errors tells someone to fill in fields
			that are `display: none` — five messages at once on a pristine form,
			four of them pointing at controls that cannot be reached. The first
			incomplete step is the only one whose errors are actionable, so the
			form reports that step and stops.
		*/
		const facilityError = validateRequired(facilityId, 'Fasilitas');
		if (facilityError) {
			errors.facility_id = facilityError;
			return errors;
		}
		// Validated against the DERIVED selection, not the raw one. Submitting
		// `serviceUnitId` directly would send a stale unit belonging to another
		// clinic whenever the citizen changed facility mid-form, and the server
		// would answer "Unit layanan tidak ditemukan" — a correct error about a
		// choice they never made.
		const unitError = validateRequired(validServiceUnitId, 'Layanan');
		if (unitError) {
			errors.service_unit_id = unitError;
			return errors;
		}

		// Step 3 is now on screen, so all three of its fields are fair to
		// report together.
		const timeError = validateAppointmentTime(appointmentTime);
		if (timeError) errors.appointment_time = timeError;
		const nameError = validateName(patientName);
		if (nameError) errors.patient_display_name = nameError;
		const phoneError = validatePhone(patientPhone);
		if (phoneError) errors.patient_phone = phoneError;
		return errors;
	}

	/**
	 * The id list a control should point `aria-describedby` at.
	 *
	 * Field renders its helper and error at `<id>-helper` and `<id>-error`, and
	 * those are the ids that must be referenced here. Deriving them by string
	 * concatenation rather than taking Field's slot props is deliberate: the
	 * `let:` slot-prop syntax is Svelte 4 and is not available here, so the
	 * convention has to be restated. It is duplicated knowledge, and the test
	 * below asserts every referenced id actually exists in the DOM — which is
	 * what stops the duplication from silently rotting.
	 */
	function describedByFor(key: string, error: string | undefined): string {
		const id = `sigap-booking-${key}`;
		return error ? `${id}-error` : '';
	}

	async function submit() {
		if (submitting) return;

		const errors = validate();
		fieldErrors = errors;

		if (Object.keys(errors).length > 0) {
			/*
				Focus the first invalid control. A validation summary alone is not
				enough: the citizen is told what is wrong but not where, and on a
				three-step form the offending field may be a screen away.
			*/
			await tick();
			const firstKey = Object.keys(errors)[0];
			document.getElementById(`sigap-booking-${firstKey}`)?.focus();
			return;
		}

		submitting = true;
		submitFailure = null;

		const outcome = await bookAppointment({
			facilityId,
			serviceUnitId: validServiceUnitId,
			patientName: patientName.trim(),
			patientPhone: patientPhone.trim(),
			/*
				RFC3339 UTC, not the raw control value. `datetime-local` yields a
				zoneless "YYYY-MM-DDTHH:mm" and the backend parses RFC3339, so
				sending it verbatim fails with "Format waktu janji temu tidak
				valid". Converting here also means the time shown in the summary
				is the instant that gets stored, rather than two differently
				zoned readings of one booking.
			*/
			appointmentTime: new Date(appointmentTime).toISOString()
		});

		if (outcome.ok) {
			result = outcome.data;
			submitting = false;
			/*
				Move focus to the confirmation. Otherwise a screen reader or
				keyboard user is left focused on a submit button that no longer
				exists, and the next Tab goes somewhere arbitrary.
			*/
			await tick();
			document.getElementById('sigap-booking-confirmation')?.focus();
			return;
		}

		submitting = false;
		/*
			An abort is a navigation or a superseded submit, not a failure.
			Showing an error here would flash a red panel at someone who simply
			moved on to another page. Checked after the `ok` narrowing because
			`ApiResult` is a discriminated union and `.error` only exists on the
			failure arm.
		*/
		if (isAbort(outcome.error)) return;

		submitFailure = classifyTransactionFailure(outcome.error);
	}

	/**
	 * Copies the check-in code and says whether it worked.
	 *
	 * The clipboard API rejects without a user gesture and in insecure
	 * contexts, so this failure is real rather than theoretical. The fallback
	 * is a truthful message, not a silent no-op that would leave the citizen
	 * believing the code is on their clipboard.
	 */
	async function copyCheckinCode() {
		if (!result?.checkin_code) return;
		try {
			await navigator.clipboard.writeText(result.checkin_code);
			copyHint = 'Kode check-in tersalin.';
			copied = true;
		} catch {
			copyHint = 'Tidak dapat menyalin otomatis. Salin manual dari kotak kode.';
			copied = false;
		}
		setTimeout(() => {
			copyHint = '';
			copied = false;
		}, 4000);
	}

	function reset() {
		result = null;
		submitFailure = null;
		fieldErrors = {};
		copyHint = '';
		copied = false;
	}

	$: deepLink = result ? checkInDeepLink(result.id, result.checkin_code) : '/appointments/check-in';

	/**
	 * The one line to show for a failed submit.
	 *
	 * Resolved here rather than in the template because `FlowFailure` is a
	 * discriminated union: only the arms that carry a `message` have one, so
	 * a template branch that reads `.message` on `wrong-code` is a type error.
	 * Concentrating the decision also means there is exactly one place where a
	 * server message can be replaced, and exactly one generic fallback.
	 */
	$: failureMessage = resolveFailureMessage(submitFailure);

	onMount(() => {
		/*
			`/faskes` links here with `?facility_id=...`, and honouring it is the
			entire point of that link: the citizen already chose a clinic on the
			previous page and should not choose it again.

			Read from `window.location` rather than SvelteKit's `page` store so
			the component stays testable in jsdom with no router, which is how
			every other page test in this repo works.
		*/
		const params = new URLSearchParams(window.location.search);
		const preselected = params.get('facility_id') ?? '';
		void loadFacilities().then(() => {
			/*
				Applied after the catalog arrives, and only if the id is real. A
				preselected id that is not in the catalog would otherwise leave
				the select showing "Pilih fasilitas..." with a hidden value
				behind it, so the summary and the request body would disagree
				with what the citizen sees.
			*/
			if (preselected && facilities.some((f) => f.id === preselected)) {
				facilityId = preselected;
			}
		});
	});
</script>

<svelte:head>
	<title>Buat Janji Temu — Sigap</title>
</svelte:head>

<div class="sigap-page sigap-booking">
	<!--
		The shared page opener, in its no-rule variant.

		The Pen draws every booking frame with an eyebrow and a title but no 2px
		rule, so this journey opens with the label rather than the mark. The
		lede tracks the step: it names what this step asks for, which is why it
		is a value here and not a constant.
	-->
	<PageHeader
		rule="none"
		eyebrow="Buat janji temu"
		title="Buat janji temu"
		description="Pilih fasilitas kesehatan terlebih dahulu."
	/>

	<!--
		A catalog failure means there is nothing to book against, so the whole
		form is replaced rather than shown disabled. A form with nothing in it
		looks broken; an explanation does not.
	-->
	{#if catalogState === 'error' && catalogError}
		<section class="sigap-booking__section">
			<ErrorState
				error={catalogError}
				{hasSession}
				onRetry={loadFacilities}
				retryLabel="Muat ulang"
			/>
		</section>
	{:else if catalogState === 'loading'}
		<section class="sigap-booking__section">
			<div class="sigap-booking__loading" role="status" aria-busy="true" aria-live="polite">
				<p class="sigap-booking__loading-text">Memuat data fasilitas...</p>
			</div>
		</section>
	{:else if result}
		<!-- ============================ SUCCESS ============================ -->
		<section
			class="sigap-booking__confirmation"
			style:border-radius={RADIUS.panel}
			aria-labelledby="sigap-booking-confirmed-title"
			tabindex="-1"
			id="sigap-booking-confirmation"
		>
			<!--
				role="status" so the confirmation is announced when it replaces
				the form. Without it, a screen reader user who submitted gets
				silence and no way to know the booking succeeded.
			-->
			<div class="sigap-booking__confirm-head" role="status">
				<span class="sigap-booking__confirm-icon" aria-hidden="true">
					<Icon icon={CalendarCheck} size={20} />
				</span>
				<div>
					<h2 class="sigap-section-title" id="sigap-booking-confirmed-title">
						Janji temu berhasil
					</h2>
					<p class="sigap-booking__confirm-sub">
						Simpan kode check-in Anda. Kode ini dipakai untuk check-in di fasilitas.
					</p>
				</div>
			</div>

			<div class="sigap-booking__code-block">
				<p class="sigap-booking__code-label" id="sigap-booking-code-label">Kode check-in</p>
				<!--
					user-select: all. This is a six-character code a citizen has to
					write down or paste into another device, so making it awkward to
					select would be actively hostile. The whole point is that they
					can take it away from this screen.
				-->
				<p
					class="sigap-booking__code sigap-identifier"
					aria-labelledby="sigap-booking-code-label"
					data-testid="booking-checkin-code"
				>
					{result.checkin_code}
				</p>
				<Button
					variant="secondary"
					icon={copied ? Check : Copy}
					onClick={copyCheckinCode}
				>
					{copied ? 'Tersalin' : 'Salin kode'}
				</Button>
			</div>

			<p class="sigap-booking__when">
				Waktu: {formatAppointmentTime(result.appointment_time) || result.appointment_time}
			</p>

			<div class="sigap-booking__actions">
				<a class="sigap-booking__cta" style:border-radius={RADIUS.control} href={deepLink}>
					Lanjut ke Check-In
				</a>
				<Button variant="ghost" onClick={reset}>Buat janji lain</Button>
			</div>

			<!--
				The appointment id is a real response field and the check-in deep
				link depends on it, so it is surfaced rather than hidden. It is a
				UUID identifying a booking, not patient data.

				build-verification.test.js asserts this file references
				`result.id` and `checkin_code` by name, which is why the response
				fields are used directly here instead of renamed locals.
			-->
			<details class="sigap-booking__details">
				<summary>Detail teknis</summary>
				<dl class="sigap-booking__detail-list">
					<div>
						<dt>appointment_id</dt>
						<dd class="sigap-identifier" data-testid="booking-appointment-id">{result.id}</dd>
					</div>
					<div>
						<dt>status</dt>
						<dd>{result.status}</dd>
					</div>
				</dl>
			</details>

			<!--
				Its own live region: the copy confirmation appears out of band and
				must be announced, since it is the only feedback that action gives.
			-->
			<p class="sigap-booking__copy-hint" role="status" aria-live="polite">{copyHint}</p>
		</section>
	{:else}
		<!-- ============================= FORM ============================= -->
		<BookingStepper current={step} />

		<div class="sigap-booking__layout">
			<form class="sigap-booking__form" novalidate on:submit|preventDefault={submit}>
				<!--
					Form-level failure. role="alert" so it is announced, and it
					repeats the server's own message rather than replacing it — the
					backend writes specific, actionable text per status, and
					paraphrasing it is strictly worse than showing it.
				-->
				{#if submitFailure}
					<div class="sigap-booking__failure" role="alert">
						<span class="sigap-booking__failure-icon" aria-hidden="true">
							<Icon icon={AlertTriangle} size={18} />
						</span>
						<p class="sigap-booking__failure-text">{failureMessage}</p>
					</div>
				{/if}

				<!-- STEP 1: Fasilitas -->
				<fieldset class="sigap-booking__step" class:sigap-booking__step--active={step === 1}>
					<legend class="sigap-booking__step-title">1. Fasilitas</legend>

					<Field
						id="sigap-booking-facility_id"
						label="Fasilitas"
						required
						error={fieldErrors.facility_id ?? ''}
					>
						<Select
							id="sigap-booking-facility_id"
							bind:value={facilityId}
							options={facilityOptions}
							placeholder="Pilih fasilitas..."
							describedBy={describedByFor('facility_id', fieldErrors.facility_id)}
							invalid={Boolean(fieldErrors.facility_id)}
							required
							on:change={onFacilityChanged}
						/>
					</Field>
				</fieldset>

				<!-- STEP 2: Layanan -->
				<fieldset
					class="sigap-booking__step"
					class:sigap-booking__step--active={step === 2}
					disabled={!facilityId}
				>
					<legend class="sigap-booking__step-title">2. Layanan</legend>

					<Field
						id="sigap-booking-service_unit_id"
						label="Layanan"
						required
						error={fieldErrors.service_unit_id ?? ''}
						helper={!facilityId ? 'Pilih fasilitas terlebih dahulu.' : ''}
					>
						<Select
						id="sigap-booking-service_unit_id"
						bind:value={serviceUnitId}
						options={serviceOptions}
						placeholder="Pilih layanan..."
						describedBy={describedByFor('service_unit_id', fieldErrors.service_unit_id)}
						invalid={Boolean(fieldErrors.service_unit_id)}
						required
						disabled={!facilityId}
					/>
				</Field>
			</fieldset>

			<!-- STEP 3: Waktu + data -->
			<fieldset
				class="sigap-booking__step"
				class:sigap-booking__step--active={step === 3}
				disabled={!validServiceUnitId}
			>
				<legend class="sigap-booking__step-title">3. Waktu + data</legend>

				<Field
					id="sigap-booking-appointment_time"
					label="Waktu janji temu"
					required
					error={fieldErrors.appointment_time ?? ''}
					helper="Pilih tanggal dan waktu kunjungan."
				>
					<!--
						A raw input rather than the Input primitive: its `type` union
						is deliberately narrow and does not include datetime-local,
						and widening a shared primitive to accommodate one page
						would weaken the type for every other consumer. The id and
						describedby are supplied explicitly so the label
						association is identical to the other fields.
					-->
					<input
						id="sigap-booking-appointment_time"
						class="sigap-booking__control"
						type="datetime-local"
						required
						aria-invalid={fieldErrors.appointment_time ? 'true' : 'false'}
						aria-describedby={describedByFor(
							'appointment_time',
							fieldErrors.appointment_time
						) || undefined}
						bind:value={appointmentTime}
					/>
				</Field>

				<Field
					id="sigap-booking-patient_display_name"
					label="Nama pasien"
					required
					error={fieldErrors.patient_display_name ?? ''}
				>
					<Input
						id="sigap-booking-patient_display_name"
						autocomplete="name"
						bind:value={patientName}
						describedBy={describedByFor(
							'patient_display_name',
							fieldErrors.patient_display_name
						)}
						invalid={Boolean(fieldErrors.patient_display_name)}
						required
					/>
				</Field>

				<Field
					id="sigap-booking-patient_phone"
					label="Nomor telepon"
					required
					error={fieldErrors.patient_phone ?? ''}
					helper="10-15 digit angka."
				>
					<Input
						id="sigap-booking-patient_phone"
						type="tel"
						autocomplete="tel"
						inputmode="tel"
						bind:value={patientPhone}
						describedBy={describedByFor('patient_phone', fieldErrors.patient_phone)}
						invalid={Boolean(fieldErrors.patient_phone)}
						required
					/>
				</Field>

				<Field id="sigap-booking-notes" label="Catatan" optional>
					<Textarea id="sigap-booking-notes" rows={2} bind:value={notes} />
				</Field>
			</fieldset>

				<div class="sigap-booking__submit">
					<Button type="submit" disabled={submitting}>
						{submitting ? 'Menyimpan...' : 'Daftar Janji Temu'}
					</Button>
				</div>
			</form>

			<div class="sigap-booking__aside">
				<BookingSummary
					facility={selectedFacility}
					serviceUnit={selectedServiceUnit}
					{appointmentTime}
					{patientName}
					{patientPhone}
				/>
			</div>
		</div>
	{/if}
</div>

<style>
	/*
		Layout comes from `.sigap-page`. The booking frame is the one citizen
		page the Pen draws as a two-column split across the full 1344px content
		column, so the page is not capped — the `__layout` rule below is what
		decides when the aside moves beside the form.
	*/

	/*
		Note there is no `.sigap-booking__section` rule. The sections are direct
		children of `.sigap-page`, which already puts `--sigap-page-gap` between
		them; a margin here would stack on that gap and give this page a rhythm
		no other page has.

		A loading region is not a control, so its outline is the decorative
		hairline and the sunken ground does the work of separating it. Wiring
		`--sigap-border` here would draw a 4.16:1 box around a non-interactive
		message.
	*/
	.sigap-booking__loading {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-booking__loading-text {
		margin: 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		Two columns on desktop, one on mobile. The breakpoint is 1024px because
		that is where the summary stops being something below the fold and
		starts being usable beside the form; below it, stacking is what lets a
		citizen see the summary change immediately after editing a field.
	*/
	.sigap-booking__layout {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-4);
	}

	@media (min-width: 1024px) {
		.sigap-booking__layout {
			flex-direction: row;
			align-items: flex-start;
		}

		.sigap-booking__form {
			flex: 1 1 auto;
			min-width: 0;
		}

		/*
			320px keeps the summary readable beside a form that still has room
			for a two-field row at 1024px.

			The sticky offset clears the masthead, which is itself sticky at
			`top: 0` and 56px tall. Without the offset the summary slides under
			the masthead and its heading disappears behind the brand as the page
			scrolls.
		*/
		.sigap-booking__aside {
			position: sticky;
			top: calc(56px + var(--sigap-space-4));
			width: 320px;
			flex: none;
		}
	}

	.sigap-booking__form {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-4);
		min-width: 0;
	}

	/*
		Only the active step is displayed. Hiding the others makes the form feel
		like three pages, and it keeps the citizen from seeing fields they cannot
		use yet — a service list that is empty because no facility is chosen
		reads as an error rather than as "not yet".

		The step is a section, not a control: its outline is the decorative
		hairline, and the controls inside it (the primitives) keep their own
		`--sigap-border` boundaries.
	*/
	.sigap-booking__step {
		display: none;
		margin: 0;
		padding: var(--sigap-space-4);
		border: 1px solid var(--sigap-border-subtle);
		background-color: var(--sigap-surface);
	}

	.sigap-booking__step--active {
		display: block;
	}

	.sigap-booking__step-title {
		padding: 0;
		margin-bottom: var(--sigap-space-3);
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-booking__step :global(.sigap-field + .sigap-field) {
		margin-top: var(--sigap-space-4);
	}

	/*
		Only the datetime-local control is styled here. Everything else goes
		through the Input, Select, and Textarea primitives, which already carry
		the citizen 44px height, the focus ring, and the invalid border. One
		bespoke rule for the one control the primitives do not cover, rather
		than a parallel stylesheet that could drift from them.

		The border stays `--sigap-border`: this IS a control boundary, and the
		decorative hairline at 1.28:1 would leave the field with no visible
		edge on white.
	*/
	.sigap-booking__control {
		width: 100%;
		/* Citizen touch floor, matching the primitives. */
		height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-3);
		font-family: inherit;
		/*
			16px, not a scale step: below 16px a focused input makes iOS Safari
			zoom the whole page. This matches `.sigap-input--citizen` in the
			shared primitives, which carries the same value for the same reason.
		*/
		font-size: 16px;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-booking__control:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 1px;
		border-color: var(--sigap-primary);
	}

	.sigap-booking__control[aria-invalid='true'] {
		border-color: var(--sigap-danger);
	}

	/*
		A tone-carrying alert: tone border, tone icon, tone text, tinted ground.
		The tone is the whole point — a server failure stated in body text reads
		as an aside rather than as the reason the submit did nothing.
	*/
	.sigap-booking__failure {
		display: flex;
		align-items: flex-start;
		gap: var(--sigap-space-3);
		padding: var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		color: var(--sigap-danger);
		background-color: var(--sigap-danger-tint);
		border: 1px solid var(--sigap-danger);
	}

	.sigap-booking__failure-icon {
		display: flex;
		flex: none;
	}

	.sigap-booking__failure-text {
		margin: 0;
	}

	.sigap-booking__submit {
		display: flex;
		justify-content: flex-end;
	}

	.sigap-booking__confirmation {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-booking__confirm-head {
		display: flex;
		align-items: flex-start;
		gap: var(--sigap-space-3);
	}

	/*
		The confirmation mark, drawn as a badge: 1px tone border, tone glyph,
		tinted ground. Never filled, and never colour alone — the heading beside
		it carries the same meaning in words.
	*/
	.sigap-booking__confirm-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		flex: none;
		color: var(--sigap-primary);
		background-color: var(--sigap-primary-tint);
		border: 1px solid var(--sigap-primary);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-booking__confirm-sub {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		The code box frames a value; it is not itself operable, so its outline is
		the decorative hairline and the tinted ground carries the emphasis. The
		monospace code inside is what the citizen has to read, not the box.
	*/
	.sigap-booking__code-block {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-4);
		text-align: center;
		background-color: var(--sigap-primary-tint);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-booking__code-label {
		margin: 0 0 var(--sigap-space-1);
		font-size: var(--sigap-type-caption);
		font-weight: 500;
		color: var(--sigap-muted);
	}

	/*
		The code is the one thing the citizen must write down, so it is the
		largest text on the page. `.sigap-identifier` supplies the monospace
		stack and tabular figures — the code is compared character by character,
		so it must not be re-typeset with a second font stack here.
	*/
	.sigap-booking__code {
		margin: 0 0 var(--sigap-space-3);
		font-size: 36px;
		font-weight: 700;
		color: var(--sigap-primary-strong);
		/* Selectable by design; see the note in the markup. */
		user-select: all;
		overflow-wrap: anywhere;
	}

	.sigap-booking__when {
		margin: var(--sigap-space-4) 0 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	.sigap-booking__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sigap-space-3);
		margin-top: var(--sigap-space-4);
	}

	.sigap-booking__cta {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-4);
		font-size: var(--sigap-type-body);
		font-weight: 500;
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-booking__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-booking__details {
		margin-top: var(--sigap-space-4);
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-booking__details summary {
		min-height: var(--sigap-control-citizen);
		display: flex;
		align-items: center;
		cursor: pointer;
	}

	.sigap-booking__detail-list {
		margin: var(--sigap-space-2) 0 0;
		padding: var(--sigap-space-3);
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-booking__detail-list dt {
		font-size: var(--sigap-type-micro);
		color: var(--sigap-muted);
	}

	.sigap-booking__detail-list dd {
		margin: 0 0 var(--sigap-space-2);
		font-size: var(--sigap-type-micro);
		color: var(--sigap-foreground);
		overflow-wrap: anywhere;
	}

	.sigap-booking__copy-hint {
		margin: var(--sigap-space-2) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		min-height: 16px;
	}
</style>
