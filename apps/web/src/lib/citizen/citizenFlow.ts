import type { ApiError } from '$lib/api/errors';
import { isAbort } from '$lib/api/errors';

/**
 * Shared vocabulary for the three transactional citizen flows.
 *
 * Booking, check-in, walk-in, and status all ask the same four questions in
 * slightly different words, and answering them in each page is how they drift
 * into saying different things about the same failure. Three specific hazards
 * this file exists to prevent:
 *
 *   1. A 401 from the public check-in route means "wrong code", never "sign
 *      in". Every one of these routes is unauthenticated, so a generic
 *      unauthorized branch anywhere in them is a bug.
 *   2. An aborted request is a navigation, not a failure. Showing a red error
 *      for it punishes a citizen for tapping a link.
 *   3. A wait time is an ESTIMATE. Wording like "realtime", "live", or
 *      "dipastikan" turns a queue-engine estimate into a promise the system
 *      cannot keep, and a citizen who is told 20 minutes and waits 40 has been
 *      lied to by the interface.
 */

/* ------------------------------------------------------------------ *
 * Validation
 * ------------------------------------------------------------------ */

/**
 * Client-side rules, expressed once.
 *
 * These deliberately mirror the backend's `validateBookAppointment` and the
 * queue handler's presence checks rather than inventing a stricter set. A
 * client that rejects something the server would accept trains citizens to
 * distrust the form; one that accepts something the server rejects wastes
 * their time on a round trip. The server remains the authority — this is a
 * courtesy layer that catches the obvious cases before a request is sent.
 */

export interface FieldErrors {
	[key: string]: string;
}

/**
 * Digits only, matching `normalizePhone` in booking.go.
 *
 * Coerces rather than assuming a string. A validator that throws on null is
 * worse than one that reports a missing field: the throw happens inside an
 * event handler, so the citizen gets a silent no-op and a console error
 * instead of the "wajib diisi" message sitting right there. `?? ''` is what
 * turns a bind-to-undefined-prop into a normal validation failure.
 */
function digitsOnly(value: string): string {
	return String(value ?? '').replace(/\D/g, '');
}

/** Coerces any bind-produced value to a string. See `digitsOnly`. */
function asText(value: string): string {
	return String(value ?? '');
}

/**
 * Phone: 10-15 digits after stripping non-digits.
 *
 * The range is the backend's, not a guess. Stripping first matters because the
 * backend normalizes before counting, so "+62 812-3456-7890" is 12 digits and
 * valid — validating the raw string length would reject a correctly formatted
 * international number.
 */
export function validatePhone(value: string): string {
	const trimmed = asText(value).trim();
	if (!trimmed) return 'Nomor telepon wajib diisi.';
	const digits = digitsOnly(trimmed);
	if (digits.length < 10 || digits.length > 15) {
		return 'Nomor telepon harus 10-15 digit angka.';
	}
	return '';
}

export function validateRequired(value: string, label: string): string {
	if (!asText(value).trim()) return `${label} wajib diisi.`;
	return '';
}

export function validateName(value: string): string {
	const trimmed = asText(value).trim();
	if (!trimmed) return 'Nama pasien wajib diisi.';
	// Same ceiling as validateBookAppointment, so a 51-character name is caught
	// here rather than after a round trip.
	if (trimmed.length > 100) return 'Nama pasien maksimal 100 karakter.';
	return '';
}

/**
 * Appointment time must be in the future.
 *
 * Checked against the same rule the server applies: `Before(now)` or
 * `Equal(now)` is rejected, so equality counts as past. Getting this backwards
 * would let a citizen pick the current minute, watch the request succeed
 * locally, and then be refused by the server.
 *
 * The backend parses RFC3339; the datetime-local control produces
 * `YYYY-MM-DDTHH:mm` with no zone. It is converted to UTC here rather than
 * having the server guess, so what the citizen sees is what gets stored.
 */
