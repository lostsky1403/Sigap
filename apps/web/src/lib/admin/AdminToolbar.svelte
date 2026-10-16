<script lang="ts">

	/**
	 * The admin toolbar: client-side filters, and the scope note beneath them.
	 *
	 * `AdminToolbar` exists so the honesty rule about filtering lives in exactly
	 * one place. Every filter in the admin read layer narrows data the server has
	 * ALREADY scoped and sent; none of them refetch anything. A toolbar that
	 * looked like a server query would let an operator believe they were
	 * querying the clinic's data when they are narrowing their own view, so the
	 * note component renders below every filter row and states the real scope.
	 *
	 * There is deliberately NO refresh control here. Every admin page's refresh
	 * lives in the header beside the freshness label, which is where the Pen
	 * draws it and what makes "Diperbarui HH.MM" and the control that updates it
	 * the same object. A second refresh in the toolbar would be a duplicate
	 * affordance for one action, and the two could disagree about their disabled
	 * state during an in-flight read.
	 */
	/**
	 * The scope statement. Shown on every admin page that reads scoped data, so
	 * "the list is already limited to your facilities" is never a per-page
	 * decision that one page can forget.
	 */
	export let note: string = '';
</script>

<!--
	The toolbar BLOCK. It exists so the filter row and the scope note share one
	bottom edge: a rule that separates the whole toolbar region from the table
	below it. A plain wrapper `<div>` with no role and no styling of its own, so
	the landmark structure and every accessible name are unchanged.
-->
<div class="sigap-admin-toolbar__block">
	<div class="sigap-admin-toolbar">
		{#if $$slots.default}
			<div class="sigap-admin-toolbar__filters">
				<slot />
			</div>
		{/if}

	</div>

	{#if note}
		<p class="sigap-admin-toolbar__note">{note}</p>
	{/if}
</div>

<style>
	/*
		The toolbar's bottom edge. DECORATIVE, so `--sigap-border-subtle`: it
		separates the filter region from the table below it and is not a control
		boundary. The block also carries the bottom margin, so the rule and the
		spacing below it can never drift apart.
	*/
	.sigap-admin-toolbar__block {
		padding-bottom: var(--sigap-space-3);
		margin-bottom: var(--sigap-space-4);
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-admin-toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--sigap-space-3);
	}

	.sigap-admin-toolbar__filters {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: var(--sigap-space-3);
	}

	.sigap-admin-toolbar__note {
		margin: var(--sigap-space-2) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}
</style>
