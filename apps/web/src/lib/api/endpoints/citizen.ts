import { apiFetch, apiFetchList, type ApiResult } from '../client';
import type { PublicServiceUnit } from '../types/api';

/**
 * Citizen-authenticated endpoints.
 *
 * The distinction from public.ts is who is asking, not what shape the answer
 * takes. A citizen may look up their own appointment status and the service
 * units they can book with, but never another citizen's data: the backend
 * scopes these by the session's identity, and the client adds no identifier of
 * its own.
 *
 * There is deliberately no `myAppointments()` here. A previous version called
 * `/api/v1/patient/appointments`, which is not registered in the Go route table
 * and has no SvelteKit proxy — it was answered by the deny-by-default
 * middleware, so it could only ever fail. Listing a citizen's appointments
 * needs an identity to scope them to, and that is a product decision (see
 * design/sigap-v2-capability-matrix.md, F1), not a client helper.
 */

/** Service units available for booking, optionally narrowed to one facility. */
export function bookableServiceUnits(
	facilityId?: string,
	signal?: AbortSignal
): Promise<ApiResult<PublicServiceUnit[]>> {
	return apiFetchList<PublicServiceUnit>('/api/v1/public/service-units', {
		signal,
		query: { facility_id: facilityId }
	});
}

/**
 * Cancels a scheduled appointment by its check-in code.
 *
 * A scheduled appointment is the one state a citizen still controls; the
 * check-in code is the credential that proves they are the one who holds it
 * (the same trust model as the public status lookup, which is code-only and
 * rate limited). The backend owns every transition rule — 409 "tidak dapat
 * dibatalkan" and 404 "kode tidak ditemukan" arrive here as distinct error
 * kinds and stay distinct for the caller to render.
 */
export function cancelAppointmentByCode(
	code: string,
	signal?: AbortSignal
): Promise<ApiResult<{ appointment_id: string; status: string }>> {
	return apiFetch<{ appointment_id: string; status: string }>('/api/v1/appointments/cancel', {
		method: 'POST',
		body: { checkin_code: code },
		signal
	});
}
