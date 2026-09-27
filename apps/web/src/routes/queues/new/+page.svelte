<script lang="ts">
	import { onMount, tick } from 'svelte';
	import WalkInForm from '$lib/citizen/WalkInForm.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { TicketCheck, Copy, Check } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { listPublicFacilities, generateQueueTicket, normalizeQueueTicket } from '$lib/api/endpoints/public';
	import { isAbort, type ApiError } from '$lib/api/errors';
	import { hasSession as readHasSession } from '$lib/stores/session';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import type { PublicFacility } from '$lib/api/types/api';
	import type { Option } from '$lib/ui/Select.svelte';
	import {
		ESTIMATE_CAVEAT,
		classifyTransactionFailure,
		estimatedWaitLabel,
		resolveFailureMessage,
		type FlowFailure
	} from '$lib/citizen/citizenFlow';
	import type { WalkInTicket } from '$lib/api/endpoints/public';

	/**
	 * /queues/new — walk-in queue registration.
	 *
	 * Deliberately separate from /appointments/check-in. The two produce the
	 * same kind of artefact, a queue number, and it is tempting to merge them.
	 * They are not the same act: this one registers a new ticket for someone
	 * who arrived without an appointment, while check-in redeems a code against
	 * a booking that already exists. Merging them would let a walk-in
	 * registration mutate an appointment's state, and would make the check-in
	 * page's careful 401 handling ambiguous.
	 *
	 * So this page calls exactly one endpoint, POST /api/v1/queues/generate,
	 * and a test asserts it never reaches for the appointment check-in route.
	 */

	type LoadState = 'loading' | 'ready' | 'error';

	let facilities: PublicFacility[] = [];
	let catalogState: LoadState = 'loading';
	let catalogError: ApiError | null = null;

	let facilityId = '';
	let fullName = '';
	let phone = '';
	let submitting = false;
	let ticket: WalkInTicket | null = null;
	let failure: FlowFailure | null = null;
	let copyHint = '';
	let copied = false;

	// The store exposes a getter function, not a value. Calling it keeps the
	// prop a plain boolean, which is all ErrorState is allowed to know about.
	$: hasSession = readHasSession();

	$: facilityOptions = facilities.map<Option>((facility) => ({
		value: facility.id,
		label: facility.name
	}));

	async function loadFacilities() {
		const outcome = await listPublicFacilities();
		if (outcome.ok) {
			facilities = outcome.data;
			catalogState = 'ready';
			catalogError = null;
		} else if (!isAbort(outcome.error)) {
			catalogState = 'error';
			catalogError = outcome.error;
		}
	}

	async function submit() {
		if (submitting) return;
		submitting = true;
		failure = null;
		copyHint = '';
		copied = false;

		const outcome = await generateQueueTicket({
			facilityId,
			fullName: String(fullName ?? "").trim(),
			phone: String(phone ?? "").trim()
		});

		submitting = false;
		if (outcome.ok) {
			/*
			 * Normalized at the wire boundary. `readFormattedNumber` accepts
			 * either casing so a queue number can never render as a dash, and
			 * the ticket the page holds is already in one shape. A dash where
			 * the number should be looks like a server failure, and a citizen
			 * would read that at the counter.
			 */
			ticket = normalizeQueueTicket(outcome.data);
			await tick();
			document.getElementById('sigap-walkin-result')?.focus();
			return;
		}
		if (isAbort(outcome.error)) return;

		failure = classifyTransactionFailure(outcome.error);
	}

	async function copyNumber() {
		if (!ticket?.formattedNumber) return;
		try {
			await navigator.clipboard.writeText(ticket.formattedNumber);
			copyHint = 'Nomor antrean tersalin.';
			copied = true;
		} catch {
			copyHint = 'Tidak dapat menyalin otomatis. Salin manual dari kotak nomor.';
		}
		setTimeout(() => {
			copyHint = '';
			copied = false;
		}, 4000);
	}

	function reset() {
		ticket = null;
		failure = null;
		copyHint = '';
		copied = false;
		facilityId = '';
		fullName = '';
		phone = '';
	}

	onMount(() => {
		const params = new URLSearchParams(window.location.search);
		const preselected = params.get('facility_id') ?? '';
		void loadFacilities().then(() => {
			if (preselected && facilities.some((f) => f.id === preselected)) {
				facilityId = preselected;
			}
		});
	});
</script>

<svelte:head>
	<title>Ambil Antrean Walk-In — Sigap</title>
</svelte:head>

