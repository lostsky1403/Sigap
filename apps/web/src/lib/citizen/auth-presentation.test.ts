import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';

/**
 * `$app/stores` is only populated by a running SvelteKit router, which does
 * not exist in jsdom. The page under test reads `$page.url.searchParams` to
 * recognise `?registered=1`, so the store is mocked here rather than the page
 * being changed to avoid the dependency: the flag genuinely belongs to the
 * page, because the frozen register action redirects to it.
 */
const pageUrl = vi.hoisted(() => ({ current: new URL('http://localhost/auth/login') }));
vi.mock('$app/stores', () => ({
	page: readable({ url: pageUrl.current })
}));

const AuthForms = (await import('./AuthForms.svelte')).default;
const Login = (await import('../../routes/auth/login/+page.svelte')).default;
const Register = (await import('../../routes/auth/register/+page.svelte')).default;
const Logout = (await import('../../routes/auth/logout/+page.svelte')).default;
const { hasSession, resetSessionForTesting } = await import('$lib/stores/session');

/**
 * Auth presentation.
 *
 * The server actions are FROZEN, and this suite is mostly about proving the
 * presentation layer did not quietly become a second place where auth rules
 * live. The failure mode being guarded against is specific: someone "improving"
 * a form adds a client-side password-length check, or a redirect, or a
 * role-based branch, and now the client and the action can disagree about what
 * a valid request is - with the client being the copy nobody reads.
 *
 * So the load-bearing assertions here are negative ones. The components must
 * not know about roles, the field names must stay exactly what the frozen
 * actions read, and the message shown must be the server's own wording.
 */

const webRoot = resolve(process.cwd());

/**
 * The executable script of a component: markup stripped, comments stripped.
 *
 * Both strips are load-bearing and both exist because of a false positive
 * rather than by taste.
 *
 * Comments go first because these files document the rules they are being
 * tested against, so a comment saying "must not reference role" would
 * otherwise trip the assertion that says exactly that.
 *
 * The markup goes second, and for a subtler reason. `role="alert"` and
 * `role="status"` are REQUIRED for an error and a confirmation to be
 * announced at all. Scanning markup for the word "role" would flag the
 * accessibility attributes themselves, and the fix that suggests itself -
 * removing them - would make the page worse. An authorization role would live
 * in the script, as state, a branch, or a property access; an ARIA role lives
 * in the markup, as a literal. Separating the two makes the rule precise
 * instead of merely strict.
 */
function scriptOf(relativePath: string): string {
	const source = readFileSync(resolve(webRoot, relativePath), 'utf8');
	const blocks = [...source.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]);
	const code = blocks.join('\n')
		.replace(/\/\*[\s\S]*?\*\//g, ' ')
		.replace(/(^|[^:])\/\/.*$/gm, '$1');
	return code;
}

/** The whole file, for plain TypeScript with no markup to separate. */
function source(relativePath: string): string {
	return readFileSync(resolve(webRoot, relativePath), 'utf8');
}

beforeEach(() => {
	document.body.innerHTML = '';
	pageUrl.current = new URL('http://localhost/auth/login');
	resetSessionForTesting();
});

describe('the frozen field names', () => {
	/*
	 * These three strings are the contract between the markup and the frozen
	 * actions. `+page.server.ts` reads them out of the form data with
	 * `data.get('email')`, `data.get('password')`, and `data.get('confirm')`,
	 * and auth-actions.test.js posts exactly those keys. A renamed input is not
	 * a cosmetic change: it is a form that submits nothing and always fails
	 * with "Email dan kata sandi wajib diisi."
	 */
	it('login posts email and password under their exact names', () => {
		render(Login);
		const form = document.querySelector('form[method="POST"]');
		expect(form).toBeTruthy();

		const email = document.querySelector('input[name="email"]') as HTMLInputElement;
		const password = document.querySelector('input[name="password"]') as HTMLInputElement;
		expect(email, 'email input must be named email').toBeTruthy();
		expect(password, 'password input must be named password').toBeTruthy();
		expect(email.type).toBe('email');
		expect(password.type).toBe('password');
	});

	it('register posts email, password, and confirm under their exact names', () => {
		render(Register);
		for (const name of ['email', 'password', 'confirm']) {
			expect(
				document.querySelector(`input[name="${name}"]`),
				`${name} input must be named ${name}`
			).toBeTruthy();
		}
	});

	it('uses the password autocomplete that matches the intent', () => {
		// A login form offering "new-password" makes password managers offer to
		// generate a new credential instead of filling the existing one.
		render(Login);
		expect(
			(document.querySelector('input[name="password"]') as HTMLInputElement).autocomplete
		).toBe('current-password');

		document.body.innerHTML = '';
		render(Register);
		for (const name of ['password', 'confirm']) {
			expect(
				(document.querySelector(`input[name="${name}"]`) as HTMLInputElement).autocomplete
			).toBe('new-password');
		}
	});
});

