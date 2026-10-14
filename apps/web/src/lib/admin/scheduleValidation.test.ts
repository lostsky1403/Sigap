import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import {
	CAPACITY_MAX,
	CAPACITY_MIN,
	SLOT_MINUTES_MAX,
	SLOT_MINUTES_MIN,
	isScheduleFormValid,
	parseClockMinutes,
	rangeMinutes,
	slotCountFor,
	validateScheduleForm,
	type ScheduleFormValues
} from './scheduleForm';

/**
 * T-3B5 §7: the schedule form's own validation.
 *
 * `validateScheduleForm` is the operator's only pre-submit defence. A rule that
 * silently stops applying here does not fail loudly — it just lets the operator
 * fill in a form, lose the round trip, and be told a rule they were never shown.
 * So every bound is asserted directly, and then the bounds themselves are
 * asserted against the Go authority.
 *
 * WHY THE SECOND HALF EXISTS. Asserting `slot_minutes` rejects 4 and accepts 5
 * only proves this file and itself agree. The rule the server enforces lives in
 * `validateCreateSchedule` in admin.go, and the failure mode that matters is
 * drift: someone widens the bound on one side and not the other, and the
 * operator gets a server error for a form the client called valid. So the Go
 * source is parsed and the constants compared against it. That turns "please
 * remember to update both" into a build failure.
 */

/**
 * Locates admin.go by walking up from cwd.
 *
 * The depth of cwd is not stable: `pnpm --filter sigap-web exec vitest` runs with
 * cwd = apps/web, while a bare `vitest` from the repo root runs with cwd = the
 * root. Hardcoding either produced a path that resolved to `F:\api\...` on one
 * invocation and worked on the other, so the candidates are probed and the
 * failure names what it tried.
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
	throw new Error(
		`Could not find admin.go. Tried: ${candidates.join(', ')} (cwd: ${process.cwd()})`
	);
}

const GO_SOURCE = readGoSource();

/** The body of `validateCreateSchedule`, the server's authority for these rules. */
function goValidationSource(): string {
	const start = GO_SOURCE.indexOf('func validateCreateSchedule');
	expect(start, 'could not find validateCreateSchedule in admin.go').toBeGreaterThan(-1);
	const end = GO_SOURCE.indexOf('func validateUpdateSchedule', start);
	return GO_SOURCE.slice(start, end === -1 ? undefined : end);
}

/** A form that is valid in every respect, so each case can break exactly one rule. */
function validForm(overrides: Partial<ScheduleFormValues> = {}): ScheduleFormValues {
	return {
		facility_id: '00000000-0000-0000-0000-00000000d000',
		service_unit_id: '00000000-0000-0000-0000-0000000000a1',
		schedule_date: '2026-09-30',
		// 09:00-12:00 is 180 minutes, divisible by 30, 60, 90, and 180.
		start_time: '09:00',
		end_time: '12:00',
		slot_minutes: '60',
		capacity_per_slot: '2',
		...overrides
	};
}

describe('slot duration is bounded to 5-180 minutes', () => {
	it('accepts the lower bound and rejects one minute below it', () => {
		expect(validateScheduleForm(validForm({ slot_minutes: '5' })).slot_minutes).toBeUndefined();
		expect(validateScheduleForm(validForm({ slot_minutes: '4' })).slot_minutes).toBe(
			'Durasi slot harus antara 5–180 menit.'
		);
	});

	it('accepts the upper bound and rejects one minute above it', () => {
		// 09:00-12:00 is exactly 180 minutes, so 180 divides it and 181 does not
		// even parse into a legal slot. The bound is what is under test, so the
		// range is widened to 09:00-15:00 to keep divisibility out of the way.
		expect(
			validateScheduleForm(
				validForm({ end_time: '15:00', slot_minutes: '180' })
			).slot_minutes
		).toBeUndefined();
		expect(
			validateScheduleForm(
				validForm({ end_time: '15:00', slot_minutes: '181' })
			).slot_minutes
		).toBe('Durasi slot harus antara 5–180 menit.');
	});

	it('rejects a non-positive or fractional slot', () => {
		expect(validateScheduleForm(validForm({ slot_minutes: '0' })).slot_minutes).toBe(
			'Durasi slot harus lebih dari 0 menit.'
		);
		expect(validateScheduleForm(validForm({ slot_minutes: '-30' })).slot_minutes).toBe(
			'Durasi slot harus lebih dari 0 menit.'
		);
		// A fractional slot would decode into an int as a JSON type error, and the
		// operator would be told their JSON was malformed rather than that the
		// field was wrong.
		expect(validateScheduleForm(validForm({ slot_minutes: '30.5' })).slot_minutes).toBe(
			'Durasi slot harus lebih dari 0 menit.'
		);
	});

	it('rejects an empty slot', () => {
		expect(validateScheduleForm(validForm({ slot_minutes: '' })).slot_minutes).toBe(
			'Durasi slot harus lebih dari 0 menit.'
		);
	});
});

