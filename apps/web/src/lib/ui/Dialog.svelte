<script lang="ts">
	import { onDestroy, tick } from 'svelte';
	import { X } from 'lucide-svelte';
	import { RADIUS } from '$lib/design/tokens';
	import Icon from './Icon.svelte';

	/**
	 * A modal dialog built on the native <dialog> element.
	 *
	 * Native is chosen for reasons that are easy to underestimate:
	 *
	 * - showModal() puts the browser in top-layer mode, which handles the
	 *   backdrop, the inert background, and Escape-to-close without us
	 *   reimplementing them.
	 * - Inerting the rest of the page is what actually traps focus. A JS-only
	 *   trap that forgets a branch lets a citizen tab into the page behind the
	 *   dialog, which is disorienting and, for a destructive confirm, dangerous.
	 *
	 * The two things the native element does NOT do, and which this component
	 * therefore owns, are labelling the dialog and returning focus to whatever
	 * opened it.
	 */
	export let open = false;
	export let title: string;
	export let description: string = '';
	export let closeLabel: string = 'Tutup';
	export let labelledBy: string | undefined = undefined;
	export let describedBy: string | undefined = undefined;
	export let onClose: () => void = () => {};

	let dialog: HTMLDialogElement | undefined;

	/**
	 * The element focused before the dialog opened. Focus has to go back there
	 * on close; otherwise a keyboard user is dumped at the top of the document
	 * and loses their place entirely.
	 */
	let previouslyFocused: HTMLElement | null = null;

	/**
	 * Elements that can receive focus inside the dialog, in DOM order.
	 *
	 * Deliberately NOT filtered by offsetParent / getClientRects. Those are
	 * layout-dependent, and jsdom has no layout engine, so they report every
	 * element as hidden — which would make the focus trap silently select nothing
	 * in tests while appearing to work in a browser. The `hidden` attribute is
	 * the only visibility signal that is meaningful in both environments.
	 */
	const FOCUSABLE =
		'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

	function focusable(): HTMLElement[] {
		if (!dialog) return [];
		return Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
			(element) => !element.hidden && element.getAttribute('aria-hidden') !== 'true'
		);
	}

	/**
	 * Keeps Tab inside the dialog. The native top layer already makes the rest
	 * of the page inert, so this only has to wrap focus at the two ends rather
	 * than intercept every Tab.
	 */
	function handleKeydown(event: KeyboardEvent) {
		if (event.key !== 'Tab' || !dialog) return;

		const targets = focusable();
		if (targets.length === 0) {
			event.preventDefault();
			return;
		}

		const first = targets[0];
		const last = targets[targets.length - 1];
		const active = document.activeElement;

		if (event.shiftKey && (active === first || !dialog.contains(active))) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && active === last) {
			event.preventDefault();
			first.focus();
		}
	}

	/** Escape arrives as a `cancel` event from the native element, not keydown. */
	function handleCancel(event: Event) {
		event.preventDefault();
		close();
	}

	function handleClick(event: MouseEvent) {
		// Light-dismiss: a click whose target is the dialog itself landed on the
		// backdrop, because the panel is a child. A click on panel content never
		// reaches here as the target.
		if (event.target === dialog) close();
	}

	/**
	 * Hands focus back to whatever held it before the dialog opened.
	 *
	 * Guarded against a missing or already-removed target: the trigger may have
	 * been unmounted in the same update (a row deleted behind a confirm dialog,
	 * for example), and calling focus() on a detached node silently no-ops.
	 */
	function restoreFocus() {
		const target = previouslyFocused;
		previouslyFocused = null;
		if (target?.isConnected) target.focus();
	}

	/**
	 * Closes the native dialog if still open, then restores focus.
	 *
	 * This runs on destroy as well as on a polite close, because a dialog can
	 * disappear without ever being closed — a route change, a parent `{#if}`, a
	 * logout. In that case focus would otherwise fall to <body>, stranding a
	 * keyboard user at the top of the document.
	 */
	function teardown() {
		if (dialog?.open) dialog.close();
		restoreFocus();
	}

	/**
	 * Polited close path. The `{#if open}` block is still mounted here, so the
	 * element is only unfocused once focus has already been handed back — the
	 * order matters, otherwise the removal re-blurs the trigger.
	 */
	function requestClose() {
		if (!open) return;
		open = false;
		restoreFocus();
		onClose();
	}

	export function close() {
		requestClose();
	}

	async function show() {
		previouslyFocused = document.activeElement as HTMLElement | null;
		// Wait for the element to exist before asking the browser to show it.
		await tick();
		if (dialog && !dialog.open) {
			dialog.showModal();
			// Focus the first control, or the panel itself if there is none, so
			// the dialog is never opened with focus stranded on the page behind.
			const [first] = focusable();
			(first ?? dialog).focus();
		}
	}

	$: if (open) {
		void show();
	}

	onDestroy(teardown);

	const generatedLabelledBy = `sigap-dialog-title-${Math.random().toString(36).slice(2, 8)}`;
	const generatedDescribedBy = `sigap-dialog-desc-${Math.random().toString(36).slice(2, 8)}`;
