<script lang="ts">
	import { DENSITY } from '$lib/ui/density';
	import Button from '$lib/ui/Button.svelte';
	import { RefreshCw } from 'lucide-svelte';

	/**
	 * The admin toolbar: client-side filters on the left, refresh on the right.
	 *
	 * `AdminToolbar` exists so the honesty rule about filtering lives in exactly
	 * one place. Every filter in the admin read layer narrows data the server has
	 * ALREADY scoped and sent; none of them refetch anything. A toolbar that
	 * looked like a server query would let an operator believe they were
	 * querying the clinic's data when they are narrowing their own view, so the
	 * note component renders below every filter row and states the real scope.
	 *
	 * The refresh control is a real `<button>` rather than a link, because it
	 * performs an action. It is also the manual counterpart to the polling
	 * controller on the queue board, and it carries an `aria-busy` state so a
	 * screen reader announces the in-flight refresh instead of leaving silence.
	 */
	export let onRefresh: (() => void) | undefined = undefined;
	export let refreshing: boolean = false;
	export let refreshLabel: string = 'Perbarui';
	/** Set false on a page with no polling or manual refresh (Ringkasan loads once). */
	export let showRefresh: boolean = true;
	/**
	 * The scope statement. Shown on every admin page that reads scoped data, so
	 * "the list is already limited to your facilities" is never a per-page
	 * decision that one page can forget.
	 */
	export let note: string = '';
</script>

<div class="sigap-admin-toolbar">
	{#if $$slots.default}
		<div class="sigap-admin-toolbar__filters">
			<slot />
		</div>
	{/if}

	{#if showRefresh && onRefresh}
		<div class="sigap-admin-toolbar__actions">
			<Button
				variant="secondary"
				size={DENSITY.adminCompact}
				icon={RefreshCw}
				label={refreshLabel}
				disabled={refreshing}
				onClick={() => onRefresh?.()}
			/>
		</div>
	{/if}
</div>

{#if note}
	<p class="sigap-admin-toolbar__note">{note}</p>
{/if}

<style>
	.sigap-admin-toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: 12px;
	}

	.sigap-admin-toolbar__filters {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 12px;
	}

	.sigap-admin-toolbar__actions {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-shrink: 0;
	}

	.sigap-admin-toolbar__note {
		margin: 8px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
