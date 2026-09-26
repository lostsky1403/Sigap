import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

/**
 * Shared Vitest setup for SIGAP web unit and component tests.
 *
 * jsdom does not implement these browser APIs, and the primitives depend on
 * them, so they are polyfilled here rather than inside individual components.
 * Keeping the polyfills central means a primitive can assume standard DOM
 * behaviour without defensive checks.
 */
afterEach(() => {
	cleanup();
});

// jsdom implements neither matchMedia nor scrollIntoView. Dialog and responsive
// primitives need both; stub them once so tests exercise real component logic.
if (!window.matchMedia) {
	Object.defineProperty(window, 'matchMedia', {
		writable: true,
		value: (query: string) => ({
			matches: false,
			media: query,
			onchange: null,
			addListener: () => {},
			removeListener: () => {},
			addEventListener: () => {},
			removeEventListener: () => {},
			dispatchEvent: () => false
		})
	});
}

if (!Element.prototype.scrollIntoView) {
	Element.prototype.scrollIntoView = () => {};
}

// Svelte 5 checks for this before dispatching transitions in jsdom.
if (!('inert' in HTMLElement.prototype)) {
	Object.defineProperty(HTMLElement.prototype, 'inert', {
		writable: true,
		value: false
	});
}
