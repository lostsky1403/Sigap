/**
 * T-3B5 §7: schedule form validation.
 *
 * WHY THIS EXISTS ALONGSIDE THE SERVER'S
 *
 * `validateCreateSchedule` in admin.go is the authority and always runs. This
 * exists for a different reason: a rejected round trip costs the operator
 * everything they typed, and the server's messages arrive only after a request
 * that may fail on a rule unrelated to the one they got wrong. Catching a slot
 * that cannot divide the range before submitting turns a wall of red text into
 * one field with one message.
 *
 * It is therefore NOT a reimplementation that could disagree. Every rule here
 * is lifted from the Go source, and `scheduleValidation.test.ts` asserts the
 * two agree by parsing that source — so if a bound changes on the server and
 * not here, the build fails instead of the operator hitting a wall.
 *
 * WORDS ARE THE SERVER'S, NOT OURS.
 *
 * The messages below are copies of the Go strings, not translations. When the
 * server says "Durasi slot harus antara 5–180 menit.", the client says exactly
 * that, so an operator who sees a rejection after submitting recognises the
 * same sentence they were just shown inline.
 */

/** Mirrors the 5–180 bound in validateCreateSchedule. */
export const SLOT_MINUTES_MIN = 5;
export const SLOT_MINUTES_MAX = 180;

/** Mirrors the capacity bounds. */
export const CAPACITY_MIN = 1;
export const CAPACITY_MAX = 100;

export interface ScheduleFormValues {
	facility_id: string;
	service_unit_id: string;
	/** YYYY-MM-DD */
	schedule_date: string;
	/** HH:MM or HH:MM:SS */
	start_time: string;
	end_time: string;
	slot_minutes: string;
	capacity_per_slot: string;
}

export type ScheduleFormErrors = Partial<Record<keyof ScheduleFormValues, string>>;

/**
 * Minutes since midnight for an `HH:MM` / `HH:MM:SS` string.
 *
 * Returns null for anything the Go parser would also reject, including the
 * unpadded form. That strictness is not pedantry: Go does a round-trip check
 * (`t.Format(layout) != req.StartTime`), so "8:00" is rejected there and must be
 * rejected here too, or the operator gets a server error for something the form
 * accepted.
 *
 * Seconds are parsed and discarded, matching Go, which builds its durations
 * from Hour() and Minute() only.
 */
export function parseClockMinutes(value: string): number | null {
	const match = /^(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value.trim());
	if (!match) return null;
	const hours = Number(match[1]);
	const minutes = Number(match[2]);
	if (hours > 23 || minutes > 59) return null;
	return hours * 60 + minutes;
}

/** Whole minutes between two clock strings, or null if either is unparseable. */
export function rangeMinutes(start: string, end: string): number | null {
	const startMinutes = parseClockMinutes(start);
	const endMinutes = parseClockMinutes(end);
	if (startMinutes === null || endMinutes === null) return null;
	return endMinutes - startMinutes;
}

/**
 * How many slots the range yields, or null when the slot length does not divide
 * it exactly.
 *
 * Exact division is the server's rule (`totalMinutes % slotMinutes != 0`), and
 * it is not arbitrary: a 09:00–12:00 range with 45-minute slots would end with a
 * 30-minute stub, which is a slot no citizen can be booked into. Refusing it is
 * the system being honest about its own data.
 */
export function slotCountFor(
	start: string,
	end: string,
	slotMinutes: number
): number | null {
	const total = rangeMinutes(start, end);
	if (total === null || slotMinutes <= 0) return null;
	if (total % slotMinutes !== 0) return null;
	return total / slotMinutes;
}

/** True for a `YYYY-MM-DD` date the Go parser accepts. */
export function isIsoDate(value: string): boolean {
	if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
	const [year, month, day] = value.split('-').map(Number);
	if (month < 1 || month > 12 || day < 1 || day > 31) return false;
	// Round-trips through Date so 2026-02-30 is rejected the way time.Parse
	// rejects it, rather than by a hand-rolled days-in-month table that would
	// have to encode leap years.
	const parsed = new Date(Date.UTC(year, month - 1, day));
	return (
		parsed.getUTCFullYear() === year &&
		parsed.getUTCMonth() === month - 1 &&
		parsed.getUTCDate() === day
	);
}

/**
 * Validates the form, returning one message per offending field.
 *
 * Every message is the Go handler's own wording. The ordering follows the
 * handler's, so an operator reading both sees the same checks in the same
 * sequence.
 */
