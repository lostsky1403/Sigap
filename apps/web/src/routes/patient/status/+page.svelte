<script lang="ts">
	import { onMount, tick } from 'svelte';
	import VisitProgress from '$lib/citizen/VisitProgress.svelte';
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Button from '$lib/ui/Button.svelte';
	import Dialog from '$lib/ui/Dialog.svelte';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { Search } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import { lookupPatientStatus } from '$lib/api/endpoints/public';
	import { cancelAppointmentByCode } from '$lib/api/endpoints/citizen';
	import { isAbort } from '$lib/api/errors';
	import type { PatientStatus } from '$lib/api/types/api';
	import {
		GENERIC_UNAVAILABLE,
		normalizeLookupCode,
		validateLookupCode,
		visitGuidance,
		visitProgressFrom
	} from '$lib/citizen/citizenFlow';

	/**
	 * /patient/status — where is my visit right now?
	 *
	 * One input, one lookup, by check-in code. The endpoint resolves check-in
	 * codes ONLY: the formatted-number strategy was removed server-side to close
	 * the enumeration vector, so a queue number is refused with a 404. The page
	 * therefore never offers one — copy that invited a ticket number would send
	 * the citizen to a lookup that cannot succeed.
	 *
	 * What this page deliberately does NOT have: a timeline, timestamps, a
	 * wait history, or anything that updates itself. `PatientStatusLookup`
	 * returns one status, not a history, and presenting more would mean
	 * inventing it. The result is explicitly the CURRENT state.
	 */

	type ResultState =
		| 'idle'
		| 'loading'
		| 'found'
		| 'not-found'
		| 'error'
		| 'cancelled';

	let code = '';
	/**
	 * The code that RESOLVED the currently displayed visit, in the backend's
	 * canonical shape. The input field stays editable after a successful lookup,
	 * so `code` can diverge from the code that produced the result on screen —
	 * and cancelling by the live field would cancel a DIFFERENT appointment than
	 * the one the citizen is looking at (possibly a family member's). Every
	 * mutation is therefore keyed to this value, never to the field.
	 */
	let resolvedCode = '';
	let state: ResultState = 'idle';
	let status: PatientStatus | null = null;
	/** 429 keeps the server's own copy, which explains the limit and the reset. */
	let rateLimitMessage = '';
	let otherErrorMessage = '';
	let fieldError = '';
	let attempted = false;

	/**
	 * Cancel-a-scheduled-appointment state, orthogonal to the lookup result.
	 *
	 * `cancelOpen` is the dialog, `cancelling` the in-flight guard, and
	 * `cancelError` the server's 409 refusal — held apart from `otherErrorMessage`
	 * so a conflict (appointment already checked in / completed) never collapses
	 * into the generic unavailable banner, and from `state === 'not-found'` so a
	 * code that vanished after booking elsewhere is not shown as an error.
	 */
	let cancelOpen = false;
	let cancelling = false;
	let cancelError = '';

	/**
	 * The "what happens next" aside is now state-aware, unlike the old static
	 * line that told an in_service citizen to keep waiting to be called. The
	 * guidance needs the raw `queue_status` in ADDITION to `visitProgressFrom`'s
	 * collapsed step: the progress indicator shows one "Dilayani" for both
	 * `called` and `in_service`, but the next action differs (go to the counter
	 * now vs. stay and be served). Empty unless a visit resolved, so the aside
	 * keeps its original explainer before a successful lookup.
	 */
	$: guidance = (() => {
		if (state === 'found' && status) {
			return visitGuidance(
				visitProgressFrom(status.checkin_status, status.queue_status),
				status.queue_status,
				status.queue_formatted_number ?? ''
			);
		}
		// The aside is "what happens next": every terminal result still has a
		// next step, and that step is NOT "wait to be called". A cancelled or
		// not-found citizen reading "tunggu nomor antrean Anda dipanggil" is
		// exactly the contradiction this page previously shipped for in_service.
		switch (state) {
			case 'cancelled':
				return 'Janji temu Anda telah dibatalkan. Anda dapat membuat janji temu baru.';
			case 'not-found':
				return 'Periksa kembali kode check-in Anda — terdiri dari enam karakter huruf dan angka.';
			case 'error':
				return 'Layanan sedang bermasalah. Coba lagi sebentar lagi.';
			default:
				return ''; // idle / loading
		}
	})();

	/**
	 * Prefills the field when another screen hands the code over in `?code=`.
	 *
	 * The check-in ticket links here, and the citizen cannot be asked to retype
	 * a code that screen never displayed. Reading it from the URL is safe in
	 * itself — this is a same-origin navigation, not a third-party referrer.
	 *
	 * What is NOT safe is leaving it there. A check-in code is a bearer token:
	 * anyone holding it can read that visit's status. In the address bar it
	 * persists in history, is copied by "share this page", and travels in the
	 * Referer header of any outbound request. So the value is moved into the
	 * field and immediately removed from the URL, keeping every other
	 * parameter intact.
	 */
	onMount(() => {
		const params = new URLSearchParams(window.location.search);
		const handedOver = params.get('code');
		if (!handedOver) return;

		code = handedOver;
		params.delete('code');
		const rest = params.toString();
		window.history.replaceState(
			null,
			'',
			`${window.location.pathname}${rest ? `?${rest}` : ''}${window.location.hash}`
		);
	});

	async function lookup() {
		attempted = true;
		fieldError = validateLookupCode(code);
		if (fieldError) {
			// Focus the field the error belongs to. Leaving focus on the submit
			// button means a keyboard user is told "Kode wajib diisi." with no
			// indication of where, and must Shift+Tab back to find it.
			await tick();
			document.getElementById('sigap-status-code')?.focus();
			return;
		}
		if (state === 'loading') return;

		state = 'loading';
		status = null;
		rateLimitMessage = '';
		otherErrorMessage = '';

		const outcome = await lookupPatientStatus(normalizeLookupCode(code));

		if (outcome.ok) {
			status = outcome.data;
			resolvedCode = normalizeLookupCode(code);
			state = 'found';
			return;
		}
		// An abort is a navigation, not a result. Leaving the state as it was
		// avoids showing an error to someone who simply moved on.
		if (isAbort(outcome.error)) {
			state = 'idle';
			return;
		}

		if (outcome.error.kind === 'not_found') {
			// "Not found" is its own state with its own guidance, distinct from
			// an error: nothing is broken, the code is simply not recognised.
			state = 'not-found';
			return;
		}
		if (outcome.error.kind === 'rate_limited') {
			// Verbatim. The backend's copy says how long to wait.
			rateLimitMessage = outcome.error.message || 'Terlalu banyak permintaan. Coba lagi nanti.';
			state = 'error';
			return;
		}
		if (outcome.error.kind === 'bad_request') {
			// The server rejected the code's shape, which is a field problem.
			fieldError = outcome.error.message || 'Kode tidak valid.';
			state = 'idle';
			return;
		}
		otherErrorMessage = GENERIC_UNAVAILABLE;
		state = 'error';
	}

	function reset() {
		state = 'idle';
		status = null;
		resolvedCode = '';
		rateLimitMessage = '';
		otherErrorMessage = '';
		fieldError = '';
		attempted = false;
		cancelError = '';
	}

	/** Opens the cancel confirm dialog, clearing any earlier inline refusal. */
	function openCancel() {
		cancelError = '';
		cancelOpen = true;
	}

	/** Closes the cancel confirm dialog. Guarded so a close during the request is a no-op. */
	function closeCancel() {
		if (cancelling) return;
		cancelOpen = false;
	}

	/**
	 * Confirms the cancellation.
	 *
	 * The four outcomes stay distinct on purpose:
	 *
	 * - 200: the pass is replaced by the cancelled state, and the dialog closes.
	 * - 409 `conflict`: the server refused (checked in / completed). The server's
	 *   own message is shown inline, and this must never read as not-found.
	 * - 404 `not_found`: the code no longer resolves (cancelled elsewhere or
	 *   expired) — that IS the not-found state, not an error.
	 * - 429 / network / 5xx / unknown: generic unavailable. The generic banner
	 *   must never masquerade as "Kode tidak ditemukan".
	 */
	async function confirmCancel() {
		if (cancelling) return;
		cancelling = true;
		cancelError = '';

		// Cancel the appointment the result was FOR. `resolvedCode` was captured
		// at lookup time; the field may have been edited since then. Cancelling
		// `code` would target whatever is in the box now, not what is on screen.
		const outcome = await cancelAppointmentByCode(resolvedCode);

		if (outcome.ok) {
			state = 'cancelled';
			cancelOpen = false;
			cancelling = false;
			return;
		}

		// An abort is a navigation away; the pending guard is all that matters.
		if (isAbort(outcome.error)) {
			cancelling = false;
			return;
		}

		if (outcome.error.kind === 'not_found') {
			state = 'not-found';
			status = null;
			cancelOpen = false;
			cancelling = false;
			return;
		}

		if (outcome.error.kind === 'conflict') {
			cancelError = outcome.error.message || 'Janji temu sudah tidak dapat dibatalkan.';
			cancelOpen = false;
			cancelling = false;
			return;
		}

		// 429 / network / 5xx / unknown: unavailable, never "not found".
		otherErrorMessage = GENERIC_UNAVAILABLE;
		state = 'error';
		cancelOpen = false;
		cancelling = false;
	}
