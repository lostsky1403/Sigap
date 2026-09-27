<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import Button from '$lib/ui/Button.svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import { Copy, Check, Ticket } from 'lucide-svelte';
	import { ESTIMATE_CAVEAT, estimatedWaitLabel } from '$lib/citizen/citizenFlow';
	import type { CheckInResult } from '$lib/api/types/api';

	/**
	 * The queue ticket a citizen holds after checking in.
	 *
	 * The number is the payload. Everything on this page exists to get a
	 * citizen from "I have an appointment" to "here is the number I read at the
	 * counter", and the number is only useful if it is (a) unmissable, (b)
	 * copyable, and (c) honest about what it does not tell you.
	 *
	 * On honesty: `estimated_wait_minutes` comes from the queue engine at
	 * registration and is not recomputed as the queue moves. Presenting it as
	 * live would be a claim the system cannot keep, and a citizen told 20
	 * minutes who waits 40 has been lied to by the interface. So the label says
	 * "estimasi" and the caveat is printed next to it rather than hidden
	 * behind a tooltip.
	 */
	export let ticket: CheckInResult;
	export let onReset: () => void = () => {};
	export let onCopy: (label: string, value: string) => void = () => {};

	let copied = false;
</script>

<section
	class="sigap-ticket"
	style:border-radius={RADIUS.panel}
	aria-labelledby="sigap-ticket-title"
>
	<div class="sigap-ticket__head" role="status">
		<span class="sigap-ticket__icon" aria-hidden="true">
			<Icon icon={Ticket} size={20} />
		</span>
		<div>
			<h2 class="sigap-ticket__title" id="sigap-ticket-title">Check-in berhasil</h2>
			<p class="sigap-ticket__sub">Simpan nomor antrean berikut dan tunjukkan di loket.</p>
		</div>
	</div>

	<div class="sigap-ticket__number-block">
		<p class="sigap-ticket__number-label" id="sigap-ticket-number-label">Nomor antrean Anda</p>
		<!--
			The largest text on the page, monospaced and letter-spaced so a
			format like "A-012" cannot be misread as a word. user-select: all
			because a number the citizen cannot copy is a number they have to
			write down by hand at the counter.
		-->
		<p
			class="sigap-ticket__number"
			aria-labelledby="sigap-ticket-number-label"
			data-testid="queue-number"
		>
			{ticket.formatted_number}
		</p>
		<Button
			variant="secondary"
			icon={copied ? Check : Copy}
			onClick={() => {
				copied = true;
				onCopy('Nomor antrean', ticket.formatted_number);
			}}
		>
			{copied ? 'Tersalin' : 'Salin nomor'}
		</Button>
	</div>

	{#if ticket.estimated_wait_minutes != null}
		<!--
			The estimate and its caveat are adjacent, not separated. A caveat the
			citizen has to go looking for is a caveat they will not read.
		-->
		<div class="sigap-ticket__wait">
			<p class="sigap-ticket__wait-value">{estimatedWaitLabel(ticket.estimated_wait_minutes)}</p>
			<p class="sigap-ticket__wait-caveat">{ESTIMATE_CAVEAT}</p>
		</div>
	{/if}

	<div class="sigap-ticket__actions">
		<a
			class="sigap-ticket__cta"
			style:border-radius={RADIUS.control}
			href="/patient/status"
			style:background-color="var(--sigap-primary)"
			style:color="var(--sigap-surface)"
		>
			Cek status kunjungan
		</a>
		<Button variant="ghost" onClick={onReset}>Check-in lain</Button>
	</div>

	<!--
		The raw identifiers. build-verification.test.js asserts this page
		surfaces appointment_id, queue_ticket_id, and formatted_number, and
		all three are genuine response fields. They are ticket and booking
		references, not patient data — no name, no phone number.
	-->
	<details class="sigap-ticket__details">
		<summary>Detail teknis</summary>
		<dl class="sigap-ticket__detail-list">
			<div>
				<dt>appointment_id</dt>
				<dd data-testid="ticket-appointment-id">{ticket.appointment_id}</dd>
			</div>
			<div>
				<dt>queue_ticket_id</dt>
				<dd data-testid="ticket-queue-id">{ticket.queue_ticket_id}</dd>
			</div>
			<div>
				<dt>formatted_number</dt>
				<dd data-testid="ticket-formatted-number">{ticket.formatted_number}</dd>
			</div>
			<div>
				<dt>status</dt>
				<dd>{ticket.status}</dd>
			</div>
		</dl>
	</details>
</section>

<style>
	.sigap-ticket {
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-ticket__head {
		display: flex;
		align-items: flex-start;
		gap: 12px;
	}

	.sigap-ticket__icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		flex: none;
		color: var(--sigap-primary);
		border: 1px solid var(--sigap-primary);
	}

	.sigap-ticket__title {
		margin: 0;
		font-size: 16px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-ticket__sub {
		margin: 2px 0 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-ticket__number-block {
		margin-top: 16px;
		padding: 16px;
		text-align: center;
		background-color: var(--sigap-canvas);
		border: 1px solid var(--sigap-border);
	}

	.sigap-ticket__number-label {
		margin: 0 0 4px;
		font-size: 12px;
		font-weight: 500;
		color: var(--sigap-muted);
	}

	.sigap-ticket__number {
		margin: 0 0 12px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 36px;
		font-weight: 700;
		letter-spacing: 0.12em;
		color: var(--sigap-foreground);
		user-select: all;
		overflow-wrap: anywhere;
	}

	.sigap-ticket__wait {
		margin-top: 16px;
		padding: 12px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-ticket__wait-value {
		margin: 0;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	/* The caveat is muted but never hidden: it qualifies the number above it. */
	.sigap-ticket__wait-caveat {
		margin: 4px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-ticket__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 12px;
		margin-top: 16px;
	}

	.sigap-ticket__cta {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		padding: 0 16px;
		font-size: 14px;
		font-weight: 500;
		text-decoration: none;
	}

	.sigap-ticket__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-ticket__details {
		margin-top: 16px;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-ticket__details summary {
		min-height: 44px;
		display: flex;
		align-items: center;
		cursor: pointer;
	}

	.sigap-ticket__detail-list {
		margin: 8px 0 0;
		padding: 12px;
		background-color: var(--sigap-canvas);
		border: 1px solid var(--sigap-border);
	}

	.sigap-ticket__detail-list dt {
		font-size: 11px;
		color: var(--sigap-muted);
	}

	.sigap-ticket__detail-list dd {
		margin: 0 0 8px;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		font-size: 11px;
		color: var(--sigap-foreground);
		overflow-wrap: anywhere;
	}
</style>
