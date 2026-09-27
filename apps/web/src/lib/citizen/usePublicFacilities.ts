import { onMount } from 'svelte';
import { apiFetchList } from '$lib/api/client';
import { isAbort, type ApiError } from '$lib/api/errors';
import type { PublicFacility } from '$lib/api/types/api';

/**
 * Loads the public facility catalog for a citizen page.
 *
 * Both Beranda and /faskes read the same public endpoint, and both need to
 * distinguish four outcomes that look alike from the outside: loading, loaded
 * with rows, loaded empty, and failed. Duplicating that state machine in two
 * pages is how the two drift — one grows a retry the other lacks, or one starts
 * showing a red error for a request that was merely aborted.
 *
 * The abort rule matters most. Navigating away mid-flight aborts the request,
 * and that is not a failure. Treating it as one would flash a red error at a
 * citizen who simply tapped another link, so an aborted request leaves the
 * previous state untouched and stays quiet.
 *
 * `apiFetchList` normalizes Go's `data: null` to `[]`, so a legitimately empty
 * catalog arrives here as an empty array. No route null-checks the collection,
 * and no route invents a fallback list to paper over an empty one.
 */

export type CatalogState = 'loading' | 'ready' | 'error';

export interface CatalogSnapshot {
	state: CatalogState;
	facilities: PublicFacility[];
	error: ApiError | null;
}

type Subscriber = (next: CatalogSnapshot) => void;

/**
 * Why this is an explicit store rather than a plain object with getters.
 *
 * The first version returned `{ get state() {...} }` over closure variables and
 * relied on `$:` to re-read them. That silently does not work in Svelte 5: a
 * `$:` block only re-runs when a tracked reactive dependency changes, and plain
 * `let`s mutated from an async callback are not dependencies of anything. The
 * request completed, the getters returned the new values, and the page kept
 * rendering the loading state forever — with no error anywhere.
 *
 * A store is the honest fix. The snapshot is immutable, subscribers are told
 * when it changes, and the component's `$catalog` is a real tracked dependency.
 */
export interface Catalog extends CatalogSnapshot {
	reload: () => void;
	/** Registers a subscriber and returns the teardown that removes it. */
	unsubscribe: (subscriber: Subscriber) => () => void;
}

export function loadPublicFacilities(): Catalog {
	let state: CatalogState = 'loading';
	let facilities: PublicFacility[] = [];
	let error: ApiError | null = null;
	let controller: AbortController | null = null;
	/** Guards against a slow earlier response overwriting a newer one. */
	let requestId = 0;
	const subscribers = new Set<Subscriber>();

	function snapshot(): CatalogSnapshot {
		// Copied on the way out so a consumer cannot mutate the loader's state
		// through the array it holds.
		return { state, facilities: [...facilities], error };
	}

	function emit() {
		const next = snapshot();
		for (const subscriber of subscribers) subscriber(next);
	}

	async function fetchFacilities(id: number) {
		controller?.abort();
		const next = new AbortController();
		controller = next;

		// The full same-origin proxy path, not the bare resource path. The client
		// does not prepend a base, so `/public/facilities` would 404 against the
		// SvelteKit router while looking perfectly reasonable in the source.
		const result = await apiFetchList<PublicFacility>('/api/v1/public/facilities', {
			signal: next.signal
		});

		// A newer request has already started; this response is stale.
		if (id !== requestId) return;

		if (result.ok) {
			state = 'ready';
			facilities = result.data;
			error = null;
			emit();
			return;
		}

		// An abort is a navigation, not a failure. Leave the current state alone
		// so a citizen is never shown an error for something they did not do.
		if (isAbort(result.error)) return;

		state = 'error';
		error = result.error;
		emit();
	}

	function reload() {
		state = 'loading';
		facilities = [];
		error = null;
		emit();
		void fetchFacilities(++requestId);
	}

	onMount(() => {
		void fetchFacilities(++requestId);
		return () => controller?.abort();
	});

	return {
		get state() {
			return state;
		},
		get facilities() {
			return facilities;
		},
		get error() {
			return error;
		},
		reload,
		unsubscribe: (subscriber: Subscriber) => {
			subscribers.add(subscriber);
			// Subscribe-and-catch-up, so a late subscriber does not render one
			// frame with the wrong value.
			subscriber(snapshot());
			return () => {
				// A block body, not an arrow expression: `subscribers.delete(...)`
				// returns a boolean, and this teardown is handed to onDestroy,
				// which expects a function returning nothing.
				subscribers.delete(subscriber);
			};
		}
	};
}
