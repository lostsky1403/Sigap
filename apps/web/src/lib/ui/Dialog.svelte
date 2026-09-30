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
	 * Hands focus back to whatever held it before the dialog opened, AFTER the
	 * dialog has left the DOM.
	 *
	 * Two things make the deferral mandatory rather than stylistic:
	 *
	 *  1. While a modal <dialog> is open, `showModal()` makes the rest of the
	 *     document INERT. Calling `target.focus()` at that moment targets an
	 *     inert element and silently does nothing — focus then falls to <body>
	 *     when the dialog is removed. This is why the pre-3B5 Escape path
	 *     appeared to work in some runs and not others: it depended on whether
	 *     the browser had finished the top-layer transition.
	 *  2. `{#if open}` removes the dialog element in the same update. Focusing
	 *     the trigger first and removing the dialog afterwards re-blurs it.
	 *
	 * Guarded against a missing or already-removed target: the trigger may have
	 * been unmounted in the same update (a row deleted behind a confirm dialog,
	 * for example), and calling focus() on a detached node silently no-ops.
	 *
	 * Idempotent by construction — `previouslyFocused` is cleared before use, so
	 * a second call is a no-op rather than a double-focus. That is what lets
	 * every close path funnel through here without coordinating with each other.
	 */
	function restoreFocus() {
		const target = previouslyFocused;
		previouslyFocused = null;
		if (target?.isConnected) target.focus();
	}

	/**
	 * Restores focus once the dialog element is gone.
	 *
	 * `tick()` resolves after Svelte has flushed the pending DOM update, which
	 * is exactly when the `{#if open}` removal has happened and the trigger is
	 * focusable again.
	 */
	function restoreFocusAfterClose() {
		void tick().then(restoreFocus);
	}

	/**
	 * Closes the native dialog if still open, then restores focus.
	 *
	 * This runs on destroy as well as on a polite close, because a dialog can
	 * disappear without ever being closed — a route change, a parent `{#if}`, a
	 * logout. In that case focus would otherwise fall to <body>, stranding a
	 * keyboard user at the top of the document.
	 *
	 * No deferral here: on destroy the element is already being removed and
	 * there is no subsequent update to wait for.
	 */
	function teardown() {
		if (dialog?.open) dialog.close();
		restoreFocus();
	}

	/**
	 * Polited close path.
	 *
	 * Focus restoration is NOT done here — it is deferred to
	 * `restoreFocusAfterClose`, because at this instant the dialog is still
	 * modal and the trigger is inert. See the note on that function.
	 */
	function requestClose() {
		if (!open) return;
		open = false;
		restoreFocusAfterClose();
		onClose();
	}

	export function close() {
		requestClose();
	}

	/**
	 * Restores focus when the parent closes the dialog by flipping `open`.
	 *
	 * T-3B5-11. This is not a redundant belt to `requestClose`; before this
	 * block it was the ONLY path that fired for the common case.
	 *
	 * `AdminConfirmDialog` (and every page that uses it) renders this component
	 * UNCONDITIONALLY and drives visibility with the `open` prop. Closing is
	 * therefore the parent flipping `open` to false, which removes the
	 * `{#if open}` block — it does NOT destroy this component, so `onDestroy`
	 * never fires and `teardown()` never runs. Focus restoration was silently
	 * dependent on the caller invoking `close()` itself, which Escape does
	 * (via the native `cancel` event) and a parent-owned Cancel button does not.
	 *
	 * Observed before this fix: dismissing an appointment-cancel dialog with
	 * its own Cancel button left `document.activeElement` on <body>, while
	 * Escape returned focus to the trigger. Both are "closing the dialog", and
	 * a keyboard user loses their place in the table on one of them.
	 *
	 * The transition is watched rather than inferred from the caller, so the
	 * two paths cannot drift apart again. `restoreFocus` is idempotent, so the
	 * Escape path (which calls `requestClose` and then flips the prop) simply
	 * restores once.
	 */
	let wasOpen = false;
	$: if (wasOpen && !open) {
		wasOpen = false;
		restoreFocusAfterClose();
	} else if (open) {
		wasOpen = true;
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
