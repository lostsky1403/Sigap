import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

/**
 * Vitest configuration for the SIGAP web application.
 *
 * Scope: unit and component tests for $lib modules (design tokens, UI
 * primitives, API client, domain helpers). Route pages are not under test yet;
 * Phase 3B1 explicitly does not redesign them.
 *
 * The existing node:test suites (tests/*.test.js) are intentionally NOT
 * collected here. They keep running through the `test` package script so the
 * proxy, build, and auth-action contracts stay exactly as they were.
 *
 * Only `sveltekit()` is registered. Adding a second, standalone
 * `@sveltejs/vite-plugin-svelte` instance makes both plugins try to compile
 * Svelte 5 runes modules inside node_modules (for example
 * @testing-library/svelte-core/src/props.svelte.js), which fails with
 * "The $ name is reserved". Letting SvelteKit own compilation avoids that.
 *
 * `resolve.conditions` forces the `browser` export condition. Without it Vite
 * resolves Svelte's server entry even under jsdom, so `mount()` is the
 * no-op `lifecycle_function_unavailable` stub and every component render
 * throws. Component tests cannot run without this line.
 */
export default defineConfig({
	plugins: [sveltekit()],
	resolve: {
		conditions: ['browser']
	},
	test: {
		environment: 'jsdom',
		globals: true,
		include: ['src/**/*.{test,spec}.{js,ts}'],
		exclude: ['node_modules/**', '.svelte-kit/**', 'e2e/**'],
		setupFiles: ['./vitest-setup.ts'],
		restoreMocks: true
	}
});
