<script lang="ts">
	import { onMount, tick } from 'svelte';
	import WalkInForm from '$lib/citizen/WalkInForm.svelte';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { AlertTriangle, TicketCheck, Copy, Check } from 'lucide-svelte';
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

<div class="sigap-page sigap-walkin">
	<PageHeader
		title="Ambil antrean tanpa janji"
		description="Datang langsung ke fasilitas dan ambil nomor antrean. Tidak perlu membuat janji temu terlebih dahulu."
	/>

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
					<h2 class="sigap-section-title" id="sigap-walkin-confirmed-title">
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
					class="sigap-walkin__number sigap-identifier"
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
				<!--
					There is deliberately no "check your status" link here.

					`/patient/status` resolves CHECK-IN CODES only. A queue number
					is sequential per facility per day ("PHB-0004"), so accepting
					it would let anyone walk the sequence and read other patients'
					visit status — the enumeration vector that endpoint exists to
					close. Offering the link anyway sent every walk-in patient to
					a lookup that could only ever answer "Kode tidak ditemukan",
					and the page then told them their number could not be used
					there: a dead end presented as a feature.

					What a walk-in patient can actually do is watch the facility's
					waiting-room display, which is where the number is called.
				-->
				<Button variant="ghost" onClick={reset}>Ambil antrean lain</Button>
			</div>

			<p class="sigap-walkin__copy-hint" role="status" aria-live="polite">{copyHint}</p>

			<p class="sigap-walkin__track-note" data-testid="walkin-track-note">
				Pantau nomor ini di layar ruang tunggu fasilitas. Untuk memantau dari ponsel,
				Anda memerlukan kode check-in, yang hanya diterbitkan untuk janji temu.
			</p>
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
					<span class="sigap-walkin__failure-icon" aria-hidden="true">
						<Icon icon={AlertTriangle} size={18} />
					</span>
					<p class="sigap-walkin__failure-text">{resolveFailureMessage(failure)}</p>
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
	/*
		Layout comes from `.sigap-page`. The Pen gives the walk-in frame the
		standard 40px desktop offset and runs its sections across the full
		content column, so there is no panel cap here.
	*/

	/*
		The form panel frames fields; it is not a control. Decorative hairline,
		per the border split — the select and inputs inside keep their own
		`--sigap-border` boundaries.
	*/
	.sigap-walkin__section {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-walkin__alt {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sigap-space-2);
	}

	.sigap-walkin__alt-label {
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		An outlined link styled as a button, so it keeps `--sigap-border`: it is
		something the citizen operates.
	*/
	.sigap-walkin__alt-link {
		display: inline-flex;
		align-items: center;
		/* Citizen touch floor. */
		min-height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-3);
		font-size: var(--sigap-type-label);
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

	/*
		A tone-carrying alert: tone border, tone icon, tone text, tinted ground.
		Verbatim server copy inside, so the citizen is told what actually failed.
	*/
	.sigap-walkin__failure {
		display: flex;
		align-items: flex-start;
		gap: var(--sigap-space-3);
		margin-bottom: var(--sigap-space-4);
		padding: var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		color: var(--sigap-danger);
		background-color: var(--sigap-danger-tint);
		border: 1px solid var(--sigap-danger);
	}

	.sigap-walkin__failure-icon {
		display: flex;
		flex: none;
	}

	.sigap-walkin__failure-text {
		margin: 0;
	}

	/*
		The ticket card frames content; it is not a control, so it takes the
		decorative hairline. The number inside carries the emphasis.
	*/
	.sigap-walkin__ticket {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-walkin__ticket-head {
		display: flex;
		align-items: flex-start;
		gap: var(--sigap-space-3);
	}

	/* A badge: 1px tone border, tone glyph, tinted ground — never filled. */
	.sigap-walkin__ticket-icon {
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

	.sigap-walkin__ticket-sub {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		The number block. Decorative outline, because the block frames a value
		rather than being operable; the tinted ground is the approved
		queue-number ground.
	*/
	.sigap-walkin__number-block {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-4);
		text-align: center;
		background-color: var(--sigap-primary-tint);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-walkin__number-label {
		margin: 0 0 var(--sigap-space-1);
		font-size: var(--sigap-type-caption);
		font-weight: 500;
		color: var(--sigap-muted);
	}

	/*
		The queue number: `.sigap-identifier` supplies the monospace stack and
		tabular figures, `--sigap-primary-strong` the readable teal. Display
		size, because this is the one value the citizen reads aloud at the
		counter.
	*/
	.sigap-walkin__number {
		margin: 0 0 var(--sigap-space-3);
		font-size: 40px;
		font-weight: 700;
		color: var(--sigap-primary-strong);
		user-select: all;
		overflow-wrap: anywhere;
	}

	/* An inner note, not a control: decorative outline. */
	.sigap-walkin__wait {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-3);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-walkin__wait-value {
		margin: 0;
		font-size: var(--sigap-type-body);
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-walkin__wait-caveat {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-walkin__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sigap-space-3);
		margin-top: var(--sigap-space-4);
	}

	/*
		Where the citizen can actually follow their number. Sits below the copy
		hint, in the same muted weight: it is guidance, not an alert.
	*/
	.sigap-walkin__track-note {
		margin: var(--sigap-space-3) 0 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	.sigap-walkin__copy-hint {
		margin: var(--sigap-space-3) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		min-height: 16px;
	}
</style>
