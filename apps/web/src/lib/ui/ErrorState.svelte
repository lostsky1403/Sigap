<script lang="ts">
	import type { ComponentType } from 'svelte';
	import { RADIUS } from '$lib/design/tokens';
	import type { ApiError } from '$lib/api/errors';
	import { describe, isRetryable, requiresAuth } from '$lib/api/errors';
	import Icon from './Icon.svelte';
	import Button from './Button.svelte';

	/**
	 * A failed request, presented for a user.
	 *
	 * The component takes a normalized ApiError plus `hasSession`, and decides
	 * what to show from those two facts. It never inspects roles, permissions, or
	 * facility scope: the client is not an authorization authority, and a
	 * component that could infer "you are a patient" from a token would be a
	 * security regression waiting to happen.
	 *
	 * The 403 split matters most here. The admin auth layer returns 403 both for
	 * a missing session and for insufficient permission, so this component asks
	 * "is there a session?" and offers sign-in or refusal accordingly.
	 */
	export let error: ApiError;
	export let hasSession: boolean = false;
	export let icon: ComponentType | undefined = undefined;
	export let onRetry: (() => void) | undefined = undefined;
	export let retryLabel: string = 'Coba lagi';
	export let signInHref: string = '/auth/login';

	$: needsAuth = requiresAuth(error, hasSession);
	$: message = describe(error, hasSession);

	// A retry affordance is only honest for failures that can actually succeed
	// on a second attempt. Offering "try again" for a 404 teaches people to
	// click a button that will never work.
	$: canRetry = !needsAuth && error.kind !== 'not_found' && (isRetryable(error) || error.kind === 'unknown');
</script>

<div
	class="sigap-error-state"
	style:border-radius={RADIUS.panel}
	role="alert"
	aria-live="assertive"
>
	{#if icon}
		<div class="sigap-error-state__icon">
			<Icon {icon} size={24} />
		</div>
	{/if}

	<p class="sigap-error-state__title">
		{needsAuth ? 'Silakan masuk kembali' : 'Gagal memuat data'}
	</p>
	<p class="sigap-error-state__message">{message}</p>

	<div class="sigap-error-state__actions">
		{#if needsAuth}
			<a class="sigap-error-state__link" href={signInHref}>Masuk</a>
		{:else if canRetry && onRetry}
			<Button variant="secondary" label={retryLabel} onClick={onRetry} />
		{/if}
		{#if $$slots.actions}
			<slot name="actions" />
		{/if}
	</div>
</div>

<style>
	.sigap-error-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 32px 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-left: 3px solid var(--sigap-danger);
	}

	.sigap-error-state__icon {
		color: var(--sigap-danger);
	}

	.sigap-error-state__title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-error-state__message {
		margin: 0;
		max-width: 48ch;
		font-size: 14px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-error-state__actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: center;
		justify-content: center;
		margin-top: 8px;
	}

	.sigap-error-state__link {
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: underline;
	}

	.sigap-error-state__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}
</style>