describe('a slot must divide the range exactly', () => {
	it('accepts an exact division', () => {
		expect(
			validateScheduleForm(validForm({ end_time: '12:00', slot_minutes: '60' }))
				.slot_minutes
		).toBeUndefined();
	});

	it('names the rule and the total when the division leaves a remainder', () => {
		// 09:00-10:30 is 90 minutes; 50 does not divide it.
		const errors = validateScheduleForm(
			validForm({ end_time: '10:30', slot_minutes: '50' })
		);
		expect(errors.slot_minutes).toBe(
			'Durasi slot harus membagi habis rentang waktu (90 menit).'
		);
	});

	it('does not blame divisibility for a range that is not usable yet', () => {
		// End before start: the time rule already failed, so a divisibility
		// message here would name the wrong problem and send the operator to fix
		// a slot that was never the issue.
		const errors = validateScheduleForm(
			validForm({ start_time: '12:00', end_time: '09:00', slot_minutes: '50' })
		);
		expect(errors.end_time).toBe('Jam selesai harus setelah jam mulai.');
		// No slot error at all, not merely a different one.
		expect(errors.slot_minutes).toBeUndefined();
	});

	it('computes the same slot count the Go modulo would', () => {
		expect(slotCountFor('09:00', '12:00', 60)).toBe(3);
		expect(slotCountFor('09:00', '12:00', 45)).toBe(4);
		// The stub case: 45 leaves a 30-minute tail no citizen could book into.
		expect(slotCountFor('09:00', '12:00', 50)).toBeNull();
	});
});

describe('the end time must be after the start time', () => {
	it('rejects an equal or earlier end', () => {
		expect(validateScheduleForm(validForm({ end_time: '09:00' })).end_time).toBe(
			'Jam selesai harus setelah jam mulai.'
		);
		expect(validateScheduleForm(validForm({ end_time: '08:00' })).end_time).toBe(
			'Jam selesai harus setelah jam mulai.'
		);
	});

	it('rejects an unparseable clock the way Go does', () => {
		// Go round-trips the layout, so the unpadded "8:00" is refused there and
		// must be refused here or the operator gets a server error for a value
		// the form accepted.
		expect(parseClockMinutes('8:00')).toBeNull();
		expect(parseClockMinutes('24:00')).toBeNull();
		expect(parseClockMinutes('09:60')).toBeNull();
		expect(parseClockMinutes('09:00:00')).toBe(540);
		expect(rangeMinutes('09:00', 'bad')).toBeNull();
	});

	it('requires both clocks before it will judge the range', () => {
		expect(validateScheduleForm(validForm({ end_time: '' })).start_time).toBe(
			'Jam mulai dan jam selesai wajib diisi (HH:MM).'
		);
	});
});

describe('capacity is bounded to 1-100', () => {
	it('rejects zero and a negative capacity', () => {
		expect(validateScheduleForm(validForm({ capacity_per_slot: '0' })).capacity_per_slot).toBe(
			'Kapasitas per slot harus lebih dari 0.'
		);
		expect(
			validateScheduleForm(validForm({ capacity_per_slot: '-1' })).capacity_per_slot
		).toBe('Kapasitas per slot harus lebih dari 0.');
	});

	it('accepts exactly the maximum and refuses one above it', () => {
		expect(
			validateScheduleForm(validForm({ capacity_per_slot: '100' })).capacity_per_slot
		).toBeUndefined();
		expect(
			validateScheduleForm(validForm({ capacity_per_slot: '101' })).capacity_per_slot
		).toBe(`Kapasitas per slot maksimal ${CAPACITY_MAX}.`);
	});
});

