import { describe, expect, it } from 'vitest';
import { indexBy, facilityName, serviceUnitName, UNRESOLVED } from '$lib/domain/joins';
import {
	EMPTY_SCOPE_DESCRIPTION,
	EMPTY_SCOPE_TITLE,
	ORDINARY_EMPTY_DESCRIPTION,
	failedState,
	formatUpdatedAt,
	initialLoad,
	isEmptyScope,
	isFirstLoad,
	isStaleData,
	loadedState
} from './readState';
import type { AdminFacility, AdminQueueTicket } from '$lib/api/types/api';
import type { ApiError } from '$lib/api/errors';

/**
 * T-3B4-02 / T-3B4-03 support: the empty-scope contract and the join rule.
 *
 * Both of these are the kind of correctness that is invisible until it is
 * exploited. An unresolved join rendered as a raw UUID puts a machine
 * identifier in front of an operator; a scope-empty state rendered as an
 * ordinary empty list tells someone their access was revoked when it was not.
 */

function facility(id: string, name: string): AdminFacility {
	return {
		id,
		name,
		type: 'puskesmas',
		address: '',
		kecamatan: '',
		kabupaten_kota: '',
		provinsi: '',
		phone: '',
		total_beds: 0,
		available_beds: 0,
		is_active: true,
		short_code: 'TST'
	};
}

describe('the join rule: resolve or "-", never a raw id', () => {
	const facilities = [facility('f1', 'RSUD Kota Sehat'), facility('f2', 'Puskesmas Sukajaya')];
	const byFacility = indexBy(facilities, (f) => f.id);
	const byUnit = indexBy([{ id: 'u1', name: 'Poli Umum' }], (u) => u.id);

	it('resolves a known reference to its human label', () => {
		expect(facilityName(byFacility, 'f1')).toBe('RSUD Kota Sehat');
		expect(serviceUnitName(byUnit, 'u1')).toBe('Poli Umum');
	});

	it('renders a NULL reference as "-", not as an empty cell or a uuid', () => {
		// A null service_unit_id is a real state: a schedule with no unit assigned.
		// An empty cell would read as "not loaded yet", which is a different and
		// wrong claim.
		expect(facilityName(byFacility, null)).toBe(UNRESOLVED);
		expect(serviceUnitName(byUnit, undefined)).toBe(UNRESOLVED);
	});

	it('renders an unresolved reference as "-", never the raw id', () => {
		// The id IS present in the row. Falling back to it would put
		// "0f3a...-..." in a cell meant for a person, and an operator may try to
		// use it. More seriously, an OUT-OF-SCOPE id is absent from the map only
		// because the server withheld it, so echoing it would leak exactly what
		// the 3B0 authorization work refuses to disclose.
		const outOfScope = '0f3a1b2c-0000-4000-8000-00000000dead';
		expect(facilityName(byFacility, outOfScope)).toBe(UNRESOLVED);
		expect(facilityName(byFacility, outOfScope)).not.toContain(outOfScope);
		expect(serviceUnitName(byUnit, '0f3a1b2c-0000-4000-8000-00000000dead')).toBe(UNRESOLVED);
	});

	it('renders "-" when the lookup table itself is missing', () => {
		// A page that forgot to load its facilities must not render raw ids; it
		// must render the honest placeholder until the join data arrives.
		expect(facilityName(undefined, 'f1')).toBe(UNRESOLVED);
		expect(serviceUnitName(undefined, 'u1')).toBe(UNRESOLVED);
	});

	it('never produces a uuid from any of the unresolved paths', () => {
		const orphanId = '0f3a1b2c-0000-4000-8000-00000000dead';
		for (const resolved of [
			facilityName(byFacility, orphanId),
			facilityName(undefined, orphanId),
			facilityName(byFacility, null),
			facilityName(byFacility, '')
		]) {
			expect(resolved).toBe(UNRESOLVED);
			expect(resolved).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}/i);
		}
	});
});

