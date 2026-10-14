<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import AdminPageHeader from '$lib/admin/AdminPageHeader.svelte';
	import AdminReadState from '$lib/admin/AdminReadState.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import Skeleton from '$lib/ui/Skeleton.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import {
		cancelNotification,
		getNotificationSummary,
		listFacilities,
		listNotifications,
		retryNotification
	} from '$lib/api/endpoints/admin';
	import { formatRelativeTime, formatNumber } from '$lib/domain/format';
	import { isEmptyScope } from '$lib/admin/readState';
	import { runAdminMutation } from '$lib/admin/adminMutation';
	import AdminMutationFeedback from '$lib/admin/AdminMutationFeedback.svelte';
	import { getSession, subscribeToSession } from '$lib/stores/session';
	import {
		NOTIFICATION_LIMIT_NOTE,
		NOTIFICATION_STATUS_LABEL,
		NOTIFICATION_STATUS_ORDER,
		NOTIFICATION_STATUS_TONE,
		applyFiltersToUrl,
		buildNotificationQuery,
		emptyFilters,
		filtersFromParams,
		hasActiveFilters,
		zeroSummary,
		type NotificationFilters
	} from '$lib/admin/notifications';
	import type { ApiError } from '$lib/api/errors';
	import type {
		AdminFacility,
		NotificationOutboxRow,
		NotificationSummary
	} from '$lib/api/types/api';

	/**
	 * T-3B4-08: the notification outbox read layer.
	 *
	 * This page had the strongest existing behaviour in the admin area and keeps
	 * it. What is corrected:
	 *
	 *  1. `relativeTime` was returning SECONDS as "d" and DAYS as "h" — a copy-paste
	 *     inversion, so a notification sent ten seconds ago read "10d lalu" and one
	 *     sent two days ago read "2h lalu". The Phase 3B1 `formatRelativeTime`
	 *     helper already orders minutes before hours before days correctly and is
	 *     used here instead.
	 *
	 *  2. The list and the summary keep SEPARATE loading and error states. They are
	 *     two requests and they fail independently: a summary outage must not blank
	 *     a list of notifications an operator needs to act on, and a list failure
	 *     must not be hidden behind five plausible-looking zero cards.
	 *
	 * Recipient masking is preserved exactly. The server sends only
	 * `recipient_contact_masked`; the raw contact and the internal dedup hash are
	 * never in the response, and nothing here reconstructs them.
	 */
	let rows: NotificationOutboxRow[] = [];
	let summary: NotificationSummary = zeroSummary();
	let listError: ApiError | null = null;
	let summaryError: ApiError | null = null;
	let listLoading = true;
	let summaryLoading = true;
	let facilities: AdminFacility[] = [];
	let facilitiesLoaded = false;
	let hasSession = getSession().hasSession;
	let unsubscribeSession: (() => void) | undefined;
	let filters: NotificationFilters = emptyFilters();

	const CHANNELS = ['dev', 'sms', 'whatsapp', 'email'] as const;

	/**
	 * Filters are DERIVED from the URL rather than held in local state and mirrored
	 * into it. The URL is the single source of truth, which is what makes a pasted
	 * link reproduce the operator's view exactly and what stops the two from
	 * drifting after a back/forward navigation.
	 */
	$: filters = filtersFromParams($page.url.searchParams);

	function onFilterChange<K extends keyof NotificationFilters>(
		key: K,
		value: NotificationFilters[K]
	) {
		const next = { ...filters, [key]: value };
		const url = applyFiltersToUrl(new URL($page.url), next);
		// replaceState rather than pushState: typing in a filter should not fill the
		// browser history with one entry per keystroke.
		history.replaceState(history.state, '', url.toString());
	}

	function clearFilters() {
		const url = applyFiltersToUrl(new URL($page.url), emptyFilters());
		history.replaceState(history.state, '', url.toString());
	}

	onMount(() => {
		unsubscribeSession = subscribeToSession((session) => {
			hasSession = session.hasSession;
		});

		// The list and the summary are independent requests with independent
		// lifetimes, so they are NOT awaited together. Combining them would mean a
		// summary outage delayed the list, and a list outage blanked the cards.
		listNotifications(buildNotificationQuery(filters))
			.then((result) => {
				if (result.ok) rows = result.data;
				else listError = result.error;
			})
			.finally(() => (listLoading = false));

		getNotificationSummary()
			.then((result) => {
				if (result.ok) summary = result.data;
				else summaryError = result.error;
			})
			.finally(() => (summaryLoading = false));

		// The scoped-facility read backs the class-1 empty state. It is separate
		// again, and its failure never becomes an empty-scope verdict: telling an
		// operator they have no facilities because one request failed is worse than
		// saying nothing.
		listFacilities()
			.then((result) => {
				if (!result.ok) return;
				facilities = result.data;
				facilitiesLoaded = true;
			})
			.catch(() => {
				/* left unloaded on purpose; see above */
			});
	});

	/**
	 * The recipient column shows ONLY the masked form the server sent.
	 *
	 * There is deliberately no fallback to any other field. A masked value like
	 * `b••••@contoh.id` is what the server chose to disclose, and inventing a
	 * longer version of it from another field would defeat the masking.
	 */
	function recipientOf(row: NotificationOutboxRow): string {
		return row.recipient_contact_masked || '-';
	}

	/* ------------------------- T-3B5-05 mutations ------------------------- */

	/**
	 * ACTIONABILITY COMES FROM THE SERVER AND ONLY FROM THE SERVER.
	 *
	 * §12 forbids recomputing `notification.manage` here, and forbids inferring
	 * it from the email, the role, the facility, or the status. The reasons that
	 * rule exists are worth keeping in view, because each of them is a way this
	 * code could have been written and been wrong:
	 *
	 *   - The permission is FACILITY-SCOPED, and the row stores one facility. An
	 *     operator may read the whole outbox and still not manage notifications
	 *     for the facility a particular row belongs to. Any global "is admin"
	 *     check would offer Retry on rows whose facility the operator cannot touch.
	 *   - STATUS alone is not permission. A `failed` row is retryable BY STATUS;
	 *     whether THIS operator may retry it is a separate question with a
	 *     separate answer.
	 *
	 * The server answers both, per row, as `can_retry` and `can_cancel`. Those
	 * two booleans are the entire input to the visibility rule below.
	 *
	 * AND THEY ARE ADVISORY ONLY. The mutation endpoints re-authorize
	 * independently on every call, so a row that still shows Retry after a grant
	 * is revoked will be refused — which is correct, and the refusal is shown
	 * with the server's own words.
	 */
	let pendingId = '';
	let pendingAction: 'retry' | 'cancel' | '' = '';
	let errorId = '';
	let errorMessage = '';

	/**
	 * The re-read after a mutation, preserving the URL filters.
	 *
	 * The filter set is held in the URL, not in local state, so a plain re-read
	 * naturally keeps the operator's narrowed view. Re-deriving `filters` here
	 * would be the bug: it is already reactive on `$page.url.searchParams`.
	 */
	async function reloadNotifications() {
		const [listResult, summaryResult] = await Promise.all([
			listNotifications(buildNotificationQuery(filters)),
			getNotificationSummary()
		]);

		if (listResult.ok) {
			rows = listResult.data;
			listError = null;
			// The actionability booleans belong to the rows just re-read, so any
			// rejection from a previous attempt is stale: the row may no longer be
			// in the state that produced it.
			errorId = '';
			errorMessage = '';
		} else {
			listError = listResult.error;
		}

		// The summary is refreshed but a failure is deliberately NOT surfaced
		// here: it has its own inline error state with its own retry, and
		// overwriting it from a mutation would replace a persistent problem with a
		// transient-looking one.
		if (summaryResult.ok) {
			summary = summaryResult.data;
		}
	}

	async function handleAction(row: NotificationOutboxRow, action: 'retry' | 'cancel') {
		errorId = '';
		errorMessage = '';
		pendingId = row.id;
		pendingAction = action;

		const outcome = await runAdminMutation(
			() => (action === 'retry' ? retryNotification(row.id) : cancelNotification(row.id)),
			{
				subject: recipientOf(row),
				fromLabel: NOTIFICATION_STATUS_LABEL[row.status],
				toLabel:
					action === 'retry'
						? NOTIFICATION_STATUS_LABEL.pending
						: NOTIFICATION_STATUS_LABEL.cancelled,
				noun: 'Notifikasi',
				reload: reloadNotifications
			},
			hasSession
		);

		pendingId = '';
		pendingAction = '';

		// §13: a rejected mutation is 409, and the message is the server's.
		// "Retry tidak diizinkan untuk status saat ini." names the actual rule;
		// any client-side substitute would be a guess about which rule fired.
		if (!outcome.ok && outcome.failure) {
			errorId = row.id;
			errorMessage = outcome.failure.message;
		}
	}
