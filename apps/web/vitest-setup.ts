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

/**
 * jsdom has no Web Animations API, so `Element.prototype.animate` is absent.
 *
 * Svelte's built-in transitions (fade, fly, slide) call it unconditionally when
 * an element enters or leaves the DOM. Without a shim, any component that uses
 * a transition throws `element.animate is not a function` the moment it tries
 * to animate — which fails the test for an environment reason while telling you
 * nothing about the component.
 *
 * The subtle part is `onfinish`. Svelte does not use `addEventListener('finish')`;
 * it assigns the `onfinish` *property* twice, and the second assignment is
 * chained inside the first one's callback:
 *
 *   1. a zero-duration "dummy" animation is created, and its `onfinish` starts
 *      the real animation;
 *   2. the real animation is created, and its `onfinish` calls the transition's
 *      completion callback, which is what dispatches `outroend` and finally
 *      removes the element from the DOM.
 *
 * A shim that returns a static object silently breaks that chain: the property
 * assignment succeeds, nothing is ever called, and an element with an outro
 * transition stays in the DOM forever. A test then fails with "expected null,
 * got element" and looks like a component bug when it is an environment gap.
 *
 * So the shim fires `onfinish` on a macrotask. A macrotask (not a microtask) is
 * required: the second animation is created *inside* the first callback, so its
 * property has to be assignable before anything fires. Each call returns a fresh
 * object so the two chained animations do not overwrite each other, and
 * `cancel()` latches so an aborted animation cannot fire a late completion.
 */
if (!Element.prototype.animate) {
	Element.prototype.animate = function animate() {
		let onFinish: (() => void) | null = null;
		let cancelled = false;

		const handle = {
			currentTime: 0,
			effect: null,
			playState: 'finished',
			persist: () => {},
			reverse: () => {},
			pause: () => {},
			play: () => {},
			finish: () => {},
			cancel: () => {
				cancelled = true;
			},
			addEventListener: () => {},
			removeEventListener: () => {},
			dispatchEvent: () => false
		} as unknown as Animation;

		Object.defineProperty(handle, 'onfinish', {
			configurable: true,
			get: () => onFinish,
			set: (handler: (() => void) | null) => {
				onFinish = handler;
				setTimeout(() => {
					if (!cancelled) onFinish?.call(handle, new Event('finish') as never);
				}, 0);
			}
		});

		return handle;
	} as Element['animate'];
}

/**
 * jsdom implements the <dialog> element but not its modal behaviour: showModal()
 * and close() are absent, and `open` is not reflected as a property.
 *
 * All three methods are required, not just the opening ones. A shim that
 * provides showModal() but not close() fails in a confusing place: teardown
 * throws `dialog.close is not a function`, and because that happens *before*
 * the focus restore in the Dialog teardown path, focus silently falls to
 * <body> and the focus-return test fails for what looks like a component bug.
 *
 * Deliberately NOT shimmed: top-layer rendering, the ::backdrop pseudo-element,
 * and background inerting. Those are visual/browser behaviours that jsdom cannot
 * meaningfully emulate. A test that depended on them would pass while proving
 * nothing, so the component's own focus management is what gets verified.
 */
if (typeof HTMLDialogElement !== 'undefined') {
	const proto = HTMLDialogElement.prototype as HTMLDialogElement & {
		showModal?: () => void;
		show?: () => void;
		close?: (returnValue?: string) => void;
	};

	const setOpen = (element: HTMLDialogElement, open: boolean) => {
		element.open = open;
		if (open) element.setAttribute('open', '');
		else element.removeAttribute('open');
	};

	if (!proto.showModal) {
		proto.showModal = function showModal(this: HTMLDialogElement) {
			setOpen(this, true);
		};
	}

	if (!proto.show) {
		proto.show = function show(this: HTMLDialogElement) {
			setOpen(this, true);
		};
	}

	if (!proto.close) {
		proto.close = function close(this: HTMLDialogElement, returnValue?: string) {
			if (returnValue !== undefined) this.returnValue = returnValue;
			setOpen(this, false);
			// The browser fires `close` after a programmatic close; matching that
			// keeps consumers that listen for the event working under test.
			this.dispatchEvent(new Event('close'));
		};
	}
}
