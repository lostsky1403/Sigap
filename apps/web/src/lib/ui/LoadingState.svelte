<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * A busy region.
	 *
	 * `aria-busy` is the point of this component. A spinner with no busy state
	 * tells a screen reader user nothing: the region simply looks empty, and
	 * they cannot tell the difference between "still loading" and "no results".
	 * Marking the region busy announces that content is on its way.
	 *
	 * The label is required so the announcement is meaningful rather than a bare
	 * "busy". It is visually hidden by default because a visible "Memuat…"
	 * spinner on every panel is noise, but a citizen on a slow connection still
	 * benefits from it, hence the `showLabel` opt-in.
	 */
	export let label: string = 'Memuat data';
	export let showLabel: boolean = false;
	export let busy: boolean = true;
</script>

<div
	class="sigap-loading"
	style:border-radius={RADIUS.panel}
	role="status"
	aria-busy={busy ? 'true' : 'false'}
	aria-live="polite"
>
	<!--
		A CSS spinner rather than an SVG or an icon package: it is pure
		decoration, is hidden from assistive tech, and needs no glyph.
	-->
	<span class="sigap-spinner" aria-hidden="true"></span>
	<span class:sigap-visually-hidden={!showLabel} class="sigap-loading__label">{label}</span>
</div>

<style>
	.sigap-loading {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 8px;
		padding: 24px;
		color: var(--sigap-muted);
	}

	.sigap-spinner {
		width: 16px;
		height: 16px;
		border: 2px solid var(--sigap-border);
		border-top-color: var(--sigap-primary);
		border-radius: 50%;
		animation: sigap-spin 700ms linear infinite;
	}

	@keyframes sigap-spin {
		to {
			transform: rotate(360deg);
		}
	}

	/* Respect a stated preference for less motion. */
	@media (prefers-reduced-motion: reduce) {
		.sigap-spinner {
			animation-duration: 2400ms;
		}
	}

	.sigap-loading__label {
		font-size: 14px;
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
