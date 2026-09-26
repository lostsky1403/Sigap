<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { DENSITY, type Density, controlHeight } from './density';

	/**
	 * A text input sized by density, never by raw pixels.
	 *
	 * `aria-invalid` and `aria-describedby` are applied even when empty, because
	 * Svelte omits empty attributes inconsistently and assistive tech reads the
	 * presence of the attribute rather than the value.
	 */
	export let value: string = '';
	export let id: string;
	export let describedBy: string = '';
	export let invalid: boolean = false;
	export let type: 'text' | 'email' | 'password' | 'tel' | 'number' | 'search' = 'text';
	export let size: Density = DENSITY.citizen;
	export let name: string | undefined = undefined;
	export let placeholder: string | undefined = undefined;
	// Narrow unions rather than bare `string`: these are enumerated attributes,
	// and letting arbitrary text through would ship a silently ignored value.
	export let autocomplete:
		| 'name'
		| 'email'
		| 'username'
		| 'current-password'
		| 'new-password'
		| 'tel'
		| 'off'
		| undefined = undefined;
	export let required: boolean = false;
	export let disabled: boolean = false;
	export let readonly: boolean = false;
	export let inputmode:
		| 'none'
		| 'search'
		| 'text'
		| 'email'
		| 'tel'
		| 'url'
		| 'numeric'
		| 'decimal'
		| undefined = undefined;

	/**
	 * Enter-to-submit, used by chatty admin search boxes. Left off by default so
	 * it does not surprise a citizen filling a multi-field form.
	 */
	export let submitOnEnter: boolean = false;

	function handleKeydown(event: KeyboardEvent) {
		if (submitOnEnter && event.key === 'Enter') {
			// currentTarget is typed as EventTarget; the cast is what it is at
			// runtime, because the listener is bound to the input itself.
			const input = event.currentTarget as HTMLElement | null;
			input?.closest('form')?.requestSubmit();
		}
	}
</script>

<input
	{id}
	bind:value
	{type}
	{name}
	{placeholder}
	{autocomplete}
	{required}
	{disabled}
	{readonly}
	{inputmode}
	aria-invalid={invalid ? 'true' : 'false'}
	aria-describedby={describedBy || undefined}
	class="sigap-input sigap-input--{size}"
	class:sigap-input--invalid={invalid}
	style:height="{controlHeight(size)}px"
	style:border-radius={RADIUS.control}
	on:keydown={handleKeydown}
	on:input
	on:change
	on:blur
/>

<style>
	.sigap-input {
		width: 100%;
		padding: 0 12px;
		font: inherit;
		font-size: 14px;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		transition: border-color 120ms ease;
	}

	.sigap-input--citizen {
		font-size: 16px;
		padding: 0 16px;
	}

	.sigap-input:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 1px;
		border-color: var(--sigap-primary);
	}

	.sigap-input--invalid {
		border-color: var(--sigap-danger);
	}

	.sigap-input:disabled {
		background-color: var(--sigap-canvas);
		color: var(--sigap-muted);
		cursor: not-allowed;
	}

	.sigap-input::placeholder {
		color: var(--sigap-muted);
	}
</style>
