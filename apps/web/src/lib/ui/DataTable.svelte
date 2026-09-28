<script lang="ts" context="module">
	/**
	 * A column definition.
	 *
	 * Declared in the module context because a Svelte component can only export
	 * runtime values from its instance script — exporting an `interface` from
	 * there produces a runtime "does not export" error rather than a type one,
	 * which is a confusing way to learn the rule.
	 */
	export interface DataTableColumn {
		/** Column header text. Rendered uppercase in the header row. */
		label: string;
		/**
		 * Hide below 1280px. Only ever true for genuinely lower-priority columns;
		 * never for status, actions, or the primary identity of the row.
		 */
		secondary?: boolean;
		/** Right-align the column, for numbers and timestamps. */
		numeric?: boolean;
		/** Header alignment, when it differs from the column's body alignment. */
		headerNumeric?: boolean;
	}
</script>

<script lang="ts">
	/**
	 * The one table primitive in SIGAP.
	 *
	 * A semantic `<table>` with `<caption>`, `<thead>`, and `scope="col"` on every
	 * column header. That is not decoration: a screen reader announces a data
	 * table by reading the header row and re-reading it per cell. A grid of
	 * `<div>`s loses the association entirely, and a table of `<td>`s without
	 * `scope` leaves the reader guessing which column a value belongs to — which
	 * on a clinical screen is the difference between "Puskesmas Sukajaya" and a
	 * number.
	 *
	 * The caption is visually hidden by default because a visible one would
	 * duplicate the page heading, but it is NOT omitted: `caption` is how the
	 * table is named for assistive tech, and a table with no name is a table a
	 * screen-reader user cannot refer to. Callers that want a visible caption
	 * pass `showCaption`.
	 *
	 * Column priority is expressed through the `secondary` flag on a column,
	 * which adds the class that hides it below 1280px. Status and action columns
	 * are never secondary: hiding the status would hide the state, and hiding the
	 * actions would make a row look read-only when it is not. This is why the
	 * hiding rule is a property of the column rather than a media query each page
	 * writes for itself.
	 */
	export let columns: readonly DataTableColumn[] = [];
	export let caption: string = '';
	export let showCaption: boolean = false;
	/**
	 * Rendered in a full-width row beneath the body, used for the empty and
	 * no-results states so they live inside the table's own geometry rather than
	 * as a sibling that could be mistaken for unrelated content.
	 */
	export let emptyMessage: string = '';
</script>