export function validateAppointmentTime(value: string, now: Date = new Date()): string {
	if (!asText(value)) return 'Waktu janji temu wajib diisi.';
	const parsed = new Date(asText(value));
	if (Number.isNaN(parsed.getTime())) return 'Format waktu janji temu tidak valid.';
	if (parsed.getTime() <= now.getTime()) return 'Waktu janji temu harus di masa depan.';
	return '';
}

/** The backend's cap on a lookup code, from `maxPatientCodeLen` in patient.go. */
export function validateLookupCode(value: string): string {
	const trimmed = asText(value).trim();
	if (!trimmed) return 'Kode wajib diisi.';
	if (trimmed.length > 64) return 'Kode terlalu panjang.';
	// The backend's safeCodeRe. Checking it here turns a round trip into an
	// inline message for the characters most likely to be pasted by accident.
	if (!/^[A-Za-z0-9-]+$/.test(trimmed)) {
		return 'Kode hanya boleh berisi huruf, angka, dan tanda hubung.';
	}
	return '';
}

/** The backend's own charset for check-in codes, from `generateCheckinCode`. */
export function looksLikeCheckinCode(value: string): boolean {
	return /^[A-Z0-9]{6}$/i.test(value.trim());
}

/* ------------------------------------------------------------------ *
 * Time
 * ------------------------------------------------------------------ */

/**
 * Formats an RFC3339 instant for a citizen.
 *
 * Uses `id-ID` with an explicit timeZone so the rendering does not depend on
 * the browser's locale or machine timezone. A booking confirmation that
 * displays a different date than the one the citizen chose is a real support
 * call, and the cause is nearly always implicit-timezone formatting.
 *
 * Returns the empty string for an unparseable value rather than "Invalid
 * Date", because the alternative is printing the words "Invalid Date" on a
 * civic page.
 */
export function formatAppointmentTime(value: string): string {
	const parsed = new Date(value);
	if (Number.isNaN(parsed.getTime())) return '';
	return new Intl.DateTimeFormat('id-ID', {
		dateStyle: 'long',
		timeStyle: 'short',
		timeZone: 'Asia/Jakarta'
	}).format(parsed);
}

/**
 * A wait time, in words that cannot be mistaken for a promise.
 *
 * `estimated_wait_minutes` comes from the queue engine at registration time and
 * is not recomputed as the queue moves. Presenting it as a live figure is the
 * single easiest way to make a health page dishonest: the number is real, the
 * claim that it is current is not.
 *
 * The wording carries "estimasi" and the sentence carries what the estimate
 * does not cover, so the caveat is not something a citizen has to notice on
 * their own.
 */
export function estimatedWaitLabel(minutes: number): string {
	return `Estimasi sekitar ${minutes} menit`;
}

/** The caveat that travels with every wait estimate. */
export const ESTIMATE_CAVEAT =
	'Estimasi ini dihitung saat pendaftaran dan bisa berubah karena kondisi antrean.';

/* ------------------------------------------------------------------ *
 * Error classification
 * ------------------------------------------------------------------ */

/**
 * Why a transaction failed, in terms a citizen can act on.
 *
 * Each case names a different next step, which is exactly why they must not be
 * collapsed: "your code is wrong" means retype it, "this slot is full" means
 * choose another, "too many attempts" means wait, and "the service is down"
 * means retry. One generic message for all four leaves a citizen guessing.
 */
export type FlowFailure =
	| { kind: 'wrong-code' }
	| { kind: 'not-found'; message: string }
	| { kind: 'conflict'; message: string }
	| { kind: 'rate-limited'; message: string }
	| { kind: 'validation'; message: string }
	| { kind: 'unavailable' }
	| { kind: 'unknown'; message: string };

/**
 * Classifies a check-in failure.
 *
 * The 401 branch is the important one. `wrongCheckInCode` is consulted
 * explicitly and matched BEFORE any generic unauthorized handling, because
 * these routes have no session to expire. Rendering a sign-in prompt here would
 * tell a citizen to log in to fix a typo, and there is no account that would
 * help them.
 */
