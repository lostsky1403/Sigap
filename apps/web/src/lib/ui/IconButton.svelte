<script lang="ts">
	import type { ComponentType } from 'svelte';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from './Icon.svelte';
	import { DENSITY, type Density, controlHeight } from './density';

	/**
	 * An icon-only control.
	 *
	 * `label` is required, not optional. An icon-only button with no accessible
	 * name is announced by screen readers as just "button", which makes the
	 * control unusable. Making the prop required moves that mistake to compile
	 * time instead of leaving it for a user to discover.
	 */
	export let icon: ComponentType;
	export let label: string;
	export let variant: 'secondary' | 'ghost' = 'ghost';
	export let size: Density = DENSITY.citizen;
	export let type: 'button' | 'submit' | 'reset' = 'button';
	export let disabled: boolean = false;
	export let onClick: (event: MouseEvent) => void = () => {};

	function handleClick(event: MouseEvent) {
		onClick(event);
	}
</script>

<button
	{type}
	{disabled}
	aria-label={label}
	class="sigap-icon-button sigap-icon-button--{variant} sigap-icon-button--{size}"
	style:width="{controlHeight(size)}px"
	style:height="{controlHeight(size)}px"
	style:border-radius={RADIUS.control}
	on:click={handleClick}
>
	<Icon {icon} size={size === DENSITY.citizen ? 20 : 16} />
</button>

<style>
	.sigap-icon-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0;
		font: inherit;
		cursor: pointer;
		border: 1px solid transparent;
		transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease;
	}

	.sigap-icon-button:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-icon-button:disabled {
		cursor: not-allowed;
		opacity: 0.55;
	}

	.sigap-icon-button--ghost {
		background-color: transparent;
		color: var(--sigap-muted);
	}

	.sigap-icon-button--ghost:hover:not(:disabled) {
		background-color: var(--sigap-canvas);
		color: var(--sigap-foreground);
	}

	.sigap-icon-button--secondary {
		background-color: var(--sigap-surface);
		color: var(--sigap-foreground);
		border-color: var(--sigap-border);
	}

	.sigap-icon-button--secondary:hover:not(:disabled) {
		border-color: var(--sigap-primary);
		color: var(--sigap-primary);
	}
</style>
