<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import type { AdminFacility } from '$lib/api/types/api';

	/**
	 * Facility narrowing over data the server has ALREADY scoped.
	 *
	 * This is a view control and nothing else. It filters an array already in
	 * memory; it issues no request, and it must never be described as though it
	 * does. That distinction is why the count label is worded the way it is and
	 * why the copy below the control says so explicitly: "Menampilkan: X dari Y"
	 * states the visible subset against the loaded total, which is the only claim
	 * a client-side filter can honestly make. Calling it a facility query would
	 * tell an operator they are searching the clinic's records when they are
	 * narrowing their own already-authorized view.
	 *
	 * The option list is built from the loaded facilities rather than fetched.
	 * That is what makes it a filter and not a second data source — and it also
	 * means the control cannot offer a facility the operator is not authorized
	 * for, because such a facility was never sent.
	 *
	 * NO raw-UUID input. The pre-3B4 queue page had a free-text "Filter ID
	 * Fasilitas" box, which invited an operator to paste an id, produced empty
	 * tables with no explanation, and displayed identifiers meant for machines in
	 * a field meant for people. Every option here is a facility name.
	 */
	export let facilities: readonly AdminFacility[] = [];
	export let selected: string = '';
	export let onSelect: (facilityId: string) => void = () => {};
	export let id: string = 'sigap-admin-facility-filter';
	export let label: string = 'Filter fasilitas';

	/**
	 * How many rows are currently shown, out of how many are loaded. `undefined`
	 * while loading, which suppresses the label rather than printing "0 dari 0"
	 * over a table that is still arriving.
	 */
	export let visible: number | undefined = undefined;
	export let total: number | undefined = undefined;

	$: shown = visible ?? 0;
	$: loaded = total ?? 0;
	/**
	 * The count is shown whenever the loaded totals are known — INCLUDING when
	 * nothing is selected.
	 *
	 * Suppressing it while unfiltered was defensible tidiness and is now wrong:
	 * "Menampilkan: Y dari Y" is the only thing on this control that states the
	 * dataset is complete and un-narrowed, and an operator who cannot see the
	 * total cannot tell the difference between "12 rows, that is all of them"
	 * and "12 rows, of many more". The single-rule form also covers every state
	 * T-3B4-02 names:
	 *
	 *   unfiltered   -> Menampilkan: Y dari Y
	 *   narrowed     -> Menampilkan: X dari Y
	 *   no matches   -> Menampilkan: 0 dari Y
	 *
	 * `0 dari Y` in particular must not be swapped for an empty-state message
	 * while the filter is active: zero results from a client-side narrowing is
	 * still a statement about a known dataset, and the empty state that replaces
	 * it says nothing about how many rows were excluded.
	 *
	 * `undefined` totals mean the parent has not finished loading, so there is
	 * nothing truthful to print yet and the label is withheld rather than
	 * printing a misleading "0 dari 0" over a table that is still arriving.
	 */
	$: showCount = visible !== undefined && total !== undefined;
</script>

<div class="sigap-facility-filter">
	<label class="sigap-facility-filter__label" for={id}>{label}</label>
	<select
		class="sigap-facility-filter__select"
		style:border-radius={RADIUS.control}
		{id}
		value={selected}
		on:change={(event) => onSelect(event.currentTarget.value)}
	>
		<option value="">Semua fasilitas</option>
		{#each facilities as facility (facility.id)}
			<option value={facility.id}>{facility.name}</option>
		{/each}
	</select>

	{#if showCount}
		<!--
			`aria-live="polite"` because this count changes as the operator types in
			the search box beside it. A screen-reader user narrowing a list needs to
			hear that the result set changed without having to go looking for it.
		-->
		<p class="sigap-facility-filter__count" aria-live="polite">
			Menampilkan: {shown} dari {loaded}
		</p>
	{/if}
</div>

<style>
	.sigap-facility-filter {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.sigap-facility-filter__label {
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-facility-filter__select {
		/* 36px, the frozen compact admin control height. */
		height: 36px;
		min-width: 190px;
		padding: 0 10px;
		border: 1px solid var(--sigap-border);
		background-color: var(--sigap-surface);
		color: var(--sigap-foreground);
		font: inherit;
		font-size: 13px;
	}

	.sigap-facility-filter__select:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-facility-filter__count {
		margin: 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