describe('AuthForms states', () => {
	it('has no status prop, because ActionData does not carry one', () => {
		/*
		 * Documents a SvelteKit constraint rather than a preference. The page
		 * receives the payload of `fail(status, data)` and NOT the status, so
		 * there is no number to branch on. An earlier version of this
		 * component took a `status` prop and every caller passed
		 * `form.status`, which does not exist - svelte-check rejected it, and
		 * the type error was the only thing that caught it, because a
		 * `status={undefined}` prop would have rendered silently and made
		 * every state fall through to the default.
		 *
		 * So the classification is by message, and this test fails if someone
		 * reintroduces the prop and starts believing they have a status.
		 */
		expect(scriptOf('src/lib/citizen/AuthForms.svelte')).not.toMatch(/export let status/);
		expect(scriptOf('src/routes/auth/login/+page.svelte')).not.toMatch(/form\?\.status/);
		expect(scriptOf('src/routes/auth/register/+page.svelte')).not.toMatch(/form\?\.status/);
	});

	it('renders a 401 with the server wording verbatim', async () => {
		// The action's exact text for a rejected credential.
		render(AuthForms, { mode: 'login', message: 'Email atau kata sandi salah.' });
		await waitFor(() => expect(screen.getByTestId('auth-error')).toBeTruthy());

		// Verbatim, because the server is being deliberately vague on purpose:
		// saying which of the two was wrong would let anyone probe for
		// accounts. Rewording it into something friendlier destroys the only
		// safe property it has.
		expect(screen.getByTestId('auth-error').textContent).toContain(
			'Email atau kata sandi salah.'
		);
		// And the form is still there, because the citizen can simply retype.
		expect(screen.getByTestId('auth-form')).toBeTruthy();
	});

	it('renders the 503 configuration failure as an outage, not as an operator string', async () => {
		render(AuthForms, {
			mode: 'login',
			message: 'Autentikasi belum dikonfigurasi.'
		});
		await waitFor(() => expect(screen.getByTestId('auth-unavailable')).toBeTruthy());

		// Nothing the citizen typed is wrong and nothing they can fix, so a
		// disabled form would be an interactive-looking thing that is not one.
		expect(screen.queryByTestId('auth-form')).toBeNull();

		// "Autentikasi belum dikonfigurasi." is an operator-facing string about
		// server configuration, and showing it to a citizen tells them the
		// department has not finished setting up the service.
		const text = screen.getByTestId('auth-unavailable').textContent ?? '';
		expect(text).toContain('Masuk gagal karena terjadi gangguan pada layanan.');
		expect(text).toContain('Silakan coba lagi dalam beberapa saat.');
		expect(text).not.toContain('Autentikasi belum dikonfigurasi.');
		expect(text).not.toContain('belum dikonfigurasi');
	});

	it('attributes a confirmation mismatch to the confirm field, not a banner', async () => {
		render(AuthForms, {
			mode: 'register',
			message: 'Konfirmasi kata sandi tidak sama.'
		});
		await waitFor(() =>
			expect(document.querySelector('input[name="confirm"]')?.getAttribute('aria-invalid')).toBe(
				'true'
			)
		);
		// No banner. The message is on the field, so there is nothing to repeat
		// at the top of the form.
		expect(screen.queryByTestId('auth-error')).toBeNull();

		// Pointed at the field that is actually wrong. A banner at the top of a
		// form tells someone something is wrong but not which input, and on a
		// three-field form that is most of the work left to do.
		const confirm = document.querySelector('input[name="confirm"]') as HTMLInputElement;
		expect(confirm.getAttribute('aria-invalid')).toBe('true');
		const describedBy = confirm.getAttribute('aria-describedby') ?? '';
		expect(describedBy).toBeTruthy();
		// The referenced element must exist, or the announcement is a dangling id.
		const target = document.getElementById(describedBy);
		expect(target?.textContent).toContain('Konfirmasi kata sandi tidak sama.');

		// The other two fields are untouched by this failure.
		expect(
			(document.querySelector('input[name="password"]') as HTMLInputElement).getAttribute(
				'aria-invalid'
			)
		).toBe('false');
	});

	it('confirms a successful registration without styling it as an error', async () => {
		render(AuthForms, { mode: 'login', message: '', registered: true });
		await waitFor(() => expect(screen.getByTestId('auth-registered')).toBeTruthy());

		// The expected end of a sign-up, not a problem. An assertive
		// announcement would interrupt a user who is already reading the form.
		expect(screen.getByTestId('auth-registered').getAttribute('role')).toBe('status');
		expect(screen.getByTestId('auth-registered').textContent).toContain(
			'Akun Anda telah dibuat. Silakan masuk.'
		);
		expect(screen.queryByTestId('auth-error')).toBeNull();
		expect(screen.queryByTestId('auth-unavailable')).toBeNull();
	});

	it('announces an error assertively and a confirmation politely', async () => {
		render(AuthForms, { mode: 'login', message: 'Email atau kata sandi salah.' });
		await waitFor(() => expect(screen.getByTestId('auth-error').getAttribute('role')).toBe('alert'));

		document.body.innerHTML = '';
		render(AuthForms, { mode: 'login', registered: true });
		await waitFor(() => expect(screen.getByTestId('auth-registered').getAttribute('role')).toBe('status'));
	});

	it('shows no banner when the action succeeded and redirected', async () => {
		// A successful action redirects, so the page never renders with an
		// absent failure. Rendering "nothing went wrong" anyway would be noise.
		render(AuthForms, { mode: 'login', message: '' });
		await waitFor(() => expect(screen.getByTestId('auth-form')).toBeTruthy());
		expect(screen.queryByTestId('auth-error')).toBeNull();
		expect(screen.queryByTestId('auth-registered')).toBeNull();
	});

	it('shows an unrecognised message verbatim rather than hiding it', async () => {
		/*
		 * The default has to be "show it". A wording nobody anticipated is
		 * better displayed than dropped, and quietly swallowing it would leave
		 * someone staring at a form that refuses to submit with no explanation.
		 */
		render(AuthForms, { mode: 'register', message: 'Email ini sudah terdaftar. Silakan masuk.' });
		await waitFor(() => expect(screen.getByTestId('auth-error')).toBeTruthy());
		expect(screen.getByTestId('auth-error').textContent).toContain(
			'Email ini sudah terdaftar. Silakan masuk.'
		);
	});
});

