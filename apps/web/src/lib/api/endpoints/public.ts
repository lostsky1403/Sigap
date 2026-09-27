import { apiFetch, apiFetchList, type ApiResult } from '../client';
import type {
	BookAppointmentResult,
	CheckInResult,
	PatientStatus,
	PublicFacility,
	PublicServiceUnit,
	QueueGenerateResult,
	QueueStatus
} from '../types/api';

/**
 * Unauthenticated endpoints.
 *
 * These are the routes a citizen hits without signing in, so they are also the
 * routes where an error code most easily gets misread as an auth problem. The
 * check-in function below carries the clearest example.
 */

/** Facility catalog. The backend accepts no query parameters here. */
export function listPublicFacilities(signal?: AbortSignal): Promise<ApiResult<PublicFacility[]>> {
	return apiFetchList<PublicFacility>('/api/v1/public/facilities', { signal });
}

/**
 * Service unit catalog.
 *
 * `facility_id` is a real backend parameter, and the proxy forwards it after
 * the Phase 3B1 P1 query-forwarding correction.
 */
export function listPublicServiceUnits(
	facilityId?: string,
	signal?: AbortSignal
): Promise<ApiResult<PublicServiceUnit[]>> {
	return apiFetchList<PublicServiceUnit>('/api/v1/public/service-units', {
		signal,
		query: { facility_id: facilityId }
	});
}

/**
 * Patient status lookup, keyed by check-in code or formatted queue number.
 *
 * Returns 401 for an unknown code, which for this route means "that code is not
 * recognised" and never "you are signed out". Callers must not route this to an
 * UnauthPanel.
 */
export function lookupPatientStatus(
	code: string,
	signal?: AbortSignal
): Promise<ApiResult<PatientStatus>> {
	return apiFetch<PatientStatus>('/api/v1/patient/status', {
		signal,
		query: { code }
	});
}

export interface BookAppointmentInput {
	facilityId: string;
	serviceUnitId: string;
	practitionerScheduleId?: string;
	patientName: string;
	patientPhone: string;
	appointmentTime: string;
	notes?: string;
}

/**
 * Books an appointment.
 *
 * The wire keys are the backend's, not ours. `BookAppointmentRequest` in
 * booking.go tags the name as `patient_display_name`; this function used to
 * send `patient_name`, which Go silently drops because unknown JSON keys are
 * ignored rather than rejected. The booking then failed with 400
 * "Nama pasien wajib diisi." — a validation error about a field the citizen
 * had filled in, which is about the worst possible thing to show someone.
 *
 * Worth stating plainly: the field name is load-bearing and a near-miss is
 * invisible. There is no error saying "unknown field", so this class of bug
 * looks exactly like a backend validation bug and gets debugged in the wrong
 * place. Every other key here is verified against the Go struct tag.
 */
export function bookAppointment(
	input: BookAppointmentInput,
	signal?: AbortSignal
): Promise<ApiResult<BookAppointmentResult>> {
	return apiFetch<BookAppointmentResult>('/api/v1/appointments', {
		method: 'POST',
		body: {
			facility_id: input.facilityId,
			service_unit_id: input.serviceUnitId,
			practitioner_schedule_id: input.practitionerScheduleId,
			patient_display_name: input.patientName,
			patient_phone: input.patientPhone,
			appointment_time: input.appointmentTime,
			notes: input.notes
		},
		signal
	});
}

/**
 * Generates a walk-in queue ticket.
 *
 * Deliberately NOT the appointment check-in route, and deliberately not
 * snake_case. `GenerateRequest` in queue.go takes `facilityId` and a nested
 * `patient` object with `fullName` — the opposite convention from
 * `BookAppointmentRequest` in the same package. Sending snake_case here is not
 * a style choice, it is a silent no-op followed by a 400.
 *
 * That inconsistency is the backend's, reproduced here rather than corrected,
 * because the frontend has no authority to rename another service's fields.
 */
export interface QueueGenerateInput {
	facilityId: string;
	fullName: string;
	phone: string;
}

export function generateQueueTicket(
	input: QueueGenerateInput,
	signal?: AbortSignal
): Promise<ApiResult<QueueGenerateResult>> {
	return apiFetch<QueueGenerateResult>('/api/v1/queues/generate', {
		method: 'POST',
		body: {
			facilityId: input.facilityId,
			patient: { fullName: input.fullName, phone: input.phone }
		},
		signal
	});
}

