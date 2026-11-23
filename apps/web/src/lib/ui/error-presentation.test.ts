import { render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it } from 'vitest';
import type { ApiError } from '$lib/api/errors';
import { isPermissionDenied, requiresAuth } from '$lib/api/errors';
import { resetSessionForTesting } from '$lib/stores/session';
import ErrorState from './ErrorState.svelte';

/**
 * Behavioral coverage for the F-1 fix.
 *
 * The source-contract tests in session.test.ts prove the root layout WRITES the
 * session flag into the shared store. These tests prove that the flag actually
 * changes the rendered outcome: a signed-in staff member refused with a 403
 * must see a permission-denied panel, while a signed-out visitor sees the
 * sign-in path. Both share HTTP 403 by design, so the split is driven only by
 * the trusted, server-derived `hasSession` boolean.
 */

const forbidden: ApiError = {
	kind: 'forbidden',
	status: 403,
	message: ''
};

const unauthorized: ApiError = {
	kind: 'unauthorized',
	status: 401,
	message: ''
};

beforeEach(() => {
	resetSessionForTesting();
	document.body.innerHTML = '';
});

describe('403 presentation split (F-1 outcome)', () => {
	it('a signed-out 403 asks the visitor to sign in', () => {
		render(ErrorState, { props: { error: forbidden, hasSession: false } });

		expect(screen.getByText('Silakan masuk kembali')).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Masuk' }).getAttribute('href')).toBe('/auth/login');
		// The permission-denied copy must not leak to a signed-out visitor.
		expect(screen.queryByText('Gagal memuat data')).toBeNull();
	});

	it('a signed-in 403 is a permission refusal, not a sign-in prompt', () => {
		render(ErrorState, { props: { error: forbidden, hasSession: true } });

		expect(screen.getByText('Gagal memuat data')).toBeTruthy();
		expect(screen.queryByText('Silakan masuk kembali')).toBeNull();
		// No sign-in link: the visitor already has a session, so offering one is
		// the exact misleading regression F-1 fixed.
		expect(screen.queryByRole('link', { name: 'Masuk' })).toBeNull();
	});

	it('a 401 always asks for sign-in regardless of the flag', () => {
		for (const hasSession of [false, true]) {
			document.body.innerHTML = '';
			render(ErrorState, { props: { error: unauthorized, hasSession } });
			expect(screen.getByText('Silakan masuk kembali')).toBeTruthy();
			expect(screen.getByRole('link', { name: 'Masuk' })).toBeTruthy();
		}
	});
});

describe('403 classification logic', () => {
	it('requiresAuth is true only for 401 or 403 without a session', () => {
		expect(requiresAuth(unauthorized, false)).toBe(true);
		expect(requiresAuth(unauthorized, true)).toBe(true);
		expect(requiresAuth(forbidden, false)).toBe(true);
		expect(requiresAuth(forbidden, true)).toBe(false);
	});

	it('isPermissionDenied requires both a 403 and a session', () => {
		expect(isPermissionDenied(forbidden, true)).toBe(true);
		expect(isPermissionDenied(forbidden, false)).toBe(false);
		expect(isPermissionDenied(unauthorized, true)).toBe(false);
	});
});
