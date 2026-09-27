/**
 * SIGAP wire types.
 *
 * These mirror the actual Go JSON responses, including the places where the
 * backend is inconsistent. The irregularities are documented rather than
 * papered over, because a type that "fixes" them would compile cleanly and then
 * misread live data.
 *
 * Sources: apps/api/internal/handler/{admin,booking,catalog,patient,queue}.go,
 * apps/api/internal/notification/types.go, and the map literals in
 * apps/api/cmd/server/main.go.
 */

/**
 * Success envelope. Every handled endpoint answers with this shape via
 * `writeJSON`, which is `json.NewEncoder(...).Encode` — hence the trailing
 * newline the client has to tolerate.
 */
export interface ApiEnvelope<T> {
	success: true;
	data: T;
}

/**
 * Error envelope. Flat: no `data` key at all, only `success` and `error`.
 *
 * Written four separate ways in the backend (writeError, writeAuthzError,
 * DenyByDefault, plus bare status codes), but all handled errors use this pair.
 */
export interface ApiErrorEnvelope {
	success: false;
	error: string;
}

/**
 * The wire type of a list `data` field: it can genuinely be `null`.
 *
 * Most list handlers in the Go API declare `var results []T` and only `append`
 * inside the scan loop, so an authorized request that matches zero rows encodes
 * a nil slice as `null` — see writeJSON, which is
 * `json.NewEncoder(...).Encode` with no nil-to-empty normalization. The
 * fail-closed early return in the same handlers passes a composite literal and
 * therefore does emit `[]`, so both shapes are reachable on the same route.
 *
 * `admin/notifications` is the one exception that always sends `[]`, because
 * its service allocates with `[]T{}` on every path.
 *
 * This type exists so the reality is visible in the signature. Endpoints do not
 * return `NullableList` to callers: they normalize with `asList`, so route code
 * receives a real array and never has to null-check a collection.
 */
export type NullableList<T> = T[] | null;

/** Queue ticket status, per validQueueTransitions. */
export type QueueStatus =
	| 'waiting'
	| 'called'
	| 'in_service'
	| 'completed'
	| 'cancelled'
	| 'skipped';

/** Appointment status, per validAppointmentTransitions. */
export type AppointmentStatus =
	| 'scheduled'
	| 'checked_in'
	| 'queued'
	| 'completed'
	| 'cancelled'
	| 'no_show';

/* ------------------------------------------------------------------ *
 * Public (unauthenticated) catalog
 * ------------------------------------------------------------------ */

/**
 * Facility classification, exactly as it exists on the wire.
 *
 * These are the two values of the database `facility_type` enum, projected
 * verbatim by catalog.go. The type is a union rather than `string` on purpose:
 * a third value cannot be added to the frontend without the compiler
 * complaining, which is the pressure that keeps this list honest against the
 * schema instead of letting it drift.
 *
 * There is deliberately no derivation from `name` or `short_code`. "RS Foo"
 * is not a verified classification, and a citizen filtering on a guess could
 * rule out their own clinic over a naming convention.
 */
export type PublicFacilityType = 'puskesmas' | 'rumah_sakit';

/**
 * Intentionally minimal: the public catalog exposes no address, phone, or bed
 * counts. See catalog.go, where those fields are deliberately absent.
 *
 * `type` is the one classification field that is public, because a citizen
 * choosing where to go needs to tell a district clinic from a hospital. It is
 * the existing column, added read-only, with no new endpoint and no new query
 * parameter behind it.
 */
export interface PublicFacility {
	id: string;
	name: string;
	short_code: string;
	type: PublicFacilityType;
	is_active: boolean;
}

export interface PublicServiceUnit {
	id: string;
	facility_id: string;
	name: string;
	code: string;
	is_active: boolean;
}

/* ------------------------------------------------------------------ *
 * Patient portal (unauthenticated, code-keyed)
 * ------------------------------------------------------------------ */

/**
 * `found_by` is "checkin_code" or "formatted_number", depending on which
 * lookup matched.
 *
 * The three queue fields are `omitempty` pointers in Go, so they are OMITTED
 * entirely when the LEFT JOIN found no ticket. They are optional here, not
 * nullable, because `undefined` and `null` are different states on the wire.
 */
export interface PatientStatus {
	found_by: 'checkin_code' | 'formatted_number';
	facility_name: string;
	appointment_status: AppointmentStatus;
	appointment_time: string;
	/** Derived label, not the raw status: e.g. "not_checked_in", "in_queue". */
	checkin_status: string;
	queue_number?: number;
	queue_status?: QueueStatus;
	queue_formatted_number?: string;
}

/* ------------------------------------------------------------------ *
 * Booking and check-in (unauthenticated)
 * ------------------------------------------------------------------ */

export interface BookAppointmentResult {
	id: string;
	checkin_code: string;
	status: 'scheduled';
	appointment_time: string;
}

/**
 * Check-in response.
 *
 * NOTE the key casing. This handler emits an untyped map, and its keys stay
 * snake_case — but see QueueGenerateResult for the one place it does not.
 */
export interface CheckInResult {
	appointment_id: string;
	queue_ticket_id: string;
	formatted_number: string;
	/** Always the literal "queued" on success. */
	status: 'queued';
	estimated_wait_minutes: number;
	processing_time: string;
}

