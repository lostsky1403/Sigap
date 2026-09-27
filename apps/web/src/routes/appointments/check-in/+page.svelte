<script lang="ts">
	import { onMount, tick } from 'svelte';
	import CheckinForm from '$lib/citizen/CheckinForm.svelte';
	import QueueTicket from '$lib/citizen/QueueTicket.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { TicketCheck } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import { checkInAppointment, wrongCheckInCode } from '$lib/api/endpoints/public';
	import { isAbort } from '$lib/api/errors';
	import type { CheckInResult } from '$lib/api/types/api';
	import {
		classifyCheckInFailure,
		resolveFailureMessage,
		type FlowFailure
	} from '$lib/citizen/citizenFlow';

	/**
	 * /appointments/check-in — public check-in for an existing appointment.
	 *
	 * NOT the walk-in flow. Those are two different things that happen to
	 * produce a queue number: this one redeems a code against a booking that
	 * already exists, while /queues/new registers someone who arrived without
	 * one. They are kept apart deliberately, because conflating them would let a
	 * walk-in registration mutate an appointment's state, and would make the
	 * 401 below ambiguous.
	 *
	 * The single most important behaviour on this page: a 401 from this
	 * endpoint is NOT an authentication failure. There is no session to expire
	 * on a public route. The backend answers 401 "Kode check-in tidak cocok."
	 * when the code is simply wrong, and rendering a sign-in prompt for that
	 * would tell a citizen to log in to fix a typo — advice that cannot help
	 * them and sends them away from the page that can.
	 */

	let appointmentId = '';
	let checkinCode = '';
	let submitting = false;
	let ticket: CheckInResult | null = null;
	/**
	 * The whole classified failure, not just its kind.
	 *
	 * Storing only the kind meant the server's own message had to be
	 * reconstructed at the render site, which is how a 409's specific wording
	 * ("...dengan status: queued") gets replaced by something vaguer. The
	 * classifier already carries the message; keeping the object intact means
	 * the page never has to invent one.
	 */
	let failure: FlowFailure | null = null;
	let copyHint = '';

	/**
	 * True only for a wrong code on an existing appointment.
	 *
	 * Drives a dedicated, specific message rather than the generic banner,
	 * because "retype your code" is a different instruction from "try again
	 * later" and a citizen who typed a 5 instead of an S needs to be told
	 * which one happened.
	 */
	$: isWrongCode = failure?.kind === 'wrong-code';

	/**
	 * Reads the deep link.
	 *
	 * Canonical names are `appointment_id` and `checkin_code`. The legacy
	 * aliases `id` and `code` are still honoured, because links generated
	 * before this phase are already in people's messages and history and
	 * breaking them would strand a citizen who has an appointment and a code.
	 * The canonical name wins when both are present, so a link can be
	 * upgraded by simply renaming the parameter.
	 */
	onMount(() => {
		const params = new URLSearchParams(window.location.search);
		appointmentId = params.get('appointment_id') ?? params.get('id') ?? '';
		checkinCode = params.get('checkin_code') ?? params.get('code') ?? '';
	});

	async function submit() {
		if (submitting) return;
		submitting = true;
		failure = null;
		copyHint = '';

		const outcome = await checkInAppointment(appointmentId.trim(), checkinCode.trim());

		// An abort is a navigation, not a failure.
		submitting = false;
		if (outcome.ok) {
			ticket = outcome.data;
			await tick();
			document.getElementById('sigap-checkin-result')?.focus();
			return;
		}
		if (isAbort(outcome.error)) return;

		/*
			The 401 branch, and the reason `wrongCheckInCode` is consulted
			explicitly rather than inferred from `kind === 'unauthorized'`. The
			helper is the single place that knows this route's 401 means a
			mistyped code. Passing it in keeps that decision visible at the call
			site instead of hiding it inside the classifier.
		*/
		failure = classifyCheckInFailure(outcome.error, wrongCheckInCode(outcome.error));
	}

	async function copyValue(label: string, value: string) {
		if (!value) return;
		try {
			await navigator.clipboard.writeText(value);
			copyHint = `${label} tersalin.`;
		} catch {
			copyHint = 'Tidak dapat menyalin otomatis. Salin manual dari kotak nomor.';
		}
		setTimeout(() => {
			copyHint = '';
		}, 4000);
	}

	function reset() {
		ticket = null;
		failure = null;
		copyHint = '';
		appointmentId = '';
		checkinCode = '';
	}
