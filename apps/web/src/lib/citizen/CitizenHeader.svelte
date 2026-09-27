<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import CitizenDesktopNav from './CitizenDesktopNav.svelte';
	import CitizenBottomNav from './CitizenBottomNav.svelte';
	import AccountMenu from './AccountMenu.svelte';

	/**
	 * The citizen shell header.
	 *
	 * Owns the wordmark, the two navigations, and the account control. The
	 * header is a landmark and the navigations inside it are separate
	 * landmarks, so assistive tech can jump straight to navigation rather than
	 * walking the page.
	 *
	 * The two navs are mutually exclusive by viewport: the bottom bar is
	 * `display: none` from 1024px up and the desktop nav is `display: none`
	 * below it. `display: none` rather than visually-hidden, because a hidden
	 * landmark stays in the accessibility tree and would leave the page
	 * advertising two full sets of navigation links.
	 */
	export let path: string = '/';
	export let hasSession: boolean = false;
	export let userEmail: string = '';
</script>

<header class="sigap-citizen-header">
	<div class="sigap-citizen-header__inner">
		<a
			class="sigap-citizen-header__brand"
			style:border-radius={RADIUS.control}
			href="/"
			aria-label="Sigap, kembali ke Beranda"
		>
			<span class="sigap-citizen-header__mark" style:border-radius={RADIUS.control} aria-hidden="true">
				S
			</span>
			<span class="sigap-citizen-header__wordmark">Sigap</span>
			<span class="sigap-citizen-header__tagline">Layanan Kesehatan Warga</span>
		</a>

		<div class="sigap-citizen-header__nav">
			<CitizenDesktopNav {path} />
		</div>

		<div class="sigap-citizen-header__account">
			<AccountMenu {hasSession} {userEmail} />
		</div>
	</div>
</header>

<!--
	The bottom bar is a sibling of the header rather than a child: it is fixed
	to the viewport, not to the header's stacking and border context.
-->
<CitizenBottomNav {path} />

<style>
	.sigap-citizen-header {
		position: sticky;
		top: 0;
		z-index: 40;
		background-color: var(--sigap-surface);
		border-bottom: 1px solid var(--sigap-border);
	}

	.sigap-citizen-header__inner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		height: 56px;
		padding: 0 16px;
	}

	@media (min-width: 1024px) {
		.sigap-citizen-header__inner {
			height: 64px;
			padding: 0 24px;
		}
	}

	.sigap-citizen-header__brand {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		text-decoration: none;
	}

	.sigap-citizen-header__brand:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-citizen-header__mark {
		display: flex;
		align-items: center;
		justify-content: center;
		flex: none;
		width: 28px;
		height: 28px;
		font-size: 15px;
		font-weight: 600;
		line-height: 1;
		color: var(--sigap-surface);
		background-color: var(--sigap-primary);
	}

	.sigap-citizen-header__wordmark {
		font-size: 17px;
		font-weight: 600;
		line-height: 1;
		color: var(--sigap-foreground);
	}

	.sigap-citizen-header__tagline {
		display: none;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	@media (min-width: 1024px) {
		.sigap-citizen-header__wordmark {
			font-size: 18px;
		}

		.sigap-citizen-header__tagline {
			display: inline;
		}
	}

	.sigap-citizen-header__nav {
		display: none;
	}

	@media (min-width: 1024px) {
		.sigap-citizen-header__nav {
			display: flex;
		}
	}

	.sigap-citizen-header__account {
		display: flex;
		align-items: center;
		flex: none;
	}
</style>
