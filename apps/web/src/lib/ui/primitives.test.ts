import { readdirSync, readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import StatusBadge from './StatusBadge.svelte';
import Skeleton from './Skeleton.svelte';
import LoadingState from './LoadingState.svelte';
import EmptyState from './EmptyState.svelte';

/**
 * Primitive visual + state contract (Phase 3B1 sections 7 and 8).
 *
 * These assertions enforce the frozen design rules as executable constraints
 * rather than review guidance. A gradient or a decorative shadow in a primitive
 * is exactly the kind of drift that looks like a small decision in a diff and
 * erodes the design system one pull request at a time.
 */

const here = dirname(fileURLToPath(import.meta.url));

const REQUIRED_PRIMITIVES = [
	'Alert.svelte',
	'Button.svelte',
	'CodeDisplay.svelte',
	'Dialog.svelte',
	'EmptyState.svelte',
	'ErrorState.svelte',
	'Field.svelte',
	'ForbiddenPanel.svelte',
	'Icon.svelte',
	'IconButton.svelte',
	'Input.svelte',
	'LoadingState.svelte',
	'Select.svelte',
	'Skeleton.svelte',
	'StatusBadge.svelte',
	'Textarea.svelte',
	'Toast.svelte',
	'UnauthPanel.svelte'
];

function primitiveFiles(): string[] {
	return readdirSync(here)
		.filter((f) => f.endsWith('.svelte'))
		.map((f) => join(here, f));
}

describe('shared primitives', () => {
	it('provides every primitive the frozen design requires', () => {
		const present = new Set(readdirSync(here).filter((f) => f.endsWith('.svelte')));
		const missing = REQUIRED_PRIMITIVES.filter((f) => !present.has(f));
		expect(missing).toEqual([]);
	});

	it('uses no gradients, glassmorphism, or blur', () => {
		const offenders: string[] = [];
		for (const file of primitiveFiles()) {
			const source = readFileSync(file, 'utf8');
			// `linear-gradient` also catches a variable named like a gradient.
			if (/linear-gradient|radial-gradient|conic-gradient/i.test(source)) {
				offenders.push(`${file}: gradient`);
			}
			if (/backdrop-filter/i.test(source)) offenders.push(`${file}: backdrop-filter`);
			if (/glass/i.test(source)) offenders.push(`${file}: glassmorphism`);
		}
		expect(offenders).toEqual([]);
	});

	it('uses no decorative box shadows', () => {
		const offenders: string[] = [];
		for (const file of primitiveFiles()) {
			const source = readFileSync(file, 'utf8');
			// Focus rings use `outline`, never `box-shadow`, so any box-shadow at
			// all in a primitive is decoration.
			if (/box-shadow/i.test(source)) offenders.push(file);
		}
		expect(offenders).toEqual([]);
	});

	it('draws no radii as literals, so only the three frozen steps are possible', () => {
		const offenders: string[] = [];
		// A primitive may either reference a token, or use one of the three
		// frozen values literally. `50%` is permitted for exactly one thing: a
		// spinner circle, which is a shape rather than a design radius.
		const allowed = new Set(['0', '6px', '8px', '12px', '50%']);
		for (const file of primitiveFiles()) {
			const source = readFileSync(file, 'utf8');
			for (const match of source.matchAll(/border-radius:\s*([^;]+);/g)) {
				const value = match[1].trim();
				if (value.startsWith('var(--sigap-')) continue;
				if (!allowed.has(value)) offenders.push(`${file}: border-radius ${value}`);
			}
		}
		expect(offenders).toEqual([]);
	});

	it('introduces no colour literals outside the token source', () => {
		const offenders: string[] = [];
		const FROZEN = new Set([
			'#0f766e', '#0b6b63', '#084f49', '#f7f6f3', '#ffffff',
			'#1c1b1a', '#57534e', '#e0ddd8', '#2f7d32', '#b45309',
			'#c4322a', '#1d6bb5'
		]);
		for (const file of primitiveFiles()) {
			const source = readFileSync(file, 'utf8');
			// rgb() is allowed here and only here: the dialog scrim needs an
			// alpha, and CSS variables cannot express one without a new token.
			const withoutScrim = source.replace(/rgb\([^)]*\)/g, '');
			for (const match of withoutScrim.matchAll(/#[0-9a-f]{3,8}\b/gi)) {
				if (!FROZEN.has(match[0].toLowerCase())) {
					offenders.push(`${file}: ${match[0]}`);
				}
			}
		}
		expect(offenders).toEqual([]);
	});

	it('uses no emoji as UI icons', () => {
		const offenders: string[] = [];
		// Ranges covering the pictographic blocks an emoji would occupy.
		const emoji = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}]/u;
		for (const file of primitiveFiles()) {
			const source = readFileSync(file, 'utf8');
			if (emoji.test(source)) offenders.push(file);
		}
		expect(offenders).toEqual([]);
	});
});

describe('StatusBadge', () => {
	it('always shows a text label, never colour alone', () => {
		render(StatusBadge, { props: { label: 'Menunggu', tone: 'warning' } });
		const badge = screen.getByText('Menunggu');
		expect(badge).toBeVisible();
		expect(badge.textContent?.trim()).toBe('Menunggu');
	});

	it('keeps the label legible in every tone', () => {
		for (const tone of ['neutral', 'success', 'warning', 'danger', 'info', 'brand'] as const) {
			const { unmount } = render(StatusBadge, { props: { label: 'Selesai', tone } });
			expect(screen.getByText('Selesai')).toBeVisible();
			unmount();
		}
	});
});

describe('Skeleton', () => {
	it('is hidden from assistive technology', () => {
		const { container } = render(Skeleton, { props: { lines: 3 } });
		// Placeholder rectangles are noise in a screen reader; the surrounding
		// region carries aria-busy instead.
		expect(container.querySelector('[aria-hidden="true"]')).not.toBeNull();
	});

	it('renders the requested number of lines', () => {
		const { container } = render(Skeleton, { props: { lines: 4 } });
		expect(container.querySelectorAll('.sigap-skeleton')).toHaveLength(4);
	});
});

describe('LoadingState', () => {
	it('marks its region busy so "loading" is distinguishable from "empty"', () => {
		render(LoadingState, { props: { label: 'Memuat antrean' } });
		const status = screen.getByRole('status');
		expect(status).toHaveAttribute('aria-busy', 'true');
		expect(status).toHaveTextContent('Memuat antrean');
	});

	it('can report a settled region', () => {
		render(LoadingState, { props: { busy: false } });
		expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'false');
	});
});

describe('EmptyState', () => {
	it('announces politely rather than as an alert', () => {
		// An empty queue is a normal outcome, not a failure, and should not
		// interrupt the way an error does.
		render(EmptyState, { props: { title: 'Belum ada antrean' } });
		expect(screen.getByRole('status')).toHaveTextContent('Belum ada antrean');
	});
});
