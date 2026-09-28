<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import { listFacilities } from '$lib/api/endpoints/admin';
	import { facilityStateLabel, isFacilityActive } from '$lib/admin/facilityState';
	import { formatNumber } from '$lib/domain/format';
	import { isEmptyScope } from '$lib/admin/readState';
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
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Fasilitas"
		subtitle="Fasilitas dalam lingkup akses Anda. Penonaktifan bersifat permanen dan tidak dapat dibatalkan."
	/>

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
				{ label: 'Status' }
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
				</tr>
			{/each}
		</DataTable>

		<p class="sigap-facility-row__note">
			Menampilkan {formatNumber(facilities.length)} fasilitas. Penonaktifan mengubah status
			menjadi nonaktif secara permanen; fasilitas tidak dapat diaktifkan kembali.
		</p>
	</AdminReadState>
</div>

<style>
	.sigap-admin-page {
		min-width: 0;
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

	.sigap-facility-row__state {
		white-space: nowrap;
	}

	.sigap-facility-row__permanent {
		display: block;
		font-size: 11px;
		color: var(--sigap-muted);
	}

	.sigap-facility-row__note {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