</script>

<svelte:head>
	<title>Status Kunjungan — Sigap</title>
</svelte:head>

<div class="sigap-page sigap-page--tight sigap-status">
	<PageHeader
		rule="compact"
		title="Status kunjungan"
		description="Masukkan kode check-in untuk melihat status kunjungan Anda saat ini."
	/>

	<!--
		Two columns from 1024px up, as the Pen draws them: the lookup on the
		left, a static explainer on the right. The explainer is not decoration —
		the most common question this page raises is "what happens after I press
		the button", and answering it here costs nothing and saves a support
		call. It is the approved copy, verbatim, and it makes no claim about
		this citizen's position: the position is only ever what the server
		returned into the panel beside it.
	-->
	<div class="sigap-status__columns">
		<section class="sigap-status__panel" style:border-radius={RADIUS.panel}>
			<form novalidate on:submit|preventDefault={lookup}>
			<Field
				id="sigap-status-code"
				label="Kode check-in"
				required
				error={fieldError}
				helper="Enam karakter, dari halaman konfirmasi janji temu Anda. Tidak peka huruf besar-kecil."
			>
				<Input
					id="sigap-status-code"
					bind:value={code}
					describedBy={fieldError ? 'sigap-status-code-error' : ''}
					invalid={Boolean(fieldError)}
					placeholder="Contoh: AB24CD"
					autocomplete="off"
					disabled={state === 'loading'}
				/>
			</Field>

			<Button type="submit" disabled={state === 'loading'} fullWidth>
				{state === 'loading' ? 'Mencari...' : 'Cek Status'}
			</Button>
		</form>

		<!--
			aria-live on the result region so a lookup that resolves after a
			pause is announced. Without it, a screen reader user presses the
			button and hears nothing until they navigate away.
		-->
		<div class="sigap-status__result" aria-live="polite">
			{#if state === 'found' && status}
				<div class="sigap-status__found">
					<div class="sigap-status__facility">
						<span class="sigap-status__facility-label">Fasilitas</span>
						<span class="sigap-status__facility-name">{status.facility_name}</span>
					</div>

					<!--
						The service is public catalog data (e.g. "Poli Umum"),
						never patient data, and omitted entirely when the backend
						did not send it. It reuses the facility label/value styling
						so the receipt reads as one list, not two visual systems.
					-->
{#if status.service_unit_name}
					<div class="sigap-status__service" data-testid="status-service-unit">
						<span class="sigap-status__service-label">Layanan</span>
						<span class="sigap-status__service-name">{status.service_unit_name}</span>
					</div>
				{/if}

					<VisitProgress
						checkinStatus={status.checkin_status}
						queueNumber={status.queue_formatted_number ?? ''}
						queueStatus={status.queue_status}
					/>

					<!--
						How the record was found, in plain words. Useful when
						someone is not sure which code they typed, and it is
						already in the response rather than invented here.
					-->
					<p class="sigap-status__found-by">
						Ditemukan lewat {status.found_by === 'checkin_code'
							? 'kode check-in'
							: 'nomor antrean'}.
					</p>

					<Button variant="ghost" onClick={reset}>Cek kode lain</Button>

					<!--
						Cancelling is only offered for a scheduled appointment: it is
						the one state a citizen still controls. A checked-in / queued /
						completed visit cannot be undone from here, so the affordance
						does not appear and the citizen is not offered an action the
						server would always refuse.
					-->
					{#if status.appointment_status === 'scheduled'}
						<div class="sigap-status__cancel" data-testid="cancel-appointment">
							<Button variant="secondary" onClick={openCancel}>Batalkan janji</Button>
						</div>
					{/if}

					{#if cancelError}
						<p class="sigap-status__cancel-error" role="alert" data-testid="cancel-error">
							{cancelError}
						</p>
					{/if}
				</div>
			{:else if state === 'cancelled' && status}
				<div class="sigap-status__cancelled" data-testid="status-cancelled">
					<div class="sigap-status__facility">
						<span class="sigap-status__facility-label">Fasilitas</span>
						<span class="sigap-status__facility-name">{status.facility_name}</span>
					</div>

					<h2 class="sigap-status__cancelled-title">Janji temu dibatalkan</h2>
					<p class="sigap-status__cancelled-body">
						Jadwal Anda telah dibatalkan. Anda dapat
						<a class="sigap-inline-link" href="/appointments/new">membuat janji temu baru</a>
						kapan saja.
					</p>

					<Button variant="ghost" onClick={reset}>Cek kode lain</Button>
				</div>
			{:else if state === 'not-found'}
				<div class="sigap-status__empty" data-testid="status-not-found">
					<span class="sigap-status__empty-icon" aria-hidden="true">
						<Icon icon={Search} size={20} />
					</span>
					<h2 class="sigap-status__empty-title">Kode tidak ditemukan</h2>
					<p class="sigap-status__empty-body">
						Kode check-in terdiri dari enam karakter dan tertulis di halaman konfirmasi
						janji temu Anda. Perhatikan huruf <strong>O</strong> dan angka <strong>0</strong>,
						serta huruf <strong>I</strong> dan angka <strong>1</strong> — keduanya mudah
						tertukar saat dibaca dari layar.
					</p>
					<!--
						A recovery path, not just a restatement of the failure. Before
						this, a citizen whose code did not resolve was told only that
						the code was wrong and had nowhere to go next: the page named
						no alternative and offered no way to reach the facility.
					-->
					<ul class="sigap-status__recovery">
						<li>
							Belum punya janji temu?
							<a class="sigap-inline-link" href="/appointments/new">Buat janji temu</a>
						</li>
						<li>
							Sudah punya janji temu tapi kodenya hilang? Hubungi fasilitas tempat Anda
							membuat janji temu — petugas dapat mencocokkan data Anda di sana.
						</li>
					</ul>
					<p class="sigap-status__empty-note">
						Hanya kode check-in yang dapat dicari di halaman ini.
					</p>
				</div>
				{:else if state === 'error'}
					<div class="sigap-status__error" role="alert" data-testid="status-error">
						{rateLimitMessage || otherErrorMessage || GENERIC_UNAVAILABLE}
					</div>
				{/if}
			</div>

			<!--
				The cancel confirmation. Rendered conditionally on `open` by
				Dialog itself; `showModal()` puts it in the top layer, so its
				placement in the DOM is irrelevant to layout. The footer's two
				buttons are a secondary "Kembali" (closes, no-op while pending)
				and a danger primary "Ya, batalkan" that carries the pending label.
			-->
			<Dialog
				open={cancelOpen}
				title="Batalkan janji temu?"
				description={status
					? `Anda akan membatalkan janji temu di ${status.facility_name}${status.service_unit_name ? ` untuk ${status.service_unit_name}` : ''}. Jadwal Anda dibebaskan untuk warga lain, dan tindakan ini tidak dapat dibatalkan.`
					: 'Jadwal Anda akan dibebaskan untuk warga lain, dan tindakan ini tidak dapat dibatalkan.'}
				onClose={closeCancel}
				closeLabel="Tutup dialog"
			>
				<svelte:fragment slot="footer">
					<Button variant="secondary" onClick={closeCancel} disabled={cancelling}>
						Kembali
					</Button>
					<Button variant="danger" onClick={confirmCancel} disabled={cancelling}>
						{cancelling ? 'Membatalkan…' : 'Ya, batalkan'}
					</Button>
				</svelte:fragment>
			</Dialog>
		</section>

		<aside class="sigap-status__aside" aria-labelledby="sigap-status-next-title">
			<h2 id="sigap-status-next-title" class="sigap-status__aside-title">
				Apa yang terjadi berikutnya
			</h2>
			<p class="sigap-status__aside-body">
				{#if guidance}
					{guidance}
				{:else}
					Masukkan kode check-in Anda untuk melihat status kunjungan.
				{/if}
			</p>
		</aside>
	</div>
</div>

<style>
	/*
		Layout, width and gutter come from `.sigap-page--tight`: the Pen draws
		this page and Faskes on the same tighter frame, 40px from the masthead on
		desktop and 20px on a phone.

		Two columns from 1024px up, which is the correction that matters at
		1440. The Pen runs the lookup at x 48 -> 959 against the left gutter and
		puts a static explainer beside it, not a centred 640px strip with two
		wide empty margins. The split is 2fr/1fr because that is the measured
		proportion of the frame's two columns.
	*/
	.sigap-status__columns {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-5);
	}

	@media (min-width: 1024px) {
		.sigap-status__columns {
			flex-direction: row;
			align-items: flex-start;
		}

		.sigap-status__panel {
			flex: 2 1 0;
			min-width: 0;
		}

		.sigap-status__aside {
			flex: 1 1 0;
			min-width: 0;
		}
	}

	/*
		The lookup panel frames a field and a button; it is not itself operable.
		Decorative hairline, per the border split — the input and the submit
		button inside keep their own `--sigap-border` boundaries.
	*/
	.sigap-status__panel {
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border-subtle);
	}

	/*
		The explainer. Deliberately NOT a panel: the Pen draws it as plain text
		beside the card, and giving it a border would make two competing boxes
		where the design has one. Its heading is a section title, one step down
		from the page title, so it reads as an aside rather than a second page.
	*/
	.sigap-status__aside-title {
		margin: 0;
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__aside-body {
		margin: var(--sigap-space-2) 0 0;
		font-size: var(--sigap-type-body);
		line-height: 1.6;
		color: var(--sigap-muted);
	}

	.sigap-status__panel form {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-4);
	}

	.sigap-status__result:not(:empty) {
		margin-top: var(--sigap-space-5);
	}

	.sigap-status__found {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-4);
	}

	.sigap-status__facility,
	.sigap-status__service {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.sigap-status__facility-label,
	.sigap-status__service-label {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-status__facility-name,
	.sigap-status__service-name {
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__found-by {
		margin: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	/*
		The cancel trigger is a secondary button whose label reads in the danger
		token ("destructive text"): low emphasis so it does not compete with the
		receipt, but the words signal that this action cannot be undone. Without
		the override, the secondary variant's text is the neutral foreground and
		its hover swings to the brand primary — which reads as navigation, not as
		an irreversible action.
	*/
	:global(.sigap-status__cancel .sigap-button--secondary),
	:global(.sigap-status__cancel .sigap-button--secondary:hover:not(:disabled)) {
		color: var(--sigap-danger);
	}

	/*
		Inline cancel refusal (409). Danger text only, no tinted panel: it sits
		inside the receipt beside the trigger, so it must read as its own message
		rather than the full-page unavailable banner, and never as "not found".
	*/
	.sigap-status__cancel-error {
		margin: 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-danger);
	}

	/*
		The post-cancel state. The pass is gone, so the receipt is replaced by a
		short confirmation: the same facility block, the outcome as a heading,
		and a concrete path back to booking.
	*/
	.sigap-status__cancelled {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-4);
	}

	.sigap-status__cancelled-title {
		margin: 0;
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__cancelled-body {
		margin: 0;
		font-size: var(--sigap-type-label);
		line-height: 1.6;
		color: var(--sigap-muted);
	}

	/*
		The not-found state is a panel, not an alert: nothing is broken, the code
		is simply unknown. Muted ground and a decorative outline keep it from
		reading as a failure.
	*/
	.sigap-status__empty {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--sigap-space-2);
		padding: var(--sigap-space-4);
		background-color: var(--sigap-surface-sunken);
		border: 1px solid var(--sigap-border-subtle);
	}

	.sigap-status__empty-icon {
		display: flex;
		color: var(--sigap-muted);
	}

	.sigap-status__empty-title {
		margin: 0;
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__empty-body {
		margin: 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	/*
		The recovery list. A numbered/ordered treatment would imply a sequence the
		citizen must follow; these are alternatives, so they are an unordered list
		with a visible marker and a real gap between the items.
	*/
	.sigap-status__recovery {
		display: flex;
		flex-direction: column;
		gap: var(--sigap-space-2);
		margin: 0;
		padding-left: 1.1em;
		font-size: var(--sigap-type-label);
		color: var(--sigap-foreground);
	}

	.sigap-status__recovery li::marker {
		color: var(--sigap-muted);
	}

	.sigap-status__empty-note {
		margin: 0;
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	/* A tone-carrying alert: tone border, tone text, tinted ground. */
	.sigap-status__error {
		padding: var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		color: var(--sigap-danger);
		background-color: var(--sigap-danger-tint);
		border: 1px solid var(--sigap-danger);
	}
</style>
