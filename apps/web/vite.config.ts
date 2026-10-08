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
 * SAME pool (49152-65535 here, minus ~2,000 ports this host reserves for
 * Hyper-V/Docker — `netsh int ipv4 show excludedportrange protocol=tcp`). A
 * closed TCP connection parks its local port in TIME_WAIT for 240s, and on
 * Windows the side that sends FIN first keeps the TIME_WAIT entry. Chromium
 * opens a connection per subresource burst and closes it when Playwright tears
 * the per-test browser context down, so each navigation leaves several
 * ephemeral ports parked in TIME_WAIT on the CLIENT side. Measured across the
 * suite, that is ~5 per navigation, and they accumulate until the next
 * `connect()` has no free port and the kernel refuses the bind with
 * `EADDRINUSE` — `ERR_ADDRESS_IN_USE` in Chromium.
 *
 * THE FIX
 *
 * Maximise connection reuse. Node's default `keepAliveTimeout` is 5s; a
 * connection idle for longer is reaped and the next request opens a fresh one,
 * with a fresh ephemeral port. Holding connections open for the length of a
 * run means Chromium reuses the sockets it already has instead of drawing new
 * ports from the pool, which is what keeps the pool from filling.
 *
 * `headersTimeout` must exceed `keepAliveTimeout`, otherwise Node closes a
 * connection while the peer still considers it reusable.
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
