import { apiFetch, type ApiResult } from '../client';
import type { PatientStatus, PublicServiceUnit } from '../types/api';

/**
 * Citizen-authenticated endpoints.
 *
 * The distinction from public.ts is who is asking, not what shape the answer
 * takes. A citizen may look up their own appointment status and the service
 * units they can book with, but never another citizen's data: the backend
 * scopes these by the session's identity, and the client adds no identifier of
 * its own.
 */

/**
 * Status of the signed-in citizen's own appointments.
 *
 * The backend resolves the citizen from the session, so no id is sent. Sending
 * one anyway would be asking the server to trust a client-supplied identifier.
 */
export function myAppointments(signal?: AbortSignal): Promise<ApiResult<PatientStatus[]>> {
	return apiFetch<PatientStatus[]>('/api/v1/patient/appointments', { signal });
}

/** Service units available for booking, optionally narrowed to one facility. */
export function bookableServiceUnits(
	facilityId?: string,
	signal?: AbortSignal
): Promise<ApiResult<PublicServiceUnit[]>> {
	return apiFetch<PublicServiceUnit[]>('/api/v1/public/service-units', {
		signal,
		query: { facility_id: facilityId }
	});
}
