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
	/**
	 * The code this ticket was produced from, carried so the status CTA can
	 * prefill the next screen. `CheckInResult` does not echo it back, and the
	 * citizen cannot be expected to retype a code the ticket never showed.
	 */
	export let checkinCode: string = '';
	export let onReset: () => void = () => {};
	export let onCopy: (label: string, value: string) => void = () => {};

	let copied = false;

	/**
	 * The status page reads `?code=`. Encoded because this is user input and a
	 * stray `&` or `#` would otherwise truncate the parameter; the fallback
	 * keeps the plain link working if the code is somehow absent.
	 */
	$: statusHref = checkinCode.trim()
		? `/patient/status?code=${encodeURIComponent(checkinCode.trim())}`
		: '/patient/status';
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
			<h2 class="sigap-section-title" id="sigap-ticket-title">Check-in berhasil</h2>
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
			class="sigap-ticket__number sigap-identifier"
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
		<a class="sigap-ticket__cta" style:border-radius={RADIUS.control} href={statusHref}>
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
				<dd class="sigap-identifier" data-testid="ticket-appointment-id">{ticket.appointment_id}</dd>
			</div>
			<div>
				<dt>queue_ticket_id</dt>
				<dd class="sigap-identifier" data-testid="ticket-queue-id">{ticket.queue_ticket_id}</dd>
			</div>
			<div>
				<dt>formatted_number</dt>
				<dd class="sigap-identifier" data-testid="ticket-formatted-number">{ticket.formatted_number}</dd>
			</div>
			<div>
				<dt>status</dt>
				<dd>{ticket.status}</dd>
			</div>
		</dl>
	</details>
</section>

<style>
	/*
		The ticket card frames content; it is not operable, so it takes the
		decorative hairline and the number inside carries the emphasis.
	*/
	.sigap-ticket {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ticket__head {
		display: flex;
		align-items: flex-start;
		gap: var(--sigap-space-3);
	}

	/* A badge: 1px tone border, tone glyph, tinted ground — never filled. */
	.sigap-ticket__icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 40px;
		height: 40px;
		flex: none;
		color: var(--sigap-primary);
		background-color: var(--sigap-primary-tint);
		border: 1px solid var(--sigap-primary);
		border-radius: var(--sigap-radius-control);
	}

	.sigap-ticket__sub {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		The number block frames a value rather than being operable: decorative
		outline, and the approved queue-number tinted ground.
	*/
	.sigap-ticket__number-block {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-4);
		text-align: center;
		background-color: var(--sigap-primary-tint);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ticket__number-label {
		margin: 0 0 var(--sigap-space-1);
		font-size: var(--sigap-type-caption);
		font-weight: 500;
		color: var(--sigap-muted);
	}

	/*
		The queue number: `.sigap-identifier` supplies the monospace stack and
		tabular figures, `--sigap-primary-strong` the readable teal, and the
		display size is what makes it unmissable at the counter.
	*/
	.sigap-ticket__number {
		margin: 0 0 var(--sigap-space-3);
		font-size: 40px;
		font-weight: 700;
		color: var(--sigap-primary-strong);
		user-select: all;
		overflow-wrap: anywhere;
	}

	/* An inner note, not a control: decorative outline. */
	.sigap-ticket__wait {
		margin-top: var(--sigap-space-4);
		padding: var(--sigap-space-3);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ticket__wait-value {
		margin: 0;
		font-size: var(--sigap-type-body);
		font-weight: 500;
		color: var(--sigap-foreground);
	}

	/* The caveat is muted but never hidden: it qualifies the number above it. */
	.sigap-ticket__wait-caveat {
		margin: var(--sigap-space-1) 0 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-ticket__actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--sigap-space-3);
		margin-top: var(--sigap-space-4);
	}

	/* The one filled primary action in this action context. */
	.sigap-ticket__cta {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: var(--sigap-control-citizen);
		padding: 0 var(--sigap-space-4);
		font-size: var(--sigap-type-body);
		font-weight: 500;
		color: var(--sigap-on-primary);
		background-color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-ticket__cta:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: 2px;
	}

	.sigap-ticket__details {
		margin-top: var(--sigap-space-4);
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-ticket__details summary {
		min-height: var(--sigap-control-citizen);
		display: flex;
		align-items: center;
		cursor: pointer;
	}

	/* A `<dl>` panel, not a control: decorative outline. */
	.sigap-ticket__detail-list {
		margin: var(--sigap-space-2) 0 0;
		padding: var(--sigap-space-3);
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-ticket__detail-list dt {
		font-size: var(--sigap-type-micro);
		color: var(--sigap-muted);
	}

	.sigap-ticket__detail-list dd {
		margin: 0 0 var(--sigap-space-2);
		font-size: var(--sigap-type-micro);
		color: var(--sigap-foreground);
		overflow-wrap: anywhere;
	}
</style>
