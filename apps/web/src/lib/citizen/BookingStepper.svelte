<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * The three-step booking indicator.
	 *
	 * Exactly three steps, and they are concepts rather than screens: Fasilitas,
	 * Layanan, Waktu + data. A four-step version would split patient
	 * details away from the time they are booking, which is the decision people
	 * make together, and a two-step version would hide the middle transition
	 * where the service list changes.
	 *
	 * Movement is one-way. A COMPLETED step is a button that clears that step's
	 * choice and takes the citizen back to it — the tick marks already claimed
	 * those steps were revisitable, and without this there was no way back once
	 * step 3 appeared. A future step stays plain text: jumping ahead with
	 * nothing chosen would promise free movement the form does not deliver,
	 * because it is validated as a whole.
	 *
	 * The component only reports which step was chosen; what "clearing" means
	 * is the page's business, since the selections live there. `aria-current`
	 * carries the position for assistive tech; the visible step numbers do the
	 * same for everyone else.
	 */
	export let current: 1 | 2 | 3 = 1;
	export let total: number = 3;

	/**
	 * Fired when a completed step is chosen again. Never fires for the current
	 * or a future step, so a caller can trust the argument is behind `current`.
	 */
	export let onRevisit: (step: number) => void = () => {};

	export const STEP_LABELS = ['Fasilitas', 'Layanan', 'Waktu + data'];

	/**
	 * A step is reachable once the one before it is satisfied. Used for the
	 * muted treatment on later steps, which is a real state rather than
	 * decoration — it tells the citizen there is more to do.
	 */
	$: reachedCount = Math.max(current, 1);
</script>

<ol class="sigap-stepper" style:border-radius={RADIUS.panel}>
	{#each STEP_LABELS.slice(0, total) as label, index (label)}
		{@const step = index + 1}
		<li
			class="sigap-stepper__item"
			class:sigap-stepper__item--current={step === current}
			class:sigap-stepper__item--reached={step <= reachedCount}
		>
			{#if step < current}
				<!--
					A real button, not a span with a click handler: it is then in
					the tab order and Enter/Space work without reimplementing
					them. The accessible name says what the control does rather
					than repeating the label, because "Fasilitas, sudah selesai"
					read as a button sounds like a status, not an action.
				-->
				<button
					type="button"
					class="sigap-stepper__action"
					aria-label={`Ubah ${label.toLowerCase()}`}
					on:click={() => onRevisit(step)}
				>
					<span class="sigap-stepper__marker" style:border-radius="9999px">
						<span aria-hidden="true">{step}</span>
					</span>
					<span class="sigap-stepper__label">{label}</span>
				</button>
			{:else}
				<!--
					Current or not yet reached: plain text, so there is nothing to
					click and nothing to focus. aria-current rather than a
					visually hidden "step N of 3" per item — the former is one
					attribute the screen reader announces in place; the latter
					would be repeated on every step and still require the list
					semantics to be interpreted.
				-->
				<span class="sigap-stepper__action">
					<span
						class="sigap-stepper__marker"
						style:border-radius="9999px"
						aria-current={step === current ? 'step' : undefined}
					>
						<!--
							The marker number is decorative. The step's meaning comes from
							the label beside it, and announcing "1" again here would
							duplicate the ordinal a screen reader already derives from the
							ordered list.
						-->
						<span aria-hidden="true">{step}</span>
						<span class="sigap-visually-hidden">
							{label}
							{#if step === current}
								(langkah sekarang)
							{/if}
						</span>
					</span>
					<span class="sigap-stepper__label">{label}</span>
				</span>
			{/if}
		</li>
	{/each}
</ol>

<style>
	/*
		The stepper frames a status row; only completed steps are operable.
		Decorative hairline, per the border split: a step dot is not a control
		boundary, so it keeps the subtle border even where the row around it can
		be clicked.
	*/
	.sigap-stepper {
		display: flex;
		gap: var(--sigap-space-2);
		margin: 0;
		padding: var(--sigap-space-3);
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		counter-reset: none;
	}

	.sigap-stepper__item {
		flex: 1 1 0;
		display: flex;
		align-items: center;
		gap: var(--sigap-space-2);
		min-width: 0;
	}

	/*
		One wrapper for both states so the row's layout is identical whether a
		step is clickable or not. The button inherits the row's type and colour
		rather than restating them, which is what keeps a completed step looking
		like a status chip until it is hovered or focused.
	*/
	.sigap-stepper__action {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-2);
		flex: 1 1 auto;
		min-width: 0;
		padding: 0;
		margin: 0;
		font: inherit;
		color: inherit;
		text-align: left;
		background: none;
		border: 0;
	}

	button.sigap-stepper__action {
		cursor: pointer;
	}

	/*
		The whole step is the target, not the 24px dot. Hover and focus are the
		only signals that a completed step can be reopened, so they are given
		the accent colour the marker already uses.
	*/
	button.sigap-stepper__action:hover .sigap-stepper__label {
		color: var(--sigap-foreground);
		text-decoration: underline;
	}

	button.sigap-stepper__action:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-stepper__marker {
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		/* Below the 44px touch floor deliberately: this is a status dot, and
		   inflating it would make the row taller than the fields it is meant to
		   sit above. Where the step is clickable the whole row segment is the
		   target, so the dot is never asked to carry the tap on its own. */
		width: 24px;
		height: 24px;
		font-size: var(--sigap-type-caption);
		font-weight: 600;
		color: var(--sigap-muted);
		border: 1px solid var(--sigap-border-subtle);
	}

	/*
		Reached vs current are two different things and are drawn differently.

		A completed step keeps its accent rather than reverting to grey: the
		filled marker is how the citizen knows their earlier choice registered
		and was not thrown away by picking a different service unit. The current
		step is distinguished by the label's weight, because a filled circle
		alone cannot tell you which step you are on.
	*/
	.sigap-stepper__item--reached .sigap-stepper__marker {
		color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-stepper__item--current .sigap-stepper__marker {
		background-color: var(--sigap-primary);
		/* Text on a primary fill is `--sigap-on-primary`, the same pairing the
		   primary button uses. Reaching for `--sigap-surface` here would work
		   only because it happens to be white today, and would silently stop
		   being readable the moment the surface token moved. */
		color: var(--sigap-on-primary);
	}

	.sigap-stepper__label {
		min-width: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		/* Long step names wrap rather than truncating: "Waktu + data" cut
		   to "Waktu + Data..." tells the citizen less than wrapping it, and at
		   390px wrapping costs one line. */
		overflow-wrap: anywhere;
	}

	.sigap-stepper__item--current .sigap-stepper__label {
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
