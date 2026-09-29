import { apiFetch, apiFetchList, type ApiResult } from '../client';
import type {
	AdminAppointment,
	AdminFacility,
	AdminQueueTicket,
	AdminSchedule,
	AdminServiceUnit,
	AppointmentStatus,
	FacilityDeactivateResult,
	NotificationOutboxRow,
	NotificationSummary,
	QueueStatus,
	ScheduleMutationOptions,
	StatusUpdateResult
} from '../types/api';

/**
 * Admin endpoints.
 *
 * Nothing here filters by facility, role, or permission. That is deliberate and
 * it is the security boundary: the Phase 3B0 facility-aware authorization lives
 * in Go, where scope and permission provenance are resolved from the database
 * on every request. A client-side filter would be a UI convenience at best and
 * an authorization claim at worst, and a client that "knows" its facilities is a
 * client an attacker can edit.
 *
 * Consequently a 403 from any of these is ambiguous on its own — missing
 * session and insufficient permission share a status. Presentation belongs to
 * ErrorState, which takes `hasSession` as an explicit input.
 */

/* ------------------------------ facilities ------------------------------ */

export function listFacilities(signal?: AbortSignal): Promise<ApiResult<AdminFacility[]>> {
	return apiFetchList<AdminFacility>('/api/v1/admin/facilities', { signal });
}

