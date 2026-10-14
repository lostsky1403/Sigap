<script lang="ts" context="module">
	/**
	 * A single option in a Select.
	 *
	 * Declared in module context rather than the instance script: a Svelte 4
	 * style instance script cannot export a type, and consumers need to be able
	 * to import this to type their option lists.
	 */
	export type Option = {
		value: string;
		label: string;
		disabled?: boolean;
	};
</script>

<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { DENSITY, type Density, controlHeight } from './density';

	/**
	 * A native <select>.
	 *
	 * Native is a deliberate choice, not a shortcut. A custom listbox has to
	 * reimplement typeahead, mobile picker behaviour, and screen reader
	 * announcement across browsers. Nothing in the frozen SIGAP design needs
	 * more than a native select can do.
	 */
	export let value: string = '';
	export let id: string;
	export let describedBy: string = '';
	export let invalid: boolean = false;
	export let options: Option[] = [];
	export let placeholder: string = 'Pilih…';
	export let size: Density = DENSITY.citizen;
	export let name: string | undefined = undefined;
	export let required: boolean = false;
	export let disabled: boolean = false;

	$: hasValue = value !== '' && value !== null && value !== undefined;
</script>

<div class="sigap-select" class:sigap-select--invalid={invalid}>
	<select
		{id}
		bind:value
		{name}
		{required}
		{disabled}
		aria-invalid={invalid ? 'true' : 'false'}
		aria-describedby={describedBy || undefined}
		class="sigap-select__control sigap-select__control--{size}"
		style:height="{controlHeight(size)}px"
		style:border-radius={RADIUS.control}
		on:change
		on:blur
	>
		<option value="" disabled>{placeholder}</option>
		{#each options as option (option.value)}
			<option value={option.value} disabled={option.disabled}>{option.label}</option>
		{/each}
	</select>
	<!--
		A chevron glyph drawn with a border, not a second icon package and not
		an emoji. The select stays the only interactive element; the span is
		pointer-events:none so clicks reach it.
	-->
	<span class="sigap-select__chevron" aria-hidden="true"></span>
</div>

<style>
	.sigap-select {
		position: relative;
		display: flex;
	}

	.sigap-select__control {
		width: 100%;
		padding: 0 12px;
		padding-right: 32px;
		font: inherit;
		font-size: 14px;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		/* Native arrow is suppressed because the chevron is drawn instead. */
		appearance: none;
	}

	.sigap-select__control--citizen {
		font-size: 16px;
		padding-left: 16px;
	}

	.sigap-select__control:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 1px;
		border-color: var(--sigap-primary);
	}

	.sigap-select--invalid .sigap-select__control {
		border-color: var(--sigap-danger);
	}

	.sigap-select__control:disabled {
		background-color: var(--sigap-canvas);
		color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-select__chevron {
		position: absolute;
		right: 12px;
		top: 50%;
		width: 8px;
		height: 8px;
		margin-top: -5px;
		pointer-events: none;
		border-right: 1px solid var(--sigap-muted);
		border-bottom: 1px solid var(--sigap-muted);
		transform: rotate(45deg);
	}
</style>