<div class="sigap-walkin">
	<section class="sigap-walkin__intro" aria-labelledby="sigap-walkin-title">
		<h1 id="sigap-walkin-title" class="sigap-page-title">Ambil Antrean Walk-In</h1>
		<p class="sigap-page-subtitle">
			Untuk kunjungan tanpa janji temu. Datang langsung ke fasilitas dan ambil nomor antrean di
			sini.
		</p>
	</section>

	<!--
		The counterpart of the check-in page's walk-in link. Someone who already
		has a booking and a code should use that route instead, and telling them
		so is cheaper than explaining a duplicate ticket later.
	-->
	<nav class="sigap-walkin__alt" aria-label="Alternatif check-in">
		<span class="sigap-walkin__alt-label">Sudah punya janji temu?</span>
		<a class="sigap-walkin__alt-link" style:border-radius={RADIUS.control} href="/appointments/check-in">
			Gunakan kode check-in
		</a>
	</nav>

	{#if catalogState === 'error' && catalogError}
		<section class="sigap-walkin__section">
			<ErrorState
				error={catalogError}
				{hasSession}
				onRetry={loadFacilities}
				retryLabel="Muat ulang"
			/>
		</section>
	{:else if ticket}
		<section
			class="sigap-walkin__ticket"
			style:border-radius={RADIUS.panel}
			id="sigap-walkin-result"
			tabindex="-1"
			aria-labelledby="sigap-walkin-confirmed-title"
		>
			<div class="sigap-walkin__ticket-head" role="status">
				<span class="sigap-walkin__ticket-icon" aria-hidden="true">
					<Icon icon={TicketCheck} size={20} />
				</span>
				<div>
					<h2 class="sigap-walkin__ticket-title" id="sigap-walkin-confirmed-title">
						Nomor antrean Anda
					</h2>
					<p class="sigap-walkin__ticket-sub">
						Simpan nomor ini dan tunjukkan di loket fasilitas yang Anda pilih.
					</p>
				</div>
			</div>

			<div class="sigap-walkin__number-block">
				<p class="sigap-walkin__number-label" id="sigap-walkin-number-label">Nomor antrean</p>
				<p
					class="sigap-walkin__number"
					aria-labelledby="sigap-walkin-number-label"
					data-testid="walkin-number"
				>
					{ticket.formattedNumber}
				</p>
				<Button variant="secondary" icon={copied ? Check : Copy} onClick={copyNumber}>
					{copied ? 'Tersalin' : 'Salin nomor'}
				</Button>
			</div>

			{#if ticket.estimatedWaitMinutes > 0}
				<div class="sigap-walkin__wait">
					<p class="sigap-walkin__wait-value">
						{estimatedWaitLabel(ticket.estimatedWaitMinutes)}
					</p>
					<!-- Printed next to the number, not hidden behind a tooltip. -->
					<p class="sigap-walkin__wait-caveat">{ESTIMATE_CAVEAT}</p>
				</div>
			{/if}

			<div class="sigap-walkin__actions">
				<a
					class="sigap-walkin__cta"
					style:border-radius={RADIUS.control}
					style:background-color="var(--sigap-primary)"
					style:color="var(--sigap-surface)"
					href="/patient/status"
				>
					Cek status kunjungan
				</a>
				<Button variant="ghost" onClick={reset}>Ambil antrean lain</Button>
			</div>

			<p class="sigap-walkin__copy-hint" role="status" aria-live="polite">{copyHint}</p>
		</section>
	{:else}
		<section class="sigap-walkin__section" style:border-radius={RADIUS.panel}>
			{#if failure}
				<!--
					Verbatim. The daily-limit 429 in particular explains the limit
					and when it resets, which is the only thing that tells someone
					whether to wait until tomorrow or go to another facility.
				-->
				<div class="sigap-walkin__failure" role="alert" data-testid="walkin-error">
					{resolveFailureMessage(failure)}
				</div>
			{/if}

			<WalkInForm
				bind:facilityId
				bind:fullName
				bind:phone
				{facilityOptions}
				{submitting}
				disabled={catalogState === 'loading'}
				onSubmit={submit}
			/>
		</section>
	{/if}
</div>

<style>
	.sigap-walkin {
		max-width: 640px;
		margin: 0 auto;
		padding: 20px 16px 8px;
	}

	@media (min-width: 768px) {
		.sigap-walkin {
			padding: 32px 24px 8px;
		}
	}

	.sigap-walkin__section {
		margin-top: 16px;
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-walkin__alt {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		margin-top: 16px;
	}

	.sigap-walkin__alt-label {
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-walkin__alt-link {
		display: inline-flex;
		align-items: center;
		/* Citizen touch floor. */
		min-height: 44px;
		padding: 0 12px;
		font-size: 13px;
		font-weight: 500;
		color: var(--sigap-primary);
		border: 1px solid var(--sigap-border);
		text-decoration: none;
	}

	.sigap-walkin__alt-link:hover {
		border-color: var(--sigap-primary);
	}

	.sigap-walkin__alt-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-walkin__failure {
		margin-bottom: 16px;
		padding: 12px;
		font-size: 13px;
		color: var(--sigap-danger);
		border: 1px solid var(--sigap-danger);
	}

	.sigap-walkin__ticket {
		margin-top: 16px;
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-walkin__ticket-head {
		display: flex;
		align-items: flex-start;
		gap: 12px;
	}

	.sigap-walkin__ticket-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		flex: none;
		color: var(--sigap-primary);
		border: 1px solid var(--sigap-primary);
	}

	.sigap-walkin__ticket-title {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-walkin__ticket-sub {
		margin: 2px 0 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-walkin__number-block {
		margin-top: 16px;
		padding: 16px;
		text-align: center;
		background-color: var(--sigap-canvas);
		border: 1px solid var(--sigap-border);
	}

	.sigap-walkin__number-label {
		margin: 0 0 4px;
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-muted);
	}

	.sigap-walkin__number {
		margin: 0 0 12px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 36px;
		font-weight: 700;
		letter-spacing: 0.12em;
		color: var(--sigap-foreground);
		user-select: all;
		overflow-wrap: anywhere;
	}

	.sigap-walkin__wait {
		margin-top: 16px;
		padding: 12px;
		border: 1px solid var(--sigap-border);
	}

	.sigap-walkin__wait-value {
		margin: 0;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-walkin__wait-caveat {
		margin: 4px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-walkin__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
		margin-top: 16px;
	}

	.sigap-walkin__cta {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		padding: 0 16px;
		font-size: 14px;
		font-weight: 500;
		text-decoration: none;
	}

	.sigap-walkin__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-walkin__copy-hint {
		margin: 12px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
		min-height: 16px;
	}
</style>
