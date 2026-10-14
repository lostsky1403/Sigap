<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { DENSITY, type Density, controlFontSize } from './density';

	/**
	 * A multi-line input.
	 *
	 * Height is intentionally not density-driven here: a textarea has no single
	 * "control height", and forcing 44px would clip a citizen writing a complaint.
	 * What the density does control is the font size, so text stays legible on
	 * both surfaces.
	 */
	export let value: string = '';
	export let id: string;
	export let describedBy: string = '';
	export let invalid: boolean = false;
	export let size: Density = DENSITY.citizen;
	export let rows: number = 4;
	export let name: string | undefined = undefined;
	export let placeholder: string | undefined = undefined;
	export let required: boolean = false;
	export let disabled: boolean = false;
	export let readonly: boolean = false;

	/**
	 * Character budget. When set, the field shows a live remaining count so a
	 * citizen is not surprised by a silent truncation on submit.
	 */
	export let maxLength: number | undefined = undefined;

	$: remaining = maxLength === undefined ? null : maxLength - value.length;
</script>

<textarea
	{id}
	bind:value
	{rows}
	{name}
	{placeholder}
	{required}
	{disabled}
	{readonly}
	maxLength={maxLength}
	aria-invalid={invalid ? 'true' : 'false'}
	aria-describedby={describedBy || undefined}
	class="sigap-textarea sigap-textarea--{size}"
	class:sigap-textarea--invalid={invalid}
	style:border-radius={RADIUS.control}
	on:input
	on:change
	on:blur
></textarea>

{#if remaining !== null}
	<p class="sigap-textarea__count" class:sigap-textarea__count--exhausted={remaining < 0}>
		{Math.max(remaining, 0)} karakter tersisa
	</p>
{/if}

<style>
	.sigap-textarea {
		width: 100%;
		padding: 12px;
		font: inherit;
		font-size: var(--sigap-textarea-font, 14px);
		line-height: 1.5;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		resize: vertical;
	}

	.sigap-textarea--citizen {
		--sigap-textarea-font: 16px;
	}

	.sigap-textarea:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 1px;
		border-color: var(--sigap-primary);
	}

	.sigap-textarea--invalid {
		border-color: var(--sigap-danger);
	}

	.sigap-textarea:disabled {
		background-color: var(--sigap-canvas);
		color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-textarea__count {
		margin: 0;
		font-size: 12px;
		text-align: right;
		color: var(--sigap-muted);
	}

	.sigap-textarea__count--exhausted {
		color: var(--sigap-danger);
	}
</style>