/**
 * Checks a citizen in using their check-in code.
 *
 * The 401 here is explicitly NOT an auth failure. The backend answers
 * 401 "Kode check-in tidak cocok." when the submitted code does not match the
 * appointment, which is a wrong-code condition and must be presented as such.
 * The `wrongCheckInCode` helper below makes that distinction available without
 * the caller having to remember the rule.
 *
 * The 409 that follows a successful code match with a wrong status is a
 * different condition again, and stays a `conflict`.
 */
export function checkInAppointment(
	appointmentId: string,
	checkinCode: string,
	signal?: AbortSignal
): Promise<ApiResult<CheckInResult>> {
	return apiFetch<CheckInResult>(
		`/api/v1/appointments/${encodeURIComponent(appointmentId)}/check-in`,
		{ method: 'POST', body: { checkin_code: checkinCode }, signal }
	);
}

/**
 * True when a 401 from the public check-in route means "wrong code".
 *
 * Exposed as a named predicate so the rule lives in one place. A caller that
 * simply checked `error.kind === 'unauthorized'` would show a sign-in prompt to
 * a citizen who typed their code wrong, which is both wrong and confusing.
 */
export function wrongCheckInCode(
	error: { kind: string; status: number } | null | undefined
): boolean {
	return Boolean(error && error.kind === 'unauthorized' && error.status === 401);
}

/**
 * A walk-in ticket, with the backend's casing already reconciled.
 *
 * This is the ONLY place that knows about the casing question, which is the
 * point of returning a separate type rather than letting the walk-in page read
 * the raw response. Once a page is writing `result.formatted_number`, a casing
 * change on either side of the boundary is a change to this file alone.
 */
export interface WalkInTicket {
	ticketId: string;
	formattedNumber: string;
	status: QueueStatus;
	estimatedWaitMinutes: number;
	processingTime: string;
	registeredAt: string;
}

/**
 * Raw shape as it may arrive from the generate endpoint.
 *
 * `GenerateResult` in service/queue.go tags `FormattedNumber` as
 * `json:"formatted_number"`, so snake_case is what Go actually emits today.
 * `TicketID` and `Status` are untagged and therefore marshal under their own
 * Go names — `TicketID` and `Status` in PascalCase, in the middle of an
 * otherwise snake_case object.
 *
 * Both `formatted_number` and `FormattedNumber` are accepted because the two
 * spellings describe the same field and a queue number is the one thing a
 * citizen must not see as blank. The failure mode of guessing wrong is a
 * ticket showing "—", which looks like a server error rather than a
 * mistyped key. `TicketID`/`Status` are accepted in both cases too, for the
 * same reason: they are read for display and logging, and a missing status
 * should not throw.
 */
type RawQueueTicket = {
	formatted_number?: string;
	FormattedNumber?: string;
	TicketID?: string;
	ticket_id?: string;
	ticketId?: string;
	Status?: string;
	status?: string;
	estimated_wait_minutes?: number;
	processing_time?: string;
	registered_at?: string;
};

/**
 * Reads a queue number out of a generate response under either casing.
 *
 * Returns the empty string when neither key is present, so a caller can decide
 * what an absent number means. It does not guess, and it does not fall back to
 * a placeholder: inventing a queue number on a civic page is exactly the
 * fabrication this project forbids, and a made-up number is worse than an
 * honest blank because a citizen would read it at the counter.
 */
export function readFormattedNumber(raw: RawQueueTicket | null | undefined): string {
	if (!raw) return '';
	return raw.formatted_number ?? raw.FormattedNumber ?? '';
}

/** Normalizes the whole generate response into a camelCase ticket. */
export function normalizeQueueTicket(raw: RawQueueTicket | null | undefined): WalkInTicket {
	const status = (raw?.Status ?? raw?.status ?? 'waiting') as QueueStatus;
	return {
		ticketId: raw?.TicketID ?? raw?.ticket_id ?? raw?.ticketId ?? '',
		formattedNumber: readFormattedNumber(raw),
		status,
		// Coerced to a number because the field is typed `int` in Go, so a
		// missing value is 0 rather than undefined. Rendered conditionally by
		// the caller, which is the only way to distinguish "no estimate" from
		// "an estimate of zero minutes".
		estimatedWaitMinutes: raw?.estimated_wait_minutes ?? 0,
		processingTime: raw?.processing_time ?? '',
		registeredAt: raw?.registered_at ?? ''
	};
}
