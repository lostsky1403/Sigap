import { kindForStatus, type ApiError } from './errors';
import type { ApiEnvelope, ApiErrorEnvelope, NullableList } from './types/api';

/**
 * The one place the browser talks to the SIGAP API.
 *
 * Every request goes through the existing same-origin SvelteKit proxies under
 * /api/v1. That is not a convenience, it is the authorization model: the proxies
 * attach the session cookie server-side, so no bearer token is ever handled in
 * browser code, and the client cannot be talked into sending credentials
 * somewhere else. This module therefore has no auth header parameter, and adding
 * one would be a security regression.
 */

/** A successful response, or a normalized failure. Never throws. */
export type ApiResult<T> = { ok: true; data: T } | { ok: false; error: ApiError };

export interface ApiFetchOptions {
	method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
	/** Request body. Serialized as JSON unless it is already a FormData. */
	body?: unknown;
	/** Aborts the request; the resulting error is `aborted`, not a failure. */
	signal?: AbortSignal;
	/** Query parameters. Omitted keys are not sent, so no empty `?flag=`. */
	query?: Record<string, string | number | boolean | undefined | null>;
	/** Extra request headers. Content-Type is set automatically for a body. */
	headers?: Record<string, string>;
}

/**
 * True for a body that fetch must not serialize itself.
 */
function isRawBody(body: unknown): body is BodyInit {
	return (
		typeof FormData !== 'undefined' && body instanceof FormData ||
		typeof Blob !== 'undefined' && body instanceof Blob ||
		typeof body === 'string'
	);
}

/**
 * Builds the request URL from a path and a query bag.
 *
 * Query values are URL-encoded, and undefined/null entries are dropped entirely
 * rather than serialized as "undefined" — an empty `?status=` is a different
 * request to the backend than an absent one.
 */
export function buildUrl(
	path: string,
	query?: ApiFetchOptions['query']
): string {
	const url = new URL(path, window.location.origin);
	if (query) {
		for (const [key, value] of Object.entries(query)) {
			if (value === undefined || value === null) continue;
			url.searchParams.set(key, String(value));
		}
	}
	// Return the path + query rather than the absolute URL: the request stays
	// same-origin, and the proxy is what attaches credentials.
	return `${url.pathname}${url.search}`;
}

/**
 * Parses a response body without ever throwing.
 *
 * Two backend realities drive this:
 *
 * 1. `json.NewEncoder(...).Encode` appends a trailing newline. `JSON.parse`
 *    tolerates surrounding whitespace, but a body that arrives as a raw string
 *    with a newline must still be handled, so the whole read is defensive.
 * 2. Some paths answer with a bare status and NO body at all — a 405 for an
 *    unlisted method, a 204 preflight. `response.json()` on an empty body
 *    throws, so emptiness is checked before parsing.
 */
async function readBody(response: Response): Promise<{ body: unknown; malformed: boolean }> {
	const contentType = response.headers.get('content-type') ?? '';
	if (!contentType.includes('application/json')) return { body: undefined, malformed: false };

	const text = await response.text();
	// Trailing newline included: the encoder appends one to every response.
	if (text.trim() === '') return { body: undefined, malformed: false };
	try {
		return { body: JSON.parse(text), malformed: false };
	} catch {
		// A malformed body is a server bug. Flagged rather than thrown, so the
		// caller reports a failure instead of rendering undefined fields.
		return { body: undefined, malformed: true };
	}
}

/** Pulls the human-facing message out of the flat {success, error} envelope. */
function messageFrom(body: unknown): string | undefined {
	if (body && typeof body === 'object' && 'error' in body) {
		const error = (body as ApiErrorEnvelope).error;
		if (typeof error === 'string' && error.trim() !== '') return error;
	}
	return undefined;
}

function errorResult(status: number, body: unknown): ApiError {
	return {
		kind: kindForStatus(status),
		status,
		message: messageFrom(body) ?? '',
		body
	};
}

