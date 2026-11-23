<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { Check } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import {
		VISIT_PROGRESS_LABELS,
		VISIT_PROGRESS_ORDER,
		visitOutcomeLabel,
		visitProgressFrom,
		type VisitProgressStep
	} from '$lib/citizen/citizenFlow';

	/**
	 * Where a visit is RIGHT NOW.
	 *
	 * Four steps, one of them current. This is a current-state indicator and
	 * nothing more, which is a deliberate limit rather than a missing feature.
	 * The backend has exactly one status value to give — `checkin_status`, plus
	 * the raw `appointment_status` — and no history. A component that drew four
	 * timestamps, a per-step duration, or a "live" counter would have to invent
	 * every one of them, and a citizen comparing the screen to the clock at the
	 * counter would find the app confidently wrong.
	 *
	 * So: no times, no durations, no progression, no claim that it updates on
	 * its own. The citizen is told where they are and offered a way to look
	 * again.
	 *
	 * The completed steps are marked with a check as well as by colour, so the
	 * state does not depend on colour alone.
	 */
	export let checkinStatus: string;
	export let queueNumber: string = '';
	/**
	 * The ticket's own state (`waiting`, `called`, `in_service`, ...).
	 *
	 * Required to tell "holding a number" from "being served": the appointment
	 * status alone cannot, because it is `queued` for both.
	 */
	export let queueStatus: string | undefined = undefined;

	$: progress = visitProgressFrom(checkinStatus, queueStatus);
	/*
	 * Narrowed once, in the script, rather than with a `: this is ...` guard in
	 * the template. The template then reads a plain optional value, and the
	 * compiler has no union left to complain about at three separate use sites.
	 *
	 * The `isStep` flag is a separate statement rather than an inline check
	 * because Svelte does not narrow a union across a reactive boundary: a
	 * `progress.kind === 'step' ? progress.step : null` written in one
	 * expression still fails, since the compiler re-reads `progress` without
	 * carrying the narrowing.
	 */
	$: isStep = progress.kind === 'step';
	$: currentStep = isStep && progress.kind === 'step' ? progress.step : null;
	$: currentIndex = currentStep ? VISIT_PROGRESS_ORDER.indexOf(currentStep) : -1;
	$: outcomeLabel = visitOutcomeLabel(progress);
</script>

<div class="sigap-visit" style:border-radius={RADIUS.panel} data-testid="visit-progress">
	<p class="sigap-visit__label" id="sigap-visit-label">Status kunjungan</p>

	{#if queueNumber}
		<!--
			The pass's signature element, and the one thing a citizen reads from
			across a waiting room: the queue number at 48px in the monospace
			identifier stack, on the primary tint, with an 11px uppercase eyebrow
			above it. The Pen draws exactly this block on all three status frames
			(48px at 1440, 40px at 390, 34px at 360), and it is the reason the
			page exists — a 13px inline "Nomor antrean: …" line buries the single
			fact the citizen came for.

			The size steps down with the viewport so the number never wraps or
			overflows a 360px phone; a queue number that breaks across two lines
			is unreadable at the counter.
		-->
		<div class="sigap-visit__pass" style:border-radius={RADIUS.control}>
			<p class="sigap-visit__pass-eyebrow">Nomor antrean</p>
			<p class="sigap-visit__pass-number sigap-identifier">{queueNumber}</p>
		</div>
	{/if}

	{#if progress.kind === 'unknown'}
		<!--
			A status the backend did not map. Shown as itself rather than
			relabelled: a guessed step would be a claim about someone's health
			visit that nobody verified.
		-->
		<p class="sigap-visit__unknown" data-testid="visit-unknown">
			Status saat ini: {progress.raw}
		</p>
	{:else if !isStep}
		<!--
			Cancelled or missed. Not progress, so not drawn as a step: an
			indicator frozen at step 1 would tell a citizen whose appointment was
			cancelled that they simply have not arrived yet.
		-->
		<p class="sigap-visit__outcome" data-testid="visit-outcome">{outcomeLabel}</p>
	{:else}
		<ol
			class="sigap-visit__steps"
			aria-labelledby="sigap-visit-label"
			data-testid="visit-steps"
		>
			{#each VISIT_PROGRESS_ORDER as step, index (step)}
				{@const state =
					index < currentIndex ? 'done' : index === currentIndex ? 'current' : 'upcoming'}
				<li class="sigap-visit__step" class:sigap-visit__step--done={state === 'done'}>
					<span
						class="sigap-visit__marker"
						style:border-radius="9999px"
						aria-current={state === 'current' ? 'step' : undefined}
					>
						{#if state === 'done'}
							<!-- The check is the non-colour signal that a step passed. -->
							<Icon icon={Check} size={14} />
							<span class="sigap-visually-hidden">(sudah)</span>
						{:else}
							<span aria-hidden="true">{index + 1}</span>
							<span class="sigap-visually-hidden">
								{VISIT_PROGRESS_LABELS[step]}
								{#if state === 'current'}(sekarang){/if}
							</span>
						{/if}
					</span>
					<span
						class="sigap-visit__step-label"
						class:sigap-visit__step-label--current={state === 'current'}
					>
						{VISIT_PROGRESS_LABELS[step as VisitProgressStep]}
					</span>
				</li>
			{/each}
		</ol>
	{/if}

</div>

<style>
	/*
		The visit panel frames a status list; nothing here is operable, so the
		panel outline and the step dots are decorative hairlines.
	*/
	.sigap-visit {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-visit__label {
		margin: 0 0 var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-visit__steps {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-3);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sigap-visit__step {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-2);
	}

	.sigap-visit__marker {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		flex: none;
		font-size: var(--sigap-type-caption);
		font-weight: 600;
		color: var(--sigap-muted);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-visit__step--done .sigap-visit__marker {
		color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-visit__step:has(.sigap-visit__marker[aria-current='step']) .sigap-visit__marker {
		background-color: var(--sigap-primary);
		color: var(--sigap-on-primary);
	}

	.sigap-visit__step-label {
		font-size: var(--sigap-type-body);
		color: var(--sigap-muted);
	}

	.sigap-visit__step-label--current {
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-visit__unknown,
	.sigap-visit__outcome {
		margin: 0;
		font-size: var(--sigap-type-body);
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	/*
		The queue-number pass. The Pen draws it on `$primary-tint` with the
		number in `$primary-strong`, which is why it is the one tinted surface on
		an otherwise white panel: it marks the block as the answer the citizen
		came for rather than as another metadata row.
	*/
	.sigap-visit__pass {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-4);
		background-color: var(--sigap-primary-tint);
	}

	.sigap-visit__pass-eyebrow {
		margin: 0;
		/* 11px is the floor, and the Pen draws this eyebrow at exactly 11px. */
		font-size: var(--sigap-type-micro);
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--sigap-primary-strong);
	}

	.sigap-visit__pass-number {
		margin: var(--sigap-space-1) 0 0;
		/* 34px at 360, 40px at 390, 48px at 1440 — the Pen's three values. */
		font-size: 34px;
		font-weight: 700;
		line-height: 1.1;
		color: var(--sigap-primary-strong);
	}

	@media (min-width: 390px) {
		.sigap-visit__pass-number {
			font-size: 40px;
		}
	}

	@media (min-width: 1024px) {
		.sigap-visit__pass-number {
			font-size: 48px;
		}
	}

	.sigap-visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
