<script lang="ts">
	import CitizenDesktopNav from './CitizenDesktopNav.svelte';
	import CitizenBottomNav from './CitizenBottomNav.svelte';
	import CitizenActions from './CitizenActions.svelte';

	/**
	 * The citizen shell header.
	 *
	 * Owns the wordmark, the two navigations, the account control, and the
	 * booking CTA. The header is a landmark and the navigations inside it are
	 * separate landmarks, so assistive tech can jump straight to navigation
	 * rather than walking the page.
	 *
	 * The layout is three regions: wordmark, navigation, actions. The nav takes
	 * the slack so the four destinations centre in the gap between the other
	 * two rather than in the viewport — which is where the reference puts them,
	 * and why the labels sit left of the page's true centre at 1440px.
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

	/**
	 * Whether the masthead's booking CTA is the page's dominant action.
	 *
	 * The frozen frames are explicit about this and it is not a style choice:
	 * `Buat Janji Temu` is filled on the catalog page, which promotes nothing
	 * itself, and outlined on the landing and auth pages, which already carry a
	 * filled primary of their own. The rule the frames encode is one filled
	 * primary per action context — so the masthead defers rather than adding a
	 * second one.
	 *
	 * It is a prop and not an inference because the header cannot see the page
	 * body. A page that promotes an action says so here; every other page takes
	 * the default and gets an outlined CTA.
	 */
	export let ctaEmphasis: 'filled' | 'outlined' = 'outlined';
</script>

<header class="sigap-citizen-header">
	<div class="sigap-citizen-header__inner">
		<a class="sigap-citizen-header__brand" href="/" aria-label="Sigap, kembali ke Beranda">
			<span class="sigap-citizen-header__mark" aria-hidden="true">S</span>
			<span class="sigap-citizen-header__wordmark">Sigap</span>
			<span class="sigap-citizen-header__tagline">Layanan Kesehatan Warga</span>
		</a>

		<div class="sigap-citizen-header__nav">
			<CitizenDesktopNav {path} />
		</div>

		<div class="sigap-citizen-header__actions">
			<CitizenActions {hasSession} {userEmail} {path} {ctaEmphasis} />
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
		/* Band delimiter, not a control edge. */
		border-bottom: 1px solid var(--sigap-border-subtle);
	}

	.sigap-citizen-header__inner {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-4);
		/*
			56px at every viewport. The reference draws the same height on the
			1440 desktop frames and the 390px mobile frames, and a taller bar on
			desktop would push the page content down for no gain.
		*/
		height: 56px;
		padding: 0 var(--sigap-space-4);
	}

	@media (min-width: 1024px) {
		.sigap-citizen-header__inner {
			max-width: 1440px;
			margin: 0 auto;
			padding: 0 var(--sigap-space-7);
		}
	}

	.sigap-citizen-header__brand {
		display: flex;
		align-items: center;
		gap: var(--sigap-space-2);
		flex: none;
		min-width: 0;
		/*
			The wordmark is a target, not a word. A 44px touch floor keeps the
			brand reachable without growing the mark's 24px visual block, so the
			56px masthead composition stays untouched.
		*/
		min-height: var(--sigap-control-citizen);
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
		width: 24px;
		height: 24px;
		font-size: 14px;
		font-weight: 700;
		line-height: 1;
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		border-radius: var(--sigap-radius-control);
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
			flex: 1 1 auto;
			min-width: 0;
		}
	}

	.sigap-citizen-header__actions {
		display: flex;
		align-items: center;
		flex: none;
	}
</style>
