<script lang="ts" context="module">
	/**
	 * Which form is being rendered.
	 *
	 * Declared in module context rather than the instance script because a
	 * Svelte instance script cannot export a type, and consumers may want to
	 * type a variable that holds one.
	 */
	export type AuthMode = 'login' | 'register';
</script>

<script lang="ts">
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Button from '$lib/ui/Button.svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { AlertTriangle, CheckCircle2, Loader2, ServerCrash } from 'lucide-svelte';

	/**
	 * The citizen auth forms.
	 *
	 * PRESENTATION ONLY. This component renders markup; it never performs
	 * authentication. The `+page.server.ts` actions own every decision about
	 * credentials, and they are frozen: this file must not be where a rule
	 * about password length or a check for a configured Supabase quietly
	 * appears, because then two places would disagree about what a valid
	 * request is.
	 *
	 * The field NAMES are not ours either. `email`, `password`, and `confirm`
	 * are what the server actions read out of the form data, and
	 * auth-actions.test.js posts exactly those keys. They are reproduced here as
	 * literals rather than derived, because a renamed input is a form that
	 * silently submits nothing.
	 */

	export let mode: AuthMode = 'login';

	/**
	 * The action's own outcome.
	 *
	 * There is exactly one input, and it is worth being precise about why.
	 * SvelteKit's `ActionData` is the payload passed to `fail(status, data)` -
	 * the STATUS CODE IS NOT IN IT. A page component only ever sees
	 * `{ message }`, so with the server actions frozen there is no way to
	 * branch on 400 versus 401 versus 503 without changing the actions, which
	 * this phase must not do.
	 *
	 * So the state is inferred from the message, and the inference is explicit
	 * and narrow rather than a guess. Two signals, both unambiguous:
	 *
	 *   - "belum dikonfigurasi" is the 503 path. The action emits it only when
	 *     Supabase is absent, which is a server configuration state, not
	 *     something the person at the keyboard can act on.
	 *   - "konfirmasi" marks the confirmation-mismatch 400, which belongs to
	 *     the field rather than the page.
	 *
	 * Everything else is shown verbatim as a banner, including the 401. That
	 * is the right default: an unexpected wording is better shown than hidden
	 * behind an invented classification, and the server is the only thing that
	 * can be authoritative about which failure occurred.
	 */
	export let message: string = '';
	export let registered: boolean = false;
	export let submitting: boolean = false;

	$: serverMessage = message;

	/**
	 * The outage state, described in the citizen's terms rather than the
	 * operator's. "Autentikasi belum dikonfigurasi." tells someone the
	 * department has not finished setting up the service, which is not useful
	 * to them and is not theirs to fix.
	 */
	$: isUnavailable = /belum dikonfigurasi/i.test(serverMessage);

	/**
	 * Only the confirmation mismatch is attributed to a field. Everything else
	 * is about the pair of credentials or the service as a whole, and hanging
	 * those on one input would point at the wrong control.
	 */
	$: isMismatch = !isUnavailable && /konfirmasi/i.test(serverMessage);
	$: mismatchMessage = isMismatch ? serverMessage : '';

	$: hasBanner = Boolean(serverMessage) && !isMismatch && !isUnavailable;

	function describedByFor(controlId: string, hasError: boolean): string {
		return hasError ? `${controlId}-error` : '';
	}
</script>

{#if isUnavailable}
	<!--
		The whole form is withheld. There is no credential to be wrong about, so
		a disabled copy of the form would be a thing that looks interactive and
		is not.
	-->
	<div
		class="sigap-auth__notice"
		style:border-radius={RADIUS.panel}
		role="alert"
		data-testid="auth-unavailable"
	>
		<span class="sigap-auth__notice-icon" aria-hidden="true">
			<Icon icon={ServerCrash} size={20} />
		</span>
		<div>
			<p class="sigap-auth__notice-title">Masuk gagal karena terjadi gangguan pada layanan.</p>
			<p class="sigap-auth__notice-hint">Silakan coba lagi dalam beberapa saat.</p>
		</div>
	</div>
{:else}
	{#if registered}
		<!--
			role="status", not role="alert". Reaching the login page with
			?registered=1 is the expected end of a successful sign-up, not
			anything wrong, and an assertive announcement would interrupt a
			sighted user who is already reading the form.
		-->
		<div
			class="sigap-auth__notice sigap-auth__notice--success"
			style:border-radius={RADIUS.panel}
			role="status"
			data-testid="auth-registered"
		>
			<span class="sigap-auth__notice-icon" aria-hidden="true">
				<Icon icon={CheckCircle2} size={20} />
			</span>
			<p class="sigap-auth__notice-title">Akun Anda telah dibuat. Silakan masuk.</p>
		</div>
	{/if}

	{#if hasBanner}
		<div
			class="sigap-auth__notice sigap-auth__notice--error"
			style:border-radius={RADIUS.panel}
			role="alert"
			data-testid="auth-error"
		>
			<span class="sigap-auth__notice-icon" aria-hidden="true">
				<Icon icon={AlertTriangle} size={20} />
			</span>
			<p class="sigap-auth__notice-title">{serverMessage}</p>
		</div>
	{/if}

	<form method="POST" class="sigap-auth__form" data-testid="auth-form">
		<Field
			id="sigap-auth-email"
			label="Email"
			required
			helper={mode === 'login' ? 'Alamat email Anda.' : ''}
		>
			<Input
				id="sigap-auth-email"
				name="email"
				type="email"
				autocomplete="email"
				placeholder="nama@email.com"
				required
			/>
		</Field>

		<Field
			id="sigap-auth-password"
			label="Kata sandi"
			required
			helper={mode === 'register' ? 'Minimal 8 karakter.' : ''}
		>
			<Input
				id="sigap-auth-password"
				name="password"
				type="password"
				autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
				required
			/>
		</Field>

		{#if mode === 'register'}
			<Field
				id="sigap-auth-confirm"
				label="Konfirmasi kata sandi"
				required
				helper={mismatchMessage ? '' : 'Ketik ulang kata sandi.'}
				error={mismatchMessage}
			>
				<Input
					id="sigap-auth-confirm"
					name="confirm"
					type="password"
					autocomplete="new-password"
					describedBy={describedByFor('sigap-auth-confirm', Boolean(mismatchMessage))}
					invalid={Boolean(mismatchMessage)}
					required
				/>
			</Field>
		{/if}

		<Button type="submit" fullWidth disabled={submitting} icon={submitting ? Loader2 : undefined}>
			{mode === 'login' ? 'Masuk' : 'Daftar'}
		</Button>
	</form>
{/if}

<style>
	.sigap-auth__form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.sigap-auth__notice {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		margin-bottom: 16px;
		padding: 12px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-auth__notice--error {
		border-color: var(--sigap-danger);
	}

	.sigap-auth__notice--success {
		border-color: var(--sigap-primary);
	}

	.sigap-auth__notice-icon {
		display: flex;
		flex: none;
		/* Sits beside 13px text, so a 20px glyph is already large. */
		color: var(--sigap-muted);
	}

	.sigap-auth__notice--error .sigap-auth__notice-icon {
		color: var(--sigap-danger);
	}

	.sigap-auth__notice--success .sigap-auth__notice-icon {
		color: var(--sigap-primary);
	}

	.sigap-auth__notice-title {
		margin: 0;
		font-size: 14px;
		line-height: 1.45;
		color: var(--sigap-foreground);
	}

	.sigap-auth__notice-hint {
		margin: 4px 0 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}
</style>
