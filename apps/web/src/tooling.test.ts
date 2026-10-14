import { describe, expect, it } from 'vitest';

/**
 * Tooling smoke test. It proves the Vitest + jsdom pipeline is wired before any
 * real suite depends on it, so a broken foundation fails here with an obvious
 * message rather than as a confusing failure inside a component test.
 */
describe('test tooling foundation', () => {
	it('runs in a jsdom environment', () => {
		expect(typeof window).toBe('object');
		expect(typeof document).toBe('object');
	});

	it('loads the jest-dom matchers', () => {
		const el = document.createElement('button');
		el.textContent = 'SIGAP';
		document.body.appendChild(el);
		expect(el).toBeVisible();
		expect(el).toBeInTheDocument();
		el.remove();
	});

	it('resolves the $lib alias', async () => {
		const { CONTROL_HEIGHT } = await import('$lib/design/tokens');
		expect(CONTROL_HEIGHT.citizen).toBe(44);
	});
});
