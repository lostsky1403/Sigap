<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * A compact status label.
	 *
	 * The rule this enforces is the important one: a badge is never colour-only.
	 * `label` is required, so there is no way to render a coloured pill with no
	 * text, which would be invisible to a screen reader and ambiguous to anyone
	 * who cannot distinguish the semantic hues.
	 *
	 * `tone` only ever changes the border and text colour. The badge is never
	 * filled with a saturated background, because a table full of filled pills
	 * reads as noise and fights the calm canvas the design is built on.
	 */
	export let label: string;
	export let tone: 'neutral' | 'success' | 'warning' | 'danger' | 'info' | 'brand' =
		'neutral';
	export let size: 'sm' | 'md' = 'md';
</script>

<span
	class="sigap-badge sigap-badge--{tone} sigap-badge--{size}"
	style:border-radius={RADIUS.control}
>
	{label}
</span>

<style>
	.sigap-badge {
		display: inline-flex;
		align-items: center;
		/* 4px horizontal padding keeps the pill tight without clipping text. */
		padding: 2px 8px;
		font-size: 12px;
		font-weight: 500;
		line-height: 1.5;
		white-space: nowrap;
		background-color: transparent;
		border: 1px solid currentColor;
	}

	.sigap-badge--md {
		padding: 4px 8px;
		font-size: 13px;
	}

	.sigap-badge--neutral {
		color: var(--sigap-muted);
	}

	.sigap-badge--success {
		color: var(--sigap-success);
	}

	.sigap-badge--warning {
		color: var(--sigap-warning);
	}

	.sigap-badge--danger {
		color: var(--sigap-danger);
	}

	.sigap-badge--info {
		color: var(--sigap-info);
	}

	.sigap-badge--brand {
		color: var(--sigap-primary);
	}
</style>
