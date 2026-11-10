<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import LoadingState from '$lib/ui/LoadingState.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import UnauthPanel from '$lib/ui/UnauthPanel.svelte';
	import StatusBadge from '$lib/ui/StatusBadge.svelte';
	import Button from '$lib/ui/Button.svelte';
	import Dialog from '$lib/ui/Dialog.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { CalendarX } from 'lucide-svelte';
	import { listMyAppointments } from '$lib/api/endpoints/patient';
	import { cancelAppointmentByCode } from '$lib/api/endpoints/citizen';
	import { describe, type ApiError } from '$lib/api/errors';
	import { formatDateTime } from '$lib/domain/format';
	import { APPOINTMENT_STATUS_LABEL, appointmentTone } from '$lib/domain/status';
	import type { AppointmentStatus, MyAppointment } from '$lib/api/types/api';
	import type { PageData } from './$types';

	export let data: PageData;

	/**
	 * /patient/appointments — "Janji Saya".
	 *
	 * The account-scoped counterpart to the code-keyed status page. There is no
	 * input and no check-in code: the list is whatever the authenticated citizen
	 * account owns, resolved server-side. A code never drives this list.
	 *
	 * Five states are reachable and each has a distinct presentation:
	 *   - loading: the first fetch is in flight;
	 *   - unauthorized: there is no session, so there is nothing to show;
	 *   - expired: a session exists but the backend rejected it (a 401);
	 *   - failure: the request failed for a network/server reason;
	 *   - ready: a successful list, which is either empty or rendered.
	 *
	 * Cancel is wired to the code-keyed cancel endpoint, but only for the
	 * citizen's OWN scheduled appointments — and the server re-checks ownership
	 * on that endpoint anyway, so the button here is a convenience, not an
	 * authorization decision.
	 */

	/** The one session fact the client is allowed to know: whether a valid
	 * session cookie existed server-side. From layout data, not from the client
	 * store (which this branch never populates). */
	$: hasSession = data.hasSession ?? false;

	type View = 'loading' | 'unauthorized' | 'expired' | 'failure' | 'ready';

	let view: View = 'loading';
	let appointments: MyAppointment[] = [];
	let error: ApiError | null = null;

	async function load(): Promise<void> {
		if (!hasSession) {
			view = 'unauthorized';
			return;
		}
		view = 'loading';
		error = null;
		const result = await listMyAppointments();
		if (result.ok) {
			appointments = result.data;
			view = 'ready';
		} else {
			error = result.error;
			// A 401 from the backend while a session is present means the token
			// was rejected — a finished session, not a missing one.
			view = result.error.kind === 'unauthorized' ? 'expired' : 'failure';
		}
	}

	onMount(() => {
		void load();
	});

	/**
	 * Cancel state, orthogonal to the list.
	 *
	 * `cancelTarget` doubles as the dialog-open signal: null means closed, a
	 * value means open and names the appointment whose code will be posted.
	 */
	let cancelTarget: MyAppointment | null = null;
	let cancelling = false;
	let cancelError: ApiError | null = null;

	function openCancel(appt: MyAppointment) {
		cancelTarget = appt;
		cancelError = null;
	}

	function closeCancel() {
		if (cancelling) return;
		cancelTarget = null;
		cancelError = null;
	}

	async function confirmCancel(): Promise<void> {
		if (!cancelTarget || cancelling) return;
		cancelling = true;
		cancelError = null;
		const result = await cancelAppointmentByCode(cancelTarget.checkin_code);
		cancelling = false;
		if (result.ok) {
			cancelTarget = null;
			// Refresh rather than locally slice: the server decided the new state.
			await load();
		} else {
			cancelError = result.error;
		}
	}

	function canCancel(status: AppointmentStatus): boolean {
		// The backend only permits the citizen-controlled transition out of
		// 'scheduled'. Everything else is staff's to handle.
		return status === 'scheduled';
	}

	function statusLabel(status: AppointmentStatus): string {
		return APPOINTMENT_STATUS_LABEL[status];
	}

	$: cancelMessage = cancelError ? describe(cancelError, hasSession) : '';
</script>

