<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { Search } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';

	/**
	 * The catalog search box.
	 *
	 * A real `<input type="search">` with a real `<label>`, visually hidden
	 * rather than replaced by the placeholder. A placeholder is not an
	 * accessible name: it disappears the moment the citizen types, and several
	 * screen readers never announce it at all.
	 *
	 * The field narrows already-loaded data. There is no server-side search
	 * contract on the public catalog endpoint, so inventing a query parameter
	 * here would produce a silently wrong result rather than a slower one.
	 */
	export let value: string = '';
	export let placeholder: string = 'Cari nama faskes';
	export let disabled: boolean = false;
	export let onInput: (value: string) => void = () => {};
</script>

<div class="sigap-facility-search">
	<label class="sigap-visually-hidden" for="sigap-facility-search-input">Cari nama faskes</label>
	<span class="sigap-facility-search__icon" aria-hidden="true">
		<Icon icon={Search} size={18} />
	</span>
	<input
		id="sigap-facility-search-input"
		type="search"
		class="sigap-facility-search__input"
		style:border-radius={RADIUS.control}
		{placeholder}
		{disabled}
		{value}
		aria-disabled={disabled ? 'true' : undefined}
		aria-controls="sigap-facility-results"
		on:input={(event) => onInput(event.currentTarget.value)}
	/>
</div>

<style>
	.sigap-facility-search {
		position: relative;
	}

	.sigap-facility-search__icon {
		position: absolute;
		top: 50%;
		left: 12px;
		display: flex;
		color: var(--sigap-muted);
		/* Nudged down because the control is 44px tall and the icon is centred
		   on the input box, not the padded wrapper. */
		transform: translateY(-50%);
		pointer-events: none;
	}

	.sigap-facility-search__input {
		width: 100%;
		/* Citizen touch floor. */
		height: 44px;
		padding: 0 16px 0 40px;
		font-family: inherit;
		font-size: 14px;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-facility-search__input::placeholder {
		color: var(--sigap-muted);
	}

	.sigap-facility-search__input:focus {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -1px;
	}

	.sigap-facility-search__input:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -1px;
	}

	.sigap-facility-search__input:disabled {
		background-color: var(--sigap-canvas);
		cursor: not-allowed;
	}
</style>
