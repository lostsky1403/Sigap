<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import AdminToolbar from '$lib/admin/AdminToolbar.svelte';
	import FacilityFilter from '$lib/admin/FacilityFilter.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import ForbiddenPanel from '$lib/ui/ForbiddenPanel.svelte';
	import ScheduleEditor from '$lib/admin/ScheduleEditor.svelte';
	import AdminConfirmDialog from '$lib/admin/AdminConfirmDialog.svelte';
	import AdminMutationFeedback from '$lib/admin/AdminMutationFeedback.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import {
		createSchedule,
		getScheduleMutationOptions,
		listFacilities,
		listSchedules,
		listServiceUnits,
		updateSchedule
	} from '$lib/api/endpoints/admin';
	import { indexBy, facilityName, serviceUnitName } from '$lib/domain/joins';
	import { formatDate, formatTime } from '$lib/domain/format';
	import { isEmptyScope, ORDINARY_EMPTY_DESCRIPTION } from '$lib/admin/readState';
	import { runAdminMutation } from '$lib/admin/adminMutation';
	import { toScheduleRequest } from '$lib/admin/scheduleForm';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type { ApiError } from '$lib/api/errors';
	import type {
		AdminFacility,
		AdminSchedule,
		AdminServiceUnit,
		ScheduleMutationOptions
	} from '$lib/api/types/api';

	/**
	 * T-3B4-06: the schedule read table.
	 *
	 * READ-ONLY, and the reason is a security requirement rather than a scoping
	 * convenience.
	 *
	 * The pre-3B4 page had a create/edit dialog with a free-text "ID Dokter"
	 * field. That is the exact thing the phase forbids: there is no practitioner
	 * catalog in the API to validate the id against, no practitioner name to
	 * resolve it to, and a schedule whose practitioner is a typed-in string is a
	 * record the system cannot reason about. This page therefore displays NO
	 * practitioner field at all — not a name (invented), not an id (a raw UUID),
	 * not a free-text box (fabrication waiting to happen).
	 *
	 * Facility and service resolve through the join helpers, so a reference that
	 * is null, missing, or out of scope renders as "-".
	 */
	let schedules: AdminSchedule[] = [];
	let facilities: AdminFacility[] = [];
	let units: AdminServiceUnit[] = [];
	let loading = true;
	let error: ApiError | null = null;
	let facilitiesLoaded = false;
	let facilityFilter = '';
	let activeOnly = false;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	/**
	 * No `manageDenied` flag, and its absence is deliberate.
	 *
	 * The pre-3B4 page decided whether to offer schedule editing from a
	 * client-side guess. That guess had to be wrong: there is no `schedule.manage`
	 * fact in this client — no permission list, no role, no facility scope, only
	 * `hasSession` — so any check would have been inference from an email address
	 * or a token claim, which is exactly what the Phase 3B0 authorization model
	 * forbids.
	 *
	 * Worse, the obvious implementation was subtly incorrect. A 403 on the READ
	 * list means "you may not read schedules"; it says nothing about whether you
	 * may change them. Treating one as evidence of the other would refuse an
	 * editor to an operator who may legitimately hold `schedule.manage`.
	 *
	 * So this phase ships no editor and asserts no permission it cannot verify.
	 * Phase 3B5 owns the editor, and the only honest way to decide whether to
	 * show it is for the server to say so — not for the client to reason about a
	 * status code.
	 */

	onMount(async () => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		const [scheduleResult, facilityResult, unitResult] = await Promise.all([
			listSchedules(),
			listFacilities(),
			listServiceUnits()
		]);

		// The affordance read is issued alongside the three, not after them: it
		// answers a different question and nothing waits on its result, so
		// sequencing it would delay the editor's availability for no reason.
		loadMutationOptions();

		if (scheduleResult.ok) {
			schedules = scheduleResult.data;
		} else {
			error = scheduleResult.error;
		}
		if (facilityResult.ok) {
			facilities = facilityResult.data;
			facilitiesLoaded = true;
		}
		if (unitResult.ok) {
			units = unitResult.data;
		}
		loading = false;
	});

	$: facilityIndex = indexBy(facilities, (facility) => facility.id);
	$: unitIndex = indexBy(units, (unit) => unit.id);

	$: filtered = schedules.filter((schedule) => {
		if (facilityFilter && schedule.facility_id !== facilityFilter) return false;
		if (activeOnly && !schedule.is_active) return false;
		return true;
	});

	/**
	 * A schedule's service unit. The join is best-effort by design: the frozen
	 * reference itself renders "-" here and states that the schedule data carries
	 * only the unit code, so a "-" is a truthful rendering rather than a gap.
	 */
	function serviceLabel(schedule: AdminSchedule): string {
		return serviceUnitName(unitIndex, schedule.service_unit_id);
	}

	/* ------------------------- T-3B5-03 mutations ------------------------- */

	/**
	 * The mutation affordance, straight from the server.
	 *
	 * `null` means "not yet known", which is different from "known to be empty"
	 * and must not collapse into it. A 200 with `facilities: []` is an
	 * authoritative "you may manage schedules nowhere", and it is a success — so
	 * it must not be rendered as a failed load, which would offer a retry button
	 * for a request that will never succeed differently.
	 */
	let mutationOptions: ScheduleMutationOptions | null = null;
	let optionsLoading = true;
	let optionsError: ApiError | null = null;

	/** The schedule being created (null target) or edited. */
	let editing: AdminSchedule | null = null;
	let editorOpen = false;
	let saving = false;
	let saveError = '';

	/**
	 * The editor instance, so the dialog's confirm button can ask it to submit.
	 *
	 * Typed as the component's own shape rather than `any`: `bind:this` on a
	 * Svelte component gives the instance, and the method it exposes is the one
	 * that runs validation first.
	 */
	let editorRef: { submit: () => void } | null = null;

	/**
	 * §5's two states, and they are genuinely different questions.
	 *
	 *  - Zero authorized facility SCOPE (the read list is empty) means the actor
	 *    can see nothing here at all. That is the pre-existing class-1 empty
	 *    state, already rendered by `AdminReadState`, and it is not a capability
	 *    message: the actor is simply out of scope.
	 *
	 *  - Readable facilities exist but `options.facilities` is empty means the
	 *    actor may READ schedules yet may not CHANGE them. That is a capability
	 *    refusal, and `ForbiddenPanel` is the right rendering for it.
	 *
	 * Conflating them would either show "you may not manage schedules" to someone
	 * who simply has no facilities, or hide a real capability refusal behind an
	 * ordinary empty list.
	 */
	$: scopeEmpty = facilitiesLoaded && isEmptyScope(facilities);
	$: capabilityDenied = !optionsLoading && !scopeEmpty && (mutationOptions?.facilities.length ?? 0) === 0;
	$: editorAvailable = !optionsLoading && (mutationOptions?.facilities.length ?? 0) > 0;

	/** The re-read after a successful mutation, preserving the operator's filters. */
	async function reloadSchedules() {
		const result = await listSchedules();
		if (result.ok) {
			schedules = result.data;
			error = null;
		} else {
			error = result.error;
		}
		loading = false;
	}

	async function loadMutationOptions() {
		optionsLoading = true;
		optionsError = null;
		const result = await getScheduleMutationOptions();
		if (result.ok) {
			mutationOptions = result.data;
		} else {
			// A failed affordance read must NOT fall back to the read list. Doing
			// so is the exact regression §5 forbids: the operator would be offered
			// facilities they cannot manage, and the mutation would be refused
			// with a 403 after they had filled in the form.
			mutationOptions = null;
			optionsError = result.error;
		}
		optionsLoading = false;
	}

	function openCreate() {
		editing = null;
		saveError = '';
		editorOpen = true;
	}

	function openEdit(schedule: AdminSchedule) {
		editing = schedule;
		saveError = '';
		editorOpen = true;
	}

	function closeEditor() {
		editorOpen = false;
		editing = null;
		saveError = '';
	}

	/**
	 * Submit. The body comes from `toScheduleRequest`, whose return type has no
	 * `practitioner_id` — so this call site cannot put one on the wire even by
	 * accident, and the request-body test asserts the serialized JSON lacks it.
	 */
	async function handleSubmit(
		body: ReturnType<typeof toScheduleRequest>,
		id: string | null
	) {
		saving = true;
		saveError = '';

		const outcome = await runAdminMutation(
			() => (id === null ? createSchedule(body) : updateSchedule(id, body)),
			{
				subject: '',
				fromLabel: '',
				toLabel: id === null ? 'jadwal baru' : 'jadwal diperbarui',
				noun: 'Jadwal',
				reload: reloadSchedules
			},
			hasSession
		);

		saving = false;

		if (outcome.ok) {
			closeEditor();
			return;
		}

		// The dialog stays open with the operator's input intact. Closing it
		// would discard everything they typed because one field was refused.
		if (outcome.failure) {
			saveError = outcome.failure.message;
		}
	}
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Jadwal"
		subtitle="Jadwal praktik dalam lingkup akses Anda. Pilihan fasilitas pada editor berasal dari server."
	/>

	<!--
		THE EDITOR ENTRY POINT, gated on a server-provided affordance.

		`optionsLoading` disables the button rather than hiding it, so the control
		does not appear and disappear as the page settles. A hidden-until-ready
		button shifts everything below it, which on a dense admin table means the
		operator's eye lands on a different row than the one they were reading.

		The button is NOT rendered when the affordance read FAILED, only when it
		succeeded with an empty list — those are different states and the failure
		one is reported below.
	-->
	{#if editorAvailable || optionsLoading}
		<div class="sigap-admin-actions">
			<button
				type="button"
				class="sigap-admin-actions__primary"
				style:border-radius={RADIUS.control}
				disabled={optionsLoading}
				on:click={openCreate}
			>
				{optionsLoading ? 'Memuat...' : 'Jadwal baru'}
			</button>
		</div>
	{/if}

	<!--
		THE CAPABILITY REFUSAL.

		Readable facilities exist but none are manageable, which is a different
		claim from "you have no facilities" (that is the class-1 empty state
		below). The panel does not name schedule.manage, the actor's role, or
		their scope: the server decided what to say, and it says less on purpose.
	-->
	{#if capabilityDenied}
		<ForbiddenPanel
			title="Tidak dapat mengelola jadwal"
			description="Anda dapat melihat jadwal pada fasilitas dalam lingkup akses Anda, tetapi tidak dapat menambah atau mengubahnya. Hubungi administrator bila Anda merasa ini keliru."
		/>
	{/if}

	<!--
		A failed affordance read. Deliberately NOT a ForbiddenPanel: the request
		did not succeed, so we do not know what the actor may do, and telling them
		"access denied" would be asserting something we never established. Nor
		does it fall back to the read list.
	-->
	{#if optionsError}
		<p class="sigap-admin-inline-error" role="alert">
			Daftar fasilitas yang dapat dikelola tidak dapat dimuat. Jadwal tetap dapat dibaca, tetapi
			penambahan dan perubahan tidak tersedia.
		</p>
	{/if}

	<AdminToolbar
		showRefresh={false}
		note="Pilihan fasilitas memantulkan fasilitas yang ada dalam data yang sudah dimuat. Nama unit layanan ditampilkan hanya bila unitnya dikenal; selain itu tampil sebagai “-”."
	>
		<FacilityFilter
			{facilities}
			selected={facilityFilter}
			visible={filtered.length}
			total={schedules.length}
			onSelect={(value) => (facilityFilter = value)}
		/>

		<div class="sigap-admin-checkfield">
			<input
				type="checkbox"
				id="schedule-active-only"
				bind:checked={activeOnly}
				class="sigap-admin-checkfield__input"
			/>
			<label class="sigap-admin-checkfield__label" for="schedule-active-only">
				Hanya jadwal aktif
			</label>
		</div>
	</AdminToolbar>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={schedules.length}
		scopeEmpty={facilitiesLoaded && isEmptyScope(facilities)}
		emptyTitle="Belum ada jadwal"
		emptyDescription={ORDINARY_EMPTY_DESCRIPTION}
	>
		{#if filtered.length === 0}
			<div class="sigap-admin-nomatch" style:border-radius={RADIUS.panel} role="status">
				<p class="sigap-admin-nomatch__title">Tidak ada jadwal yang cocok</p>
				<p class="sigap-admin-nomatch__body">Ubah atau hapus filter Anda.</p>
				<button
					type="button"
					class="sigap-admin-nomatch__reset"
					style:border-radius={RADIUS.control}
					on:click={() => {
						facilityFilter = '';
						activeOnly = false;
					}}
				>
					Hapus filter
				</button>
			</div>
		{:else}
			<DataTable
				caption="Jadwal praktik dalam lingkup akses"
				columns={[
					{ label: 'Tanggal' },
					{ label: 'Waktu' },
					{ label: 'Fasilitas', secondary: true },
					{ label: 'Layanan', secondary: true },
					{ label: 'Slot' },
					{ label: 'Kapasitas' },
					{ label: 'Status' },
					{ label: 'Aksi' }
				]}
			>
				{#each filtered as schedule (schedule.id)}
					<tr>
						<td class="sigap-schedule-row__date">{formatDate(schedule.schedule_date)}</td>
						<td class="sigap-schedule-row__time">
							{formatTime(schedule.start_time)}–{formatTime(schedule.end_time)}
						</td>
						<td class="sigap-table-col-secondary">
							{facilityName(facilityIndex, schedule.facility_id)}
						</td>
						<td class="sigap-table-col-secondary">{serviceLabel(schedule)}</td>
						<td class="sigap-schedule-row__num">{schedule.slot_minutes} mnt</td>
						<td class="sigap-schedule-row__num">{schedule.capacity_per_slot}</td>
						<td>
							<StatusBadge
								label={schedule.is_active ? 'Aktif' : 'Nonaktif'}
								tone={schedule.is_active ? 'success' : 'neutral'}
								size="sm"
							/>
						</td>
						<td class="sigap-schedule-row__actions">
							<!--
								Edit is offered for a row only when that row's OWN
								facility appears in the affordance list.

								This is per-row rather than page-wide, and the difference
								matters for the mixed actor: they may read schedules at
								facility A and only manage at facility B, so a global
								"you can edit" would put an Edit button on rows they
								cannot change. The row's facility is matched against the
								SERVER's list — never inferred from whether the row is
								visible.
							-->
							{#if editorAvailable &&
								mutationOptions?.facilities.some(
									(facility) => facility.id === schedule.facility_id
								)}
								<button
									type="button"
									class="sigap-schedule-row__edit"
									style:border-radius={RADIUS.control}
									on:click={() => openEdit(schedule)}
								>
									Ubah
								</button>
							{:else}
								<span class="sigap-schedule-row__readonly">Hanya baca</span>
							{/if}
						</td>
					</tr>
				{/each}
			</DataTable>
		{/if}
	</AdminReadState>
</div>

<!--
	The editor, in the shared accessible Dialog.

	`Dialog.svelte` owns focus entry, the focus trap, Escape, aria-labelledby /
	aria-describedby, and returning focus to the button that opened it — so this
	only supplies the content and the two callbacks. No window.confirm anywhere:
	a native confirm cannot be styled, cannot be labelled, and is not focus
	managed.
-->
<AdminConfirmDialog
	open={editorOpen}
	title={editing ? 'Ubah jadwal' : 'Jadwal baru'}
	description="Pilih fasilitas dan unit layanan dari daftar yang disediakan server. Jadwal tanpa nama praktisi tetap dapat disimpan."
	confirmLabel={editing ? 'Simpan perubahan' : 'Simpan jadwal'}
	confirmVariant="primary"
	busy={saving}
	onCancel={closeEditor}
	onConfirm={() => {
		/*
		 * The editor form owns submission, because only it knows whether the
		 * values are valid. The dialog's confirm button asks the form to submit
		 * rather than submitting a payload the dialog does not have — a dialog
		 * that could submit on its own would be a second path to the mutation
		 * that skipped the validation.
		 */
		editorRef?.submit();
	}}
>
	<ScheduleEditor
		bind:this={editorRef}
		options={mutationOptions}
		{optionsLoading}
		{editing}
		busy={saving}
		submitError={saveError}
		onSubmit={handleSubmit}
		onCancel={closeEditor}
	/>
</AdminConfirmDialog>

<AdminMutationFeedback />

<style>
	.sigap-admin-page {
		min-width: 0;
	}

	.sigap-admin-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}

	.sigap-admin-actions__primary {
		height: 40px;
		padding: 0 16px;
		font: inherit;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		border: 1px solid var(--sigap-primary);
		cursor: pointer;
	}

	.sigap-admin-actions__primary:disabled {
		background-color: var(--sigap-muted);
		border-color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-admin-actions__primary:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-admin-inline-error {
		margin: 0 0 12px;
		padding: 8px 12px;
		font-size: 13px;
		line-height: 1.45;
		color: var(--sigap-danger);
		background-color: var(--sigap-surface);
		border-left: 3px solid var(--sigap-danger);
	}

	.sigap-schedule-row__actions {
		white-space: nowrap;
	}

	.sigap-schedule-row__edit {
		height: 32px;
		padding: 0 12px;
		font: inherit;
		font-size: 13px;
		font-weight: 500;
		color: var(--sigap-primary);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		cursor: pointer;
	}

	.sigap-schedule-row__edit:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-schedule-row__readonly {
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-admin-checkfield {
		display: flex;
		align-items: center;
		gap: 6px;
		/* Aligns the checkbox with the 36px control row beside it. */
		height: 36px;
	}

	.sigap-admin-checkfield__input {
		width: 16px;
		height: 16px;
	}

	.sigap-admin-checkfield__label {
		font-size: 13px;
		color: var(--sigap-foreground);
	}

	.sigap-admin-checkfield__input:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-schedule-row__date {
		white-space: nowrap;
	}

	.sigap-schedule-row__time {
		color: var(--sigap-muted);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.sigap-schedule-row__num {
		font-variant-numeric: tabular-nums;
	}

	.sigap-admin-nomatch {
		padding: 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px dashed var(--sigap-border);
	}

	.sigap-admin-nomatch__title {
		margin: 0 0 4px;
		font-size: 15px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-admin-nomatch__body {
		margin: 0 0 12px;
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-admin-nomatch__reset {
		height: 36px;
		padding: 0 14px;
		font: inherit;
		font-size: 13px;
		font-weight: 500;
		color: var(--sigap-primary);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		cursor: pointer;
	}

	.sigap-admin-nomatch__reset:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