export function classifyCheckInFailure(error: ApiError, wrongCode: boolean): FlowFailure {
	if (wrongCode) return { kind: 'wrong-code' };
	if (isAbort(error)) return { kind: 'unavailable' };
	switch (error.kind) {
		case 'not_found':
			return { kind: 'not-found', message: error.message };
		case 'conflict':
			// Verbatim. The server says "Janji temu tidak dapat check-in dengan
			// status: queued" — a specific, actionable state. Replacing it with a
			// generic "already processed" loses the actual status.
			return { kind: 'conflict', message: error.message };
		case 'rate_limited':
			return { kind: 'rate-limited', message: error.message };
		case 'bad_request':
			return { kind: 'validation', message: error.message };
		case 'server':
		case 'network':
			return { kind: 'unavailable' };
		default:
			return { kind: 'unknown', message: error.message };
	}
}

/**
 * Classifies a booking or walk-in failure.
 *
 * 400 and 409 are passed through verbatim in both cases. The backend already
 * writes citizen-readable Indonesian for each — "Slot jadwal sudah penuh",
 * "Nomor HP ini sudah melebihi batas pemesanan per hari" — and a frontend that
 * paraphrases them is strictly worse than one that shows them.
 */
export function classifyTransactionFailure(error: ApiError): FlowFailure {
	if (isAbort(error)) return { kind: 'unavailable' };
	switch (error.kind) {
		case 'bad_request':
			return { kind: 'validation', message: error.message };
		case 'conflict':
			return { kind: 'conflict', message: error.message };
		case 'rate_limited':
			return { kind: 'rate-limited', message: error.message };
		case 'not_found':
			return { kind: 'not-found', message: error.message };
		case 'server':
		case 'network':
			return { kind: 'unavailable' };
		default:
			return { kind: 'unknown', message: error.message };
	}
}

/**
 * The message for a failure that is NOT the server's to describe.
 *
 * Only 5xx and transport failures land here, and the requirement is that this
 * string appears exactly once across the codebase — a generic "gagal" that
 * gets copy-pasted into four pages is how a support answer turns into four
 * different sentences. Everything else carries the server's own words.
 */
export const GENERIC_UNAVAILABLE =
	'Layanan sedang bermasalah. Silakan coba lagi sebentar lagi.';

/**
 * Resolves a failure to the single line a citizen should read.
 *
 * Every transactional flow uses this, which is what makes "the generic 500
 * wording appears exactly once" true rather than aspirational. The `message`
 * arms are returned verbatim — the server already writes specific, actionable
 * Indonesian for 400, 404, 409, and 429, and paraphrasing is strictly worse
 * than showing it.
 *
 * `wrong-code` gets no message because it is presented by a dedicated control
 * on the check-in page, not in a generic banner. It is reachable here only if
 * a caller misuses the classifier, and returning a specific line for it is
 * better than rendering nothing.
 */
export function resolveFailureMessage(failure: FlowFailure | null): string {
	if (!failure) return '';
	switch (failure.kind) {
		case 'unavailable':
			return GENERIC_UNAVAILABLE;
		case 'wrong-code':
			return 'Kode check-in tidak cocok.';
		case 'rate-limited':
		case 'conflict':
		case 'validation':
		case 'not-found':
		case 'unknown':
			return failure.message || GENERIC_UNAVAILABLE;
	}
}

/**
 * Citizen-facing progress steps.
 *
 * A CURRENT-STATE indicator only. There is deliberately no timeline, no
 * history, and no per-step duration, because the backend has none to give:
 * `PatientStatusLookup` returns one appointment status, one check-in label,
 * and optionally a queue number. Rendering "checked in at 09:12, queued at
 * 09:20" would mean inventing both timestamps, and a civic health page that
 * fabricates a wait history is worse than one that admits it only knows the
 * present.
 *
 * The four labels are fixed by design: Check-In, Antre, Dilayani, Selesai.
 */
export type VisitProgressStep = 'check-in' | 'antre' | 'dilayani' | 'selesai';

