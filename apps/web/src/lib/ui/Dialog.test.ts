import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Dialog from './Dialog.svelte';

/**
 * Dialog accessibility contract (Phase 3B1 section 8).
 *
 * Each of these guards a failure that is invisible in a screenshot. A dialog
 * that renders beautifully but leaves focus behind it, does not close on Escape,
 * or dumps a keyboard user at the top of the document on close looks correct
 * and is unusable.
 *
 * Every test awaits `waitForOpen` first. The component calls showModal() after a
 * tick, so `dialog.open` is false on the synchronous frame after render, and
 * jsdom computes `display: none` for a closed <dialog> — which removes the whole
 * subtree from role queries. Asserting without awaiting would be testing the
 * polyfill's timing, not the dialog.
 */

async function waitForOpen(container: HTMLElement) {
	await waitFor(() => {
		expect((container.querySelector('dialog') as HTMLDialogElement).open).toBe(true);
	});
}

describe('Dialog', () => {
	it('renders as a native dialog and opens modally', async () => {
		const { container } = render(Dialog, { props: { open: true, title: 'Konfirmasi' } });

		const dialog = container.querySelector('dialog');
		expect(dialog).not.toBeNull();
		expect(dialog?.tagName).toBe('DIALOG');
		await waitForOpen(container);
	});

	it('labels itself with its title', async () => {
		const { container } = render(Dialog, {
			props: { open: true, title: 'Konfirmasi', description: 'Tindakan ini tidak dapat dibatalkan.' }
		});

		const dialog = container.querySelector('dialog') as HTMLDialogElement;
		const labelledBy = dialog.getAttribute('aria-labelledby');
		expect(labelledBy).toBeTruthy();
		expect(document.getElementById(labelledBy as string)).toHaveTextContent('Konfirmasi');

		const describedBy = dialog.getAttribute('aria-describedby');
		expect(describedBy).toBeTruthy();
		expect(document.getElementById(describedBy as string)).toHaveTextContent(
			'Tindakan ini tidak dapat dibatalkan.'
		);
	});

	it('omits aria-describedby when there is no description', () => {
		const { container } = render(Dialog, { props: { open: true, title: 'Konfirmasi' } });
		const dialog = container.querySelector('dialog') as HTMLDialogElement;
		expect(dialog.getAttribute('aria-describedby')).toBeNull();
	});

	it('closes on Escape', async () => {
		const onClose = vi.fn();
		const { container } = render(Dialog, {
			props: { open: true, title: 'Konfirmasi', onClose }
		});
		await waitForOpen(container);

		// The browser turns Escape into a cancellable `cancel` event; that is
		// what the component listens for, so the test fires the real event.
		const dialog = container.querySelector('dialog') as HTMLDialogElement;
		dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it('exposes a labelled close control', async () => {
		const { container } = render(Dialog, { props: { open: true, title: 'Konfirmasi' } });
		await waitForOpen(container);
		expect(screen.getByRole('button', { name: 'Tutup' })).toBeInTheDocument();
	});

	it('closes when the close control is activated', async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();
		const { container } = render(Dialog, {
			props: { open: true, title: 'Konfirmasi', onClose }
		});
		await waitForOpen(container);

		await user.click(screen.getByRole('button', { name: 'Tutup' }));
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it('moves focus into the dialog when it opens', async () => {
		const trigger = document.createElement('button');
		trigger.textContent = 'Buka';
		document.body.appendChild(trigger);
		trigger.focus();
		expect(document.activeElement).toBe(trigger);

		const { container } = render(Dialog, { props: { open: true, title: 'Konfirmasi' } });
		await waitForOpen(container);

		// Focus must land inside the dialog, not stay on the page behind it.
		await waitFor(() => {
			const dialog = container.querySelector('dialog');
			expect(dialog?.contains(document.activeElement)).toBe(true);
		});
		trigger.remove();
	});

	it('keeps Tab inside the dialog', async () => {
		const user = userEvent.setup();
		const { container } = render(Dialog, { props: { open: true, title: 'Konfirmasi' } });
		await waitForOpen(container);

		await waitFor(() => {
			const dialog = container.querySelector('dialog');
			expect(dialog?.contains(document.activeElement)).toBe(true);
		});

		// Tab many times: focus must never escape to document.body, which is
		// what a broken trap looks like.
		for (let i = 0; i < 6; i += 1) {
			await user.tab();
			const dialog = container.querySelector('dialog');
			expect(dialog?.contains(document.activeElement)).toBe(true);
		}
	});

	it('returns focus to the element that opened it', async () => {
		const trigger = document.createElement('button');
		trigger.textContent = 'Buka';
		document.body.appendChild(trigger);
		trigger.focus();

		const { container, unmount } = render(Dialog, {
			props: { open: true, title: 'Konfirmasi' }
		});
		await waitForOpen(container);
		await waitFor(() => {
			const dialog = container.querySelector('dialog');
			expect(dialog?.contains(document.activeElement)).toBe(true);
		});

		// Closing is what must restore focus. Unmounting is the destructive
		// equivalent: onDestroy closes the dialog and returns focus, so a dialog
		// removed by a route change does not strand focus on the body.
		unmount();
		await waitFor(() => expect(document.activeElement).toBe(trigger));
		trigger.remove();
	});

	/**
	 * T-3B5-11. The case above does NOT cover how a real dialog closes.
	 *
	 * `AdminConfirmDialog` renders `Dialog` unconditionally and drives visibility
	 * with the `open` prop, so closing is `open: true -> false` and the component
	 * is never destroyed. `onDestroy` — and therefore `teardown()` — never runs on
	 * that path, so focus restoration used to depend entirely on the caller
	 * invoking `close()`. Escape does; a parent-owned Cancel button does not.
	 *
	 * Found by the real-browser mutation E2E: dismissing the appointment-cancel
	 * dialog with its own Cancel button left `document.activeElement` on <body>,
	 * while Escape returned focus correctly. A keyboard user lost their place in
	 * the table in one case and not the other, which is exactly the sort of
	 * asymmetry no screenshot reveals.
	 */
	it('returns focus when the parent closes it by flipping the open prop', async () => {
		const trigger = document.createElement('button');
		trigger.textContent = 'Buka';
		document.body.appendChild(trigger);
		trigger.focus();

		const { container, rerender } = render(Dialog, {
			props: { open: true, title: 'Konfirmasi' }
		});
		await waitForOpen(container);
		await waitFor(() => {
			const dialog = container.querySelector('dialog');
			expect(dialog?.contains(document.activeElement)).toBe(true);
		});

		// The parent's own close path: no close(), no unmount, just open -> false.
		await rerender({ open: false, title: 'Konfirmasi' });

		await waitFor(() => expect(document.activeElement).toBe(trigger));
		trigger.remove();
	});

	it('restores focus exactly once when a close also goes through close()', async () => {
		const trigger = document.createElement('button');
		trigger.textContent = 'Buka';
		document.body.appendChild(trigger);
		trigger.focus();

		const { container, rerender } = render(Dialog, {
			props: { open: true, title: 'Konfirmasi' }
		});
		await waitForOpen(container);

		// The Escape path calls close() (which restores focus) AND the parent's
		// onClose flips the prop. Both must run without the second one stealing
		// focus or throwing — restoreFocus is idempotent by nulling its target,
		// and this is the test that keeps that true.
		const dialog = container.querySelector('dialog') as HTMLDialogElement;
		dialog.dispatchEvent(new Event('cancel', { cancelable: true }));
		await rerender({ open: false, title: 'Konfirmasi' });

		await waitFor(() => expect(document.activeElement).toBe(trigger));
		trigger.remove();
	});
});
