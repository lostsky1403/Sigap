<script lang="ts">
	import type { ComponentType } from 'svelte';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from './Icon.svelte';
	import Button from './Button.svelte';

	/**
	 * "There is nothing here" — a legitimate, expected state.
	 *
	 * Distinct from ErrorState on purpose. An empty queue is a normal outcome,
	 * not a failure, and showing it in danger red would train people to ignore
	 * red. This is the calm version: neutral, with an optional way out.
	 */
	export let title: string = 'Belum ada data';
	export let description: string = '';
	export let icon: ComponentType | undefined = undefined;
	export let actionLabel: string = '';
	export let onAction: () => void = () => {};
</script>

<!--
	Announced as a status rather than an alert: appearing empty should not
	interrupt a screen reader user the way an error would.
-->
<div
	class="sigap-empty"
	style:border-radius={RADIUS.panel}
	role="status"
	aria-live="polite"
>
	{#if icon}
		<div class="sigap-empty__icon">
			<Icon {icon} size={24} />
		</div>
	{/if}
	<p class="sigap-empty__title">{title}</p>
	{#if description}
		<p class="sigap-empty__description">{description}</p>
	{/if}
	{#if actionLabel}
		<div class="sigap-empty__action">
			<Button variant="secondary" label={actionLabel} onClick={() => onAction()} />
		</div>
	{/if}
</div>

<style>
	.sigap-empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 32px 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px dashed var(--sigap-border);
	}

	.sigap-empty__icon {
		color: var(--sigap-muted);
	}

	.sigap-empty__title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-empty__description {
		margin: 0;
		max-width: 44ch;
		font-size: 14px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-empty__action {
		margin-top: 8px;
	}
</style>
