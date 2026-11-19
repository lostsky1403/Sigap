<script lang="ts">
	import Button from '$lib/ui/Button.svelte';
	import { RefreshCw } from 'lucide-svelte';
	import { formatUpdatedAt } from './readState';
	import { DENSITY } from '$lib/ui/density';

	/**
	 * The admin header's right-hand cluster: when the data was true, and the
	 * control that re-reads it.
	 *
	 * Every admin page that loads scoped data carries both, and the Pen draws
	 * them identically on all six headers — so they live here rather than in six
	 * page files. "Diperbarui" is the load-bearing half: an operator reading a
	 * queue board needs to know how old the rows are before acting on them, and
	 * a per-page implementation is how one page ends up stating a freshness it
	 * cannot support.
	 *
	 * The label stays visible while refreshing, showing the PREVIOUS read's
	 * time. That is deliberate: blanking it during a reload would take away the
	 * one fact the operator has about the rows still on screen.
	 */
	export let loadedAt: Date | null = null;
	export let refreshing: boolean = false;
	export let onRefresh: (() => void) | undefined = undefined;
	export let refreshLabel: string = 'Perbarui';
</script>

{#if loadedAt}
	<span class="sigap-admin-freshness" aria-live="polite">{formatUpdatedAt(loadedAt)}</span>
{/if}

{#if onRefresh}
	<Button
		variant="secondary"
		size={DENSITY.adminCompact}
		icon={RefreshCw}
		label={refreshLabel}
		disabled={refreshing}
		onClick={onRefresh}
	/>
{/if}

<style>
	.sigap-admin-freshness {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
		/* Tabular so the label does not shift width as the minute changes. */
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
</style>
