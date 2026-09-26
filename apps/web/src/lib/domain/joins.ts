/**
 * Resolving foreign-key ids into the human labels a person would use.
 *
 * The single rule, and the reason this module exists: an unresolvable reference
 * renders as "-". Never a fabricated name, never the raw id.
 *
 * A raw UUID in a table cell is worse than a blank: it looks like data, it
 * looks like a bug, and an operator may try to use it. Inventing "Pusat Kota"
 * because the id looks familiar is worse still, because a wrong name on a
 * clinical screen is a safety problem. "-" is honest, and it is what the design
 * intends.
 */

/** What every unresolved reference renders as. */
export const UNRESOLVED = '-';

/** A lookup table keyed by id. */
export type Lookup<T> = ReadonlyMap<string, T> | Readonly<Record<string, T>>;

function entries<T>(table: Lookup<T>): [string, T][] {
	if (table instanceof Map) {
		return [...table.entries()];
	}
	return Object.entries(table as Record<string, T>);
}

function find<T>(table: Lookup<T> | undefined, id: string | null | undefined): T | undefined {
	if (!table || id === null || id === undefined) return undefined;
	if (table instanceof Map) return table.get(id);
	return (table as Record<string, T>)[id];
}

/**
 * Resolves an id to its label, or "-" when it cannot be resolved.
 *
 * Covers every unresolvable case with the same output: a missing table, a null
 * id, an id absent from the table, and an empty or whitespace label. An
 * out-of-scope reference simply is not in the map, so it lands here too — which
 * is exactly right, because revealing that an id exists but is out of scope
 * would leak the very thing the Phase 3B0 authorization work withholds.
 */
export function resolveLabel<T>(
	table: Lookup<T> | undefined,
	id: string | null | undefined,
	label: (item: T) => string | null | undefined
): string {
	if (!table || id === null || id === undefined) return UNRESOLVED;

	const item = find(table, id);
	if (item === undefined) return UNRESOLVED;

	const text = label(item);
	if (typeof text !== 'string') return UNRESOLVED;

	const trimmed = text.trim();
	return trimmed === '' ? UNRESOLVED : trimmed;
}

/** Convenience wrapper for string-keyed label tables. */
export function resolveName(
	table: Lookup<string> | undefined,
	id: string | null | undefined
): string {
	return resolveLabel(table, id, (value) => value);
}

/** Resolves a facility id to its display name. */
export function facilityName(
	table: Lookup<{ name: string }> | undefined,
	id: string | null | undefined
): string {
	return resolveLabel(table, id, (facility) => facility.name);
}

/** Resolves a service unit id to its display name. */
export function serviceUnitName(
	table: Lookup<{ name: string }> | undefined,
	id: string | null | undefined
): string {
	return resolveLabel(table, id, (unit) => unit.name);
}

/** Resolves a practitioner id to their display name. */
export function practitionerName(
	table: Lookup<{ full_name?: string | null; name?: string | null }> | undefined,
	id: string | null | undefined
): string {
	// Either field may be absent depending on the projection, so both are tried
	// before giving up. A practitioner with neither renders as "-".
	return resolveLabel(table, id, (person) => person.full_name ?? person.name);
}

/**
 * Builds a lookup table from a list of rows.
 *
 * Rows without a usable id are skipped rather than mapped to "", which would
 * make every unresolvable reference collapse onto one bogus entry.
 */
export function indexBy<T>(
	rows: readonly T[],
	id: (row: T) => string | null | undefined
): Map<string, T> {
	const table = new Map<string, T>();
	for (const row of rows) {
		const key = id(row);
		if (typeof key === 'string' && key !== '') table.set(key, row);
	}
	return table;
}

/**
 * Resolves a batch of ids, preserving input order and de-duplicating lookups.
 *
 * A table view with forty rows should not perform forty independent lookups
 * against a table the caller already has in memory.
 */
export function resolveMany<T>(
	table: Lookup<T> | undefined,
	ids: readonly (string | null | undefined)[],
	label: (item: T) => string | null | undefined
): string[] {
	if (!table) return ids.map(() => UNRESOLVED);

	const cache = new Map<string, string>();
	const material = table instanceof Map ? table : new Map(entries(table));

	return ids.map((id) => {
		if (id === null || id === undefined) return UNRESOLVED;
		const cached = cache.get(id);
		if (cached !== undefined) return cached;

		const resolved = resolveLabel(material, id, label);
		cache.set(id, resolved);
		return resolved;
	});
}

/** True when a label resolved to nothing, i.e. it is the placeholder. */
export function isUnresolved(label: string): boolean {
	return label === UNRESOLVED;
}
