<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import type { PublicFacility, PublicServiceUnit } from '$lib/api/types/api';
	import { facilityTypeLabel } from '$lib/citizen/facilityType';
	import { formatAppointmentTime } from '$lib/citizen/citizenFlow';

	/**
	 * The running summary of a booking in progress.
	 *
	 * Shown beside the form on desktop, above it on mobile, and always visible
	 * once both a facility and a service unit are chosen. The reason to keep it
	 * in view is that the form asks for a date, a time, and a name in sequence,
	 * and by the time the citizen reaches the submit button they are three
	 * fields away from the decision they actually care about — which clinic,
	 * on what day. A summary that only appears after submission does not help
	 * them notice a mistake first.
	 *
	 * It shows only what is actually chosen. A row that cannot be resolved yet
	 * renders as an explicit "not chosen" placeholder rather than being
	 * omitted, because a summary that silently grows rows as the form fills in
	 * reads as a bug and makes the layout jump.
	 */
	export let facility: PublicFacility | null = null;
	export let serviceUnit: PublicServiceUnit | null = null;
	export let appointmentTime: string = '';
	export let patientName: string = '';
	export let patientPhone: string = '';

	$: hasAnything =
		Boolean(facility) || Boolean(serviceUnit) || Boolean(appointmentTime) || Boolean(patientName);
</script>

<aside
	class="sigap-booking-summary"
	style:border-radius={RADIUS.panel}
	aria-labelledby="sigap-booking-summary-title"
>
	<h2 class="sigap-booking-summary__title" id="sigap-booking-summary-title">Ringkasan janji temu</h2>

	<!--
		aria-live="polite" on the summary body because every row here changes
		as a direct result of a citizen action. Without it, a screen reader user
		who picks a facility hears nothing confirming the summary caught up.
	-->
	<div class="sigap-booking-summary__body" aria-live="polite">
		<dl class="sigap-booking-summary__list">
			<div class="sigap-booking-summary__row">
				<dt class="sigap-booking-summary__term">Fasilitas</dt>
				<dd class="sigap-booking-summary__value">
					{#if facility}
						<span class="sigap-booking-summary__primary">{facility.name}</span>
						<span class="sigap-booking-summary__secondary">
							{facilityTypeLabel(facility.type)}
							{#if facility.short_code} · {facility.short_code}{/if}
						</span>
					{:else}
						<span class="sigap-booking-summary__empty">Belum dipilih</span>
					{/if}
				</dd>
			</div>

			<div class="sigap-booking-summary__row">
				<dt class="sigap-booking-summary__term">Layanan</dt>
				<dd class="sigap-booking-summary__value">
					{#if serviceUnit}
						<span class="sigap-booking-summary__primary">{serviceUnit.name}</span>
						{#if serviceUnit.code}
							<span class="sigap-booking-summary__secondary">{serviceUnit.code}</span>
						{/if}
					{:else}
						<span class="sigap-booking-summary__empty">Belum dipilih</span>
					{/if}
				</dd>
			</div>

			<div class="sigap-booking-summary__row">
				<dt class="sigap-booking-summary__term">Waktu</dt>
				<dd class="sigap-booking-summary__value">
					{#if appointmentTime}
						<span class="sigap-booking-summary__primary">
							{formatAppointmentTime(appointmentTime) || appointmentTime}
						</span>
					{:else}
						<span class="sigap-booking-summary__empty">Belum dipilih</span>
					{/if}
				</dd>
			</div>

			<div class="sigap-booking-summary__row">
				<dt class="sigap-booking-summary__term">Pasien</dt>
				<dd class="sigap-booking-summary__value">
					{#if patientName || patientPhone}
						<span class="sigap-booking-summary__primary">
							{patientName || '—'}
						</span>
						{#if patientPhone}
							<span class="sigap-booking-summary__secondary">{patientPhone}</span>
						{/if}
					{:else}
						<span class="sigap-booking-summary__empty">Belum diisi</span>
					{/if}
				</dd>
			</div>
		</dl>

		{#if !hasAnything}
			<p class="sigap-booking-summary__hint">
				Ringkasan terisi otomatis saat Anda mengisi formulir.
			</p>
		{/if}
	</div>
</aside>

<style>
	.sigap-booking-summary {
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	.sigap-booking-summary__title {
		margin: 0;
		padding: 12px 16px;
		font-size: 13px;
		font-weight: 600;
		color: var(--sigap-foreground);
		border-bottom: 1px solid var(--sigap-border);
	}

	.sigap-booking-summary__body {
		padding: 16px;
	}

	.sigap-booking-summary__list {
		display: flex;
		flex-direction: column;
		gap: 14px;
		margin: 0;
	}

	.sigap-booking-summary__row {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.sigap-booking-summary__term {
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-booking-summary__value {
		margin: 0;
		min-width: 0;
	}

	.sigap-booking-summary__primary {
		display: block;
		font-size: 14px;
		font-weight: 500;
		color: var(--sigap-foreground);
		overflow-wrap: anywhere;
	}

	.sigap-booking-summary__secondary {
		display: block;
		font-size: 12px;
		color: var(--sigap-muted);
		overflow-wrap: anywhere;
	}

	/*
		"Not chosen" is styled as muted text, not hidden and not an error.
		It is a normal state, and colouring it as a problem would tell the
		citizen they had already done something wrong.
	*/
	.sigap-booking-summary__empty {
		display: block;
		font-size: 14px;
		color: var(--sigap-muted);
	}

	.sigap-booking-summary__hint {
		margin: 14px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}
</style>
