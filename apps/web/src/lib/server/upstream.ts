import { json } from '@sveltejs/kit';
import { apiBase } from './auth';

/**
 * Resolves the upstream Go API base, or produces the response to send instead.
 *
 * Every `/api/v1/*` proxy used to carry its own
 * `process.env.SIGAP_API_INTERNAL || 'http://api:8080'`. That fallback is worse
 * than no fallback:
 *
 *  - `api:8080` is a Docker Compose service name. On a bare-metal dev machine
 *    it does not resolve, so the request fails as a transport error and the
 *    citizen is told "Layanan sedang bermasalah" — the same sentence a genuine
 *    outage produces. A missing environment variable and a dead database were
 *    indistinguishable, which is exactly the confusion this project keeps
 *    paying for.
 *  - It silently disagrees with `apiBase()` above, which throws on purpose
 *    (AUDIT-1805). Two answers to the same question means one of them is wrong.
 *
 * So the configuration question is answered once, here. When it is unset the
 * caller returns a 503 with the JSON envelope the client's error model already
 * understands (`kind: 'server'` → retryable "layanan bermasalah"), and the
 * server logs the precise cause. The citizen still gets a plain answer; the
 * operator gets the actual reason.
 */
export function resolveApiBase(): { base: string } | { unavailable: Response } {
	try {
		return { base: apiBase() };
	} catch (cause) {
		console.error(
			'[sigap] upstream API is not configured; refusing to guess a host.',
			cause instanceof Error ? cause.message : cause
		);
		return {
			unavailable: json(
				{ success: false, error: 'Layanan sedang tidak tersedia.' },
				{ status: 503 }
			)
		};
	}
}
