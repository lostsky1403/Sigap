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

	$: progress = visitProgressFrom(checkinStatus);
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

	{#if queueNumber}
		<p class="sigap-visit__queue">
			Nomor antrean: <span class="sigap-visit__queue-number">{queueNumber}</span>
		</p>
	{/if}
</div>

<style>
	.sigap-visit {
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-visit__label {
		margin: 0 0 12px;
		font-size: 13px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-visit__steps {
		display: flex;
		flex-direction: column;
		gap: 12px;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.sigap-visit__step {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.sigap-visit__marker {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		flex: none;
		font-size: 12px;
		font-weight: 600;
		color: var(--sigap-muted);
		border: 1px solid var(--sigap-border);
	}

	.sigap-visit__step--done .sigap-visit__marker {
		color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-visit__step:has(.sigap-visit__marker[aria-current='step']) .sigap-visit__marker {
		background-color: var(--sigap-primary);
		color: var(--sigap-surface);
	}

	.sigap-visit__step-label {
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-visit__step-label--current {
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-visit__unknown,
	.sigap-visit__outcome {
		margin: 0;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-visit__queue {
		margin: 12px 0 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-visit__queue-number {
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-weight: 600;
		color: var(--sigap-foreground);
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
