<script lang="ts" context="module">
	/**
	 * Field owns the id contract for form controls.
	 *
	 * The association problem is the whole reason this component exists. A
	 * `<label>` that is merely adjacent to an input is not associated with it,
	 * so clicking the label does not focus the control and screen readers do
	 * not announce the label. Wrapping the control in the label handles the
	 * click-to-focus case but breaks the helper and error text, which must be
	 * separate elements referenced by aria-describedby.
	 *
	 * So Field hands out a real `for`/`id` pair and a describedby list. A control
	 * that renders a helper or error registers it; a control that does not,
	 * simply has no describedby.
	 */
	let fieldCount = 0;
</script>

<script lang="ts">
	export let id: string = `sigap-field-${++fieldCount}`;
	export let label: string;
	export let helper: string = '';
	export let error: string = '';
	export let required: boolean = false;
	export let optional: boolean = false;

	/**
	 * Id of the control this field labels. Inputs pass their own id so the
	 * association is explicit rather than inferred, which keeps Field
	 * reusable for Select and Textarea without guessing element types.
	 */
	export let controlId: string = id;

	$: describedBy = [helper ? `${id}-helper` : '', error ? `${id}-error` : '']
		.filter(Boolean)
		.join(' ');
</script>

<div class="sigap-field" class:sigap-field--invalid={Boolean(error)}>
	<div class="sigap-field__head">
		<label class="sigap-field__label" for={controlId}>
			{label}
			{#if required}
				<span class="sigap-field__required" aria-hidden="true">*</span>
				<span class="sigap-visually-hidden">(wajib diisi)</span>
			{:else if optional}
				<span class="sigap-field__optional">(opsional)</span>
			{/if}
		</label>
		{#if $$slots.meta}
			<div class="sigap-field__meta"><slot name="meta" /></div>
		{/if}
	</div>

	<slot controlId={controlId} describedBy={describedBy} invalid={Boolean(error)} />

	<!--
		Helper and error are both live regions so a validation message that
		appears after submit is announced. They are referenced by the control's
		aria-describedby via the slot props above.
	-->
	{#if helper}
		<p class="sigap-field__helper" id="{id}-helper">{helper}</p>
	{/if}
	{#if error}
		<p class="sigap-field__error" id="{id}-error" role="alert">{error}</p>
	{/if}
</div>

<style>
	.sigap-field {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.sigap-field__head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 8px;
	}

	.sigap-field__label {
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	.sigap-field__required {
		color: var(--sigap-danger);
	}

	.sigap-field__optional {
		font-size: 12px;
		font-weight: 400;
		color: var(--sigap-muted);
	}

	.sigap-field__helper {
		margin: 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-field__error {
		margin: 0;
		font-size: 12px;
		color: var(--sigap-danger);
	}

	.sigap-visually-hidden {
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
</style>
