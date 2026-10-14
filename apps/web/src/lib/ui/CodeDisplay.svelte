<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * A short human-entered code, such as a queue check-in code.
	 *
	 * Two details are deliberate:
	 *
	 * - The code is selectable text, not an image. A citizen reading a code off
	 *   another screen has to be able to select and retype it.
	 * - Letter spacing is fixed and numerals are tabular, so a code read aloud
	 *   (O vs 0, I vs 1) is less ambiguous and a mistyped digit is more visible.
	 *
	 * `tone` changes only the text colour. It never becomes the only signal: the
	 * caption always states what the code is for.
	 */
	export let code: string;
	export let caption: string = '';
	export let tone: 'neutral' | 'brand' | 'danger' = 'brand';
	export let size: 'md' | 'lg' = 'lg';
</script>

<div class="sigap-code-display">
	{#if caption}
		<p class="sigap-code-display__caption" id="sigap-code-caption">{caption}</p>
	{/if}
	<!--
		Monospaced and tabular so characters align. Spelled out rather than
		driven by a token: a letter-spacing constant is typography, not a visual
		colour token, and adding it to the token set would be scope creep.
	-->
	<p
		class="sigap-code-display__code sigap-code-display__code--{size} sigap-code-display__code--{tone}"
		aria-describedby={caption ? 'sigap-code-caption' : undefined}
	>
		{code}
	</p>
	{#if $$slots.actions}
		<div class="sigap-code-display__actions"><slot name="actions" /></div>
	{/if}
</div>

<style>
	.sigap-code-display {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-radius: var(--sigap-radius-dialog);
	}

	.sigap-code-display__caption {
		margin: 0;
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-code-display__code {
		margin: 0;
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-variant-numeric: tabular-nums;
		font-weight: 600;
		letter-spacing: 0.12em;
		/* Selectable: a citizen retyping a code must be able to copy it. */
		user-select: all;
	}

	.sigap-code-display__code--md {
		font-size: 24px;
	}

	.sigap-code-display__code--lg {
		font-size: 32px;
	}

	.sigap-code-display__code--neutral {
		color: var(--sigap-foreground);
	}

	.sigap-code-display__code--brand {
		color: var(--sigap-primary);
	}

	.sigap-code-display__code--danger {
		color: var(--sigap-danger);
	}

	.sigap-code-display__actions {
		display: flex;
		gap: 8px;
	}
</style>
