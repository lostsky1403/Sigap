<script lang="ts">
	/**
	 * The single icon entry point for SIGAP.
	 *
	 * Every glyph in the product comes from Lucide through this wrapper. That
	 * constraint is what keeps the icon language coherent: one family, one
	 * stroke width, one sizing scale. Adding a second icon package or a
	 * hand-written SVG is a design-system break, not a convenience.
	 *
	 * Stroke width is deliberately NOT configurable per call site. Lucide
	 * defaults to 2, which is heavier than the frozen SIGAP design calls for;
	 * ICON_STROKE_WIDTH pins it at 1.75 everywhere. A caller that needs a
	 * different weight is asking for a second visual language.
	 */
	import { ICON_STROKE_WIDTH } from '$lib/design/tokens';
	import type { ComponentType } from 'svelte';

	/**
	 * The Lucide component to render, e.g. the `Check` export from lucide-svelte.
	 *
	 * Typed as `ComponentType` rather than `Component` because lucide-svelte
	 * 1.0.1 ships Svelte 4 class components. `ComponentType` accepts those and
	 * Svelte 5 function components alike, so the wrapper does not need to
	 * change if the icon package is later rebuilt for runes.
	 */
	export let icon: ComponentType;

	/**
	 * Icon size in pixels. Sized on an 8px-aligned scale so icons sit correctly
	 * beside both citizen (44px) and admin (36/40px) controls.
	 */
	export let size: number = 24;

	/**
	 * Accessible treatment.
	 *
	 * `decorative` is the default and the right answer almost everywhere: the
	 * icon sits next to a text label, so exposing it again just adds noise for
	 * screen reader users.
	 *
	 * Set `decorative={false}` only when the icon is the sole carrier of
	 * meaning, and then `label` is required and becomes the accessible name.
	 * That combination belongs in IconButton, which forces the caller to
	 * provide the label rather than letting it be forgotten.
	 */
	export let decorative: boolean = true;
	export let label: string = '';

	/**
	 * CSS color for the glyph. Defaults to currentColor so the icon inherits
	 * the surrounding text color, which is what keeps it from drifting into a
	 * second palette.
	 */
	export let color: string = 'currentColor';
</script>

{#if decorative}
	<svelte:component
		this={icon}
		{size}
		{color}
		strokeWidth={ICON_STROKE_WIDTH}
		aria-hidden="true"
		role="presentation"
	/>
{:else if label}
	<svelte:component
		this={icon}
		{size}
		{color}
		strokeWidth={ICON_STROKE_WIDTH}
		role="img"
		aria-label={label}
	/>
{/if}
