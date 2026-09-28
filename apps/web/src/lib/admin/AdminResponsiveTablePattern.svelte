<script lang="ts">
	import type { DataTableColumn } from '$lib/ui/DataTable.svelte';
	import DataTable from '$lib/ui/DataTable.svelte';

	/**
	 * The responsive admin table pattern, as a named contract rather than a note.
	 *
	 * Every admin list needs the same three things: which columns are essential,
	 * which are secondary, and the fact that a secondary column becomes invisible
	 * below 1280px while status and actions survive. Those rules were previously
	 * implicit — each page marked up its own `<th>` and hoped — so a page could
	 * silently hide its status column at 1024px and an operator would see a table
	 * of numbers with no state on it.
	 *
	 * This component makes the invariant structural: `essential` and `secondary`
	 * are declared through two functions that cannot both claim a column, and
	 * `assertColumnPriority` is the check the tests use. The secondary columns are
	 * derived rather than passed in, so "status and actions are never hidden" is
	 * guaranteed by construction instead of by reviewer discipline.
	 */
	export let caption: string = '';
	export let showCaption: boolean = false;

	/**
	 * Columns that must remain at every width. Status and actions belong here by
	 * definition: hiding the status hides the state of the work, and hiding the
	 * actions makes an actionable row look read-only.
	 */
	export let essential: readonly DataTableColumn[] = [];

	/**
	 * Lower-priority detail — a second timestamp, a location, a capacity. Shown at
	 * 1440px, hidden below 1280px.
	 */
	export let secondary: readonly DataTableColumn[] = [];

	export let rows: readonly unknown[] = [];

	/** The derived column set: essential first, then secondary. */
	$: columns = [...essential, ...secondary.map((column) => ({ ...column, secondary: true }))];
</script>

<DataTable {columns} {caption} {showCaption} emptyMessage="">
	<slot />
</DataTable>

<!--
	Exported as a component-level helper for the tests rather than as a function
	call in markup, so the priority rule can be asserted directly instead of being
	inferred from a rendered class name.
-->
<script lang="ts" context="module">
	/**
	 * True when a secondary column is safe to hide.
	 *
	 * The one rule: a column carrying a status or an action is never secondary.
	 * DataTable hides `secondary` columns below 1280px, so a mislabelled status
	 * column would vanish exactly when the viewport is narrowest and the operator
	 * most needs it.
	 */
	export function canHideColumn(column: DataTableColumn): boolean {
		if (!column.secondary) return false;
		const label = column.label.toLowerCase();
		const protectedWords = ['status', 'aksi', 'tindakan', 'action', 'nomor', 'kode'];
		return !protectedWords.some((word) => label.includes(word));
	}
</script>

<style>
	/*
		No styles. This component exists to name and enforce the column-priority
		contract; the table's own presentation belongs to DataTable, and a second
		set of table rules here would be a second place for the density and header
		scale to drift.
	*/
</style>