describe('structure and accessibility', () => {
	it('has exactly one h1 on each auth page', async () => {
		// Rendered one at a time rather than as an array: Login and Register
		// take a `form` prop and Logout takes none, so a single array of the
		// three has a union element type that `render` will not accept.
		for (const [Page, name] of [
			[Login, 'login'],
			[Register, 'register'],
			[Logout, 'logout']
		] as const) {
			document.body.innerHTML = '';
			render(Page as never);
			await waitFor(() => expect(document.querySelector('h1')).toBeTruthy());
			expect(
				document.querySelectorAll('h1'),
				`${name} must have exactly one h1`
			).toHaveLength(1);
		}
	});

	it('labels every control in the login form', async () => {
		render(Login);
		await waitFor(() => expect(document.querySelector('input[name="email"]')).toBeTruthy());
		expect(screen.getByLabelText(/Email/)).toBeTruthy();
		expect(screen.getByLabelText(/Kata sandi/)).toBeTruthy();
	});

	it('gives the register form its own hint for each of the three fields', async () => {
		render(Register);
		await waitFor(() => expect(document.querySelector('input[name="confirm"]')).toBeTruthy());
		expect(screen.getByText('Minimal 8 karakter.')).toBeTruthy();
		expect(screen.getByText('Ketik ulang kata sandi.')).toBeTruthy();
	});

	it('points every aria-describedby at an element that exists', async () => {
		// Same reason as the h1 loop: the two pages have different prop types.
		for (const [Page, name] of [
			[Login, 'login'],
			[Register, 'register']
		] as const) {
			document.body.innerHTML = '';
			render(Page as never);
			await waitFor(() => expect(document.querySelector('form')).toBeTruthy());

			for (const control of Array.from(document.querySelectorAll('[aria-describedby]'))) {
				const describedBy = control.getAttribute('aria-describedby') ?? '';
				expect(describedBy, `${name}: aria-describedby must be set`).toBeTruthy();
				for (const id of describedBy.split(/\s+/)) {
					expect(
						document.getElementById(id),
						`${name}: aria-describedby points at a missing element: ${id}`
					).toBeTruthy();
				}
			}
		}
	});
});

