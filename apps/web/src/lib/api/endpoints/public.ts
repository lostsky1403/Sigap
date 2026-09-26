import { apiFetch, apiFetchList, type ApiResult } from '../client';
import type {
	BookAppointmentResult,
	CheckInResult,
	PatientStatus,
	PublicFacility,
	PublicServiceUnit
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
			patient_name: input.patientName,
			patient_phone: input.patientPhone,
			appointment_time: input.appointmentTime,
			notes: input.notes
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
