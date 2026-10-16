<script lang="ts">
	/**
	 * The auth card: one measure, one offset, one heading device.
	 *
	 * Login, register, and logout are three pages that are visually the same
	 * object — a 440px card, centred, starting 64px below the masthead, with a
	 * heading, a lede, a body, and a footer line. Three copies of that CSS is
	 * how the login card ends up 8px wider than the register card, so it lives
	 * here and the pages supply content.
	 *
	 * The measure is measured, not chosen: the reference draws the card at
	 * x 500–939.5 on a 1440 viewport, which is 440px wide and centred, with a
	 * 32px content inset and a 64px top offset from the 56px masthead.
	 *
	 * The footer slot is where the "Belum punya akun? Daftar" line goes. It is
	 * inside the card, not below it — the reference puts it within the card's
	 * own boundary, as the last line of the same object rather than a caption
	 * floating underneath.
	 */
	export let title: string;
	export let description: string = '';
</script>

<div class="sigap-page sigap-page--roomy sigap-auth">
	<section class="sigap-auth__panel">
		<h1 class="sigap-auth__title">{title}</h1>
		{#if description}
			<p class="sigap-auth__description">{description}</p>
		{/if}

		<div class="sigap-auth__body">
			<slot />
		</div>

		{#if $$slots.footer}
			<div class="sigap-auth__footer">
				<slot name="footer" />
			</div>
		{/if}
	</section>

	{#if $$slots.note}
		<div class="sigap-auth__note">
			<slot name="note" />
		</div>
	{/if}
</div>

<style>
	/*
		The container is the shared one — `.sigap-page--roomy` gives the Pen's
		64px top offset at 1440, the standard 48px gutter, the symmetric bottom
		padding, and the tab-bar reserve on a phone. Only two things are local:
		the children centre, and the card itself is 440px.

		The measure is measured, not chosen: the reference draws the card at
		x 500–939.5 on a 1440 viewport, which is 440px wide and centred.
	*/
	.sigap-auth {
		align-items: center;
	}

	.sigap-auth__panel {
		width: 100%;
		max-width: 440px;
		padding: var(--sigap-space-5);
		background-color: var(--sigap-surface);
		/* Card edge, not a control edge: the fields inside carry their own. */
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	@media (min-width: 768px) {
		.sigap-auth__panel {
			padding: var(--sigap-space-6);
		}
	}

	.sigap-auth__title {
		margin: 0;
		font-size: var(--sigap-type-page-title);
		font-weight: 600;
		line-height: 1.3;
		color: var(--sigap-foreground);
	}

	.sigap-auth__description {
		margin: var(--sigap-space-2) 0 0;
		font-size: var(--sigap-type-body);
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-auth__body {
		margin-top: var(--sigap-space-5);
	}

	/*
		The footer is separated by space, not by a rule. The reference draws no
		divider above this line: it is the last sentence of the card, and a
		hairline there would make it read as a separate section with its own
		subject.
	*/
	.sigap-auth__footer {
		margin-top: var(--sigap-space-5);
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	.sigap-auth__footer :global(a) {
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-auth__footer :global(a:hover) {
		text-decoration: underline;
	}

	.sigap-auth__footer :global(a:focus-visible) {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	/* Outside the card: a caveat about the flow, not part of the form. */
	.sigap-auth__note {
		margin-top: var(--sigap-space-3);
		font-size: var(--sigap-type-caption);
		line-height: 1.5;
		color: var(--sigap-muted);
	}
</style>
