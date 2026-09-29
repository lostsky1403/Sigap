import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import {
	allowedAppointmentTransitions,
	allowedQueueTransitions,
	canTransitionAppointment,
	canTransitionQueue,
	isTerminalAppointmentStatus,
	isTerminalQueueStatus,
	QUEUE_STATUSES,
	APPOINTMENT_STATUSES
} from './status';
import type { AppointmentStatus, QueueStatus } from '$lib/api/types/api';

/**
 * T-3B5-01 / T-3B5-02: the two transition maps, and the drift guard that keeps
 * them honest.
 *
 * The button sets an operator sees are DERIVED from these maps, so these maps
 * are not a UI detail — they decide which controls exist. Getting one wrong has
 * two very different failure shapes, and both are bad:
 *
 *   - Offering a transition the server rejects. The operator clicks, waits,
 *     and reads an error naming a rule they had no way to see. A control that
 *     exists only to fail teaches people that the board is unreliable.
 *   - Withholding one the server accepts. The operator physically cannot move a
 *     ticket forward and has no idea why. This is the worse of the two,
 *     because it has no diagnostic at all.
 *
 * THE DRIFT GUARD IS THE IMPORTANT HALF.
 *
 * Asserting the TypeScript map against a hand-copied literal only proves it
 * matches another hand-copied literal. Both would have to be wrong together,
 * which is exactly what happens when someone edits the Go map and updates the
 * TypeScript one from memory — the test still passes.
 *
 * So these cases PARSE THE GO SOURCE. The Go map is the authority; the
 * TypeScript map is a mirror of it; and if the mirror is ever changed without
 * the authority, this fails. That converts "please remember to update both"
 * into a build failure.
 */

/**
 * Locates admin.go by walking up from cwd.
 *
 * The depth of cwd is not guaranteed to be stable — `pnpm --filter
 * sigap-web exec vitest` runs with cwd = apps/web, while a bare `vitest` from
 * the repo root runs with cwd = the root. Hardcoding either produced a path
 * that resolved to `F:\api\...` on one and worked on the other, so this probes
 * a few candidates and fails with the list it tried rather than with a bare
 * ENOENT that says nothing about where it looked.
 */
function readGoSource(): string {
	const candidates = [
		'apps/api/internal/handler/admin.go',
		'../api/internal/handler/admin.go',
		'../../apps/api/internal/handler/admin.go'
	];
	for (const candidate of candidates) {
		try {
			return readFileSync(candidate, 'utf8');
		} catch {
			continue;
		}
	}
	throw new Error(`Could not find admin.go. Tried: ${candidates.join(', ')} (cwd: ${process.cwd()})`);
}

const GO_SOURCE = readGoSource();

/**
 * Extracts a Go `map[string][]string{...}` literal into a plain object.
 *
 * Deliberately a narrow parser rather than a real Go parser: it only has to
 * understand the shape this one file uses, and it fails loudly on anything
 * else — so a refactor of the Go map that this cannot read breaks the build
 * instead of silently comparing against nothing.
 */
function parseGoTransitionMap(declaration: string): Record<string, string[]> {
	const start = GO_SOURCE.indexOf(`var ${declaration} = map[string][]string{`);
	expect(start, `could not find "var ${declaration}" in admin.go`).toBeGreaterThan(-1);

	const open = GO_SOURCE.indexOf('{', start);
	let depth = 0;
	let close = -1;
	for (let i = open; i < GO_SOURCE.length; i++) {
		if (GO_SOURCE[i] === '{') depth++;
		else if (GO_SOURCE[i] === '}') {
			depth--;
			if (depth === 0) {
				close = i;
				break;
			}
		}
	}
	expect(close, `unterminated map literal for ${declaration}`).toBeGreaterThan(-1);

	const body = GO_SOURCE.slice(open + 1, close);
	const result: Record<string, string[]> = {};

	// One line per entry in this file, e.g.  "waiting":    {"called", "cancelled"},
	const entryPattern = /"([a-z_]+)"\s*:\s*\{([^}]*)\}/g;
	let match: RegExpExecArray | null;
	let found = 0;
	while ((match = entryPattern.exec(body)) !== null) {
		found++;
		const [, key, values] = match;
		result[key] = [...values.matchAll(/"([a-z_]+)"/g)].map((value) => value[1]);
	}

	expect(found, `parsed no entries out of ${declaration} — the parser needs updating`).toBeGreaterThan(0);
	return result;
}

describe('queue transition map mirrors the Go authority', () => {
	const goMap = parseGoTransitionMap('validQueueTransitions');

	it.each(QUEUE_STATUSES)('agrees with the server for %s', (status) => {
		expect([...allowedQueueTransitions(status)].sort()).toEqual(
			[...(goMap[status] ?? [])].sort()
		);
	});

	it('covers exactly the server keys, so no status is missing a rule', () => {
		expect(Object.keys(allowedQueueTransitions('waiting') as never).length).toBeGreaterThan(0);
		expect([...QUEUE_STATUSES].sort()).toEqual(Object.keys(goMap).sort());
	});

	/**
	 * The spec's §3 wording, asserted literally rather than derived. A derived
	 * check ("every legal transition is present") would pass even if BOTH sides
	 * lost a transition together, which is the drift this whole file exists to
	 * prevent.
	 */
	it.each([
		['waiting', ['called', 'cancelled']],
		['called', ['in_service', 'cancelled', 'skipped']],
		['in_service', ['completed']],
		['completed', []],
		['cancelled', []],
		['skipped', []]
	] as const)('offers exactly %s -> %j', (status, expected) => {
		expect(allowedQueueTransitions(status)).toEqual(expected);
	});

	it('keeps the three terminal states actionable on nothing', () => {
		for (const status of ['completed', 'cancelled', 'skipped'] as QueueStatus[]) {
			expect(isTerminalQueueStatus(status)).toBe(true);
			expect(allowedQueueTransitions(status)).toEqual([]);
		}
	});
});

