<script lang="ts">
	import { onMount, tick } from 'svelte';
	import CheckinForm from '$lib/citizen/CheckinForm.svelte';
	import QueueTicket from '$lib/citizen/QueueTicket.svelte';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { AlertTriangle, TicketCheck } from 'lucide-svelte';
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
	 * The wrong-code failure is also a field problem, so it marks the code
	 * control as well as raising the alert.
	 *
	 * The alert above the form says what happened; the red boundary and the
	 * inline message say WHERE. Every other failure — 404, 409, 429, 5xx — is
	 * about the booking or the service rather than about the text in the
	 * field, so those keep the alert alone: red-bordering the input for a
	 * rate limit would point at the wrong control and invite the citizen to
	 * retype a code that was already correct.
	 *
	 * The message is a named constant rather than an inline literal so the
	 * ternary's two branches share a declared type. Inline, TypeScript widens
	 * the empty branch to `{ checkin_code?: undefined }` and the union stops
	 * being assignable to the `Record<string, string>` the form expects.
	 */
	const WRONG_CODE_ERROR: Record<string, string> = {
		checkin_code: 'Periksa kembali kode ini.'
	};

	let fieldErrors: Record<string, string> = {};
	$: fieldErrors = isWrongCode ? WRONG_CODE_ERROR : {};

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

		const outcome = await checkInAppointment(String(appointmentId ?? "").trim(), String(checkinCode ?? "").trim());

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

<div class="sigap-page sigap-checkin">
	<PageHeader
		title="Check-in janji temu"
		description="Masukkan ID janji temu dan kode check-in yang Anda terima. Kode bersifat case-insensitive."
	/>

	<!--
		Two mutually exclusive outcomes. An appointment check-in redeems a code
		against an existing booking; a walk-in registers someone who arrived
		without one. Offering the wrong one first would send a citizen with a
		booking down a path that cannot find it.
	-->
	<nav class="sigap-checkin__alt" aria-label="Alternatif check-in">
		<span class="sigap-checkin__alt-label">Belum punya janji temu?</span>
		<a
			class="sigap-checkin__alt-link"
			style:border-radius={RADIUS.control}
			href="/queues/new"
		>
			Ambil nomor antrean tanpa janji
		</a>
	</nav>

	{#if ticket}
		<div id="sigap-checkin-result" tabindex="-1">
			<QueueTicket {ticket} {checkinCode} onReset={reset} onCopy={copyValue} />
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
						<span class="sigap-checkin__failure-icon" aria-hidden="true">
							<Icon icon={AlertTriangle} size={18} />
						</span>
						<p class="sigap-checkin__failure-title">{resolveFailureMessage(failure)}</p>
					{/if}
				</div>
			{/if}

			<CheckinForm
				bind:appointmentId
				bind:checkinCode
				{submitting}
				errors={fieldErrors}
				onSubmit={submit}
			/>

			<p class="sigap-checkin__copy-hint" role="status" aria-live="polite">{copyHint}</p>
		</section>
	{/if}
</div>

<style>
	/*
		Layout comes from `.sigap-page`: the standard 40px desktop offset and
		48px gutter.

		The 560px cap is on the PANEL, not on the page, and that is the whole
		point of the correction. The Pen draws the check-in card from x 48 to
		x 607 — 560px wide, sitting against the left gutter — so capping the
		container instead would centre a narrow column on a 1440 screen and
		leave margins the approved frame does not have.
	*/
	.sigap-checkin__alt {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sigap-space-2);
	}

	.sigap-checkin__alt-label {
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		An outlined link styled as a button, so it keeps `--sigap-border`: it is
		something the citizen operates, and the decorative hairline would leave
		it with no visible boundary at all.
	*/
	.sigap-checkin__alt-link {
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

	.sigap-checkin__alt-link:hover {
		border-color: var(--sigap-primary);
	}

	.sigap-checkin__alt-link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	/*
		The result panel frames content; it is not a control. Decorative
		hairline, per the border split — the inputs inside keep their own
		`--sigap-border` boundaries.
	*/
	.sigap-checkin__panel {
		max-width: 560px;
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	/*
		A tone-carrying alert: tone border, tone icon, tone text, tinted ground.
		The tint is what separates it from the panel behind it — a red hairline
		alone on white reads as a stray rule rather than as a failure.
	*/
	.sigap-checkin__failure {
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
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-checkin__copy-hint {
		margin: var(--sigap-space-3) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		min-height: 16px;
	}
</style>