describe('the empty-scope contract: class-1 versus an ordinary empty list', () => {
	it('treats zero authorized facilities as scope-empty', () => {
		expect(isEmptyScope([])).toBe(true);
	});

	it('does NOT treat an existing scope with no rows as scope-empty', () => {
		// The distinction the spec exists to protect: a scope that exists but whose
		// queues are empty is a healthy, ordinary state. Claiming "you have no
		// facilities" there would tell an operator their access was revoked moments
		// after they watched a patient arrive.
		expect(isEmptyScope([facility('f1', 'RSUD Kota Seah')])).toBe(false);
	});

	it('does not claim scope-empty before the facility read has answered', () => {
		// Flashing "no facilities" at an operator who has them is worse than
		// showing nothing, because it is a false statement about their access.
		expect(isEmptyScope(undefined)).toBe(false);
	});

	it('uses a distinct message for the two states', () => {
		expect(EMPTY_SCOPE_TITLE).toBe('Anda belum memiliki fasilitas dalam cakupan');
		expect(EMPTY_SCOPE_DESCRIPTION).toMatch(/ administrator/i);
		// The ordinary empty message must not reuse the scope-empty wording, or the
		// two states become indistinguishable to the reader.
		expect(ORDINARY_EMPTY_DESCRIPTION).not.toBe(EMPTY_SCOPE_DESCRIPTION);
		expect(ORDINARY_EMPTY_DESCRIPTION).not.toMatch(/cakupan akses\.$/);
	});
});

describe('load state: a failed refresh never blanks the board', () => {
	function ticket(id: string): AdminQueueTicket {
		return {
			id,
			facility_id: 'f1',
			queue_number: 1,
			formatted_number: 'RSK-0001',
			status: 'waiting',
			registered_at: '2026-09-28T10:00:00Z'
		};
	}

	it('starts as a first load', () => {
		const state = initialLoad<AdminQueueTicket>();
		expect(state.loading).toBe(true);
		expect(isFirstLoad(state)).toBe(true);
		expect(state.rows).toEqual([]);
	});

	it('holds the last successful rows and flags them stale when a refresh fails', () => {
		const loaded = loadedState([ticket('a'), ticket('b')]);
		const error: ApiError = { kind: 'server', status: 500, message: '' };
		// The whole point: a 30-second poll failing must not destroy the one thing
		// the operator is looking at.
		expect(isStaleData(loaded, error)).toBe(true);
		expect(loaded.rows).toHaveLength(2);
	});

	it('copies rows rather than aliasing them', () => {
		const source = [ticket('a')];
		const state = loadedState(source);
		source.push(ticket('b'));
		// Aliasing would let a filtered view quietly rewrite the rows a polling
		// board is holding onto.
		expect(state.rows).toHaveLength(1);
	});

	it('treats an abort as no state change at all', () => {
		// An abort is the app cancelling its own request: navigating away, or a
		// poll superseded by a manual refresh. Surfacing it would show a failure to
		// someone who did nothing wrong and is no longer looking.
		const abort: ApiError = { kind: 'aborted', status: 0, message: '' };
		const state = failedState<AdminQueueTicket>(abort);
		expect(state.loading).toBe(true);
		expect(state.failed).toBe(false);
	});

	it('uses a failed FIRST load to replace the page', () => {
		const error: ApiError = { kind: 'forbidden', status: 403, message: '' };
		const state = failedState<AdminQueueTicket>(error);
		expect(state.failed).toBe(true);
		expect(state.error).toBe(error);
		expect(isFirstLoad(state)).toBe(false);
	});

	it('does not show a skeleton over rows that are already visible', () => {
		// Replacing visible rows with a skeleton on every poll would make the board
		// unreadable for a second every thirty seconds.
		const loaded = loadedState([ticket('a')]);
		expect(isFirstLoad(loaded)).toBe(false);
	});
});

describe('the freshness label', () => {
	it('reads "Diperbarui pukul HH.MM" and states nothing before data arrives', () => {
		expect(formatUpdatedAt(new Date(2026, 8, 28, 14, 5))).toBe('Diperbarui pukul 14.05');
		// Empty rather than a placeholder: a page with no data yet has no update
		// time, and printing "00.00" would assert a freshness it cannot support.
		expect(formatUpdatedAt(null)).toBe('');
	});
});
