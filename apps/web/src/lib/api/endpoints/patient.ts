import { apiFetchList, type ApiResult } from '../client';
import type { MyAppointment } from '../types/api';

/**
 * Patient-portal endpoints for a signed-in citizen.
 *
 * The difference from citizen.ts is the trust model. citizen.ts is code-keyed:
 * a citizen proves they hold a check-in code, and the endpoint keys off that
 * bearer token. This module is account-keyed: the request carries the session,
 * the Go API resolves the citizen's internal patient identity server-side, and
 * the answer is scoped to exactly the appointments that identity owns.
 *
 * The client never sends an identifier. There is no `accountId` parameter on
 * `listMyAppointments` for the same reason there is no `facilityId` on the
 * admin read surfaces: the identity is the session's, and the client is never
 * an authorization authority.
 */

/**
 * Lists the signed-in citizen's own appointments, newest first.
 *
 * The backend returns only appointments whose `patient_account_id` matches the
 * authenticated identity; a missing or staff identity answers 401, and a valid
 * identity with no appointments answers an empty list.
 */
export function listMyAppointments(signal?: AbortSignal): Promise<ApiResult<MyAppointment[]>> {
	return apiFetchList<MyAppointment>('/api/v1/patient/appointments', { signal });
}