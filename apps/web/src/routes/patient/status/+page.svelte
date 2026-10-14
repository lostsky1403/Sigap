<script lang="ts">
	import VisitProgress from '$lib/citizen/VisitProgress.svelte';
	import Field from '$lib/ui/Field.svelte';
	import Input from '$lib/ui/Input.svelte';
	import Button from '$lib/ui/Button.svelte';
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
	 * One input, one lookup. The backend accepts a check-in code or a formatted
	 * queue number in the same `code` parameter, and it tries them in that
	 * order, so a single field covers both without asking the citizen which
	 * kind of code they are holding.
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

<div class="sigap-status">
	<section class="sigap-status__intro" aria-labelledby="sigap-status-title">
		<h1 id="sigap-status-title" class="sigap-page-title">Status Kunjungan</h1>
		<p class="sigap-page-subtitle">
			Masukkan kode check-in atau nomor antrean untuk melihat posisi kunjungan Anda saat ini.
		</p>
	</section>

	<section class="sigap-status__panel" style:border-radius={RADIUS.panel}>
		<form novalidate on:submit|preventDefault={lookup}>
			<Field
				id="sigap-status-code"
				label="Kode check-in atau nomor antrean"
				required
				error={fieldError}
				helper="Kode diberikan saat membuat janji temu atau saat check-in."
			>
				<Input
					id="sigap-status-code"
					bind:value={code}
					describedBy={fieldError ? 'sigap-status-code-error' : ''}
					invalid={Boolean(fieldError)}
					placeholder="Contoh: AB12CD atau A-012"
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
						diterima saat berhasil check-in. Jika Anda sudah check-in, gunakan nomor antrean
						yang tertera di tiket.
					</p>
				</div>
			{:else if state === 'error'}
				<div class="sigap-status__error" role="alert" data-testid="status-error">
					{rateLimitMessage || otherErrorMessage || GENERIC_UNAVAILABLE}
				</div>
			{/if}
		</div>
	</section>
</div>

<style>
	.sigap-status {
		max-width: 640px;
		margin: 0 auto;
		padding: 20px 16px 8px;
	}

	@media (min-width: 768px) {
		.sigap-status {
			padding: 32px 24px 8px;
		}
	}

	.sigap-status__panel {
		margin-top: 16px;
		padding: 16px;
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-status__panel form {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.sigap-status__result:not(:empty) {
		margin-top: 20px;
	}

	.sigap-status__found {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.sigap-status__facility {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.sigap-status__facility-label {
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-status__facility-name {
		font-size: 16px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__when {
		margin: 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-status__found-by {
		margin: 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-status__empty {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 8px;
		padding: 16px;
		background-color: var(--sigap-canvas);
		border: 1px solid var(--sigap-border);
	}

	.sigap-status__empty-icon {
		display: flex;
		color: var(--sigap-muted);
	}

	.sigap-status__empty-title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: var(--sigap-foreground);
	}

	.sigap-status__empty-body {
		margin: 0;
		font-size: 13px;
		color: var(--sigap-muted);
	}

	.sigap-status__error {
		padding: 12px;
		font-size: 13px;
		color: var(--sigap-danger);
		border: 1px solid var(--sigap-danger);
	}
</style>
