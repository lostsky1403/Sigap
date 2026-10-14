import type { PublicFacilityType } from '$lib/api/types/api';

/**
 * Facility type labels, in one place.
 *
 * The wire carries the database enum verbatim (`puskesmas`, `rumah_sakit`).
 * Those are the right thing to store and transport and the wrong thing to show
 * a citizen, so exactly one function stands between the two — used by both
 * /faskes and the Beranda preview, because two mappings of the same enum is how
 * they drift apart on the next schema change.
 *
 * There is no derivation from `name` or `short_code` anywhere in this file, and
 * there should never be one. "RS Mitra Sehat" tells you nothing verified
 * about the facility's classification; a name is chosen by whoever typed it
 * into the database, whereas `type` is an enum the system enforces. A filter
 * built on a name prefix would silently hide a real hospital called "Klinik
 * Sehat" from every hospital filter, and the citizen could not tell that the
 * app had guessed wrong.
 *
 * The record is typed as a total `Record<PublicFacilityType, string>` rather
 * than a lookup that tolerates gaps. That is the useful part: adding a third
 * enum value to the backend turns every label here into a compile error
 * instead of a blank space on a civic health page.
 */
export const FACILITY_TYPE_LABELS: Record<PublicFacilityType, string> = {
	puskesmas: 'Puskesmas',
	rumah_sakit: 'Rumah Sakit'
};

/**
 * The selectable types, derived from the label record.
 *
 * Deriving instead of restating means the filter list cannot end up with a
 * control that renders an empty label, and cannot fall out of order relative
 * to the labels. The order here is the order the frozen reference uses.
 */
export const PUBLIC_FACILITY_TYPES = Object.keys(FACILITY_TYPE_LABELS) as PublicFacilityType[];

/**
 * Human-readable label for a wire value.
 *
 * An unrecognised value is returned as-is rather than relabelled. Silently
 * calling something "Puskesmas" because it was not recognised as anything else
 * is exactly the fabrication this project forbids — a wrong label reads as a
 * verified fact, whereas the raw value at least looks wrong and is reportable.
 */
export function facilityTypeLabel(type: PublicFacilityType): string {
	return FACILITY_TYPE_LABELS[type] ?? type;
}

/**
 * Runtime check for a value arriving off the wire.
 *
 * TypeScript types describe what the backend promises, not what it sends, and
 * the catalog is fetched JSON. An unrecognised value is therefore a real
 * possibility after a backend deploy lands ahead of a frontend release, and the
 * type filter has to decide what to do with it.
 *
 * The decision is to match nothing: a facility whose type is not recognised is
 * never counted as a Puskesmas or a Rumah Sakit. It still appears under "Semua",
 * which is honest — hiding it entirely would understate the catalog — but it
 * cannot be silently sorted into a category it was never verified as.
 */
export function isPublicFacilityType(value: unknown): value is PublicFacilityType {
	return typeof value === 'string' && value in FACILITY_TYPE_LABELS;
}