export function getFacility(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<AdminFacility>> {
	return apiFetch<AdminFacility>(`/api/v1/admin/facilities/${encodeURIComponent(id)}`, {
		signal
	});
}

/**
 * The facility create/update body, matching `CreateFacilityRequest` /
 * `UpdateFacilityRequest` in admin.go field for field.
 *
 * `available_beds` was missing from the pre-3B5 type. That is not a cosmetic
 * gap: the API declares the field, so omitting it from the body meant Go
 * decoded the zero value and every facility created through this client got
 * `available_beds = 0` — a real facility reporting no free beds to a patient
 * looking for one. Adding it is the difference between the form and the API
 * agreeing about what a facility is.
 */
export interface FacilityInput {
	name: string;
	type: 'rumah_sakit' | 'puskesmas';
	address: string;
	kecamatan: string;
	kabupaten_kota: string;
	provinsi: string;
	phone: string;
	total_beds: number;
	available_beds: number;
	short_code: string;
}

export function createFacility(
	input: FacilityInput,
	signal?: AbortSignal
): Promise<ApiResult<{ id: string }>> {
	return apiFetch<{ id: string }>('/api/v1/admin/facilities', {
		method: 'POST',
		body: input,
		signal
	});
}

export function updateFacility(
	id: string,
	input: Partial<FacilityInput>,
	signal?: AbortSignal
): Promise<ApiResult<{ id: string }>> {
	return apiFetch<{ id: string }>(`/api/v1/admin/facilities/${encodeURIComponent(id)}`, {
		method: 'PATCH',
		body: input,
		signal
	});
}

/**
 * Deactivates a facility. Soft delete: the row is retained and `is_active`
 * flips to false, which is why this is PATCH and not DELETE.
 *
 * The response types `is_active` as a string, matching the backend's
 * map[string]string literal.
 */
export function deactivateFacility(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<FacilityDeactivateResult>> {
	return apiFetch<FacilityDeactivateResult>(
		`/api/v1/admin/facilities/${encodeURIComponent(id)}/deactivate`,
		{ method: 'PATCH', signal }
	);
}

/* -------------------------------- queues -------------------------------- */

export function listQueueTickets(
	signal?: AbortSignal
): Promise<ApiResult<AdminQueueTicket[]>> {
	return apiFetchList<AdminQueueTicket>('/api/v1/admin/queues', { signal });
}

export function getQueueTicket(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<AdminQueueTicket>> {
	return apiFetch<AdminQueueTicket>(`/api/v1/admin/queues/${encodeURIComponent(id)}`, {
		signal
	});
}

/**
 * Advances a ticket.
 *
 * The response is `{id, status, updated_at}` — the server does NOT echo the
 * whole ticket, so a caller must reload to see the `called_at` / `completed_at`
 * columns the transition stamps. Patching the local row instead would claim a
 * timestamp the server has not confirmed.
 *
 * STATUS CODES, corrected against the source rather than assumed. An illegal
 * transition is **400**, not 409, and the message names both statuses:
 * "Transisi status tidak valid: waiting → in_service." The client surfaces that
 * text verbatim — see `mutationFailure`, which prefers the server's words over
 * any generic fallback. The only 409s in this admin surface belong to the
 * notification endpoints, so a caller that assumes 409 here will mis-classify
 * every rejected transition.
 */
export function updateQueueTicketStatus(
	id: string,
	status: QueueStatus,
	signal?: AbortSignal
): Promise<ApiResult<StatusUpdateResult>> {
	return apiFetch<StatusUpdateResult>(
		`/api/v1/admin/queues/${encodeURIComponent(id)}/status`,
		{ method: 'PATCH', body: { status }, signal }
	);
}

/* ----------------------------- service units ---------------------------- */

export function listServiceUnits(
	signal?: AbortSignal
): Promise<ApiResult<AdminServiceUnit[]>> {
	return apiFetchList<AdminServiceUnit>('/api/v1/admin/service-units', { signal });
}

export function getServiceUnit(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<AdminServiceUnit>> {
	return apiFetch<AdminServiceUnit>(`/api/v1/admin/service-units/${encodeURIComponent(id)}`, {
		signal
	});
}

export interface ServiceUnitInput {
	name: string;
	code?: string;
	description?: string;
}

export function createServiceUnit(
	facilityId: string,
	input: ServiceUnitInput,
	signal?: AbortSignal
): Promise<ApiResult<{ id: string }>> {
	return apiFetch<{ id: string }>(
		`/api/v1/admin/facilities/${encodeURIComponent(facilityId)}/service-units`,
		{ method: 'POST', body: input, signal }
	);
}

/* ------------------------------- schedules ------------------------------ */

export function listSchedules(signal?: AbortSignal): Promise<ApiResult<AdminSchedule[]>> {
	return apiFetchList<AdminSchedule>('/api/v1/admin/schedules', { signal });
}

export function getSchedule(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<AdminSchedule>> {
	return apiFetch<AdminSchedule>(`/api/v1/admin/schedules/${encodeURIComponent(id)}`, {
		signal
	});
}

/**
 * Schedule MUTATION options (Phase 3B5.0).
 *
 * The list above is the read table: it answers "what schedules exist?".
 * This one answers the different question a create/edit form has to ask
 * first — "which facilities may I actually build a schedule for?" — and the
 * server answers it by including only facilities where the actor holds
 * schedule.manage AT that facility.
 *
 * That is the whole point, and it is why the browser is not asked to work it
 * out. The client holds `hasSession` and nothing else; there is no permission
 * list here to reason from, by design. The list of options IS the answer, and
 * it is a list of ids and names with no accompanying reason, no role, and no
 * scope echo, so it cannot be used to infer the authorization model.
 *
 * An empty `facilities` array is a SUCCESSFUL response meaning "you may manage
 * schedules at no facility" — not a 403 and not an error. The caller renders a
 * capability refusal from it. Treating it as a failure would be wrong: the
 * request was authorized, it simply had nothing to offer.
 *
 * ADVISORY ONLY. The mutation endpoints re-derive authorization from the
 * submitted facility on every call, so a grant revoked between this call and a
 * later POST is still refused.
 */
export function getScheduleMutationOptions(
	signal?: AbortSignal
): Promise<ApiResult<ScheduleMutationOptions>> {
	return apiFetch<ScheduleMutationOptions>('/api/v1/admin/schedules/options', { signal });
}

/**
 * The schedule create/update body.
 *
 * `practitioner_id` IS DELIBERATELY ABSENT, and its absence is enforced by the
 * type system rather than by a test. The backend accepts the field
 * (`CreateScheduleRequest.PractitionerID` is `string` with `omitempty`), so a
 * body that carried it would be accepted, persisted, and silently attach a real
 * schedule row to a practitioner. That is exactly the surface this phase must
 * not create:
 *
 *   - SIGAP has no practitioner UI, no practitioner directory, and no
 *     practitioner E2E. There is nothing for a human to check a name against,
 *     so any value here would be unfalsifiable.
 *   - The column is nullable (`NULLIF($2, '')::uuid` on create, and update can
 *     set it to NULL), so OMITTING it leaves the row's practitioner untouched
 *     on update and NULL on create. Sending `""`, `null`, or the row's current
 *     value would each be a different, wrong behaviour.
 *
 * So the key is not merely unset by convention: it is absent from the type, so
 * the editor that builds this body has nothing to pass it through, and there is
 * no form field anywhere that produces one. A test additionally asserts the
 * SERIALIZED body lacks the key — a type alone would not catch a
 * `...(rest as any)` spread, and the requirement is about what goes on the
 * wire, not about what the compiler allows.
 */
export interface ScheduleInput {
	facility_id: string;
	service_unit_id: string;
	/** YYYY-MM-DD */
	schedule_date: string;
	/** HH:MM or HH:MM:SS */
	start_time: string;
	/** HH:MM or HH:MM:SS */
	end_time: string;
	slot_minutes: number;
	capacity_per_slot: number;
}

/**
 * Schedule fields that can be changed after creation.
 *
 * `is_active` is present because the backend honours it on PATCH, and every
 * other field is optional there too — a partial update is legal. Note the
 * asymmetry with `ScheduleInput`, which is total: create requires everything,
 * update requires nothing. That is the backend's contract, not a client
 * convenience.
 */
export type ScheduleUpdateInput = Partial<ScheduleInput> & { is_active?: boolean };

export function createSchedule(
	input: ScheduleInput,
	signal?: AbortSignal
): Promise<ApiResult<{ id: string }>> {
	return apiFetch<{ id: string }>('/api/v1/admin/schedules', {
		method: 'POST',
		body: input,
		signal
	});
}

export function updateSchedule(
	id: string,
	input: ScheduleUpdateInput,
	signal?: AbortSignal
): Promise<ApiResult<{ id: string }>> {
	return apiFetch<{ id: string }>(`/api/v1/admin/schedules/${encodeURIComponent(id)}`, {
		method: 'PATCH',
		body: input,
		signal
	});
}

/* ----------------------------- appointments ----------------------------- */

export function listAppointments(
	signal?: AbortSignal
): Promise<ApiResult<AdminAppointment[]>> {
	return apiFetchList<AdminAppointment>('/api/v1/admin/appointments', { signal });
}

export function updateAppointmentStatus(
	id: string,
	status: AppointmentStatus,
	signal?: AbortSignal
): Promise<ApiResult<StatusUpdateResult>> {
	return apiFetch<StatusUpdateResult>(
		`/api/v1/admin/appointments/${encodeURIComponent(id)}/status`,
		{ method: 'PATCH', body: { status }, signal }
	);
}

/* ----------------------------- notifications ---------------------------- */

/**
 * Outbox listing.
 *
 * The query bag is the one place a real backend filter is forwarded; the P1
 * query-forwarding correction made this actually reach the handler, which reads
 * limit, status, channel, template_key, created_from, and created_to.
 */
export interface NotificationQuery {
	limit?: number;
	status?: string;
	channel?: string;
	template_key?: string;
	created_from?: string;
	created_to?: string;
	/**
	 * Index signature so the query bag satisfies the client option type. Every
	 * value is still optional, and `undefined` values are dropped when the URL
	 * is built, so no `?status=` is ever sent for an unset filter.
	 */
	[key: string]: string | number | boolean | undefined | null;
}

export function listNotifications(
	query: NotificationQuery = {},
	signal?: AbortSignal
): Promise<ApiResult<NotificationOutboxRow[]>> {
	return apiFetchList<NotificationOutboxRow>('/api/v1/admin/notifications', {
		signal,
		query
	});
}

export function getNotification(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<NotificationOutboxRow>> {
	return apiFetch<NotificationOutboxRow>(
		`/api/v1/admin/notifications/${encodeURIComponent(id)}`,
		{ signal }
	);
}

/** Counts by status. Always carries all five keys, zero-filled. */
export function getNotificationSummary(
	facilityId?: string,
	signal?: AbortSignal
): Promise<ApiResult<NotificationSummary>> {
	return apiFetch<NotificationSummary>('/api/v1/admin/notifications/summary', {
		signal,
		query: { facility_id: facilityId }
	});
}

/**
 * Retries a failed delivery. Answers 409 when the state machine forbids it,
 * for example a row that is already delivered.
 */
export function retryNotification(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<NotificationOutboxRow>> {
	return apiFetch<NotificationOutboxRow>(
		`/api/v1/admin/notifications/${encodeURIComponent(id)}/retry`,
		{ method: 'POST', signal }
	);
}

/** Cancels a pending delivery. Also answers 409 for a terminal state. */
export function cancelNotification(
	id: string,
	signal?: AbortSignal
): Promise<ApiResult<NotificationOutboxRow>> {
	return apiFetch<NotificationOutboxRow>(
		`/api/v1/admin/notifications/${encodeURIComponent(id)}/cancel`,
		{ method: 'POST', signal }
	);
}
