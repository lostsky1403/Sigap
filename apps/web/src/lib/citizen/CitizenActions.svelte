<script lang="ts">
	import AccountMenu from './AccountMenu.svelte';
	import { Menu } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';

	/**
	 * The masthead's right-hand cluster: the booking CTA and the account
	 * control, in one place, with two presentations.
	 *
	 * At 1024px and up it is an inline row, which is how the reference draws
	 * it — the CTA outermost, the account control beside it.
	 *
	 * Below that it collapses behind a disclosure. That is not a stylistic
	 * choice: the 390px reference frames show a menu affordance where the
	 * desktop frames show two labelled buttons, and two 44px text buttons plus
	 * the wordmark do not fit a 390px bar without either truncating the
	 * wordmark or shrinking the targets under the touch floor.
	 *
	 * The alternative — rendering the cluster twice and hiding one copy with
	 * CSS, the way the two navigations do — is wrong here. `AccountMenu` owns
	 * a popover with a fixed id, so two instances would put a duplicate id in
	 * the document and give every account assertion two candidates. One
	 * instance, one id, two presentations.
	 *
	 * `open` is inert above the breakpoint: the trigger is `display: none`
	 * there, so it is out of the tab order and out of the accessibility tree,
	 * and the panel renders inline regardless of the flag.
	 */
	export let path: string = '/';
	export let hasSession: boolean = false;
	export let userEmail: string = '';
	export let ctaEmphasis: 'filled' | 'outlined' = 'outlined';

	let open = false;
	let root: HTMLDivElement | null = null;
	let trigger: HTMLButtonElement | null = null;

	/**
	 * Dismiss on outside click and on Escape, same contract as the account
	 * menu. A disclosure that only closes on its own trigger traps a keyboard
	 * user behind a panel covering the page.
	 */
	function handleWindowClick(event: MouseEvent) {
		if (!open) return;
		const target = event.target as Node | null;
		if (target && root && !root.contains(target)) open = false;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && open) {
			open = false;
			trigger?.focus();
		}
	}
</script>

<svelte:window on:click={handleWindowClick} on:keydown={handleKeydown} />

<div class="sigap-actions" bind:this={root}>
	<!--
		Mobile only. `aria-controls` names the panel it toggles, and the panel
		is a plain container rather than a `menu` role: it holds a link, a
		disclosure, and a form, not a list of menuitems, and `role="menu"` would
		promise arrow-key navigation that nothing implements.
	-->
	<button
		bind:this={trigger}
		type="button"
		class="sigap-actions__trigger"
		aria-expanded={open ? 'true' : 'false'}
		aria-controls="sigap-actions-panel"
		aria-label="Menu"
		on:click={() => (open = !open)}
	>
		<Icon icon={Menu} size={22} />
	</button>

	<div
		id="sigap-actions-panel"
		class="sigap-actions__panel"
		class:sigap-actions__panel--open={open}
	>
		<!--
			Booking is a primary action, not a destination, so it is not a tab
			and not a nav link. It is outlined by default and filled only on the
			one page that promotes nothing itself; see CitizenHeader for why.
		-->
		<a
			class="sigap-actions__cta"
			class:sigap-actions__cta--filled={ctaEmphasis === 'filled'}
			href="/appointments/new"
		>
			Buat Janji Temu
		</a>

		<AccountMenu {hasSession} {userEmail} {path} />
	</div>
</div>

<style>
	.sigap-actions {
		position: relative;
		display: flex;
		align-items: center;
		flex: none;
	}

	/*
		44px square: the citizen touch floor, and the size the mobile frames
		draw. No border — the reference renders it as a bare icon, and an
		outline here would make the masthead carry a control edge it does not
		need on a surface with nothing behind it.
	*/
	.sigap-actions__trigger {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		padding: 0;
		color: var(--sigap-foreground);
		background-color: transparent;
		border: 0;
		border-radius: var(--sigap-radius-control);
		cursor: pointer;
	}

	.sigap-actions__trigger:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-actions__trigger:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	/*
		Mobile: a popover anchored under the masthead, right-aligned to the
		trigger, closed until the disclosure is opened.
	*/
	.sigap-actions__panel {
		position: absolute;
		top: calc(100% + var(--sigap-space-2));
		right: 0;
		z-index: 50;
		display: none;
		flex-direction: column;
		align-items: stretch;
		gap: var(--sigap-space-3);
		min-width: 232px;
		padding: var(--sigap-space-3);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
		border-radius: var(--sigap-radius-panel);
	}

	.sigap-actions__panel--open {
		display: flex;
	}

	/*
		Desktop: the same two controls, inline. The panel stops being a popover
		entirely — no anchor, no background, no edge — because the masthead
		already provides all three.
	*/
	@media (min-width: 1024px) {
		.sigap-actions__trigger {
			display: none;
		}

		.sigap-actions__panel {
			position: static;
			display: flex;
			flex-direction: row;
			align-items: center;
			min-width: 0;
			padding: 0;
			background-color: transparent;
			border: 0;
		}
	}

	.sigap-actions__cta {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		height: 44px;
		padding: 0 var(--sigap-space-4);
		font-size: var(--sigap-type-body);
		font-weight: 500;
		color: var(--sigap-primary);
		background-color: transparent;
		/* Operable, so its edge carries the 3:1 requirement. */
		border: 1px solid var(--sigap-border);
		border-radius: var(--sigap-radius-control);
		text-decoration: none;
	}

	.sigap-actions__cta:hover {
		color: var(--sigap-primary-hover);
		border-color: var(--sigap-primary-hover);
	}

	.sigap-actions__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-actions__cta--filled {
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-actions__cta--filled:hover {
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary-hover);
		border-color: var(--sigap-primary-hover);
	}

	.sigap-actions__cta--filled:active {
		background-color: var(--sigap-primary-active);
		border-color: var(--sigap-primary-active);
	}
</style>
