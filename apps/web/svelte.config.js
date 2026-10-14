import adapter from '@sveltejs/adapter-node';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter(),

		/**
		 * Content Security Policy.
		 *
		 * SvelteKit owns this. It generates a fresh nonce per request, stamps
		 * that nonce on its inline hydration script, and writes the matching
		 * header — so the policy can stay free of 'unsafe-inline' and an
		 * injected <script> still has no valid nonce and is still blocked.
		 *
		 * This configuration is not optional polish. The policy used to be
		 * hardcoded in hooks.server.ts as `script-src 'self'`, which blocked
		 * the hydration script outright: every page rendered its server HTML,
		 * every server-side test passed, and in a real browser nothing was
		 * interactive at all. `connect-src 'self'` is still correct and load-
		 * bearing — every API call goes through the same-origin proxy, so a
		 * tighter value would break the app rather than protect it.
		 */
		csp: {
			mode: 'nonce',
			directives: {
				'default-src': ['self'],
				'script-src': ['self'],
				// SvelteKit injects <style> elements for scoped component styles.
				'style-src': ['self', 'unsafe-inline'],
				// Same-origin only: the SvelteKit proxies carry the credentials,
				// so the browser never talks to the Go API directly.
				'connect-src': ['self'],
				'img-src': ['self', 'data:'],
				'frame-ancestors': ['none']
			}
		}
	}
};

export default config;
