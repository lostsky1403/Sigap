<script lang="ts">
	import { fly } from 'svelte/transition';
	import { RADIUS } from '$lib/design/tokens';
	import { User, LogIn, LogOut, ChevronDown } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';

	/**
	 * Session identity, presented and nothing more.
	 *
	 * The only fact this component is given is `hasSession`. It deliberately has
	 * no access to role, permission, facility scope, or `super_admin`, and it
	 * infers nothing from the email address. A component that could decide
	 * "this person is an administrator" would be an authorization authority in
	 * the browser, and an authorization authority is something a user can edit
	 * — so the capability is not shipped here even as a convenience.
	 *
	 * That is also why there is no role-conditional admin link. The citizen
	 * shell and the admin shell are separate products; the citizen shell never
	 * offers a route into admin.
	 */
	export let hasSession: boolean = false;
	export let userEmail: string = '';

	let open = false;
	let root: HTMLDivElement | null = null;
	let trigger: HTMLButtonElement | null = null;

	/**
	 * Dismiss on outside click and on Escape.
	 *
	 * A disclosure that only closes on its own trigger is a trap for keyboard
	 * and mouse users alike: the menu covers content and the only way out is
	 * the button you may not know you pressed. Escape plus an outside click is
	 * the expected minimum.
	 */
	function handleWindowClick(event: MouseEvent) {
		if (!open) return;
		const target = event.target as Node | null;
		if (target && root && !root.contains(target)) open = false;
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && open) {
			open = false;
			// Return focus to the control that opened the menu, so keyboard
			// position is preserved rather than dumped at the top of the page.
			trigger?.focus();
		}
	}

	/**
	 * Moves focus into the panel after it opens.
	 *
	 * Without this, opening the menu leaves focus on the trigger and the next
	 * Tab lands on the page behind the panel rather than its first action.
	 *
	 * Bound to Svelte's `introend` rather than the DOM's `transitionend`. Svelte
	 * drives transitions through the Web Animations API, which dispatches
	 * `animationend` and never `transitionend` — a `transitionend` listener here
	 * would look correct and never fire, so focus would silently never move.
	 *
	 * The search is scoped to the panel, not to the component root. The trigger
	 * lives inside that root and comes first in document order, so an unscoped
	 * `querySelector('button')` finds the trigger and focuses it again — which
	 * reads as "focus management was implemented" while moving focus nowhere.
	 */
	function focusPanel() {
		const panel = root?.querySelector<HTMLElement>('#sigap-account-menu');
		const first = panel?.querySelector<HTMLElement>('a, button, [tabindex]');
		first?.focus();
	}

	function closeAndFocus() {
		open = false;
		trigger?.focus();
	}
</script>

<svelte:window on:click={handleWindowClick} on:keydown={handleKeydown} />

<div class="sigap-account" bind:this={root}>
	{#if hasSession}
		<button
			bind:this={trigger}
			type="button"
			class="sigap-account__trigger"
			style:border-radius={RADIUS.control}
			aria-expanded={open ? 'true' : 'false'}
			aria-haspopup="menu"
			aria-controls="sigap-account-menu"
			on:click={() => (open = !open)}
		>
			<Icon icon={User} size={20} />
			<span class="sigap-account__label">Akun</span>
			<Icon icon={ChevronDown} size={16} />
		</button>

		{#if open}
			<div
				id="sigap-account-menu"
				class="sigap-account__menu"
				style:border-radius={RADIUS.panel}
				role="menu"
				aria-label="Menu akun"
				transition:fly={{ y: -4, duration: 120 }}
				on:introend={focusPanel}
			>
				<!--
					Email is the identity already established at sign-in. It is
					displayed as text, never parsed: no domain check, no role
					inference, no admin affordance derived from it.
				-->
				{#if userEmail}
					<p class="sigap-account__email" title={userEmail}>{userEmail}</p>
				{/if}

				<form method="POST" action="/auth/logout" role="none">
					<button type="submit" class="sigap-account__item" role="menuitem">
						<Icon icon={LogOut} size={18} />
						<span>Keluar</span>
					</button>
				</form>
			</div>
		{/if}
	{:else}
		<!--
			Sign-in and register are navigation, so they are real links. The shared
			Button primitive renders a <button> by design and deliberately has no
			link mode, so these two repeat its visual contract rather than faking an
			anchor with a button.
		-->
		<div class="sigap-account__anonymous">
			<a class="sigap-account__link" style:border-radius={RADIUS.control} href="/auth/login">
				<Icon icon={LogIn} size={18} />
				<span>Masuk</span>
			</a>
			<a
				class="sigap-account__link sigap-account__link--primary"
				style:border-radius={RADIUS.control}
				href="/auth/register"
			>
				<span>Daftar</span>
			</a>
		</div>
	{/if}
</div>

<style>
	.sigap-account {
		position: relative;
		display: flex;
		align-items: center;
	}

	.sigap-account__trigger {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		/* Citizen touch target floor. */
		height: 44px;
		padding: 0 12px;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		cursor: pointer;
	}

	.sigap-account__trigger:hover {
		border-color: var(--sigap-primary);
		color: var(--sigap-primary);
	}

	.sigap-account__trigger:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-account__label {
		display: none;
	}

	.sigap-account__menu {
		position: absolute;
		top: calc(100% + 8px);
		right: 0;
		z-index: 50;
		min-width: 232px;
		padding: 8px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-account__email {
		margin: 0;
		padding: 8px 12px 12px;
		font-size: 13px;
		color: var(--sigap-muted);
		/* Long addresses must not stretch the panel past the viewport. */
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		border-bottom: 1px solid var(--sigap-border);
	}

	.sigap-account__item {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		height: 44px;
		padding: 0 12px;
		font-size: 14px;
		color: var(--sigap-foreground);
		background-color: transparent;
		border: 0;
		cursor: pointer;
		text-align: left;
	}

	.sigap-account__item:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-account__item:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}

	.sigap-account__anonymous {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.sigap-account__link {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		height: 44px;
		padding: 0 16px;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		text-decoration: none;
	}

	.sigap-account__link:hover {
		border-color: var(--sigap-primary);
		color: var(--sigap-primary);
	}

	.sigap-account__link--primary {
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
		border-color: var(--sigap-primary);
	}

	.sigap-account__link--primary:hover {
		color: var(--sigap-surface);
		background-color: var(--sigap-primary-hover);
		border-color: var(--sigap-primary-hover);
	}

	.sigap-account__link:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	@media (min-width: 1024px) {
		.sigap-account__label {
			display: inline;
		}
	}
</style>