<div class="sigap-data-table__region">
	<table class="sigap-data-table">
		{#if caption}
			<caption class:show-caption={showCaption} class="sigap-data-table__caption">
				{caption}
			</caption>
		{/if}

		<thead>
			<tr>
				{#each columns as column (column.label)}
					<th
						scope="col"
						class:sigap-table-col-secondary={column.secondary}
						class:sigap-data-table__th--numeric={column.numeric ?? column.headerNumeric}
					>
						{column.label}
					</th>
				{/each}
			</tr>
		</thead>

		<tbody>
			{#if $$slots.default}
				<slot />
			{:else if emptyMessage}
				<tr>
					<td class="sigap-data-table__empty" colspan={columns.length}>
						{emptyMessage}
					</td>
				</tr>
			{/if}
		</tbody>
	</table>
</div>

<style>
	/*
		The scroll container is the whole point of this component's outer element.
		A table wider than its column must scroll HERE — inside its own region —
		rather than widening the page, because a whole-page horizontal scrollbar on
		an admin panel means the sidebar scrolls away and the operator loses their
		navigation. The region is also keyboard-scrollable, so a keyboard user can
		reach the hidden columns.
	*/
	.sigap-data-table__region {
		overflow-x: auto;
		max-width: 100%;
	}

	.sigap-data-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}

	.sigap-data-table__caption {
		/*
			Visually hidden but present. `position: absolute` + clip rather than
			`display: none`, because a display-none caption is removed from the
			accessibility tree along with the visual treatment and the table goes
			back to being an unnamed table.
		*/
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	.sigap-data-table__caption.show-caption {
		position: static;
		width: auto;
		height: auto;
		margin: 0 0 8px;
		overflow: visible;
		clip: auto;
		font-size: 13px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	/*
		Declared global for the same reason as the row rule: the header cells are
		rendered here, but the `th` in the empty-state and the row components'
		geometry are not this component's to scope, and a zero-specificity
		`:where()` chain is the kind of selector that fails silently. Measured in
		the browser at 11px uppercase by e2e/admin-table-density.spec.ts.
	*/
	:global(.sigap-data-table thead th) {
		position: sticky;
		top: 0;
		z-index: 1;
		background-color: var(--sigap-surface);
		padding: 10px 12px;
		text-align: left;
		/* 11px uppercase: the frozen admin header scale. */
		font-size: 11px;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--sigap-muted);
		border-bottom: 1px solid var(--sigap-border);
		white-space: nowrap;
	}

	.sigap-data-table__th--numeric {
		text-align: right;
	}

	/*
		`:global` is LOAD-BEARING, and this is the second time this component has
		been bitten by Svelte scoping — the first was the secondary-column rule.

		Svelte compiles a scoped descendant selector by adding the component's own
		scope hash to EVERY element in the chain. So
		`.sigap-data-table tbody td` shipped as:

		    .sigap-data-table.svelte-1rpfqoq tbody:where(.svelte-1rpfqoq) td:where(.svelte-1rpfqoq)

		But the `<td>` elements do not live in this component. They live in the
		caller's row components, each with its own hash — measured on
		/admin/facilities, the cell carried `svelte-u76v92` while the rule wanted
		`svelte-1rpfqoq`. The selector therefore matched nothing, and every admin
		row silently fell back to its content height: a measured 26px, not the
		44px (or 40px) the stylesheet claimed.

		So the whole selector is declared global, and the same is true of the
		`thead th` rule above. `:global` is what makes a PRIMITIVE able to style
		content it does not own — which is the entire purpose of a table that
		takes rows through a slot.

		Row height is then pinned by a measured E2E test
		(e2e/admin-table-density.spec.ts), because a scoped rule that silently
		fails to match produces a stylesheet that looks correct and renders
		incorrectly, and only a real browser can tell the difference.
	*/
	:global(.sigap-data-table tbody td) {
		/*
			ADMIN TABLE ROW DENSITY: 40px, the top of the 36-40px band.

			WHY 40 AND NOT 36. T-3B4-02 in
			design/sigap-redesign-task-breakdown.md sets the acceptance criterion as
			"admin density (36-40px rows, 11px uppercase headers)". The top of the
			band is chosen deliberately:

			  - The action column holds a real admin-density `Button` (36px compact
			    today, 40px comfortable available). A cell cannot be shorter than
			    its content without clipping the control, so 36px would force the
			    buttons down and shrink the target of the only interactive thing in
			    the row. 40px holds a 40px control exactly, with nothing clipped
			    and nothing shrunk to hit a number.
			  - 40px is `controlHeight(DENSITY.adminComfortable)`, so the row and
			    its controls share one number instead of drifting apart.

			NOTE ON A CONFLICTING SIGNAL, recorded rather than hidden. The frozen
			mockup in design/generated/sigap-admin-desktop declares
			`--admin-row-h: 44px` in colors_and_type.css. That is a CITIZEN-scale
			value applied to an ADMIN row, and the canonical task breakdown — which
			the phase spec names as the authority for acceptance criteria — narrows
			admin rows to 36-40px. Where the mockup and the canonical acceptance
			criterion disagree, the acceptance criterion governs, and the 44px token
			in the frozen reference is left untouched because it is a generated
			artefact that must not be regenerated.

			Scope check: `DataTable` is imported only by admin surfaces
			(`routes/admin/{appointments,schedules,facilities,notifications}` and
			`lib/admin/{QueueBoard,AdminTable,AdminResponsiveTablePattern}`). No
			citizen route uses it, so this is admin presentation by construction
			and not by a fragile naming convention. Citizen controls keep their
			>= 44px floor through `lib/ui/density.ts`, untouched here.
		*/
		height: 40px;
		padding: 0 12px;
		border-bottom: 1px solid var(--sigap-border);
		vertical-align: middle;
		background-color: var(--sigap-surface);
	}

	/*
		Hover, global for the same reason as the row rule above: the `td` belongs
		to the caller's row component, so a scoped selector would not match it and
		the hover state would silently do nothing.
	*/
	:global(.sigap-data-table tbody tr:hover td) {
		background-color: var(--sigap-canvas);
	}

	/*
		The empty-state cell belongs to THIS component, so it keeps its scoped
		selector. It also sets `height: auto` deliberately: it is a message, not a
		data row, and it carries 24px of padding. Forcing the row band onto it
		would clip the message.
	*/
	.sigap-data-table__empty {
		height: auto;
		padding: 24px 12px;
		text-align: center;
		color: var(--sigap-muted);
	}

	/*
		Below 1280px, secondary columns go. The 1179px sidebar collapse and this
		1280px column drop are deliberately different breakpoints: the shell
		reflows first, then the table sheds its least important columns, so the
		operator is never asked to read a table that has not finished adapting.

		`:global` is load-bearing. The matching `<td>` lives in a ROW component
		whose styles are scoped, so a scoped rule here would hide the header and
		leave the cell visible — a table with seven headers and nine columns, and
		every value after the hidden one shifted under the wrong label. Declaring
		the rule globally means one breakpoint governs both halves of a column.
	*/
	@media (max-width: 1279px) {
		:global(.sigap-table-col-secondary) {
			display: none;
		}
	}
</style>
