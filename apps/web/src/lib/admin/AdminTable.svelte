<script lang="ts">
	import DataTable from '$lib/ui/DataTable.svelte';
	import LoadingState from '$lib/ui/LoadingState.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import type { DataTableColumn } from '$lib/ui/DataTable.svelte';

	/**
	 * The admin table: DataTable plus the four states every admin list has.
	 *
	 * The states live here rather than on each page because getting them wrong is
	 * easy and getting them consistently right is what makes an admin list
	 * trustworthy. A list must be able to say, in order: still loading, failed to
	 * load, legitimately empty, or has rows. Pages that hand-roll this produce
	 * the classic bug where an empty result and a failed request look identical,
	 * and an operator concludes their clinic has no queue when the truth is that
	 * the request failed.
	 *
	 * The error is NOT rendered here. It is delegated to the page through the
	 * `error` slot, because choosing between UnauthPanel and ForbiddenPanel needs
	 * the `hasSession` fact, and that fact belongs to the page. A generic wrapper
	 * guessing "403 means sign in" is exactly how a signed-in operator gets told
	 * to sign in again.
	 */
	export let columns: readonly DataTableColumn[] = [];
	export let caption: string = '';
	export let showCaption: boolean = false;

	export let loading: boolean = false;
	/** True when the request failed. Suppresses the table entirely. */
	export let failed: boolean = false;
	export let rows: readonly unknown[] = [];

	export let emptyTitle: string = 'Belum ada data';
	export let emptyDescription: string = 'Tidak ada data dalam lingkup akses Anda saat ini.';
	/**
	 * A dedicated message for "your filter hid everything", which is NOT the same
	 * as "there is nothing here". Telling an operator to clear a filter when the
	 * underlying list is genuinely empty sends them to inspect a control that is
	 * not the problem.
	 */
	export let noMatchesTitle: string = '';
	export let noMatchesDescription: string = '';
	export let resetLabel: string = 'Hapus filter';
	export let onReset: (() => void) | undefined = undefined;
</script>

{#if loading}
	<LoadingState label="Memuat data" />
{:else if failed}
	<slot name="error" />
{:else if rows.length === 0}
	{#if noMatchesTitle}
		<EmptyState
			title={noMatchesTitle}
			description={noMatchesDescription}
			actionLabel={onReset ? resetLabel : ''}
			onAction={() => onReset?.()}
		/>
	{:else}
		<EmptyState
			title={emptyTitle}
			description={emptyDescription}
			actionLabel={onReset ? resetLabel : ''}
			onAction={() => onReset?.()}
		/>
	{/if}
{:else}
	<DataTable {columns} {caption} {showCaption}>
		<slot />
	</DataTable>
{/if}
