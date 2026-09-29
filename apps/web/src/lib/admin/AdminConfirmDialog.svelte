<script lang="ts">
	import Dialog from '$lib/ui/Dialog.svelte';
	import Button from '$lib/ui/Button.svelte';
	import { DENSITY } from '$lib/ui/density';

	/**
	 * Confirmation for an action that cannot be undone.
	 *
	 * Built on the native `Dialog` primitive, which is what makes this
	 * accessible without reimplementing a focus trap: `showModal()` moves the
	 * browser into top-layer mode, the rest of the page becomes inert, and
	 * Escape closes — all handled by the platform. What this component adds is
	 * the wording, which is the part that actually matters here.
	 *
	 * WHY THE DESCRIPTION IS NOT GENERIC. A confirmation dialog that says "Are
	 * you sure?" makes the operator work out what is about to happen, which is
	 * exactly the work the dialog should be doing for them. The caller supplies
	 * the consequence in the operator's terms, and this component requires one —
	 * there is no default body, so a destructive action cannot reach a dialog
	 * with an empty explanation.
	 *
	 * WHY `window.confirm` IS NOT USED ANYWHERE. It is modal but not
	 * accessible: it cannot be styled, its text cannot be emphasised, and on
	 * some platforms it silences the screen reader entirely. It also cannot
	 * express a two-paragraph consequence, which the facility deactivation
	 * needs in order to state the public impact honestly.
	 */
	export let open = false;
	export let title: string;
	/** Required. The consequence, in the operator's words. */
	export let description: string;
	export let confirmLabel: string = 'Ya, lanjutkan';
	export let cancelLabel: string = 'Batal';
	export let confirmVariant: 'primary' | 'danger' = 'danger';
	export let busy = false;
	export let onConfirm: () => void = () => {};
	export let onCancel: () => void = () => {};

	/**
	 * Escape and the close button both route here.
	 *
	 * They must, because a dialog dismissed any other way leaves the caller
	 * believing it is still open, and the next Escape would then act on a
	 * stale pending action. One exit path means one state transition.
	 */
	function handleClose() {
		onCancel();
	}
</script>

<Dialog {open} {title} {description} onClose={handleClose} closeLabel="Tutup dialog konfirmasi">
	<svelte:fragment slot="footer">
		<Button
			variant="secondary"
			size={DENSITY.adminCompact}
			label={cancelLabel}
			disabled={busy}
			onClick={handleClose}
		/>
		<Button
			variant={confirmVariant}
			size={DENSITY.adminCompact}
			label={confirmLabel}
			disabled={busy}
			onClick={onConfirm}
		/>
	</svelte:fragment>
</Dialog>