describe('presentation only: the client is not an authorization authority', () => {
	/*
	 * The security property. The server resolves role, permissions, facility
	 * scope, and super_admin on every request; a client that could infer any of
	 * them would be an authority a user can edit. Hiding a control is not access
	 * control, and the server has to refuse regardless.
	 *
	 * These assertions read the SOURCE, not the rendered output, because a
	 * forbidden import or a dead branch would be tree-shaken out of the DOM and
	 * a green render would prove nothing.
	 */
	const FILES = [
		'src/lib/citizen/AuthForms.svelte',
		'src/routes/auth/login/+page.svelte',
		'src/routes/auth/register/+page.svelte',
		'src/routes/auth/logout/+page.svelte'
	];

	it.each([
		['role', /\brole\b/i],
		['permission', /\bpermission/i],
		['facility scope', /facility[_ ]?scope|facilityGrant|facility_grant/i],
		['super_admin', /super[_]?admin/i],
		['bearer token', /bearer|jwt/i],
		['admin eligibility', /isAdmin|canAccessAdmin|adminEligible/i]
	])('never references %s', (_label, pattern) => {
		for (const file of FILES) {
			expect(
				scriptOf(file),
				`${file} must not reference ${_label}`
			).not.toMatch(pattern);
		}
	});

	it('learns only hasSession from the session store, and nothing else', () => {
		// The one thing the client is allowed to know. It is enough to choose
		// between "please sign in" and "you may not see this" for a 403, and
		// nothing more is needed for that.
		expect(typeof hasSession()).toBe('boolean');

		const store = source('src/lib/stores/session.ts');
		const interfaceBody = store.slice(
			store.indexOf('export interface SessionState'),
			store.indexOf('}', store.indexOf('export interface SessionState'))
		);
		// Exactly one field. An extra key here would reach every consumer.
		const fields = [...interfaceBody.matchAll(/^\s*(\w+)\s*[?:;]/gm)].map((m) => m[1]);
		expect(fields).toEqual(['hasSession']);
	});

	it('posts to the route action rather than calling an API directly', () => {
		// The forms are SvelteKit form actions, not fetch calls. A fetch to
		// Supabase from the client would move credential handling to the
		// browser and bypass the frozen action entirely.
		for (const file of [
			'src/lib/citizen/AuthForms.svelte',
			'src/routes/auth/login/+page.svelte',
			'src/routes/auth/register/+page.svelte',
			'src/routes/auth/logout/+page.svelte'
		]) {
			const code = scriptOf(file);
			expect(code, `${file} must not fetch`).not.toMatch(/\bfetch\s*\(/);
			expect(code, `${file} must not import supabase`).not.toMatch(/supabase/i);
			expect(code, `${file} must not import apiFetch`).not.toMatch(/apiFetch/);
		}
	});
});

describe('logout', () => {
	it('offers a POST form, because a GET cannot run the action', async () => {
		render(Logout);
		await waitFor(() => expect(document.querySelector('form[method="POST"]')).toBeTruthy());

		// The old page claimed "Sedang keluar..." on load. This route's load
		// function does nothing and the action only runs on POST, so loading
		// the page signed nobody out. The page now offers the action instead of
		// describing work it was not doing.
		expect(document.body.textContent).not.toContain('Sedang keluar');
	});

	it('still names the submit control Keluar', async () => {
		// navigation.test.ts asserts a logout form whose button says Keluar.
		// That is the account menu's form, but the wording is the same action,
		// so it stays.
		render(Logout);
		await waitFor(() => expect(screen.getByRole('button', { name: /Keluar/ })).toBeTruthy());
	});

	it('offers a way back without signing out', async () => {
		render(Logout);
		await waitFor(() => expect(screen.getByRole('link', { name: /Batal/ })).toBeTruthy());
		expect(screen.getByRole('link', { name: /Batal/ }).getAttribute('href')).toBe('/');
	});
});
