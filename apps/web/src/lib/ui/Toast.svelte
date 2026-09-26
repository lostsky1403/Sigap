<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { ComponentType } from 'svelte';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from './Icon.svelte';
	import IconButton from './IconButton.svelte';
	import { X } from 'lucide-svelte';

	/**
	 * A transient message anchored to the bottom of the viewport.
	 *
	 * Announced as a live region so a confirmation is not silent, but politely:
	 * a toast should not cut off whatever a screen reader user is in the middle
	 * of. Errors use `role="alert"`, which is assertive, because a failure the
	 * user did not initiate deserves an interruption.
	 *
	 * The timer only starts once the toast is mounted, and it is cleared on
	 * destroy. A toast that keeps ticking after being removed is a leak that
	 * shows up later as a toast vanishing before anyone read it.
	 */
	export let open = false;
	export let message: string;
	export let title: string = '';
	export let variant: 'info' | 'success' | 'warning' | 'danger' = 'info';
	export let icon: ComponentType | undefined = undefined;
	export let duration: number = 5000;
	export let onClose: () => void = () => {};
	export let closeLabel: string = 'Tutup notifikasi';

	let timer: ReturnType<typeof setTimeout> | undefined;

	function startTimer() {
		clearTimer();
		// A zero or negative duration means the toast stays until dismissed,
		// which is the right behaviour for an error the user must act on.
		if (duration <= 0) return;
		timer = setTimeout(() => {
			timer = undefined;
			onClose();
		}, duration);
	}

	function clearTimer() {
		if (timer) {
			clearTimeout(timer);
			timer = undefined;
		}
	}

	$: if (open) {
		startTimer();
	} else {
		clearTimer();
	}

	onDestroy(clearTimer);
</script>

{#if open}
	<div class="sigap-toast-region" role="status" aria-live="polite" aria-atomic="true">
		<div
			class="sigap-toast sigap-toast--{variant}"
			style:border-radius={RADIUS.panel}
			role={variant === 'danger' ? 'alert' : 'status'}
		>
			{#if icon}
				<span class="sigap-toast__icon"><Icon {icon} size={20} /></span>
			{/if}
			<div class="sigap-toast__body">
				{#if title}
					<p class="sigap-toast__title">{title}</p>
				{/if}
				<p class="sigap-toast__message">{message}</p>
			</div>
			<div class="sigap-toast__close">
				<IconButton icon={X} label={closeLabel} size="admin-compact" onClick={onClose} />
			</div>
		</div>
	</div>
{/if}

<style>
	.sigap-toast-region {
		position: fixed;
		bottom: 24px;
		left: 50%;
		transform: translateX(-50%);
		z-index: 50;
		display: flex;
		flex-direction: column;
		gap: 8px;
		width: min(480px, calc(100vw - 32px));
		pointer-events: none;
	}

	.sigap-toast {
		display: flex;
		align-items: flex-start;
		gap: 12px;
		padding: 12px 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-left-width: 3px;
		/* The only surface above the page. No decorative shadow. */
		pointer-events: auto;
	}

	.sigap-toast--info {
		border-left-color: var(--sigap-info);
	}

	.sigap-toast--info .sigap-toast__icon {
		color: var(--sigap-info);
	}

	.sigap-toast--success {
		border-left-color: var(--sigap-success);
	}

	.sigap-toast--success .sigap-toast__icon {
		color: var(--sigap-success);
	}

	.sigap-toast--warning {
		border-left-color: var(--sigap-warning);
	}

	.sigap-toast--warning .sigap-toast__icon {
		color: var(--sigap-warning);
	}

	.sigap-toast--danger {
		border-left-color: var(--sigap-danger);
	}

	.sigap-toast--danger .sigap-toast__icon {
		color: var(--sigap-danger);
	}

	.sigap-toast__body {
		flex: 1;
		min-width: 0;
	}

	.sigap-toast__title {
		margin: 0 0 4px;
		font-size: 14px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-toast__message {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
		color: var(--sigap-foreground);
	}

	.sigap-toast__close {
		flex-shrink: 0;
	}
</style>