describe('required selections and a valid whole form', () => {
	it('names each missing required field', () => {
		expect(validateScheduleForm(validForm({ facility_id: '  ' })).facility_id).toBe(
			'Pilih fasilitas.'
		);
		expect(validateScheduleForm(validForm({ service_unit_id: '' })).service_unit_id).toBe(
			'Pilih unit layanan.'
		);
		expect(validateScheduleForm(validForm({ schedule_date: '' })).schedule_date).toBe(
			'Tanggal jadwal wajib diisi (YYYY-MM-DD).'
		);
	});

	it('rejects a date Go time.Parse would refuse', () => {
		expect(validateScheduleForm(validForm({ schedule_date: '30-09-2026' })).schedule_date).toBe(
			'Format tanggal jadwal tidak valid (YYYY-MM-DD).'
		);
		// Round-trips through Date, so an impossible day is caught without a
		// hand-rolled leap-year table.
		expect(validateScheduleForm(validForm({ schedule_date: '2026-02-30' })).schedule_date).toBe(
			'Format tanggal jadwal tidak valid (YYYY-MM-DD).'
		);
	});

	it('accepts the whole valid form', () => {
		expect(validateScheduleForm(validForm())).toEqual({});
		expect(isScheduleFormValid(validForm())).toBe(true);
	});

	it('reports every offending field at once, not just the first', () => {
		// An operator who fixed one field at a time would retype the form five
		// times for a form that was wrong in five places.
		expect(Object.keys(validateScheduleForm(validForm({ facility_id: '', slot_minutes: '1' })))).toEqual(
			expect.arrayContaining(['facility_id', 'slot_minutes'])
		);
	});
});

/**
 * The drift guard: these bounds are COPIES of the Go handler's, and a copy that
 * drifts is the whole failure this module exists to prevent.
 */
describe('the bounds match the Go authority', () => {
	const go = goValidationSource();

	it('mirrors the 5-180 slot guard', () => {
		expect(SLOT_MINUTES_MIN).toBe(5);
		expect(SLOT_MINUTES_MAX).toBe(180);
		// Read the literal comparison out of the Go source rather than the
		// message text, so a widened bound with an unchanged message still fails.
		expect(go).toMatch(
			new RegExp(
				`req\\.SlotMinutes\\s*<\\s*${SLOT_MINUTES_MIN}\\s*\\|\\|\\s*req\\.SlotMinutes\\s*>\\s*${SLOT_MINUTES_MAX}`
			)
		);
	});

	it('mirrors the capacity guard', () => {
		expect(CAPACITY_MIN).toBe(1);
		expect(CAPACITY_MAX).toBe(100);
		expect(go).toMatch(new RegExp(`req\\.CapacityPerSlot\\s*>\\s*${CAPACITY_MAX}`));
	});

	it('mirrors the exact-division rule', () => {
		expect(go).toMatch(/totalMinutes%req\.SlotMinutes != 0/);
	});

	it('mirrors the ordering rule, so a zero-length range is refused', () => {
		expect(go).toMatch(/endDuration <= startDuration/);
	});

	it('uses the same sentences the operator is shown, so the two agree', () => {
		// The whole point of copying the strings rather than paraphrasing: an
		// operator who sees a rejection after submitting should recognise the
		// same sentence they were shown inline.
		// "Pilih fasilitas." and "Pilih unit layanan." are deliberately absent
		// from this list: they are client-only wording, because the server's UUID
		// parse error would name an id the operator never chose.
		for (const sentence of [
			'Durasi slot harus antara 5–180 menit.',
			'Kapasitas per slot maksimal 100.',
			'Jam selesai harus setelah jam mulai.',
			'Tanggal jadwal wajib diisi (YYYY-MM-DD).',
			'Format jam tidak valid (pilih HH:MM atau HH:MM:SS).'
		]) {
			expect(go).toContain(sentence);
		}
	});
});