</script>

<div class="sigap-admin-page">
	<AdminPageHeader
		title="Notifikasi"
		subtitle="Outbox notifikasi dalam lingkup akses Anda. Penerima ditampilkan dalam bentuk tersamar."
	/>

	<!--
		Summary cards, with their OWN loading and error states.

		Five `aria-busy` skeletons while the counts load, and a single inline error
		if the summary fails — deliberately separate from the list's error below, so
		one failing endpoint never masks the other's state.
	-->
	<section class="sigap-notif-summary" aria-label="Ringkasan notifikasi">
		{#if summaryLoading}
			<div class="sigap-notif-summary__grid" aria-busy="true">
				{#each NOTIFICATION_STATUS_ORDER as status (status)}
					<div class="sigap-notif-summary__card">
						<Skeleton height="22px" width="100%" />
					</div>
				{/each}
			</div>
		{:else if summaryError}
			<div class="sigap-notif-summary__error" role="alert" style:border-radius={RADIUS.panel}>
				Ringkasan notifikasi tidak dapat dimuat. Daftar notifikasi di bawah tetap dapat
				dibaca.
			</div>
		{:else}
			<div class="sigap-notif-summary__grid">
				{#each NOTIFICATION_STATUS_ORDER as status (status)}
					<div class="sigap-notif-summary__card" style:border-radius={RADIUS.panel}>
						<StatusBadge
							label={NOTIFICATION_STATUS_LABEL[status]}
							tone={NOTIFICATION_STATUS_TONE[status]}
							size="sm"
						/>
						<!--
							These counts come from the scoped summary endpoint only. They are
							never derived from the filtered list below, which would produce a
							number scoped to the current filter and present it as a total.
						-->
						<p class="sigap-notif-summary__count">{formatNumber(summary[status])}</p>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<div class="sigap-notif-filters">
		<div class="sigap-admin-field">
			<label class="sigap-admin-field__label" for="notif-status">Status</label>
			<select
				class="sigap-admin-field__select"
				style:border-radius={RADIUS.control}
				id="notif-status"
				value={filters.status}
				on:change={(event) => onFilterChange('status', event.currentTarget.value)}
			>
				<option value="">Semua status</option>
				{#each NOTIFICATION_STATUS_ORDER as status (status)}
					<option value={status}>{NOTIFICATION_STATUS_LABEL[status]}</option>
				{/each}
			</select>
		</div>

		<div class="sigap-admin-field">
			<label class="sigap-admin-field__label" for="notif-channel">Kanal</label>
			<select
				class="sigap-admin-field__select"
				style:border-radius={RADIUS.control}
				id="notif-channel"
				value={filters.channel}
				on:change={(event) => onFilterChange('channel', event.currentTarget.value)}
			>
				<option value="">Semua kanal</option>
				{#each CHANNELS as channel (channel)}
					<option value={channel}>{channel}</option>
				{/each}
			</select>
		</div>

		<div class="sigap-admin-field">
			<label class="sigap-admin-field__label" for="notif-template">Template</label>
			<input
				class="sigap-admin-field__input"
				style:border-radius={RADIUS.control}
				id="notif-template"
				type="search"
				placeholder="template_key"
				value={filters.templateKey}
				on:change={(event) => onFilterChange('templateKey', event.currentTarget.value)}
			/>
		</div>

		<div class="sigap-admin-field">
			<label class="sigap-admin-field__label" for="notif-from">Dibuat dari</label>
			<input
				class="sigap-admin-field__input"
				style:border-radius={RADIUS.control}
				id="notif-from"
				type="date"
				value={filters.createdFrom}
				on:change={(event) => onFilterChange('createdFrom', event.currentTarget.value)}
			/>
		</div>

		<div class="sigap-admin-field">
			<label class="sigap-admin-field__label" for="notif-to">Dibuat sampai</label>
			<input
				class="sigap-admin-field__input"
				style:border-radius={RADIUS.control}
				id="notif-to"
				type="date"
				value={filters.createdTo}
				on:change={(event) => onFilterChange('createdTo', event.currentTarget.value)}
			/>
		</div>
	</div>

	<!--
		The LIST's own state, separate from the summary's above.
	-->
	<AdminReadState
		loading={listLoading}
		error={listError}
		{hasSession}
		rowCount={rows.length}
		scopeEmpty={facilitiesLoaded && isEmptyScope(facilities)}
		emptyTitle="Belum ada notifikasi"
	>
		{#if rows.length === 0}
			<div class="sigap-admin-nomatch" style:border-radius={RADIUS.panel} role="status">
				<p class="sigap-admin-nomatch__title">
					{hasActiveFilters(filters)
						? 'Tidak ada notifikasi yang cocok'
						: 'Belum ada notifikasi dalam lingkup operator saat ini.'}
				</p>
				{#if hasActiveFilters(filters)}
					<p class="sigap-admin-nomatch__body">Ubah atau hapus filter Anda.</p>
					<button
						type="button"
						class="sigap-admin-nomatch__reset"
						style:border-radius={RADIUS.control}
						on:click={clearFilters}
					>
						Hapus filter
					</button>
				{/if}
			</div>
		{:else}
			<DataTable
				caption="Outbox notifikasi dalam lingkup akses"
				columns={[
					{ label: 'Dibuat' },
					{ label: 'Kanal' },
					{ label: 'Subjek' },
					{ label: 'Penerima', secondary: true },
					{ label: 'Fasilitas', secondary: true },
					{ label: 'Status' },
					{ label: 'Percobaan', secondary: true, numeric: true },
				{ label: 'Aksi' }
				]}
			>
				{#each rows as row (row.id)}
					<tr>
						<td class="sigap-notif-row__time">
							<!--
								`formatRelativeTime` from Phase 3B1, not the local helper this
								page used to carry. The old one returned seconds as "d" and days
								as "h", so a notification ten seconds old read "10d lalu".
							-->
							{formatRelativeTime(row.created_at)}
						</td>
						<td>{row.channel}</td>
						<td class="sigap-notif-row__subject">{row.subject}</td>
						<td class="sigap-table-col-secondary">{recipientOf(row)}</td>
						<!--
							Always "-". A notification row carries `facility_id`, but
							resolving it to a name here would need a facility catalog this
							page does not load, and falling back to the raw id would put a
							UUID in the UI. The join rule says: no label, so "-".
						-->
						<td class="sigap-table-col-secondary">-</td>
						<td>
							<StatusBadge
								label={NOTIFICATION_STATUS_LABEL[row.status]}
								tone={NOTIFICATION_STATUS_TONE[row.status]}
								size="sm"
							/>
						</td>
						<td class="sigap-table-col-secondary sigap-notif-row__attempts">
						{formatNumber(row.attempt_count)}
					</td>
					<td class="sigap-notif-row__actions">
						<!--
							THE ONLY INPUT IS THE SERVER'S TWO BOOLEANS.

							Retry appears if and only if `can_retry` is true; Cancel if and
							only if `can_cancel` is true. There is no status test here, no
							role test, no facility test, and no email-shaped heuristic —
							§12 rules all of those out, and each would produce a control
							that exists only to be refused.
						-->
						{#if row.can_retry}
							<button
								type="button"
								class="sigap-notif-row__action"
								style:border-radius={RADIUS.control}
								disabled={pendingId === row.id}
								on:click={() => handleAction(row, 'retry')}
							>
								{pendingId === row.id && pendingAction === 'retry'
									? 'Mengirim ulang...'
									: 'Kirim ulang'}
							</button>
						{/if}
						{#if row.can_cancel}
							<button
								type="button"
								class="sigap-notif-row__action sigap-notif-row__action--danger"
								style:border-radius={RADIUS.control}
								disabled={pendingId === row.id}
								on:click={() => handleAction(row, 'cancel')}
							>
								{pendingId === row.id && pendingAction === 'cancel'
									? 'Membatalkan...'
									: 'Batalkan'}
							</button>
						{/if}
						<!--
							A row with neither action is not an error and not empty by
							accident — it is a settled or unauthorized row. Saying so keeps
							an operator from wondering whether the buttons failed to load.
						-->
						{#if !row.can_retry && !row.can_cancel}
							<span class="sigap-notif-row__noaction">Tidak ada aksi</span>
						{/if}
					</td>
				</tr>
				{#if errorId === row.id && errorMessage !== ''}
					<tr class="sigap-notif-row__outcome">
						<td colspan="8" class="sigap-notif-row__outcome-cell">
							<!--
								VERBATIM. The 409 message from the server names the rule that
								actually refused the action; substituting our own wording
								would hide which one it was.
							-->
							<span role="alert">{errorMessage}</span>
						</td>
					</tr>
				{/if}
				{/each}
			</DataTable>
		{/if}
	</AdminReadState>

	<p class="sigap-notif-row__note">{NOTIFICATION_LIMIT_NOTE}</p>
</div>

<!--
	The polite live region and the toast queue, mounted once per page.

	No dialog on this page: retry and cancel are both reversible enough states to
	act on directly, and §11 requires a dialog only where an action cannot be
	taken back. A failed delivery re-sent, or a pending one cancelled, are
	recoverable by another action on the same row.
-->
<AdminMutationFeedback />

<style>
	.sigap-admin-page {
		min-width: 0;
	}

	.sigap-notif-row__actions {
		white-space: nowrap;
	}

	.sigap-notif-row__action {
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

	.sigap-notif-row__action--danger {
		color: var(--sigap-danger);
		margin-left: 6px;
	}

	.sigap-notif-row__action:disabled {
		color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-notif-row__action:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-notif-row__noaction {
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-notif-row__outcome-cell {
		padding: 8px 12px;
		font-size: 13px;
		line-height: 1.45;
		background-color: var(--sigap-surface);
		border-left: 3px solid var(--sigap-border);
	}

	.sigap-notif-summary {
		margin-bottom: 16px;
	}

	.sigap-notif-summary__grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
		gap: 12px;
	}

	.sigap-notif-summary__card {
		display: flex;
		flex-direction: column;
		gap: 6px;
		padding: 12px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-notif-summary__count {
		margin: 0;
		font-size: 20px;
		font-weight: 600;
		color: var(--sigap-foreground);
		font-variant-numeric: tabular-nums;
	}

	.sigap-notif-summary__error {
		padding: 12px;
		font-size: 13px;
		color: var(--sigap-danger);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-left: 3px solid var(--sigap-danger);
	}

	.sigap-notif-filters {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 12px;
		margin-bottom: 12px;
	}

	.sigap-admin-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.sigap-admin-field__label {
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-admin-field__select,
	.sigap-admin-field__input {
		height: 36px;
		min-width: 150px;
		padding: 0 10px;
		border: 1px solid var(--sigap-border);
		background-color: var(--sigap-surface);
		color: var(--sigap-foreground);
		font: inherit;
		font-size: 13px;
	}

	.sigap-admin-field__select:focus-visible,
	.sigap-admin-field__input:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-notif-row__time {
		color: var(--sigap-muted);
		white-space: nowrap;
	}

	.sigap-notif-row__subject {
		font-weight: 500;
	}

	.sigap-notif-row__attempts {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}

	.sigap-notif-row__note {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
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