/**
 * POST /api/v1/queues/generate result.
 *
 * WARNING: `TicketID` and `status` here are PascalCase in the JSON. The Go
 * struct has no tag on TicketID, and Go marshals the untagged `Status` field
 * under its own name. Every sibling field is snake_case. This is the single
 * most likely source of frontend bugs against this endpoint, so the casing is
 * reproduced exactly here rather than normalized away.
 */
export interface QueueGenerateResult {
	TicketID: string;
	formatted_number: string;
	Status: QueueStatus;
	registered_at: string;
	estimated_wait_minutes: number;
	processing_time: string;
	signature: string;
}

/* ------------------------------------------------------------------ *
 * Admin: facilities, queues, service units, schedules, appointments
 * ------------------------------------------------------------------ */

/**
 * `created_at`/`updated_at` are omitempty and the list query does not select
 * them, so they are present on detail and ABSENT on list. `address` likewise
 * comes back as "" from the list query.
 */
export interface AdminFacility {
	id: string;
	name: string;
	type: 'rumah_sakit' | 'puskesmas';
	address: string;
	kecamatan: string;
	kabupaten_kota: string;
	provinsi: string;
	phone: string;
	total_beds: number;
	available_beds: number;
	is_active: boolean;
	short_code: string;
	created_at?: string;
	updated_at?: string;
}

export interface AdminQueueTicket {
	id: string;
	facility_id: string;
	queue_number: number;
	formatted_number: string;
	status: QueueStatus;
	registered_at: string;
	called_at?: string;
	completed_at?: string;
}

/** `code` and `description` are omitempty pointers, so they can vanish. */
export interface AdminServiceUnit {
	id: string;
	facility_id: string;
	name: string;
	code?: string;
	description?: string;
	is_active: boolean;
	created_at?: string;
	updated_at?: string;
}

/**
 * `practitioner_id` is a plain string with omitempty, so a NULL practitioner
 * drops the key. Dates and times arrive as Postgres text casts, hence
 * "YYYY-MM-DD" and "HH:MM:SS" rather than RFC3339.
 */
export interface AdminSchedule {
	id: string;
	facility_id: string;
	practitioner_id?: string;
	service_unit_id: string;
	schedule_date: string;
	start_time: string;
	end_time: string;
	slot_minutes: number;
	capacity_per_slot: number;
	is_active: boolean;
	created_at?: string;
	updated_at?: string;
}

/**
 * The one PHI-bearing type. Gated behind appointment.read on the server.
 *
 * `notes` is declared on the Go struct but never selected by the list query,
 * so it is effectively always absent today.
 */
export interface AdminAppointment {
	id: string;
	facility_id: string;
	service_unit_id: string;
	practitioner_id?: string;
	practitioner_schedule_id?: string;
	appointment_time?: string;
	status: AppointmentStatus;
	patient_display_name: string;
	checkin_code?: string;
	queue_ticket_id?: string;
	notes?: string;
	created_at?: string;
	updated_at?: string;
}

/** Response of a status PATCH, which is an untyped map on the server. */
export interface StatusUpdateResult {
	id: string;
	status: QueueStatus | AppointmentStatus;
	updated_at: string;
}

/**
 * Facility deactivate response. `is_active` is a STRING here, not a boolean,
 * because the handler builds a map[string]string. Reproduced as-is; the client
 * coerces at the call site rather than lying about the wire type.
 */
export interface FacilityDeactivateResult {
	id: string;
	is_active: 'false' | 'true';
}

/* ------------------------------------------------------------------ *
 * Admin: notifications
 * ------------------------------------------------------------------ */

export type NotificationChannel = 'dev' | 'sms' | 'whatsapp' | 'email';
export type NotificationStatus =
	| 'pending'
	| 'processing'
	| 'delivered'
	| 'failed'
	| 'cancelled';
export type NotificationRecipientType = 'patient' | 'staff' | 'facility_admin';

/**
 * Privacy-critical. The internal `RecipientContactHash` dedup key is
 * deliberately absent; only the masked contact is exposed.
 */
export interface NotificationOutboxRow {
	id: string;
	facility_id?: string;
	channel: NotificationChannel;
	template_key: string;
	subject: string;
	body_template: string;
	recipient_type: NotificationRecipientType;
	recipient_contact_masked: string;
	status: NotificationStatus;
	attempt_count: number;
	next_attempt_at: string;
	last_error_code?: string;
	related_resource_type?: string;
	related_resource_id?: string;
	created_at: string;
	updated_at: string;
}

/**
 * Summary counts. A flat map keyed by status, not a struct.
 *
 * All five keys are always present and zero-filled by ZeroSummary, so the card
 * set can render in one pass.
 */
export interface NotificationSummary {
	pending: number;
	processing: number;
	delivered: number;
	failed: number;
	cancelled: number;
}

/* ------------------------------------------------------------------ *
 * Health
 * ------------------------------------------------------------------ */

export interface HealthStatus {
	status: string;
	service: string;
}

export interface ReadyStatus extends HealthStatus {
	audit?: string;
	detail?: string;
}
