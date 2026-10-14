<script lang="ts">
	import type { ComponentType } from 'svelte';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from './Icon.svelte';

	/**
	 * An inline message about the state of the thing it sits next to.
	 *
	 * Error severity announces itself (role="alert"); the rest are polite, so a
	 * success message appearing after a save does not interrupt a screen reader
	 * user mid-sentence.
	 *
	 * Colour is never the only signal. Every variant carries a text label and an
	 * icon, so the alert is legible to someone who cannot distinguish the
	 * semantic hues.
	 */
	export let variant: 'info' | 'success' | 'warning' | 'danger' = 'info';
	export let title: string = '';
	export let icon: ComponentType | undefined = undefined;
	export let role: 'alert' | 'status' | undefined = undefined;
	export let live: boolean = false;

	$: resolvedRole = role ?? (variant === 'danger' ? 'alert' : live ? 'status' : undefined);
</script>

<!--
	A hairline left border carries the severity instead of a filled background.
	It is legible in bright light, does not fight the canvas colour, and avoids
	the decorative tint blocks that made the old pages feel heavy.
-->
<div
	class="sigap-alert sigap-alert--{variant}"
	style:border-radius={RADIUS.control}
	role={resolvedRole}
	aria-live={resolvedRole === 'status' ? 'polite' : undefined}
>
	{#if icon}
		<span class="sigap-alert__icon">
			<Icon {icon} size={20} />
		</span>
	{/if}
	<div class="sigap-alert__body">
		{#if title}
			<p class="sigap-alert__title">{title}</p>
		{/if}
		<div class="sigap-alert__message"><slot /></div>
	</div>
	{#if $$slots.action}
		<div class="sigap-alert__action"><slot name="action" /></div>
	{/if}
</div>

<style>
	.sigap-alert {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		padding: 12px 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		/* Severity is a 3px left rule, not a fill. */
		border-left-width: 3px;
	}

	.sigap-alert--info {
		border-left-color: var(--sigap-info);
		color: var(--sigap-foreground);
	}

	.sigap-alert--info .sigap-alert__icon {
		color: var(--sigap-info);
	}

	.sigap-alert--success {
		border-left-color: var(--sigap-success);
		color: var(--sigap-foreground);
	}

	.sigap-alert--success .sigap-alert__icon {
		color: var(--sigap-success);
	}

	.sigap-alert--warning {
		border-left-color: var(--sigap-warning);
		color: var(--sigap-foreground);
	}

	.sigap-alert--warning .sigap-alert__icon {
		color: var(--sigap-warning);
	}

	.sigap-alert--danger {
		border-left-color: var(--sigap-danger);
		color: var(--sigap-foreground);
	}

	.sigap-alert--danger .sigap-alert__icon {
		color: var(--sigap-danger);
	}

	.sigap-alert__body {
		flex: 1;
		min-width: 0;
	}

	.sigap-alert__title {
		margin: 0 0 4px;
		font-size: 14px;
		font-weight: 600;
	}

	.sigap-alert__message {
		font-size: 14px;
		line-height: 1.5;
	}

	.sigap-alert__action {
		flex-shrink: 0;
	}
</style>
