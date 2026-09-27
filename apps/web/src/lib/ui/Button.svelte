<script lang="ts" context="module">
	/** Monotonic counter backing per-instance IDs for aria-describedby targets. */
	let buttonCount = 0;
</script>

<script lang="ts">
	import type { ComponentType } from 'svelte';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from './Icon.svelte';
	import { DENSITY, type Density, controlHeight } from './density';

	/**
	 * Unique per instance, so two disabled buttons on one screen do not share
	 * an aria-describedby target. Without this, assistive tech would announce
	 * the wrong reason for one of them.
	 */
	let instanceId = `sigap-btn-${++buttonCount}`;

	/**
	 * The one button in SIGAP.
	 *
	 * Two decisions worth stating explicitly:
	 *
	 * 1. It renders a real <button>. A clickable <div> loses keyboard focus,
	 *    loses the disabled semantics, and loses the screen reader affordance
	 *    for an action. There is no "styled link" mode here on purpose: a
	 *    navigation control is an <a href>, and that belongs in the route, not
	 *    in a button that fakes it.
	 *
	 * 2. Variants map to semantic tokens only. There is no gradient, no
	 *    decorative shadow, and no palette drift: primary is the brand, and
	 *    the rest are surface/muted/line combinations.
	 */
	export let variant: 'primary' | 'secondary' | 'ghost' | 'danger' = 'primary';
	export let size: Density = DENSITY.citizen;
	export let type: 'button' | 'submit' | 'reset' = 'button';
	export let disabled: boolean = false;
	export let fullWidth: boolean = false;

	/** Optional leading icon. Decorative: the label already carries meaning. */
	export let icon: ComponentType | undefined = undefined;

	/**
	 * Accessible name for icon-only usage. When there is no visible text this
	 * becomes the button's accessible name, so an icon-only control is never
	 * announced as just "button".
	 */
	export let label: string = '';

	export let ariaLabel: string | undefined = undefined;
	export let disabledReason: string | undefined = undefined;

	/**
	 * Click callback, matching the prop-based convention used elsewhere in this
	 * codebase. The native event is forwarded as well, so a consumer that
	 * prefers `on:click` keeps working.
	 */
	export let onClick: (event: MouseEvent) => void = () => {};

	function handleClick(event: MouseEvent) {
		onClick(event);
	}

	$: height = controlHeight(size);
	// "No visible text" means no slot AND no label prop. Testing only
	// $$slots.default, as this originally did, made a `label`-only button render
	// as an empty control: EmptyState and ErrorState both pass `label` without a
	// slot, so their retry and reset buttons were invisible and unnamed. An
	// unlabelled button is an accessibility failure, not a cosmetic one — a
	// screen reader announces "button" and a sighted user sees a blank pill.
	$: hasVisibleText = Boolean($$slots.default) || label.length > 0;
	$: isIconOnly = !hasVisibleText && icon !== undefined;
</script>

<button
	{type}
	{disabled}
	class="sigap-button sigap-button--{variant} sigap-button--{size}"
	style:height="{height}px"
	style:border-radius={RADIUS.control}
	style:width={fullWidth ? '100%' : undefined}
	aria-label={ariaLabel ?? (isIconOnly ? label : undefined)}
	aria-describedby={disabled && disabledReason ? `${instanceId}-reason` : undefined}
	aria-disabled={disabled ? 'true' : undefined}
	on:click={handleClick}
>
	{#if icon}
		<Icon {icon} size={size === DENSITY.citizen ? 20 : 16} />
	{/if}
	{#if $$slots.default}
		<span class="sigap-button__label"><slot /></span>
	{:else if label}
		<!--
			The label prop is a real, visible text path, not only an accessible
			name. Consumers that pass `label` instead of a slot must still see the
			words they asked for.
		-->
		<span class="sigap-button__label">{label}</span>
	{/if}
</button>

{#if disabled && disabledReason}
	<span id="{instanceId}-reason" class="sigap-visually-hidden">{disabledReason}</span>
{/if}

<style>
	.sigap-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		padding: 0 16px;
		font: inherit;
		font-size: 14px;
		font-weight: 500;
		line-height: 1;
		cursor: pointer;
		transition: background-color 120ms ease, border-color 120ms ease, color 120ms ease;
		/* Focus must be visible on every variant, including on the brand fill. */
		border: 1px solid transparent;
	}

	.sigap-button:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-button:disabled {
		cursor: not-allowed;
		opacity: 0.55;
	}

	/* Citizen density gets the larger touch type. */
	.sigap-button--citizen {
		font-size: 16px;
		padding: 0 16px;
	}

	.sigap-button--admin-compact {
		padding: 0 12px;
	}

	.sigap-button--admin-comfortable {
		padding: 0 16px;
	}

	.sigap-button--primary {
		background-color: var(--sigap-primary);
		color: var(--sigap-surface);
	}

	.sigap-button--primary:hover:not(:disabled) {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-button--primary:active:not(:disabled) {
		background-color: var(--sigap-primary-active);
	}

	.sigap-button--secondary {
		background-color: var(--sigap-surface);
		color: var(--sigap-foreground);
		border-color: var(--sigap-border);
	}

	.sigap-button--secondary:hover:not(:disabled) {
		border-color: var(--sigap-primary);
		color: var(--sigap-primary);
	}

	.sigap-button--ghost {
		background-color: transparent;
		color: var(--sigap-primary);
	}

	.sigap-button--ghost:hover:not(:disabled) {
		background-color: var(--sigap-canvas);
	}

	.sigap-button--danger {
		background-color: var(--sigap-danger);
		color: var(--sigap-surface);
	}

	.sigap-button--danger:hover:not(:disabled) {
		background-color: var(--sigap-primary-active);
	}

	.sigap-button__label {
		display: inline-block;
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
