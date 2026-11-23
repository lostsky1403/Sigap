<script lang="ts">
	/**
	 * The admin page header: title, subtitle, and the right-hand status area.
	 *
	 * The right-hand slot is where the last-updated label and the refresh control
	 * live, and it is the reason this is a component rather than markup repeated
	 * on six pages. "Diperbarui pukul HH:MM" must appear on every page that
	 * loads scoped data, and a per-page implementation is how one page ends up
	 * claiming a freshness it cannot support.
	 *
	 * The header is sticky and sits above the scrolling content so the update
	 * time stays visible while an operator reads a long table — which is exactly
	 * when knowing how old the data is matters.
	 */
	export let title: string = '';
	export let subtitle: string = '';
</script>

<header class="sigap-admin-header">
	<div class="sigap-admin-header__inner">
		<div class="sigap-admin-header__text">
			<h1 class="sigap-admin-header__title">{title}</h1>
			{#if subtitle}
				<p class="sigap-admin-header__subtitle">{subtitle}</p>
			{/if}
		</div>
		{#if $$slots.aside}
			<div class="sigap-admin-header__aside">
				<slot name="aside" />
			</div>
		{/if}
	</div>
</header>

<style>
	.sigap-admin-header {
		position: sticky;
		top: 0;
		z-index: 30;
		background-color: var(--sigap-canvas);
		/*
			DECORATIVE, not a control boundary. The header's bottom edge is a rule
			separating the sticky bar from the content that scrolls under it, so it
			belongs to `--sigap-border-subtle`. Wiring it to `--sigap-border`
			(#767E7C) draws a 4.16:1 line across the top of every admin page, which
			reads as a wireframe rather than as a rule.
		*/
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-admin-header__inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--sigap-space-4);
		height: 56px;
		padding: 0 var(--sigap-space-5);
	}

	.sigap-admin-header__text {
		min-width: 0;
	}

	.sigap-admin-header__title {
		margin: 0;
		font-size: var(--sigap-type-page-title);
		font-weight: 600;
		line-height: 24px;
		letter-spacing: -0.01em;
		color: var(--sigap-foreground);
	}

	.sigap-admin-header__subtitle {
		margin: 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	.sigap-admin-header__aside {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-3);
		flex-shrink: 0;
	}

	/*
		Below 1024px the header grows instead of holding 56px.

		The Pen's fixed height is safe for the Pen because it SHORTENS the
		subtitle as the screen narrows: the long sentence at 1440, "Pembaruan
		otomatis sekitar 30 detik." at 768, and the freshness label alone at 390.
		These pages carry one subtitle at every width, and it is the long one — so
		at 56px the text block is narrower than its own sentence, the subtitle
		wraps past the box, and the second line paints over the table below it.

		Wrapping the row and letting it grow is the honest fix: nothing is hidden
		and nothing is clipped, and the header is simply taller on a narrow screen.
		The freshness label and the refresh control stay put at every width, which
		is what keeps the data's age visible while an operator scrolls a long table.
	*/
	@media (max-width: 1023px) {
		.sigap-admin-header__inner {
			flex-wrap: wrap;
			height: auto;
			min-height: 56px;
			row-gap: var(--sigap-space-2);
			padding-top: var(--sigap-space-3);
			padding-bottom: var(--sigap-space-3);
		}

		.sigap-admin-header__text {
			flex: 1 1 auto;
		}
	}

	/*
		Below 768px: a tighter gutter, and the header stops being sticky.

		The gutter drops from 32px to 16px because the freshness label and the
		refresh control together need ~242px, and at 360px the 32px gutters left
		240 — a 3px shortfall that pushed the aside past its own box. The rail
		already costs 56px, so the horizontal space here is genuinely scarce.

		The header also stops sticking. A two-line header is ~152px, which is 18%
		of a 844px phone viewport; pinned to the top it would hold that strip for
		the entire session. The Pen avoids the question by cutting the mobile
		subtitle to a freshness label, but that sentence is where these pages state
		the facility scope their data is limited to, and dropping it on a phone
		would remove the one claim that keeps a scoped list honest. So the copy
		stays and the header scrolls away with the rest of the page instead.
	*/
	@media (max-width: 767px) {
		.sigap-admin-header {
			position: static;
		}

		.sigap-admin-header__inner {
			padding-left: var(--sigap-space-4);
			padding-right: var(--sigap-space-4);
		}
	}
</style>
