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
</style>