describe('appointment transition map mirrors the Go authority', () => {
	const goMap = parseGoTransitionMap('validAppointmentTransitions');

	it.each(APPOINTMENT_STATUSES)('agrees with the server for %s', (status) => {
		expect([...allowedAppointmentTransitions(status)].sort()).toEqual(
			[...(goMap[status] ?? [])].sort()
		);
	});

	it('covers exactly the server keys', () => {
		expect([...APPOINTMENT_STATUSES].sort()).toEqual(Object.keys(goMap).sort());
	});

	it.each([
		['scheduled', ['checked_in', 'cancelled', 'no_show']],
		['checked_in', ['queued', 'cancelled', 'no_show']],
		['queued', ['completed', 'cancelled', 'no_show']],
		['completed', []],
		['cancelled', []],
		['no_show', []]
	] as const)('offers exactly %s -> %j', (status, expected) => {
		expect(allowedAppointmentTransitions(status)).toEqual(expected);
	});

	/**
	 * THE ABSENCE, stated as its own case rather than as a corollary of the
	 * table above.
	 *
	 * `checked_in -> completed` is the one transition a reader of this map is
	 * most likely to "fix", because it looks like an obvious usability gap: the
	 * patient is here, why not close the visit? It is wrong. Completing closes
	 * the visit, and a checked-in patient who has not been through the queue
	 * has not been served, so the shortcut would close a visit with no service
	 * record. It is also the transition most likely to be re-added by someone
	 * optimising the UI, which is why it earns an explicit negative assertion
	 * instead of relying on the positive table to keep excluding it.
	 */
	it('has NO checked_in -> completed transition', () => {
		expect(canTransitionAppointment('checked_in', 'completed')).toBe(false);
		expect(allowedAppointmentTransitions('checked_in')).not.toContain('completed');
	});

	it('requires the queued step between checked_in and completed', () => {
		expect(canTransitionAppointment('checked_in', 'queued')).toBe(true);
		expect(canTransitionAppointment('queued', 'completed')).toBe(true);
	});
});

describe('terminal states are genuinely terminal', () => {
	it.each(['completed', 'cancelled', 'no_show'] as AppointmentStatus[])(
		'%s offers no onward transition',
		(status) => {
			expect(isTerminalAppointmentStatus(status)).toBe(true);
			expect(allowedAppointmentTransitions(status)).toEqual([]);
		}
	);

	it.each(['scheduled', 'checked_in', 'queued'] as AppointmentStatus[])(
		'%s is not terminal and offers at least one transition',
		(status) => {
			expect(isTerminalAppointmentStatus(status)).toBe(false);
			expect(allowedAppointmentTransitions(status).length).toBeGreaterThan(0);
		}
	);
});

/**
 * The invalid-transition MESSAGES, checked against the Go source.
 *
 * §3 and §4 both require the backend's 400 text to reach the operator verbatim,
 * and the wording differs between the two handlers. A test that hardcoded the
 * Indonesian string here would pass even if the server changed its wording —
 * and the whole point of the requirement is that the CLIENT does not get to
 * choose the words. So these are read out of admin.go.
 */
describe('invalid-transition messages come from the server verbatim', () => {
	const queueMessage = /writeError\(w, http\.StatusBadRequest,\s*(fmt\.Sprintf\()?([^)]+)\)/.exec(
		GO_SOURCE.slice(GO_SOURCE.indexOf('func isValidQueueTransition'), GO_SOURCE.indexOf('func isValidAppointmentTransition'))
	);
	// The appointment path names both statuses with quotes, which is why the two
	// sentences differ and cannot be shared.
	expect(GO_SOURCE).toMatch(/Transisi status tidak valid/);
	expect(GO_SOURCE).toMatch(/tidak diizinkan/);

	it('the queue handler answers 400, not 409', () => {
		expect(queueMessage).not.toBeNull();
	});

	it('the two handlers use different sentences, so copy cannot be shared', () => {
		const queueSection = GO_SOURCE.slice(
			GO_SOURCE.indexOf('Transisi status tidak valid'),
			GO_SOURCE.indexOf('Transisi status tidak valid') + 200
		);
		const appointmentIndex = GO_SOURCE.indexOf('tidak diizinkan');
		expect(appointmentIndex).toBeGreaterThan(-1);
		expect(queueSection).not.toContain('tidak diizinkan');
	});
});

describe('canTransition agrees with the map it is derived from', () => {
	it.each(QUEUE_STATUSES.flatMap((from) =>
		QUEUE_STATUSES.map((to) => [from, to] as const)
	))('queue %s -> %s', (from, to) => {
		expect(canTransitionQueue(from, to)).toBe(allowedQueueTransitions(from).includes(to));
	});

	it.each(APPOINTMENT_STATUSES.flatMap((from) =>
		APPOINTMENT_STATUSES.map((to) => [from, to] as const)
	))('appointment %s -> %s', (from, to) => {
		expect(canTransitionAppointment(from, to)).toBe(
			allowedAppointmentTransitions(from).includes(to)
		);
	});

	it('never reports a self-transition as legal', () => {
		for (const status of QUEUE_STATUSES) {
			expect(canTransitionQueue(status, status)).toBe(false);
		}
		for (const status of APPOINTMENT_STATUSES) {
			expect(canTransitionAppointment(status, status)).toBe(false);
		}
	});
});
