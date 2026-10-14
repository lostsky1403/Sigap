<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * A placeholder block for content that has not arrived.
	 *
	 * Marked `aria-hidden` unconditionally and without exception. A skeleton is
	 * pure visual scaffolding: exposing grey rectangles to a screen reader adds
	 * a column of meaningless content and actively slows comprehension. The
	 * surrounding region should carry the loading semantics instead — that is
	 * what LoadingState's aria-busy is for.
	 *
	 * The shimmer is a flat opacity pulse, not a gradient sweep. A moving
	 * gradient is decoration the frozen design does not call for, and an
	 * endlessly animated page is a problem for readers with vestibular issues.
	 */
	export let width: string = '100%';
	export let height: string = '16px';
	export let radius: 'control' | 'panel' = 'control';
	export let lines: number = 1;
</script>

<div class="sigap-skeleton-group" aria-hidden="true" style:gap={lines > 1 ? '8px' : '0'}>
	{#each Array.from({ length: Math.max(lines, 1) }) as _, index (index)}
		<div
			class="sigap-skeleton"
			style:width={index === lines - 1 && lines > 1 ? '60%' : width}
			style:height={height}
			style:border-radius={RADIUS[radius]}
		></div>
	{/each}
</div>

<style>
	.sigap-skeleton-group {
		display: flex;
		flex-direction: column;
		width: 100%;
	}

	.sigap-skeleton {
		background-color: var(--sigap-border);
		/* Flat pulse. No gradient sweep, per the frozen visual rules. */
		animation: sigap-pulse 1.4s ease-in-out infinite;
	}

	@keyframes sigap-pulse {
		0%,
		100% {
			opacity: 1;
		}
		50% {
			opacity: 0.5;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.sigap-skeleton {
			animation: none;
			opacity: 0.6;
		}
	}
</style>
