<script lang="ts">
	/**
	 * The citizen page header: a 2px brand rule, a title, and an optional lede.
	 *
	 * This is the signature device of the design. Every citizen page opens the
	 * same way, so it lives in one component rather than in seven page styles
	 * that would drift apart.
	 *
	 * The rule is a content-measure rule, not a full-bleed band: it is 140px on
	 * a phone and 620px from the `md` breakpoint up. It is the only decorative
	 * teal on a page besides the primary action and the active nav item.
	 */
	export let title: string;
	export let description: string = '';

	/**
	 * An uppercase micro-label above the title. Optional, because a page that
	 * is already inside a named section does not need to restate it.
	 */
	export let eyebrow: string = '';
</script>

<header class="sigap-page-header">
	{#if eyebrow}
		<p class="sigap-eyebrow">{eyebrow}</p>
	{/if}
	<div class="sigap-rule" aria-hidden="true"></div>
	<h1 class="sigap-page-title">{title}</h1>
	{#if description}
		<p class="sigap-page-subtitle">{description}</p>
	{/if}
</header>

<style>
	.sigap-page-header {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-2);
	}

	.sigap-page-subtitle {
		max-width: 62ch;
	}
</style>
