<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import Toast from '$lib/ui/Toast.svelte';
	import {
		clearToasts,
		dismissToast,
		subscribeToToasts,
		type AdminToast
	} from './toastStore';
	import { subscribeToAnnouncements } from './announcer';

	/**
	 * The one place a mutation result becomes visible and audible.
	 *
	 * Mounted once per admin page. Every mutation surface — a queue row, the
	 * schedule editor, the facility deactivate dialog — reports through the two
	 * stores this component subscribes to, and none of them needs to know that
	 * anything renders them. That is the whole reason the stores exist.
	 *
	 * WHY THE ANNOUNCER IS RENDERED ALWAYS, NOT CONDITIONALLY. A live region
	 * that is inserted at the same moment as its text is frequently missed: the
	 * assistive technology observes the DOM change and the text change together,
	 * and some implementations announce only the delta. Keeping the region in
	 * the document from mount and only changing its text is the reliable form.
	 *
	 * `aria-atomic` is set so the whole message is read as one unit. Without it
	 * a two-sentence message can be split and the halves separated by whatever
	 * the user does next.
	 */
	let toasts: readonly AdminToast[] = [];
	let announcement = '';
	let unsubscribeToasts: (() => void) | undefined;
	let unsubscribeAnnouncements: (() => void) | undefined;

	onMount(() => {
		unsubscribeToasts = subscribeToToasts((next) => (toasts = next));
		unsubscribeAnnouncements = subscribeToAnnouncements((next) => (announcement = next));
	});

	onDestroy(() => {
		// A page transition must not leave a subscriber attached to a store that
		// outlives the page, or a toast raised on the next page updates markup
		// that is no longer mounted.
		unsubscribeToasts?.();
		unsubscribeAnnouncements?.();
		clearToasts();
	});

	/**
	 * Dismisses a toast and clears the live region with it.
	 *
	 * Clearing the announcement matters: leaving "moved to Selesai" queued after
	 * the operator has moved on to another row would describe a state they are
	 * no longer looking at.
	 */
	function dismiss(id: number) {
		dismissToast(id);
	}
</script>

<!--
	Always mounted, always empty when nothing is pending. See the note above:
	inserting the region together with its text is unreliable.
-->
<div class="sigap-admin-announcer" role="status" aria-live="polite" aria-atomic="true">
	{announcement}
</div>

{#each toasts as toast (toast.id)}
	<Toast
		open
		message={toast.message}
		title={toast.title}
		variant={toast.variant}
		duration={toast.duration}
		onClose={() => dismiss(toast.id)}
	/>
{/each}

<style>
	/*
		Visually hidden but NOT display:none, aria-hidden, or hidden.
		Each of those removes the element from the accessibility tree, and a live
		region that is not in the tree announces nothing at all. This is the
		standard clip pattern: 1px, clipped, still rendered and still announced.
	*/
	.sigap-admin-announcer {
		position: absolute;
		width: 1px;
		height: 1px;
		margin: -1px;
		padding: 0;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
</style>
