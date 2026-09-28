<script lang="ts">
	import { requiresAuth, type ApiError } from '$lib/api/errors';
	import UnauthPanel from '$lib/ui/UnauthPanel.svelte';
	import ForbiddenPanel from '$lib/ui/ForbiddenPanel.svelte';
	import ErrorState from '$lib/ui/ErrorState.svelte';
	import LoadingState from '$lib/ui/LoadingState.svelte';
	import EmptyState from '$lib/ui/EmptyState.svelte';
	import {
		EMPTY_SCOPE_DESCRIPTION,
		EMPTY_SCOPE_TITLE,
		ORDINARY_EMPTY_DESCRIPTION
	} from './readState';

	/**
	 * The six states every admin READ page has to be able to show.
	 *
	 * Centralised because the mistake this prevents is not a typo, it is a
	 * judgement call made six times: which panel a 403 deserves. The admin auth
	 * layer answers 403 for BOTH a missing session and insufficient permission, so
	 * the status alone is ambiguous. Only `hasSession` resolves it, and
	 * `hasSession` is the single session fact the client is ever given.
	 *
	 * Concretely, this component refuses to let a page render a sign-in prompt for
	 * a signed-in operator. That error is not cosmetic: it tells someone their
	 * session expired when it did not, sends them to a login form they do not
	 * need, and — on a clinic floor — reads as "the system thinks I am a patient".
	 *
	 * Note there is deliberately NO admin rate-limit state. The citizen pages have
	 * a specific 429 message because a citizen can act on it. An operator's read
	 * is not rate-limited by the same rules, and inventing an admin 429 screen
	 * would be a state the backend never produces.
	 */
	export let loading: boolean = false;
	export let error: ApiError | null = null;
	export let hasSession: boolean = false;
	export let rowCount: number = 0;
	/** True when the authoritative scoped-facility read returned zero facilities. */
	export let scopeEmpty: boolean = false;
	export let onRetry: (() => void) | undefined = undefined;
	export let emptyTitle: string = 'Belum ada data';
	export let emptyDescription: string = ORDINARY_EMPTY_DESCRIPTION;

	// A 403 with no session is an authentication problem; a 403 with a session is
	// a permission refusal. Nothing else in the client may decide this.
	$: needsAuth = error !== null && requiresAuth(error, hasSession);
	$: isForbidden = error !== null && !needsAuth && error.kind === 'forbidden';
</script>

{#if loading}
	<LoadingState label="Memuat data" />
{:else if needsAuth}
	<UnauthPanel />
{:else if isForbidden}
	<ForbiddenPanel />
{:else if error}
	<ErrorState {error} {hasSession} {onRetry} />
{:else if scopeEmpty}
	<!--
		The class-1 state. Reached only from the scoped-facility read, never from a
		module list happening to be empty: a scope that EXISTS with no rows is a
		healthy outcome and is rendered as such below.
	-->
	<EmptyState title={EMPTY_SCOPE_TITLE} description={EMPTY_SCOPE_DESCRIPTION} />
{:else if rowCount === 0}
	<EmptyState title={emptyTitle} description={emptyDescription} />
{:else}
	<slot />
{/if}
