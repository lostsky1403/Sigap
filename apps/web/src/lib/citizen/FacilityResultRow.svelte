<script lang="ts">
	import { RADIUS } from '$lib/design/tokens';
	import { ChevronRight } from 'lucide-svelte';
	import Icon from '$lib/ui/Icon.svelte';
	import { facilityTypeLabel } from '$lib/citizen/facilityType';
	import type { PublicFacility } from '$lib/api/types/api';

	/**
	 * One catalog row.
	 *
	 * The field allowlist is the whole point of this component, so it is worth
	 * being explicit about what is missing and why:
	 *
	 * The public catalog endpoint returns exactly `id`, `name`, `short_code`,
	 * `type`, and `is_active`. Everything else a facility listing usually shows
	 * is therefore absent, not hidden: no address, no distance, no travel
	 * time, no opening hours, no doctor availability, no live queue, no bed
	 * count, no rating, no "open now", no map.
	 *
	 * Those are not omissions to be filled in later from a guess. Several of
	 * them (beds, live queue) exist in the database but are deliberately not
	 * public, and the rest (distance, "open now") are not tracked at all. A
	 * citizen has no way to distinguish an invented value from a real one, so
	 * the row shows the truth or it does not show it.
	 *
	 * `type` is the one classification shown, because a citizen choosing where
	 * to go needs to tell a district clinic from a hospital. It is the real
	 * database enum, translated through the shared label map — never inferred
	 * from the name, so a facility named "RS Foo" that is registered as a
	 * Puskesmas displays as "Puskesmas".
	 *
	 * The UUID is used for the booking link and keyed list rendering, but never
	 * displayed: a raw identifier is noise to a citizen, and it is not a field
	 * anyone needs to read.
	 */
	export let facility: PublicFacility;

	/**
	 * Booking preselection.
	 *
	 * `facility_id` is carried as a query parameter on the canonical booking
	 * route so the appointment form can preselect this facility. Booking itself
	 * is Phase 3B3: this phase only preserves the contract by arriving there
	 * with the facility already chosen, which is the difference between a
	 * citizen picking their faskes twice and picking them once.
	 */
	$: bookingHref = `/appointments/new?facility_id=${encodeURIComponent(facility.id)}`;
	$: typeLabel = facilityTypeLabel(facility.type);
</script>

<div class="sigap-facility-row">
	<div class="sigap-facility-row__text">
		<div class="sigap-facility-row__head">
			<span class="sigap-facility-row__name">{facility.name}</span>
			{#if facility.short_code}
				<span class="sigap-facility-row__code" style:border-radius={RADIUS.control}>
					{facility.short_code}
				</span>
			{/if}
		</div>
		<p class="sigap-facility-row__type">{typeLabel}</p>
	</div>

	<a
		class="sigap-facility-row__action"
		style:border-radius={RADIUS.control}
		href={bookingHref}
		aria-label="Buat janji di {facility.name}"
	>
		<span>Buat janji</span>
		<Icon icon={ChevronRight} size={16} />
	</a>
</div>

<style>
	.sigap-facility-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 12px 16px;
	}

	.sigap-facility-row__text {
		min-width: 0;
	}

	.sigap-facility-row__head {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.sigap-facility-row__name {
		min-width: 0;
		font-size: 15px;
		font-weight: 500;
		color: var(--sigap-foreground);
		/* Long facility names wrap instead of overflowing at 390px. */
		overflow-wrap: anywhere;
	}

	.sigap-facility-row__code {
		flex: none;
		display: inline-flex;
		align-items: center;
		height: 20px;
		padding: 0 6px;
		font-size: 11px;
		font-weight: 500;
		color: var(--sigap-muted);
		background-color: var(--sigap-surface);
		border: 1px solid var(--sigap-border);
	}

	/*
		The classification, on its own line under the identity.

		Below the name rather than beside it: at 390px the name and the short
		code already compete for a single row, and a third badge there pushes
		long facility names into two cramped lines. Muted, because it is
		orientation, not the thing the citizen is scanning for — the booking
		action stays the only high-contrast element in the row.
	*/
	.sigap-facility-row__type {
		margin: 2px 0 0;
		font-size: 12px;
		color: var(--sigap-muted);
	}

	.sigap-facility-row__action {
		flex: none;
		display: inline-flex;
		align-items: center;
		gap: 4px;
		/* Citizen touch floor. */
		min-height: 44px;
		padding: 0 12px;
		font-size: 13px;
		font-weight: 500;
		color: var(--sigap-primary);
		text-decoration: none;
	}

	.sigap-facility-row__action:hover {
		background-color: var(--sigap-canvas);
	}

	.sigap-facility-row__action:focus-visible {
		outline: 2px solid var(--sigap-primary);
		outline-offset: -2px;
	}
</style>
