<script lang="ts">
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { page } from '$app/stores';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminFreshness from '$lib/admin/AdminFreshness.svelte';
	import AdminStatusBadge, { activeStateIcon } from '$lib/admin/AdminStatusBadge.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import LoadingState from '$lib/ui/LoadingState.svelte';
	import { getOrganizationOverview, type AdminOrganizationOverview } from '$lib/api/endpoints/admin';
	import { formatNumber } from '$lib/domain/format';
	import { RADIUS } from '$lib/design/tokens';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import type { ApiError } from '$lib/api/errors';

	/**
	 * T-regional: the per-organization overview.
	 *
	 * Every figure is the backend's scoped aggregate. `GetOrganizationOverview`
	 * re-resolves the caller's organization scope AND their facility read-set on
	 * each request, so the four counts describe only the facilities the actor is
	 * actually authorized to observe — never a blind org-wide join.
	 *
	 * A forged or sibling `id` is answered 404 (not 403), so org existence is
	 * never leaked across organizations. The page therefore collapses 404 into a
	 * single "tidak ditemukan / tidak dapat diakses" state, and does NOT offer a
	 * retry: a retry for a 404 teaches the operator to click through a door the
	 * server will keep closed.
	 */
	let id = get(page).params.id ?? '';
	let overview: AdminOrganizationOverview | null = null;
	let loading = true;
	let error: ApiError | null = null;
	let refreshing = false;
	let loadedAt: Date | null = null;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;

	onMount(async () => {
		id = get(page).params.id ?? '';
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		await loadOverview();
	});

	async function loadOverview() {
		const result = await getOrganizationOverview(id);
		if (result.ok) {
			overview = result.data;
			error = null;
			loadedAt = new Date();
		} else {
			overview = null;
			error = result.error;
		}
		loading = false;
	}

	async function refreshOverview() {
		refreshing = true;
		try {
			await loadOverview();
		} finally {
			refreshing = false;
		}
	}

	function orgStateLabel(active: boolean): string {
		return active ? 'Aktif' : 'Nonaktif';
	}
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Rincian Organisasi"
		subtitle="Angka operasional untuk satu organisasi, dibatasi pada fasilitas dalam lingkup akses Anda."
	>
		<svelte:fragment slot="aside">
			<AdminFreshness
				{loadedAt}
				refreshing={refreshing}
				onRefresh={() => void refreshOverview()}
			/>
		</svelte:fragment>
	</AdminPageHeader>

	{#if loading}
		<LoadingState label="Memuat data organisasi" />
	{:else if error !== null && error.kind === 'not_found'}
		<!--
			The only place org existence across scopes leaks would be a disloyal 404
			message. It does not: a forged id, a sibling org, and a missing
			`organization.read` permission all arrive here, and the page says
			"not found / not authorized" without distinguishing them.
		-->
		<div class="sigap-org-overview-missing" style:border-radius={RADIUS.panel} role="alert">
			<p class="sigap-org-overview-missing__code">404</p>
			<p class="sigap-org-overview-missing__title">Organisasi tidak ditemukan</p>
			<p class="sigap-org-overview-missing__description">
				Organisasi ini tidak dapat diakses oleh akun Anda. Bila Anda yakin
				seharusnya dapat melihatnya, hubungi administrator regional Anda.
			</p>
		</div>
	{:else if error !== null}
		<ErrorState {error} {hasSession} onRetry={() => void loadOverview()} />
	{:else if overview}
		<section class="sigap-org-overview" aria-labelledby="org-overview-name">
			<div class="sigap-org-overview__head">
				<div>
					<p class="sigap-org-overview__eyebrow">Organisasi</p>
					<h2 id="org-overview-name" class="sigap-org-overview__name">
						{overview.name}
					</h2>
					<p class="sigap-org-overview__region">{overview.region || '-'}</p>
				</div>
				<AdminStatusBadge
					label={orgStateLabel(overview.is_active)}
					tone={overview.is_active ? 'success' : 'neutral'}
					size="md"
					icon={activeStateIcon(overview.is_active)}
				/>
			</div>

			<!--
				Labelled lines, not KPI tiles. These four counts are one scoped
				snapshot each, never a comparison or trend, so a row of shadowed
				tiles would claim a dashboard meaning the data does not support.
			-->
			<dl class="sigap-org-overview__counts">
				<div class="sigap-org-overview__count">
					<dt>Fasilitas</dt>
					<dd>{formatNumber(overview.facility_count)}</dd>
				</div>
				<div class="sigap-org-overview__count">
					<dt>Unit layanan</dt>
					<dd>{formatNumber(overview.service_unit_count)}</dd>
				</div>
				<div class="sigap-org-overview__count">
					<dt>Antrean aktif</dt>
					<dd>{formatNumber(overview.active_queue_count)}</dd>
				</div>
				<div class="sigap-org-overview__count">
					<dt>Janji temu</dt>
					<dd>{formatNumber(overview.appointment_count)}</dd>
				</div>
			</dl>

			<p class="sigap-org-overview__note">
				Bukan data real-time. Angka memuat ulang saja saat tombol Perbarui
				ditekan, dan setiap angka hanya mencakup fasilitas dalam lingkup akses
				Anda.
			</p>
		</section>
	{/if}
</div>

<style>
	.sigap-admin-page {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-5);
	}

	.sigap-org-overview {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-6);
		padding: var(--sigap-space-6);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-org-overview__head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--sigap-space-4);
		border-bottom: 1px solid var(--sigap-border-subtle);
		padding-bottom: var(--sigap-space-5);
	}

	.sigap-org-overview__eyebrow {
		margin: 0;
		font-size: var(--sigap-type-caption);
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--sigap-muted);
	}

	.sigap-org-overview__name {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-page-title);
		font-weight: 600;
		line-height: 1.3;
		color: var(--sigap-foreground);
	}

	.sigap-org-overview__region {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-body);
		color: var(--sigap-muted);
	}

	.sigap-org-overview__counts {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		gap: var(--sigap-space-4);
		margin: 0;
	}

	.sigap-org-overview__count {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface-sunken);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-org-overview__count dt {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-org-overview__count dd {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-page-title);
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		color: var(--sigap-foreground);
	}

	.sigap-org-overview__note {
		margin: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-org-overview-missing {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: var(--sigap-space-2);
		padding: var(--sigap-space-6);
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-left: 3px solid var(--sigap-warning);
	}

	.sigap-org-overview-missing__code {
		margin: 0;
		font-size: var(--sigap-type-caption);
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--sigap-muted);
	}

	.sigap-org-overview-missing__title {
		margin: 0;
		font-size: var(--sigap-type-page-title);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-org-overview-missing__description {
		margin: 0;
		max-width: 52ch;
		font-size: var(--sigap-type-body);
		line-height: 1.6;
		color: var(--sigap-muted);
	}
</style>