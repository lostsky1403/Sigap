<script lang="ts">
	import { onMount } from 'svelte';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminFreshness from '$lib/admin/AdminFreshness.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import AdminStatusBadge, { activeStateIcon } from '$lib/admin/AdminStatusBadge.svelte';
	import { listOrganizations, type AdminOrganization } from '$lib/api/endpoints/admin';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type { ApiError } from '$lib/api/errors';

	/**
	 * T-regional: the organization directory.
	 *
	 * READ-ONLY and scoped by the backend. `ListOrganizations` returns only the
	 * organizations inside the caller's organization scope — an org-scoped
	 * `organization_admin` sees its one org, a `regional_admin`/`super_admin`
	 * sees all, and a caller without `organization.read` gets an empty list. The
	 * page renders exactly what it receives and never invents a row, so the
	 * directory is the backend's answer, not a client-side reconstruction.
	 *
	 * Because a missing permission and an empty scope both arrive as `[]`, there
	 * is no separate "denied" pane here: an empty directory is rendered as the
	 * ordinary empty state with copy that points a wrongly-scoped operator at
	 * their regional administrator rather than claiming a permission fact the
	 * client does not hold.
	 */
	let organizations: AdminOrganization[] = [];
	let loading = true;
	let error: ApiError | null = null;
	let refreshing = false;
	let loadedAt: Date | null = null;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	onMount(async () => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		const result = await listOrganizations();
		if (result.ok) {
			organizations = result.data;
			loadedAt = new Date();
		} else {
			error = result.error;
		}
		loading = false;
	});

	async function reloadOrganizations() {
		const result = await listOrganizations();
		if (result.ok) {
			organizations = result.data;
			error = null;
			loadedAt = new Date();
		} else {
			error = result.error;
		}
		loading = false;
	}

	/** Re-reads without blanking the table, so a refresh keeps rows on screen. */
	async function refreshOrganizations() {
		refreshing = true;
		try {
			await reloadOrganizations();
		} finally {
			refreshing = false;
		}
	}

	/**
	 * The org `is_active` is a plain boolean on the wire, but the label is still
	 * paired with a glyph and never a colour alone.
	 */
	function orgStateLabel(active: boolean): string {
		return active ? 'Aktif' : 'Nonaktif';
	}
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Organisasi"
		subtitle="Organisasi dalam lingkup akses Anda. Daftar ini hanya memuat organisasi yang diberikan kepada akun Anda."
	>
		<svelte:fragment slot="aside">
			<AdminFreshness
				{loadedAt}
				refreshing={refreshing}
				onRefresh={() => void refreshOrganizations()}
			/>
		</svelte:fragment>
	</AdminPageHeader>

	<AdminReadState
		{loading}
		{error}
		{hasSession}
		rowCount={organizations.length}
		emptyTitle="Belum ada organisasi"
		emptyDescription="Tidak ada organisasi dalam lingkup akses Anda. Hubungi administrator regional bila akun Anda seharusnya dapat melihat organisasi tertentu."
		onRetry={() => void reloadOrganizations()}
	>
		<DataTable
			caption="Organisasi dalam lingkup akses"
			columns={[
				{ label: 'Nama' },
				{ label: 'Wilayah', secondary: true },
				{ label: 'Status' }
			]}
		>
			{#each organizations as organization (organization.id)}
				{@const active = organization.is_active}
				<tr>
					<td>
						<a
							class="sigap-org-row__name"
							href={`/admin/organizations/${organization.id}`}
						>
							{organization.name}
						</a>
					</td>
					<td class="sigap-table-col-secondary">{organization.region || '-'}</td>
					<td>
						<AdminStatusBadge
							label={orgStateLabel(active)}
							tone={active ? 'success' : 'neutral'}
							size="sm"
							icon={activeStateIcon(active)}
						/>
					</td>
				</tr>
			{/each}
		</DataTable>
	</AdminReadState>
</div>

<style>
	.sigap-admin-page {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-5);
	}

	.sigap-org-row__name {
		color: var(--sigap-primary-strong);
		font-weight: 600;
		text-decoration: none;
	}

	.sigap-org-row__name:hover {
		text-decoration: underline;
	}
</style>