/**
 * Performs one API call and normalizes the outcome.
 *
 * Returns a result rather than throwing. A route that renders a list has to
 * handle failure on every path, and a result type makes that path impossible to
 * leave out — while also keeping an abort from surfacing as an unhandled
 * rejection when the user navigates mid-request.
 */
export async function apiFetch<T>(
	path: string,
	options: ApiFetchOptions = {}
): Promise<ApiResult<T>> {
	const { method = 'GET', body, signal, query, headers = {} } = options;
	const url = buildUrl(path, query);

	const requestHeaders: Record<string, string> = { Accept: 'application/json', ...headers };
	let payload: BodyInit | undefined;
	if (body !== undefined) {
		if (isRawBody(body)) {
			payload = body;
		} else {
			payload = JSON.stringify(body);
			// Only claim JSON when we are actually sending JSON. Setting it for a
			// FormData body would break the multipart boundary.
			if (!requestHeaders['Content-Type'] && !requestHeaders['content-type']) {
				requestHeaders['Content-Type'] = 'application/json';
			}
		}
	}

	let response: Response;
	try {
		response = await fetch(url, {
			method,
			headers: requestHeaders,
			body: payload,
			// Same-origin: the session cookie rides along and nothing else does.
			credentials: 'same-origin',
			signal
		});
	} catch (cause) {
		// An abort is a deliberate cancellation, not a failure to report. It gets
		// its own kind so a route can ignore it instead of showing a scary error.
		if (cause instanceof DOMException && cause.name === 'AbortError') {
			return { ok: false, error: { kind: 'aborted', status: 0, message: '' } };
		}
		if (signal?.aborted) {
			return { ok: false, error: { kind: 'aborted', status: 0, message: '' } };
		}
		return {
			ok: false,
			error: { kind: 'network', status: 0, message: '', body: cause }
		};
	}

	const { body: parsed, malformed } = await readBody(response);

	if (!response.ok) {
		return { ok: false, error: errorResult(response.status, parsed) };
	}

	// A 200 whose JSON body would not parse is a failure, not a success with no
	// data. Reporting it as success would render a page of undefined fields and
	// look like an application bug rather than a broken response.
	if (malformed) {
		return {
			ok: false,
			error: { kind: 'unknown', status: response.status, message: '' }
		};
	}

	// 204 and friends: a success with no payload to unwrap.
	if (parsed === undefined) {
		return { ok: true, data: undefined as T };
	}

	// Unwrap {success, data}. When the body is already the payload — a proxy or
	// a health endpoint that answers bare — hand it back untouched rather than
	// returning undefined, which is the failure this shape is meant to avoid.
	if (typeof parsed === 'object' && parsed !== null && 'success' in parsed) {
		const envelope = parsed as ApiEnvelope<T> | ApiErrorEnvelope;
		if (envelope.success === false) {
			return {
				ok: false,
				error: { kind: 'unknown', status: response.status, message: envelope.error, body: parsed }
			};
		}
		return { ok: true, data: envelope.data };
	}

	return { ok: true, data: parsed as T };
}

/**
 * Fetches a list endpoint and normalizes the Go `data: null` empty result.
 *
 * The backend encodes a nil slice as `null` (see NullableList in types/api.ts),
 * so a perfectly healthy "you have no rows" response arrives as `null`. Passing
 * that straight to a route would put `rows.map(...)` one keystroke away from a
 * null dereference, and the resulting crash would read as an application bug
 * rather than a backend contract quirk.
 *
 * Normalizing here — at the boundary, once — means every list call site gets a
 * real array. The alternative of teaching each route to null-check is exactly
 * the kind of duplication that rots, so it is deliberately not done that way.
 *
 * This is a presentation concern only. It does not invent rows, and it does not
 * mask a failure: a non-2xx or malformed response still comes back as
 * `ok: false` with its error intact.
 */
export async function apiFetchList<T>(
	path: string,
	options: ApiFetchOptions = {}
): Promise<ApiResult<T[]>> {
	const result = await apiFetch<NullableList<T>>(path, options);
	if (!result.ok) return result;
	return { ok: true, data: result.data ?? [] };
}
