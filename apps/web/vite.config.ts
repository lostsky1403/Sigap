import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, type Plugin } from 'vite';

/**
 * Preview-server tuning for the E2E gate.
 *
 * THE SYMPTOM
 *
 * The canonical browser suite intermittently failed with
 * `net::ERR_ADDRESS_IN_USE` — on a navigation, and (more often) on a
 * subresource load, which left the page unhydrated so the shared fixture's
 * `data-sigap-hydrated` wait then timed out 30s later. A trace from a failing
 * run shows the cause directly:
 *
 *   console[error] Failed to load resource: net::ERR_ADDRESS_IN_USE
 *
 * That is the CLIENT failing to bind a local source port for a connection to
 * the preview, not the preview failing to serve.
 *
 * THE MECHANISM
 *
 * On loopback the client and the server draw their ephemeral ports from the
 * SAME pool (49152-65535 here; this host reserves ~1,400 of those 16,384 for
 * Hyper-V/WinNAT, measured by binding every port in the range — see
 * `netsh int ipv4 show excludedportrange protocol=tcp`). A closed TCP
 * connection parks its local port in TIME_WAIT, and on Windows the side that
 * sends FIN first keeps the TIME_WAIT entry; on this host the entry lingers
 * for about two minutes. Chromium opens a connection per subresource burst and
 * closes it when Playwright tears the per-test browser context down, so each
 * navigation leaves several ephemeral ports parked in TIME_WAIT on the CLIENT
 * side. They accumulate, and once the pool is full the next `connect()` has no
 * free port and the kernel refuses the bind with `EADDRINUSE` —
 * `ERR_ADDRESS_IN_USE` in Chromium.
 *
 * THE FIX
 *
 * Maximise connection reuse, which is what reduces how many ports are drawn.
 * Measured on this host with the same driver (40 page loads, fresh browser
 * context each): Node's default `keepAliveTimeout` of 5s left 5.28 client
 * TIME_WAIT entries per navigation, because a connection idle for longer than
 * that is reaped and the next request opens a fresh one with a fresh port. At
 * 120s the same driver left 1.05 per navigation — the sockets stay warm and
 * Chromium reuses them. Fewer ports drawn is what keeps the pool from filling.
 *
 * `headersTimeout` must exceed `keepAliveTimeout`, otherwise Node closes a
 * connection while the peer still considers it reusable.
 *
 * Scope: this reduces the churn, it does not make the pool infinite. A suite
 * that drives enough concurrent browser contexts can still exhaust it, so this
 * is a mitigation for the observed load, not a proof against every load.
 *
 * This is deliberately NOT a retry, a widened assertion timeout, or a reduced
 * worker count: nothing about what the suite asserts changes, and the same
 * tests run against the same stack.
 *
 * Preview-only: `configurePreviewServer` does not affect `vite dev` or the
 * production build.
 */
function previewKeepAlive(): Plugin {
	return {
		name: 'sigap-preview-keep-alive',
		configurePreviewServer(server) {
			// `httpServer` is typed as the HTTP/1 or HTTP/2 union; only the HTTP/1
			// server (what `vite preview` creates here) exposes the keep-alive
			// knobs, so narrow to it explicitly rather than casting blindly.
			const http1 = server.httpServer;
			if (!http1 || !('keepAliveTimeout' in http1)) return;
			http1.keepAliveTimeout = 120_000;
			http1.headersTimeout = 130_000;
		}
	};
}

export default defineConfig({
	plugins: [sveltekit(), previewKeepAlive()]
});
