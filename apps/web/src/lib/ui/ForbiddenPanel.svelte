<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';

	/**
	 * "You are signed in, but this is not yours."
	 *
	 * The counterpart to UnauthPanel, and the reason both exist: the admin auth
	 * layer returns 403 for a missing session and for insufficient permission
	 * alike, so the status code alone cannot tell the two apart. This panel is
	 * shown only when a session is known to exist, and it says exactly that.
	 *
	 * It does not name the missing permission, the user's role, or their facility
	 * scope. The client is not an authorization authority, and telling a user
	 * precisely what they lack is information the server deliberately withholds.
	 */
	export let title: string = 'Akses ditolak';
	export let description: string =
		'akun Anda tidak memiliki izin untuk membuka halaman ini. Hubungi administrator bila Anda merasa ini keliru.';
</script>

<div class="sigap-forbidden" style:border-radius={RADIUS.panel} role="alert">
	<!--
		The badge is text, not a lock glyph alone. A shield icon next to a
		sentence is decoration; the sentence is the message.
	-->
	<p class="sigap-forbidden__code">403</p>
	<p class="sigap-forbidden__title">{title}</p>
	<p class="sigap-forbidden__description">{description}</p>

	{#if $$slots.actions}
		<div class="sigap-forbidden__actions"><slot name="actions" /></div>
	{/if}
</div>

<style>
	.sigap-forbidden {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
		padding: 32px 24px;
		text-align: center;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
		border-left: 3px solid var(--sigap-warning);
	}

	.sigap-forbidden__code {
		margin: 0;
		font-size: 12px;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--sigap-muted);
	}

	.sigap-forbidden__title {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-forbidden__description {
		margin: 0;
		max-width: 52ch;
		font-size: 14px;
		line-height: 1.6;
		color: var(--sigap-muted);
	}

	.sigap-forbidden__actions {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		justify-content: center;
		margin-top: 8px;
	}
</style>
