<script lang="ts">
	import VisitProgress from '$lib/citizen/VisitProgress.svelte';
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Button from '$lib/ui/Button.svelte';
	import PageHeader from '$lib/ui/PageHeader.svelte';
	import { RADIUS } from '$lib/design/tokens';
	import { Search } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import { lookupPatientStatus } from '$lib/api/endpoints/public';
	import { isAbort } from '$lib/api/errors';
	import type { PatientStatus } from '$lib/api/types/api';
	import {
		GENERIC_UNAVAILABLE,
		formatAppointmentTime,
		validateLookupCode
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

	type ResultState = 'idle' | 'loading' | 'found' | 'not-found' | 'error';

	let code = '';
	let state: ResultState = 'idle';
	let status: PatientStatus | null = null;
	/** 429 keeps the server's own copy, which explains the limit and the reset. */
	let rateLimitMessage = '';
	let otherErrorMessage = '';
	let fieldError = '';
	let attempted = false;

	async function lookup() {
		attempted = true;
		fieldError = validateLookupCode(code);
		if (fieldError) return;
		if (state === 'loading') return;

		state = 'loading';
		status = null;
		rateLimitMessage = '';
		otherErrorMessage = '';

		const outcome = await lookupPatientStatus(String(code ?? "").trim());

		if (outcome.ok) {
			status = outcome.data;
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
		rateLimitMessage = '';
		otherErrorMessage = '';
		fieldError = '';
		attempted = false;
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
				helper="Kode diberikan saat membuat janji temu atau saat check-in."
			>
				<Input
					id="sigap-status-code"
					bind:value={code}
					describedBy={fieldError ? 'sigap-status-code-error' : ''}
					invalid={Boolean(fieldError)}
					placeholder="Contoh: AB12CD"
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

					{#if status.appointment_time}
						<p class="sigap-status__when">
							Jadwal: {formatAppointmentTime(status.appointment_time) || status.appointment_time}
						</p>
					{/if}

					<VisitProgress
						checkinStatus={status.checkin_status}
						queueNumber={status.queue_formatted_number ?? ''}
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
				</div>
			{:else if state === 'not-found'}
				<div class="sigap-status__empty" data-testid="status-not-found">
					<span class="sigap-status__empty-icon" aria-hidden="true">
						<Icon icon={Search} size={20} />
					</span>
					<h2 class="sigap-status__empty-title">Kode tidak ditemukan</h2>
					<p class="sigap-status__empty-body">
						Periksa kembali kode Anda. Kode check-in dibuat saat Anda membuat janji temu dan
						ditampilkan di halaman konfirmasi janji temu Anda.
					</p>
				</div>
				{:else if state === 'error'}
					<div class="sigap-status__error" role="alert" data-testid="status-error">
						{rateLimitMessage || otherErrorMessage || GENERIC_UNAVAILABLE}
					</div>
				{/if}
			</div>
		</section>

		<aside class="sigap-status__aside" aria-labelledby="sigap-status-next-title">
			<h2 id="sigap-status-next-title" class="sigap-status__aside-title">
				Apa yang terjadi berikutnya
			</h2>
			<p class="sigap-status__aside-body">
				Tunggu nomor antrean Anda dipanggil. Nomor akan muncul di layar tunggu fasilitas.
				Pastikan Anda berada di ruang tunggu.
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

	.sigap-status__facility {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.sigap-status__facility-label {
		font-size: var(--sigap-type-caption);
		color: var(--sigap-muted);
	}

	.sigap-status__facility-name {
		font-size: var(--sigap-type-subtitle);
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__when {
		margin: 0;
		font-size: var(--sigap-type-label);
		color: var(--sigap-muted);
	}

	.sigap-status__found-by {
		margin: 0;
		font-size: var(--sigap-type-caption);
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

	/* A tone-carrying alert: tone border, tone text, tinted ground. */
	.sigap-status__error {
		padding: var(--sigap-space-3);
		font-size: var(--sigap-type-label);
		color: var(--sigap-danger);
		background-color: var(--sigap-danger-tint);
		border: 1px solid var(--sigap-danger);
	}
</style>
