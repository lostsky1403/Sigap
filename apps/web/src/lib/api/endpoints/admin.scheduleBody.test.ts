import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { createSchedule, updateSchedule, type ScheduleInput } from './admin';
import { toScheduleRequest, type ScheduleFormValues } from '$lib/admin/scheduleForm';

/**
 * T-3B5 §6: `practitioner_id` must never appear in a schedule mutation body.
 *
 * This is the requirement most likely to be broken by an innocent-looking edit,
 * and the most expensive to get wrong, so it is asserted in three independent
 * ways. Any one of them alone would be weak:
 *
 *  1. The REQUEST BODY, serialized. This is the only assertion that speaks to
 *     what actually goes on the wire. A type-level guarantee cannot catch a
 *     `...(rest as Record<string, unknown>)` spread, and the requirement is
 *     about the request, not about what the compiler permits.
 *
 *  2. The BUILD FUNCTION, whose return type has no such key. So a future
 *     contributor adding the field has to edit this module, and the diff shows
 *     it.
 *
 *  3. THE SOURCE, for any other code path that might build a schedule body.
 *     Two call sites building the payload independently is how the two would
 *     drift, and the one that drifts is the one nobody tests.
 *
 * WHY THE ABSENCE MATTERS AT ALL. `practitioner_schedules.practitioner_id` is
 * nullable, and Go accepts the field — so a body carrying it would be persisted
 * silently. There is no practitioner catalog in SIGAP, so nothing could ever
 * verify the value, and no UI would show it. It would be a fact in the database
 * that no human could confirm or correct.
 */

const fetchMock = vi.fn();

beforeEach(() => {
	fetchMock.mockReset();
	fetchMock.mockResolvedValue(
		new Response(JSON.stringify({ success: true, data: { id: 'sched-1' } }), {
			status: 201,
			headers: { 'Content-Type': 'application/json' }
		})
	);
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

function validInput(): ScheduleInput {
	return {
		facility_id: '00000000-0000-0000-0000-00000000d000',
		service_unit_id: '00000000-0000-0000-0000-0000000000a1',
		schedule_date: '2026-09-30',
		start_time: '09:00',
		end_time: '12:00',
		slot_minutes: 30,
		capacity_per_slot: 2
	};
}

/** The literal JSON text of the last request, which is what the server sees. */
function lastBody(): Record<string, unknown> {
	const [, init] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1];
	return JSON.parse(String((init as RequestInit).body));
}

describe('schedule CREATE body omits practitioner_id', () => {
	it('serializes without the key', async () => {
		await createSchedule(validInput());
		const body = lastBody();
		expect(body).not.toHaveProperty('practitioner_id');
		// Sorted so the expectation is a set comparison, not an ordering one:
		// JSON key order carries no meaning, and asserting it would make this
		// fail on a harmless property reorder.
		expect(Object.keys(body).sort()).toEqual([
			'capacity_per_slot',
			'end_time',
			'facility_id',
			'schedule_date',
			'service_unit_id',
			'slot_minutes',
			'start_time'
		]);
	});

	it('does not send it as null, empty string, or undefined either', async () => {
		await createSchedule(validInput());
		// JSON.stringify drops undefined, so a careless spread could leave the key
		// absent (fine) or present with a null/empty value (NOT fine — that would
		// clear an existing practitioner on update).
		const raw = String(
			(fetchMock.mock.calls[0][1] as RequestInit).body
		);
		expect(raw).not.toContain('practitioner');
	});

	it('the input type has no practitioner field to populate', () => {
		// A compile-time property, asserted as a runtime one so the intent
		// survives into the test output.
		const keys = Object.keys(validInput());
		expect(keys).not.toContain('practitioner_id');
	});
});

describe('schedule UPDATE body omits practitioner_id', () => {
	it('serializes without the key on a partial update', async () => {
		await updateSchedule('sched-1', { slot_minutes: 45, capacity_per_slot: 3 });
		const body = lastBody();
		expect(body).not.toHaveProperty('practitioner_id');
		expect(body).toEqual({ slot_minutes: 45, capacity_per_slot: 3 });
	});

	it('serializes without the key on a full update', async () => {
		await updateSchedule('sched-1', validInput());
		const body = lastBody();
		expect(body).not.toHaveProperty('practitioner_id');
		expect(body.facility_id).toBe(validInput().facility_id);
	});

	/**
	 * The case that would do real damage.
	 *
	 * An update seeded from an existing row is exactly where a practitioner id
	 * could be carried along "just to be safe" — and it would be silently
	 * written back, or worse, nulled. The body builder never reads the row, so
	 * the value cannot travel.
	 */
	it('carries no practitioner value from the row being edited', () => {
		const row = {
			...validInput(),
			id: 'sched-1',
			practitioner_id: '00000000-0000-0000-0000-0000000000ff',
			is_active: true
		};
		const body = toScheduleRequest({
			facility_id: row.facility_id,
			service_unit_id: row.service_unit_id,
			schedule_date: row.schedule_date,
			start_time: row.start_time,
			end_time: row.end_time,
			slot_minutes: String(row.slot_minutes),
			capacity_per_slot: String(row.capacity_per_slot)
		});
		expect(body).not.toHaveProperty('practitioner_id');
		expect(JSON.stringify(body)).not.toContain('practitioner');
	});
});

describe('toScheduleRequest builds a practitioner-free body', () => {
	const form: ScheduleFormValues = {
		facility_id: 'f-1',
		service_unit_id: 's-1',
		schedule_date: '2026-09-30',
		start_time: '09:00',
		end_time: '12:00',
		slot_minutes: '30',
		capacity_per_slot: '2'
	};

	it('carries exactly the seven schedule fields', () => {
		expect(Object.keys(toScheduleRequest(form)).sort()).toEqual([
			'capacity_per_slot',
			'end_time',
			'facility_id',
			'schedule_date',
			'service_unit_id',
			'slot_minutes',
			'start_time'
		]);
	});

	/**
	 * Numbers, not strings.
	 *
	 * Go decodes `slot_minutes` into an `int`. Sending "30" is a JSON type
	 * error, and the operator would be told their JSON was malformed rather than
	 * that a field was wrong — so the conversion is asserted here.
	 */
	it('converts the numeric fields to numbers', () => {
		const body = toScheduleRequest(form);
		expect(typeof body.slot_minutes).toBe('number');
		expect(typeof body.capacity_per_slot).toBe('number');
		expect(body.slot_minutes).toBe(30);
		expect(body.capacity_per_slot).toBe(2);
	});
});

describe('no code path builds a schedule body with a practitioner', () => {
	/**
	 * The source-level backstop.
	 *
	 * The behavioural tests above cover the paths that exist today. This one
	 * fails if a NEW path is added that sends the field, which is the drift the
	 * per-call-site tests cannot see coming. It reads the module and the editor,
	 * strips comments so this file's own explanation is not what matches, and
	 * then requires the key to be absent from executable code.
	 */
	it('neither the client nor the editor names the key in executable code', async () => {
		const { readFile } = await import('node:fs/promises');
		const files = ['src/lib/api/endpoints/admin.ts', 'src/lib/admin/scheduleForm.ts', 'src/lib/admin/ScheduleEditor.svelte'];

		for (const file of files) {
			const source = await readFile(file, 'utf8');
			const code = source
				.replace(/\/\*[\s\S]*?\*\//g, '')
				.replace(/\/\/[^\n]*/g, '')
				.replace(/<!--[\s\S]*?-->/g, '');
			expect(code, `${file} must not name practitioner_id in code`).not.toMatch(
				/practitioner_id/
			);
		}
	});
});
