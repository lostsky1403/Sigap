<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * The three-step booking indicator.
	 *
	 * Exactly three steps, and they are concepts rather than screens: Fasilitas,
	 * Layanan, Waktu + Data Diri. A four-step version would split patient
	 * details away from the time they are booking, which is the decision people
	 * make together, and a two-step version would hide the middle transition
	 * where the service list changes.
	 *
	 * This is a status indicator, not navigation. Steps are not links: a
	 * citizen must not be able to jump to step 3 with nothing chosen, because
	 * the form is validated as a whole and a stepper that implies free movement
	 * would promise something the page does not deliver. `aria-current="step"`
	 * carries the position for assistive tech; the visible step numbers do the
	 * same for everyone else.
	 */
	export let current: 1 | 2 | 3 = 1;
	export let total: number = 3;

	export const STEP_LABELS = ['Fasilitas', 'Layanan', 'Waktu + Data Diri'];

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
			<!--
				aria-current rather than a visually hidden "step N of 3" per item.
				The former is one attribute the screen reader announces in place;
				the latter would be repeated on every step and still require the
				list semantics to be interpreted.
			-->
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
					{:else if step < current}
						(sudah selesai)
					{/if}
				</span>
			</span>
			<span class="sigap-stepper__label">{label}</span>
		</li>
	{/each}
</ol>

<style>
	.sigap-stepper {
		display: flex;
		gap: 8px;
		margin: 0;
		padding: 12px;
		list-style: none;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		counter-reset: none;
	}

	.sigap-stepper__item {
		flex: 1 1 0;
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}

	.sigap-stepper__marker {
		flex: none;
		display: flex;
		align-items: center;
		justify-content: center;
		/* Below the 44px touch floor deliberately: this is not an interactive
		   target, it is a status dot, and inflating it would make the row
		   taller than the fields it is meant to sit above. */
		width: 24px;
		height: 24px;
		font-size: 12px;
		font-weight: 600;
		color: var(--sigap-muted);
		border: 1px solid var(--sigap-border);
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
		color: var(--sigap-primary-foreground, #ffffff);
	}

	.sigap-stepper__label {
		min-width: 0;
		font-size: 12px;
		color: var(--sigap-muted);
		/* Long step names wrap rather than truncating: "Waktu + Data Diri" cut
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