</script>

{#if open}
	<!--
		svelte:element would be overkill here; a plain <dialog> with showModal()
		is the correct element, and jsdom needs the polyfill in vitest-setup.
	-->
	<dialog
		bind:this={dialog}
		class="sigap-dialog"
		style:border-radius={RADIUS.dialog}
		aria-labelledby={labelledBy ?? generatedLabelledBy}
		aria-describedby={description ? (describedBy ?? generatedDescribedBy) : describedBy}
		on:keydown={handleKeydown}
		on:cancel={handleCancel}
		on:click={handleClick}
	>
		<div class="sigap-dialog__panel">
			<header class="sigap-dialog__head">
				<h2 class="sigap-dialog__title" id={generatedLabelledBy}>{title}</h2>
				<!--
					A close control is always present, so a dialog is dismissible
					without relying on Escape, which a keyboard user on a switchable
					input may never reach.
				-->
				<button
					type="button"
					class="sigap-dialog__close"
					aria-label={closeLabel}
					on:click={close}
				>
					<Icon icon={X} size={20} />
				</button>
			</header>

			{#if description}
				<p class="sigap-dialog__description" id={generatedDescribedBy}>{description}</p>
			{/if}

			<div class="sigap-dialog__body"><slot /></div>

			{#if $$slots.footer}
				<footer class="sigap-dialog__footer"><slot name="footer" /></footer>
			{/if}
		</div>
	</dialog>
{/if}

<style>
	.sigap-dialog {
		width: min(560px, calc(100vw - 32px));
		max-height: calc(100vh - 64px);
		padding: 0;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-dialog::backdrop {
		/* A flat scrim, not a gradient. */
		background-color: rgb(28 27 26 / 45%);
	}

	.sigap-dialog__panel {
		display: flex;
		flex-direction: column;
		gap: 16px;
		padding: 24px;
	}

	.sigap-dialog__head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
	}

	.sigap-dialog__title {
		margin: 0;
		font-size: 18px;
		font-weight: 600;
		line-height: 1.3;
	}

	.sigap-dialog__close {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 32px;
		height: 32px;
		padding: 0;
		color: var(--sigap-muted);
		background-color: transparent;
		border: 1px solid transparent;
		border-radius: var(--sigap-radius-control);
		cursor: pointer;
	}

	.sigap-dialog__close:hover {
		color: var(--sigap-foreground);
		background-color: var(--sigap-canvas);
	}

	.sigap-dialog__close:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-dialog__description {
		margin: 0;
		font-size: 14px;
		line-height: 1.5;
		color: var(--sigap-muted);
	}

	.sigap-dialog__body {
		font-size: 14px;
		line-height: 1.5;
	}

	.sigap-dialog__footer {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		justify-content: flex-end;
	}
</style>