export function validateScheduleForm(values: ScheduleFormValues): ScheduleFormErrors {
	const errors: ScheduleFormErrors = {};

	if (values.facility_id.trim() === '') {
		errors.facility_id = 'Pilih fasilitas.';
	}

	if (values.service_unit_id.trim() === '') {
		errors.service_unit_id = 'Pilih unit layanan.';
	}

	if (values.schedule_date.trim() === '') {
		errors.schedule_date = 'Tanggal jadwal wajib diisi (YYYY-MM-DD).';
	} else if (!isIsoDate(values.schedule_date)) {
		errors.schedule_date = 'Format tanggal jadwal tidak valid (YYYY-MM-DD).';
	}

	if (values.start_time.trim() === '' || values.end_time.trim() === '') {
		errors.start_time = 'Jam mulai dan jam selesai wajib diisi (HH:MM).';
	} else if (
		parseClockMinutes(values.start_time) === null ||
		parseClockMinutes(values.end_time) === null
	) {
		errors.start_time = 'Format jam tidak valid (pilih HH:MM atau HH:MM:SS).';
	} else if (rangeMinutes(values.start_time, values.end_time)! <= 0) {
		errors.end_time = 'Jam selesai harus setelah jam mulai.';
	}

	// Whether the times themselves are usable, tracked separately so the
	// divisibility check below can run only when they are. Reporting "the slot
	// does not divide the range" for a range that could not be parsed would name
	// the wrong problem.
	const timesUsable =
		values.start_time.trim() !== '' &&
		values.end_time.trim() !== '' &&
		parseClockMinutes(values.start_time) !== null &&
		parseClockMinutes(values.end_time) !== null &&
		(rangeMinutes(values.start_time, values.end_time) ?? 0) > 0;

	const slot = Number(values.slot_minutes);
	if (values.slot_minutes.trim() === '') {
		errors.slot_minutes = 'Durasi slot harus lebih dari 0 menit.';
	} else if (!Number.isInteger(slot) || slot <= 0) {
		errors.slot_minutes = 'Durasi slot harus lebih dari 0 menit.';
	} else if (slot < SLOT_MINUTES_MIN || slot > SLOT_MINUTES_MAX) {
		errors.slot_minutes = `Durasi slot harus antara ${SLOT_MINUTES_MIN}–${SLOT_MINUTES_MAX} menit.`;
	} else if (timesUsable && slotCountFor(values.start_time, values.end_time, slot) === null) {
		const total = rangeMinutes(values.start_time, values.end_time);
		errors.slot_minutes = `Durasi slot harus membagi habis rentang waktu (${total} menit).`;
	}

	const capacity = Number(values.capacity_per_slot);
	if (values.capacity_per_slot.trim() === '') {
		errors.capacity_per_slot = 'Kapasitas per slot harus lebih dari 0.';
	} else if (!Number.isInteger(capacity) || capacity <= 0) {
		errors.capacity_per_slot = 'Kapasitas per slot harus lebih dari 0.';
	} else if (capacity > CAPACITY_MAX) {
		errors.capacity_per_slot = `Kapasitas per slot maksimal ${CAPACITY_MAX}.`;
	}

	return errors;
}

/** True when the form is clean enough to submit. */
export function isScheduleFormValid(values: ScheduleFormValues): boolean {
	return Object.keys(validateScheduleForm(values)).length === 0;
}

/**
 * Builds the request body from validated form values.
 *
 * `practitioner_id` is ABSENT BY CONSTRUCTION: it is not a property of this
 * return type, and it is not derived from anywhere. There is no code path from
 * these values to that key, which is the property the requirement actually asks
 * for — not "we remembered not to set it".
 *
 * Numbers are converted here rather than in the component so the values that
 * reach JSON are the ones the validation just approved. A form that validates
 * "30" and sends `"30"` would be refused by Go's int decode with a message
 * about JSON format rather than about the slot.
 */
export function toScheduleRequest(values: ScheduleFormValues): {
	facility_id: string;
	service_unit_id: string;
	schedule_date: string;
	start_time: string;
	end_time: string;
	slot_minutes: number;
	capacity_per_slot: number;
} {
	return {
		facility_id: values.facility_id,
		service_unit_id: values.service_unit_id,
		schedule_date: values.schedule_date,
		start_time: values.start_time,
		end_time: values.end_time,
		slot_minutes: Number(values.slot_minutes),
		capacity_per_slot: Number(values.capacity_per_slot)
	};
}