</script>

<svelte:head>
	<title>Check-In — Sigap</title>
</svelte:head>

<div class="sigap-checkin">
	<section class="sigap-checkin__intro" aria-labelledby="sigap-checkin-title">
		<h1 id="sigap-checkin-title" class="sigap-page-title">Check-In</h1>
		<p class="sigap-page-subtitle">
			Masukkan kode check-in dari janji temu Anda untuk mendapat nomor antrean.
		</p>
	</section>

	<!--
		Two mutually exclusive outcomes. An appointment check-in redeems a code
		against an existing booking; a walk-in registers someone who arrived
		without one. Offering the wrong one first would send a citizen with a
		booking down a path that cannot find it.
	-->
	<nav class="sigap-checkin__alt" aria-label="Alternatif check-in">
		<span class="sigap-checkin__alt-label">Datang tanpa janji temu?</span>
		<a
			class="sigap-checkin__alt-link"
			style:border-radius={RADIUS.control}
			href="/queues/new"
		>
			Ambil antrean walk-in
		</a>
	</nav>

	{#if ticket}
		<div id="sigap-checkin-result" tabindex="-1">
			<QueueTicket {ticket} onReset={reset} onCopy={copyValue} />
		</div>
	{:else}
		<section class="sigap-checkin__panel" style:border-radius={RADIUS.panel}>
			{#if failure}
				<!--
					The wrong-code case gets its own message. `role="alert"` because
					it appears in response to a submit the citizen just made, and
					without an announcement a screen reader user gets no
					confirmation that anything happened.
				-->
				<div class="sigap-checkin__failure" role="alert" data-testid="checkin-error">
					{#if isWrongCode}
						<span class="sigap-checkin__failure-icon" aria-hidden="true">
							<Icon icon={TicketCheck} size={18} />
						</span>
						<div>
							<p class="sigap-checkin__failure-title">Kode check-in tidak cocok.</p>
							<p class="sigap-checkin__failure-body">
								Periksa kembali kode yang Anda terima saat membuat janji temu. Kode
								bersifat case-insensitive.
							</p>
						</div>
					{:else}
						<p>{resolveFailureMessage(failure)}</p>
					{/if}
				</div>
			{/if}

			<CheckinForm bind:appointmentId bind:checkinCode {submitting} onSubmit={submit} />

			<p class="sigap-checkin__copy-hint" role="status" aria-live="polite">{copyHint}</p>
		</section>
	{/if}
</div>

<style>
	.sigap-checkin {
		max-width: 640px;
		margin: 0 auto;
		padding: 20px 16px 8px;
	}

	@media (min-width: 768px) {
		.sigap-checkin {
			padding: 32px 24px 8px;
		}
	}

	.sigap-checkin__alt {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px;
		margin-top: 16px;
	}

	.sigap-checkin__alt-label {
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-checkin__alt-link {
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

	.sigap-checkin__alt-link:hover {
		border-color: var(--sigap-primary);
	}

	.sigap-checkin__alt-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-checkin__panel {
		margin-top: 16px;
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-checkin__failure {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		margin-bottom: 16px;
		padding: 12px;
		font-size: 13px;
		color: var(--sigap-danger);
		border: 1px solid var(--sigap-danger);
	}

	.sigap-checkin__failure-icon {
		display: flex;
		flex: none;
		margin-top: 1px;
	}

	.sigap-checkin__failure-title {
		margin: 0;
		font-weight: 500;
	}

	.sigap-checkin__failure-body {
		margin: 2px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-checkin__copy-hint {
		margin: 12px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
		min-height: 16px;
	}
</style>
