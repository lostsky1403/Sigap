/**
 * Normalizes a facility's `is_active` at the wire boundary.
 *
 * WHY THIS EXISTS. The backend is genuinely inconsistent here, and the shape of
 * the inconsistency is a trap:
 *
 *   - A facility inside a list or detail response carries a real BOOLEAN:
 *     `"is_active": true`.
 *   - The DEACTIVATE response builds a `map[string]string`, so the same field
 *     arrives as the STRING `"false"`.
 *
 * The obvious fix, `Boolean(value)`, is wrong, and dangerously so: in JavaScript
 * every non-empty string is truthy, so `Boolean("false") === true`. A facility
 * that was just deactivated would render as active — on a screen whose whole job
 * is telling an operator which facilities are still taking patients. That is the
 * kind of bug that survives review because it looks like a one-line tidy-up.
 *
 * So the string is compared, never coerced, and the comparison is case-insensitive
 * because the backend does not guarantee which casing it emits. Anything
 * unrecognised resolves to active, which fails safe: showing a facility as active
 * when its state is genuinely unknown is recoverable, while hiding a live facility
 * sends a patient to a closed door.
 *
 * `facilitiesLoaded` and the deactivation response are Phase 3B5's concern; this
 * helper exists now because the LIST response is read in this phase and the next
 * one will need the same rule in a second place.
 */
export function isFacilityActive(value: boolean | string | null | undefined): boolean {
	if (typeof value === 'boolean') return value;
	if (typeof value === 'string') {
		const normalized = value.trim().toLowerCase();
		// Only an explicit falsy token means inactive. Anything else — including
		// null, undefined, and an unrecognised value — is treated as active.
		if (normalized === 'false' || normalized === '0' || normalized === 'no') return false;
		return true;
	}
	return true;
}

/** Label for the state. Always paired with the colour, never used alone. */
export function facilityStateLabel(active: boolean): string {
	return active ? 'Aktif' : 'Nonaktif';
}