<svelte:head>
	<title>Janji Saya — Sigap</title>
</svelte:head>

<div class="sigap-page sigap-page--tight sigap-janjisaya">
	<PageHeader
		rule="compact"
		title="Janji saya"
		description="Daftar janji temu yang terdaftar pada akun Anda, dari yang paling baru."
	/>

	{#if view === 'loading'}
		<LoadingState label="Memuat janji temu Anda" showLabel />
	{:else if view === 'unauthorized'}
		<UnauthPanel
			title="Silakan masuk"
			description="Masuk terlebih dahulu untuk melihat daftar janji temu Anda."
			signInHref="/auth/login"
			signInLabel="Masuk"
		/>
	{:else if view === 'expired'}
		<UnauthPanel
			title="Sesi berakhir"
			description="Sesi Anda telah berakhir. Silakan masuk kembali untuk melihat janji temu Anda."
			signInHref="/auth/login"
			signInLabel="Masuk kembali"
		/>
	{:else if view === 'failure' && error}
		<ErrorState {error} {hasSession} onRetry={() => void load()} retryLabel="Muat ulang" />
	{:else if appointments.length === 0}
		<EmptyState
			title="Belum ada janji temu"
			description="Anda belum memiliki janji temu. Mulai dengan membuat janji temu baru di faskes terdekat."
			icon={CalendarX}
			actionLabel="Buat janji temu"
			onAction={() => goto('/appointments/new')}
		/>
	{:else}
		<section class="sigap-janjisaya__list" aria-label="Daftar janji temu Anda">
			{#each appointments as appt (appt.id)}
				<article class="sigap-janjisaya__card" style:border-radius={RADIUS.panel}>
					<div class="sigap-janjisaya__card-main">
						<h2 class="sigap-janjisaya__facility">{appt.facility_name}</h2>
						{#if appt.service_unit_name}
							<p class="sigap-janjisaya__unit">{appt.service_unit_name}</p>
						{/if}
						<p class="sigap-janjisaya__time">{formatDateTime(appt.appointment_time)}</p>
					</div>
					<div class="sigap-janjisaya__card-side">
						<StatusBadge
							label={statusLabel(appt.appointment_status)}
							tone={appointmentTone(appt.appointment_status)}
						/>
						{#if canCancel(appt.appointment_status)}
							<Button
								variant="danger"
								label="Batalkan"
								onClick={() => openCancel(appt)}
							/>
						{/if}
					</div>
				</article>
			{/each}
		</section>
	{/if}

	<Dialog
		open={cancelTarget !== null}
		title="Batalkan janji temu"
		description={
			cancelTarget
				? 'Janji temu di ' +
				  cancelTarget.facility_name +
				  ' pada ' +
				  formatDateTime(cancelTarget.appointment_time) +
				  ' akan dibatalkan. Tindakan ini tidak dapat dibatalkan.'
				: ''
		}
		onClose={closeCancel}
	>
		{#if cancelMessage}
			<p class="sigap-janjisaya__cancel-error" role="alert">{cancelMessage}</p>
		{/if}
		<svelte:fragment slot="footer">
			<Button variant="ghost" label="Kembali" onClick={closeCancel} disabled={cancelling} />
			<Button
				variant="danger"
				label={cancelling ? 'Membatalkan…' : 'Ya, batalkan'}
				onClick={() => void confirmCancel()}
				disabled={cancelling}
			/>
		</svelte:fragment>
	</Dialog>
</div>

<style>
	.sigap-janjisaya__list {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-3);
	}

	.sigap-janjisaya__card {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-janjisaya__card-main {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.sigap-janjisaya__facility {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-janjisaya__unit {
		margin: 0;
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-janjisaya__time {
		margin: 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-janjisaya__card-side {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 8px;
		flex-shrink: 0;
	}

	.sigap-janjisaya__cancel-error {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
		color: var(--sigap-danger);
	}

	@media (max-width: 480px) {
		.sigap-janjisaya__card {
			flex-direction: column;
		}

		.sigap-janjisaya__card-side {
			flex-direction: row;
			align-items: center;
			justify-content: space-between;
			width: 100%;
		}
	}
</style>