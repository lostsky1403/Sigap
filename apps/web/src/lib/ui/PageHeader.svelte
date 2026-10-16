<script lang="ts">
	/**
	 * The citizen page header: a 2px brand rule, an optional eyebrow, a title,
	 * and an optional lede.
	 *
	 * This is the signature device of the design, and the ORDER is part of it:
	 * the Pen draws the rule first, at the very top of the content column, then
	 * the uppercase eyebrow, then the title, then the description. The rule
	 * reads as the top edge of the page rather than as an underline beneath a
	 * label, which is why it sits above the eyebrow and not below it.
	 *
	 * Every citizen page opens the same way, so it lives in one component
	 * rather than in seven page styles that would drift apart.
	 *
	 * The rule is a content-measure rule, not a full-bleed band: 620px from the
	 * `md` breakpoint up, 140px on a phone. It is the only decorative teal on a
	 * page besides the primary action and the active nav item.
	 */
	export let title: string;
	export let description: string = '';

	/**
	 * An uppercase micro-label between the rule and the title. Optional: only
	 * Beranda and Status carry one, and a page whose title is already inside a
	 * named section does not need to restate it.
	 */
	export let eyebrow: string = '';

	/**
	 * The rule's measure, or its absence.
	 *
	 * `default` is the 140px the Pen draws on the landing, check-in, walk-in
	 * and Jejak Kunjungan frames. `compact` is the 100px it draws on the two
	 * browse pages, Faskes and Status. The desktop measure is 620px on every
	 * page that has one, so neither of those changes the desktop value.
	 *
	 * `none` is for the booking page, and it is a real variant rather than a
	 * convenience: the Pen draws every booking frame with an eyebrow and a
	 * title but NO rule, so the booking journey opens with the label instead of
	 * the mark. Rendering the rule there anyway would add a fourth teal rule to
	 * a flow the design deliberately starts differently.
	 */
	export let rule: 'default' | 'compact' | 'none' = 'default';
</script>

<header class="sigap-page-header">
	{#if rule !== 'none'}
		<div class="sigap-rule" class:sigap-rule--compact={rule === 'compact'} aria-hidden="true"></div>
	{/if}
	{#if eyebrow}
		<!--
			Desktop only. The Pen gives Beranda a page eyebrow at 1440 and drops
			it on the 390px frame, where the rule and the title alone open the
			page. Section eyebrows are a different thing and stay visible at
			every viewport — see `.sigap-eyebrow` in app.css.
		-->
		<p class="sigap-eyebrow sigap-page-header__eyebrow">{eyebrow}</p>
	{/if}
	<h1 class="sigap-page-title">{title}</h1>
	{#if description}
		<p class="sigap-page-subtitle">{description}</p>
	{/if}
</header>

<style>
	.sigap-page-header {
		display: flex;
		flex-direction: column;
		/* The Pen's `Head` runs an 8px gap between the rule, the eyebrow, the
		   title and the description. */
		gap: var(--sigap-space-2);
	}

	.sigap-page-header__eyebrow {
		display: none;
	}

	@media (min-width: 768px) {
		.sigap-page-header__eyebrow {
			display: block;
		}
	}

	.sigap-page-subtitle {
		max-width: 62ch;
	}
</style>
