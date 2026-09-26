<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * "You need to sign in."
	 *
	 * The counterpart to ForbiddenPanel. Shown when the server answered 403 and
	 * the client knows there is no session — the one case where a 403 genuinely
	 * means "authenticate", not "you may not".
	 *
	 * It carries no message about why the request was refused, and makes no
	 * attempt to guess what the user was trying to do. The destination after
	 * sign-in is the server's concern, not the client panel's.
	 */
	export let title: string = 'Silakan masuk';
	export let description: string = 'Anda perlu masuk terlebih dahulu untuk melihat halaman ini.';
	export let signInHref: string = '/auth/login';
	export let signInLabel: string = 'Masuk';
</script>

<div class="sigap-unauth" style:border-radius={RADIUS.panel} role="alert">
	<p class="sigap-unauth__title">{title}</p>
	<p class="sigap-unauth__description">{description}</p>

	<div class="sigap-unauth__actions">
		<a class="sigap-unauth__cta" href={signInHref}>{signInLabel}</a>
		{#if $$slots.actions}
			<slot name="actions" />
		{/if}
	</div>
</div>

<style>
	.sigap-unauth {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 32px 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-left: 3px solid var(--sigap-info);
	}

	.sigap-unauth__title {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-unauth__description {
		margin: 0;
		max-width: 52ch;
		font-size: 14px;
		line-height: 1.6;
		color: var(--sigap-muted);
	}

	.sigap-unauth__actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: center;
		justify-content: center;
		margin-top: 8px;
	}

	.sigap-unauth__cta {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		/* Citizen density: 44px touch target. */
		height: 44px;
		padding: 0 16px;
		font-size: 16px;
		font-weight: 500;
		text-decoration: none;
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-unauth__cta:hover {
		background-color: var(--sigap-primary-hover);
	}

	.sigap-unauth__cta:active {
		background-color: var(--sigap-primary-active);
	}

	.sigap-unauth__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