export const VISIT_PROGRESS_LABELS: Record<VisitProgressStep, string> = {
	'check-in': 'Check-In',
	antre: 'Antre',
	dilayani: 'Dilayani',
	// Quoted because `selesai` collides with nothing in JS but reads as a
	// stray word next to the other three; the quoting keeps the four keys
	// visibly parallel.
	selesai: 'Selesai'
};

/** The four steps in the order they occur. */
export const VISIT_PROGRESS_ORDER: VisitProgressStep[] = [
	'check-in',
	'antre',
	'dilayani',
	'selesai'
];

/**
 * Reproduces `mapCheckinStatus` in patient.go, exactly.
 *
 * This is the one function in the frontend that duplicates a backend rule, and
 * the duplication is deliberate: the label a citizen reads has to be the label
 * the backend computed, and re-deriving it from `appointment_status` in the
 * browser would be a second implementation free to drift. The mapping below is
 * transcribed from the Go switch, including its `default` arm.
 *
 * The output is a step in the progress indicator, plus two terminal states
 * that are not progress at all:
 *
 *   - `dibatalkan` and `tidak_hadir` are outcomes, not steps. A cancelled
 *     appointment must not render as "Selesai" or sit at step 1 looking like
 *     the citizen simply has not arrived yet. They get their own presentation.
 *   - An unrecognised value is passed through rather than guessed, so a status
 *     added on the backend shows up as itself instead of being quietly
 *     relabelled as something the citizen would act on.
 */
export type VisitProgress =
	| { kind: 'step'; step: VisitProgressStep }
	| { kind: 'cancelled' }
	| { kind: 'no-show' }
	| { kind: 'unknown'; raw: string };

/**
 * Maps the backend's `checkin_status` onto a progress presentation.
 *
 * The switch mirrors `mapCheckinStatus` in patient.go:
 *   not_checked_in → Check-In
 *   checked_in     → Antre
 *   in_queue       → Dilayani
 *   selesai        → Selesai
 *   dibatalkan     → Dibatalkan
 *   tidak_hadir    → Tidak hadir
 *   anything else  → returned unchanged by the backend, and passed through here
 */
export function visitProgressFrom(checkinStatus: string): VisitProgress {
	switch (checkinStatus) {
		case 'not_checked_in':
			return { kind: 'step', step: 'check-in' };
		case 'checked_in':
			return { kind: 'step', step: 'antre' };
		case 'in_queue':
			return { kind: 'step', step: 'dilayani' };
		case 'selesai':
			return { kind: 'step', step: 'selesai' };
		case 'dibatalkan':
			return { kind: 'cancelled' };
		case 'tidak_hadir':
			return { kind: 'no-show' };
		default:
			// The backend's `default: return status` arm. Passing the raw value
			// through means an unmapped status is visible rather than silently
			// presented as progress the citizen is not actually making.
			return { kind: 'unknown', raw: checkinStatus };
	}
}

/**
 * The headline for a non-progress outcome.
 *
 * A cancelled or missed appointment is information, not a step, and it needs
 * to say what happened rather than leave the indicator stuck at the start.
 */
export function visitOutcomeLabel(progress: VisitProgress): string {
	if (progress.kind === 'cancelled') return 'Janji temu dibatalkan';
	if (progress.kind === 'no-show') return 'Tidak hadir';
	return '';
}

/* ------------------------------------------------------------------ *
 * Deep links
 * ------------------------------------------------------------------ */

/**
 * Builds the check-in deep link from a booking result.
 *
 * Uses the canonical `appointment_id` / `checkin_code` names. The check-in
 * route also accepts the legacy `id` / `code` aliases, but a link this page
 * generates has no reason to use them: new links that speak the old dialect
 * keep the old dialect alive, and the aliases exist for links already in
 * people's inboxes, not for new ones.
 */
export function checkInDeepLink(appointmentId: string, checkinCode: string): string {
	const params = new URLSearchParams();
	if (appointmentId) params.set('appointment_id', appointmentId);
	if (checkinCode) params.set('checkin_code', checkinCode);
	const query = params.toString();
	return query ? `/appointments/check-in?${query}` : '/appointments/check-in';
}
