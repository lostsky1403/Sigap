import { expect, test } from './support/test';
import { PRODUCTION_ORIGIN, ProductionTargetError, assertLocalTarget } from '../playwright.config';

/**
 * The production-target guard (Phase 3B1 section 3).
 *
 * This suite must never need a browser. It asserts that the shared target
 * validator rejects production and every other non-loopback destination, and
 * that the running configuration's baseURL is itself local. If someone points
 * SIGAP_E2E_BASE_URL at production, this fails before any page is opened.
 */

test.describe('production E2E target guard', () => {
	test('rejects the known production origin', () => {
		expect(() => assertLocalTarget(PRODUCTION_ORIGIN)).toThrow(ProductionTargetError);
	});

	test('rejects production with an explicit port or path', () => {
		for (const target of [
			`${PRODUCTION_ORIGIN}/`,
			`${PRODUCTION_ORIGIN}/admin/queues`,
			'sigap.chaerulchalik.web.id',
			'http://sigap.chaerulchalik.web.id',
			'https://sigap.chaerulchalik.web.id:443/admin'
		]) {
			expect(() => assertLocalTarget(target), `${target} must be rejected`).toThrow(ProductionTargetError);
		}
	});

	test('rejects other non-loopback hosts, including bare IPs', () => {
		for (const target of [
			'https://example.com',
			'http://staging.internal:8080',
			'http://10.0.0.5:4173',
			'http://192.168.1.20:3000',
			'http://localhost.evil.test:4173',
			// The forms below all rely on the SAME thing the guard does: the
			// WHATWG URL parser's host normalisation. Each is a way a host can
			// LOOK local without being it, so each pins a class of bypass rather
			// than a single string.
			'https://sigap.chaerulchalik.web.id.', // trailing dot: a distinct host
			'http://localhost.:4173', // trailing dot keeps the dot in the host
			'http://localhost%2Eevil.test:4173', // %2E decodes to a dot
			'http://localhost@evil.test:4173', // userinfo, host is evil.test
			'http://evil.test@sigap.chaerulchalik.web.id', // userinfo, host is prod
			'http://[::ffff:127.0.0.1]:4173', // IPv4-mapped IPv6 is not the allow-list form
			'http://127.0.0.1.evil.test', // local prefix, evil suffix
			'//evil.test', // protocol-relative
			'http://local\nhost.evil.test:4173', // LF stripped by the parser
			'http://localhost\t.evil.test:4173' // TAB stripped by the parser
		]) {
			expect(() => assertLocalTarget(target), `${target} must be rejected`).toThrow(ProductionTargetError);
		}
	});

	test('rejects unsupported schemes and unparseable targets', () => {
		for (const target of ['file:///etc/passwd', 'ftp://localhost/x', 'not a url at all']) {
			expect(() => assertLocalTarget(target), `${target} must be rejected`).toThrow(ProductionTargetError);
		}
	});

	test('accepts loopback hosts only', () => {
		for (const target of [
			'http://localhost:4173',
			'http://127.0.0.1:4173',
			'http://127.0.0.1:3000/admin/queues',
			'localhost:4173',
			// Alternate spellings of loopback. These are accepted because the
			// check and the connection use the same normalised hostname, so each
			// resolves to 127.0.0.1/::1 and stays on the machine. Pinned so a
			// future "tighten the allow-list" change is a deliberate decision
			// rather than a silent break of a legitimate local target.
			'http://127.1:4173',
			'http://2130706433:4173',
			'http://0x7f.0.0.1:4173',
			'http://LOCALHOST:4173',
			'http://[0:0:0:0:0:0:0:1]:4173',
			'http://user:pass@127.0.0.1:4173'
		]) {
			expect(() => assertLocalTarget(target), `${target} must be accepted`).not.toThrow();
		}
	});

	test('the active Playwright baseURL is local', async ({ baseURL }) => {
		expect(baseURL).toBeTruthy();
		expect(() => assertLocalTarget(baseURL as string)).not.toThrow();
		expect(baseURL).not.toContain('chaerulchalik');
	});
});
