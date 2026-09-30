<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import FacilityEditor from '$lib/admin/FacilityEditor.svelte';
	import AdminConfirmDialog from '$lib/admin/AdminConfirmDialog.svelte';
	import AdminMutationFeedback from '$lib/admin/AdminMutationFeedback.svelte';
	import {
		createFacility,
		deactivateFacility,
		listFacilities,
		updateFacility
	} from '$lib/api/endpoints/admin';
	import { facilityStateLabel, isFacilityActive } from '$lib/admin/facilityState';
	import {
		blankFacility,
		toFacilityCreateRequest,
		toFacilityUpdateRequest,
		type FacilityFormValues
	} from '$lib/admin/facilityForm';
	import { runAdminMutation } from '$lib/admin/adminMutation';
	import { formatNumber } from '$lib/domain/format';
	import { isEmptyScope } from '$lib/admin/readState';
	import { RADIUS } from '$lib/design/tokens';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type { ApiError } from '$lib/api/errors';
	import type { AdminFacility } from '$lib/api/types/api';

	/**
	 * T-3B4-07: the facility read table.
	 *
	 * READ-ONLY. The pre-3B4 page carried a create form, an edit dialog, and a
	 * deactivate button wired to a `confirm()`. All of that is Phase 3B5's
	 * mutation work and none of it is here — this phase changes what an operator
	 * can READ and see, not what they can change.
	 *
	 * Two things this page must get right, both verified against the backend
	 * rather than assumed:
	 *
	 *  1. The inactive state is a LABEL plus a colour, never a colour alone. A
	 *     greyed-out row tells a colour-blind operator nothing; "Nonaktif" does.
	 *
	 *  2. Deactivation is ONE-WAY. The backend flips `is_active` to false and
	 *     retains the row, and there is no reverse operation. The UI states that
	 *     plainly, because an operator who believes a facility can be reactivated
	 *     will confidently tell a clinic it can reopen — a false promise made to a
	 *     third party about a system they do not own.
	 */
	let facilities: AdminFacility[] = [];
	let loading = true;
	let error: ApiError | null = null;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	onMount(async () => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		const result = await listFacilities();
		if (result.ok) {
			facilities = result.data;
		} else {
			error = result.error;
		}
		loading = false;
	});

	/** A location line built only from fields the list actually returns. */
	function locationOf(facility: AdminFacility): string {
		const parts = [facility.kecamatan, facility.kabupaten_kota, facility.provinsi].filter(
			(part) => typeof part === 'string' && part.trim() !== ''
		);
		// "-" rather than a raw value when nothing is known. The list query returns
		// an empty address, so an empty cell would be the common case.
		return parts.length > 0 ? parts.join(', ') : '-';
	}

	/* ------------------------- T-3B5-04 mutations ------------------------- */

	/** The facility being created (null) or edited. */
	let editing: AdminFacility | null = null;
	let editorOpen = false;
	let saving = false;
	let saveError = '';
	let editorRef: { submit: () => void } | null = null;

	/** The facility awaiting deactivation confirmation, or null. */
	let deactivateTarget: AdminFacility | null = null;
	let deactivating = false;
	let deactivateError = '';

	/** The unedited form values, so an update can send only what changed. */
	let pristine: FacilityFormValues = blankFacility();

	async function reloadFacilities() {
		const result = await listFacilities();
		if (result.ok) {
			facilities = result.data;
			error = null;
		} else {
			error = result.error;
		}
		loading = false;
	}

	function openCreate() {
		editing = null;
		pristine = blankFacility();
		saveError = '';
		editorOpen = true;
	}

	function openEdit(facility: AdminFacility) {
		editing = facility;
		// Captured so `toFacilityUpdateRequest` can diff against what the server
		// held, not against what the operator may have changed and changed back.
		pristine = {
			name: facility.name ?? '',
			type: (facility.type ?? '') as FacilityFormValues['type'],
			address: facility.address ?? '',
			kecamatan: facility.kecamatan ?? '',
			kabupaten_kota: facility.kabupaten_kota ?? '',
			provinsi: facility.provinsi ?? '',
			phone: facility.phone ?? '',
			total_beds: String(facility.total_beds ?? 0),
			available_beds: String(facility.available_beds ?? 0),
			short_code: facility.short_code ?? ''
		};
		saveError = '';
		editorOpen = true;
	}

	function closeEditor() {
		editorOpen = false;
		editing = null;
		saveError = '';
	}

	async function handleSubmit(values: FacilityFormValues) {
		saving = true;
		saveError = '';

		const isCreate = editing === null;
		const outcome = await runAdminMutation(
			() =>
				isCreate
					? createFacility(toFacilityCreateRequest(values))
					: updateFacility(editing!.id, toFacilityUpdateRequest(values, pristine)),
			{
				subject: '',
				fromLabel: '',
				toLabel: isCreate ? 'fasilitas baru' : 'fasilitas diperbarui',
				noun: 'Fasilitas',
				reload: reloadFacilities
			},
			hasSession
		);

		saving = false;

		if (outcome.ok) {
			closeEditor();
			return;
		}

		// The form keeps its input. Closing on a single refused field would make
		// the operator retype nine fields to fix one.
		if (outcome.failure) {
			saveError = outcome.failure.message;
		}
	}

	function requestDeactivate(facility: AdminFacility) {
		deactivateError = '';
		deactivateTarget = facility;
	}

	function dismissDeactivate() {
		deactivateTarget = null;
		deactivateError = '';
	}

	/**
	 * §10: PATCH, one-way, and the string "false" handled correctly.
	 *
	 * The response carries `is_active` as the STRING "false", because the
	 * handler builds a `map[string]string`. `Boolean("false")` is `true` in
	 * JavaScript, so the row is never patched from the response — it is
	 * re-read from the list, which carries a real boolean. The alternative would
	 * be to trust the response and get the state exactly backwards on the one
	 * screen whose job is to say whether a facility is still taking patients.
	 */
	async function confirmDeactivate() {
		const target = deactivateTarget;
		if (!target) return;
		deactivating = true;
		deactivateError = '';

		const outcome = await runAdminMutation(
			() => deactivateFacility(target.id),
			{
				subject: target.name,
				fromLabel: 'aktif',
				toLabel: 'nonaktif',
				noun: 'Fasilitas',
				reload: reloadFacilities
			},
			hasSession
		);

		deactivating = false;

		if (outcome.ok) {
			deactivateTarget = null;
			return;
		}

		if (outcome.failure) {
			deactivateError = outcome.failure.message;
		}
	}

	/**
	 * The deactivation consequence, in the terms the operator and the public
	 * share.
	 *
	 * §10 requires the dialog to state the approved public impact. It does, and
	 * it does not soften it: the facility stops appearing for citizens, and
	 * nothing in the product can undo that. An operator who has to be told this
	 * by a colleague after the fact has been given a false impression.
	 */
	$: deactivateDescription = deactivateTarget
		? `${deactivateTarget.name} akan dinonaktifkan permanen. Setelah itu, faskes ini tidak lagi muncul untuk warga. Tindakan ini tidak dapat dibatalkan dan tidak ada cara mengaktifkan kembali fasilitas yang sudah dinonaktifkan.`
		: '';
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Fasilitas"
		subtitle="Fasilitas dalam lingkup akses Anda. Penonaktifan bersifat permanen dan tidak dapat dibatalkan."
	/>

	<div class="sigap-admin-actions">
		<button
			type="button"
			class="sigap-admin-actions__primary"
			style:border-radius={RADIUS.control}
			disabled={loading}
			on:click={openCreate}
		>
			Fasilitas baru
		</button>
	</div>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={facilities.length}
		scopeEmpty={loading === false && error === null && isEmptyScope(facilities)}
		emptyTitle="Belum ada fasilitas"
	>
		<DataTable
			caption="Fasilitas dalam lingkup akses"
			columns={[
				{ label: 'Kode' },
				{ label: 'Nama' },
				{ label: 'Tipe' },
				{ label: 'Lokasi', secondary: true },
				{ label: 'Kontak', secondary: true },
				{ label: 'Kasur', secondary: true },
				{ label: 'Status' },
				{ label: 'Aksi' }
			]}
		>
			{#each facilities as facility (facility.id)}
				{@const active = isFacilityActive(facility.is_active)}
				<tr>
					<td class="sigap-facility-row__code">{facility.short_code}</td>
					<td class="sigap-facility-row__name">{facility.name}</td>
					<td>{facility.type === 'rumah_sakit' ? 'Rumah Sakit' : 'Puskesmas'}</td>
					<td class="sigap-table-col-secondary">{locationOf(facility)}</td>
					<td class="sigap-table-col-secondary">{facility.phone || '-'}</td>
					<td class="sigap-table-col-secondary sigap-facility-row__beds">
						{formatNumber(facility.available_beds)} / {formatNumber(facility.total_beds)}
					</td>
					<td class="sigap-facility-row__state">
						<!--
							Label AND tone, never tone alone. The frozen status badge is
							outline-only with the label as its text, so the state survives a
							monochrome display and a colour-blind operator.
						-->
						<StatusBadge
							label={facilityStateLabel(active)}
							tone={active ? 'success' : 'neutral'}
							size="sm"
						/>
						{#if !active}
							<!--
								Stated on the row itself rather than only in a legend, because
								the operator reading this specific row is the one who needs to
								know it cannot be undone.
							-->
							<span class="sigap-facility-row__permanent">Penonaktifan bersifat permanen.</span>
						{/if}
					</td>
					<td class="sigap-facility-row__actions">
						<button
							type="button"
							class="sigap-facility-row__edit"
							style:border-radius={RADIUS.control}
							on:click={() => openEdit(facility)}
						>
							Ubah
						</button>
						<!--
							NO REACTIVATE ACTION, and the absence is the point.

							The backend has no reverse operation, so a "Aktifkan" button here
							would be a control that exists only to fail. The condition is
							`active`, not a permission check: an already-inactive facility
							simply has nothing left to deactivate, and showing the operator a
							second identical action would imply a second outcome exists.
						-->
						{#if active}
							<button
								type="button"
								class="sigap-facility-row__deactivate"
								style:border-radius={RADIUS.control}
								on:click={() => requestDeactivate(facility)}
							>
								Nonaktifkan
							</button>
						{/if}
					</td>
				</tr>
			{/each}
		</DataTable>

		<p class="sigap-facility-row__note">
			Menampilkan {formatNumber(facilities.length)} fasilitas. Penonaktifan mengubah status
			menjadi nonaktif secara permanen; fasilitas tidak dapat diaktifkan kembali.
		</p>
	</AdminReadState>
</div>

<!-- The create/edit form. Accessible Dialog, no window.confirm. -->
<AdminConfirmDialog
	open={editorOpen}
	title={editing ? 'Ubah fasilitas' : 'Fasilitas baru'}
	description="Data fasilitas yang tersimpan akan langsung dipakai pada halaman warga."
	confirmLabel="Simpan"
	confirmVariant="primary"
	busy={saving}
	onCancel={closeEditor}
	onConfirm={() => editorRef?.submit()}
>
	<FacilityEditor
		bind:this={editorRef}
		hideActions
		facility={editing}
		busy={saving}
		submitError={saveError}
		onSubmit={handleSubmit}
		onCancel={closeEditor}
	/>
</AdminConfirmDialog>

<!--
	The deactivation confirmation.

	§10's exact requirement is that this states the public impact — "faskes ini
	tidak lagi muncul untuk warga" — and it does, in `deactivateDescription`. The
	dialog is required rather than optional because the backend has no reverse
	operation: there is no undo, no confirmation-after, and no way for the
	operator to discover the mistake later except by being told.
-->
<AdminConfirmDialog
	open={deactivateTarget !== null}
	title="Nonaktifkan fasilitas?"
	description={deactivateDescription}
	confirmLabel="Ya, nonaktifkan"
	cancelLabel="Batal"
	confirmVariant="danger"
	busy={deactivating}
	onCancel={dismissDeactivate}
	onConfirm={confirmDeactivate}
/>

<AdminMutationFeedback />

{#if deactivateError}
	<p class="sigap-admin-inline-error" role="alert">{deactivateError}</p>
{/if}

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
		margin: 12px 0 0;
		padding: 8px 12px;
		font-size: 13px;
		line-height: 1.45;
		color: var(--sigap-danger);
		background-color: var(--sigap-surface);
		border-left: 3px solid var(--sigap-danger);
	}

	.sigap-facility-row__actions {
		white-space: nowrap;
	}

	.sigap-facility-row__edit,
	.sigap-facility-row__deactivate {
		height: 32px;
		padding: 0 12px;
		font: inherit;
		font-size: 13px;
		font-weight: 500;
		cursor: pointer;
	}

	.sigap-facility-row__edit {
		color: var(--sigap-primary);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-facility-row__deactivate {
		color: var(--sigap-danger);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		margin-left: 6px;
	}

	.sigap-facility-row__edit:focus-visible,
	.sigap-facility-row__deactivate:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-facility-row__code {
		/* The short code is the operator's handle for a facility, so it is styled as
		   an identifier rather than as body text. */
		font-weight: 600;
		letter-spacing: 0.02em;
	}

	.sigap-facility-row__name {
		font-weight: 500;
	}

	.sigap-facility-row__beds {
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	/*
		The status cell carries the one-way notice stacked UNDER the status label.

		It must not push the row past the frozen 40px admin band, and the first
		attempt at that was wrong in an instructive way. Overlaying it absolutely
		kept the row at 40px and produced an ILLEGIBLE row: the notice collided
		with the status badge and was clipped mid-word, which a passing height
		assertion happily reported as fixed. The measured 41px was only one pixel
		over, so the honest fix is to make the content genuinely fit rather than to
		hide the overflow.

		Two things buy that pixel back:

		  - `line-height: 1` on an 11px label makes its line box 11px instead of
		    ~13px. That is the whole deficit, and it costs nothing visually at
		    this size.
		  - `nowrap` keeps the notice on ONE line, so it can never wrap and grow
		    the row — the failure mode that would otherwise come back the first
		    time a longer facility name is used.

		`position: relative` is retained deliberately: it establishes the cell as
		the positioning context so the notice cannot be captured by an ancestor
		with a transform, and it costs no layout when nothing inside is absolute.
	*/
	.sigap-facility-row__state {
		position: relative;
		white-space: nowrap;
	}

	.sigap-facility-row__permanent {
		display: block;
		margin-top: 1px;
		font-size: 11px;
		line-height: 1;
		color: var(--sigap-muted);
		white-space: nowrap;
	}

	.sigap-facility-row__note {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
