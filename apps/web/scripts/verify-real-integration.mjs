/**
 * Real end-to-end integration verification for the SIGAP V2.2 local stack.
 *
 * This script exists because the previous round of evidence was produced with
 * INTERCEPTED API responses: the database was down, so the admin read states
 * were verified by feeding controlled payloads into the browser. That proves
 * the presentation layer renders each state correctly, and it proves nothing
 * about whether the stack works. This script is the other half — every request
 * below reaches the real SvelteKit server, the real Go API, and the real
 * PostgreSQL database. Nothing is mocked, stubbed, or intercepted.
 *
 * TWO BASES, ON PURPOSE
 *
 *   WEB   http://127.0.0.1:5173   the browser's actual path. The /api/v1
 *                                 proxies attach the dev actor server-side from
 *                                 the process environment, so this is exactly
 *                                 what a browser gets.
 *   API   http://127.0.0.1:18080  the Go service directly. Used ONLY for the
 *                                 authorization cases that need a DIFFERENT
 *                                 actor or NO actor. The web proxy deliberately
 *                                 cannot be told to change identity from a
 *                                 request — that is the security property — so
 *                                 those cases have to bypass it to be tested.
 *
 * Every row this script creates is disposable and tagged so it can be found and
 * removed: patient names start with `VERIFY-`, and the script reports the ids it
 * created. It refuses to run against a non-loopback host.
 *
 * Usage:
 *   node scripts/verify-real-integration.mjs [webBase] [apiBase]
 */

const WEB = (process.argv[2] ?? 'http://127.0.0.1:5173').replace(/\/$/, '');
const API = (process.argv[3] ?? 'http://127.0.0.1:18080').replace(/\/$/, '');

const LOOPBACK = /^https?:\/\/(127\.0\.0\.1|localhost|\[::1\])(:\d+)?$/;
if (!LOOPBACK.test(WEB) || !LOOPBACK.test(API)) {
	throw new Error(`Refusing to run against non-loopback hosts: WEB=${WEB} API=${API}`);
}

const results = [];
let createdAppointmentId = null;
let createdQueueId = null;

function record(name, ok, detail) {
	results.push({ name, ok, detail });
	const tag = ok ? 'PASS' : 'FAIL';
	console.log(`  [${tag}] ${name}${detail ? ` — ${detail}` : ''}`);
}

async function call(url, init = {}) {
	const res = await fetch(url, init);
	const text = await res.text();
	let body = null;
	try {
		body = text ? JSON.parse(text) : null;
	} catch {
		body = text;
	}
	return { status: res.status, body, headers: res.headers };
}

function postJson(url, payload, headers = {}) {
	return call(url, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json', ...headers },
		body: JSON.stringify(payload)
	});
}

function patchJson(url, payload, headers = {}) {
	return call(url, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json', ...headers },
		body: JSON.stringify(payload)
	});
}

/** A slot tomorrow, so the booking is always in the future regardless of run time. */
function tomorrowSlot() {
	const d = new Date();
	d.setUTCDate(d.getUTCDate() + 1);
	return `${d.toISOString().slice(0, 10)}T09:30:00Z`;
}

/**
 * A phone unique to this run.
 *
 * `uq_active_booking_per_patient_day` allows one active booking per
 * (patient phone, facility, day). A fixed number made the second run of this
 * script fail with a 500 on a duplicate key — the server was right and the
 * test was wrong. A timestamp suffix keeps every run independent.
 */
const RUN_ID = Date.now().toString().slice(-8);
const PHONE = `0812${RUN_ID}`;

async function main() {
	console.log(`\nReal integration: WEB=${WEB}  API=${API}\n`);

	/* ------------------------------ citizen ------------------------------ */

	console.log('Citizen');

	const facilities = await call(`${WEB}/api/v1/public/facilities`);
	const facilityList = Array.isArray(facilities.body?.data) ? facilities.body.data : [];
	record(
		'facility discovery',
		facilities.status === 200 && facilityList.length > 0,
		`${facilities.status}, ${facilityList.length} facilities`
	);

	// The service-unit endpoint answered 500 on the live database before this
	// session: `code` is nullable and the handler scanned it into a string.
	// This assertion is the regression guard for that fix.
	const units = await call(`${WEB}/api/v1/public/service-units`);
	const unitList = Array.isArray(units.body?.data) ? units.body.data : [];
	const codesAreStrings = unitList.every((u) => typeof u.code === 'string');
	record(
		'service-unit catalog (nullable code)',
		units.status === 200 && unitList.length > 0 && codesAreStrings,
		`${units.status}, ${unitList.length} units, all codes strings=${codesAreStrings}`
	);

	const facilityId = facilityList[0]?.id;
	const unit = unitList.find((u) => u.facility_id === facilityId) ?? unitList[0];

	// Validation: an empty name must be refused, not silently accepted.
	const invalid = await postJson(`${WEB}/api/v1/appointments`, {
		facility_id: facilityId,
		service_unit_id: unit?.id,
		patient_display_name: '',
		patient_phone: PHONE,
		appointment_time: tomorrowSlot()
	});
	record(
		'booking validation (empty name refused)',
		invalid.status === 400,
		`${invalid.status}`
	);

	const booking = await postJson(`${WEB}/api/v1/appointments`, {
		facility_id: facilityId,
		service_unit_id: unit?.id,
		patient_display_name: `VERIFY-Integration-${RUN_ID}`,
		patient_phone: PHONE,
		appointment_time: tomorrowSlot(),
		notes: 'disposable integration record'
	});
	const appointmentId = booking.body?.data?.id ?? booking.body?.id;
	const checkinCode = booking.body?.data?.checkin_code ?? booking.body?.checkin_code;
	createdAppointmentId = appointmentId ?? null;
	// The handler answers 200, not 201: it returns the created record in the
	// success envelope rather than a Location. Asserting 201 would fail against
	// a correct server.
	record(
		'appointment booking',
		booking.status === 200 && Boolean(appointmentId) && Boolean(checkinCode),
		`${booking.status}, id=${appointmentId ? 'yes' : 'no'}, checkin_code=${checkinCode ? 'issued' : 'missing'}`
	);

	// There is NO public GET for a single appointment — PublicAppointmentsRouter
	// handles POST only, and the confirmation card is rendered from the booking
	// response itself. Persistence is therefore proven by a different read path
	// later (the appointment must appear in the admin list), not by fetching it
	// back here.

	// Wrong code first: the backend answers 401 for a code mismatch, which is
	// NOT an auth failure and must not be presented as one.
	if (appointmentId) {
		const wrong = await postJson(
			`${WEB}/api/v1/appointments/${encodeURIComponent(appointmentId)}/check-in`,
			{ checkin_code: 'ZZZZZZ' }
		);
		record('check-in rejects a wrong code', wrong.status === 401, `${wrong.status}`);
	}

	const checkin = appointmentId
		? await postJson(`${WEB}/api/v1/appointments/${encodeURIComponent(appointmentId)}/check-in`, {
				checkin_code: checkinCode
			})
		: { status: 0, body: null };
	const checkinBody = checkin.body?.data ?? checkin.body;
	createdQueueId = checkinBody?.queue_ticket_id ?? null;
	record(
		'check-in with appointment id + code',
		checkin.status === 200 && checkinBody?.status === 'queued',
		`${checkin.status}, ticket=${checkinBody?.formatted_number ?? 'none'}`
	);

	// Re-check-in must be refused: the appointment is already queued.
	if (appointmentId) {
		const again = await postJson(
			`${WEB}/api/v1/appointments/${encodeURIComponent(appointmentId)}/check-in`,
			{ checkin_code: checkinCode }
		);
		record(
			'double check-in refused',
			again.status === 409 || again.status === 400,
			`${again.status}`
		);
	}

	const walkin = await postJson(`${WEB}/api/v1/queues/generate`, {
		facilityId,
		patient: { fullName: `VERIFY-WalkIn-${RUN_ID}`, phone: PHONE }
	});
	const walkinTicket = walkin.body?.TicketID ?? walkin.body?.data?.TicketID;
	if (walkinTicket) createdQueueId = walkinTicket;
	record(
		'walk-in queue creation',
		walkin.status === 200 || walkin.status === 201,
		`${walkin.status}, ticket=${walkin.body?.formatted_number ?? walkin.body?.data?.formatted_number ?? 'none'}`
	);

	if (checkinCode) {
		const status = await call(
			`${WEB}/api/v1/patient/status?code=${encodeURIComponent(checkinCode)}`
		);
		const found = status.body?.data?.found_by;
		record(
			'patient status by check-in code',
			status.status === 200 && found === 'checkin_code',
			`${status.status}, found_by=${found}`
		);
	}

	const unknown = await call(`${WEB}/api/v1/patient/status?code=NOSUCHCODE`);
	record(
		'patient status refuses an unknown code',
		unknown.status === 404,
		`${unknown.status}`
	);

	/* ------------------------------- admin ------------------------------- */

	console.log('\nAdmin (browser path, dev actor from the server process)');

	const adminQueues = await call(`${WEB}/api/v1/admin/queues`);
	const queueRows = Array.isArray(adminQueues.body?.data) ? adminQueues.body.data : [];
	record(
		'queue listing',
		adminQueues.status === 200 && queueRows.length > 0,
		`${adminQueues.status}, ${queueRows.length} rows`
	);

	for (const [label, path] of [
		['appointment management', '/api/v1/admin/appointments'],
		['facility management', '/api/v1/admin/facilities'],
		['schedule management', '/api/v1/admin/schedules'],
		['notifications', '/api/v1/admin/notifications']
	]) {
		const res = await call(`${WEB}${path}`);
		const rows = Array.isArray(res.body?.data) ? res.body.data.length : 'n/a';
		record(label, res.status === 200, `${res.status}, rows=${rows}`);
	}

	// A real permitted transition on a row this script owns, so the assertion
	// cannot disturb an operator's data. The successor is read from the row's
	// ACTUAL status: check-in reports `status: "queued"` in its response, but
	// the stored status is `waiting`, so assuming the response value here would
	// probe a state that does not exist.
	const NEXT = {
		waiting: 'called',
		called: 'in_service',
		in_service: 'completed'
	};
	const target = queueRows.find((q) => q.id === createdQueueId) ?? null;
	if (target) {
		const current = target.status;
		const next = NEXT[current];
		if (next) {
			const moved = await patchJson(
				`${WEB}/api/v1/admin/queues/${encodeURIComponent(target.id)}/status`,
				{ status: next }
			);
			record(`queue transition ${current} -> ${next}`, moved.status === 200, `${moved.status}`);
		} else {
			record(`queue transition from ${current}`, true, 'terminal state, no transition applicable');
		}
	} else {
		record('queue transition', false, 'newly created ticket not visible in the admin list');
	}

	// An illegal transition must be refused by the state machine, not silently
	// accepted. `waiting -> completed` skips the call and the service.
	if (target && target.status === 'waiting') {
		const bad = await patchJson(
			`${WEB}/api/v1/admin/queues/${encodeURIComponent(target.id)}/status`,
			{ status: 'completed' }
		);
		record(
			'illegal queue transition refused',
			bad.status === 409 || bad.status === 400 || bad.status === 422,
			`${bad.status}`
		);
	}

	console.log('\nAuthorization (direct API, alternate actors)');

	// With the RBAC selector armed, a request that names nobody resolves to a
	// zero actor. The admin routes answer 403 rather than 401: the surface does
	// not distinguish "no session" from "insufficient permission" to the client,
	// which is deliberate. The assertion is that it is REFUSED, not the code.
	const noActor = await call(`${API}/api/v1/admin/queues`);
	record(
		'admin read without an actor is refused',
		noActor.status === 401 || noActor.status === 403,
		`${noActor.status}`
	);

	// The dev-identity header is a DIFFERENT mechanism from the RBAC selector.
	// While the selector is armed it takes precedence, so a client-supplied
	// X-Sigap-Dev-User-ID must not authenticate anyone — that is the property
	// the web tier relies on when it refuses to forward browser headers.
	const forged = await call(`${API}/api/v1/admin/queues`, {
		headers: { 'X-Sigap-Dev-User-ID': '00000000-0000-0000-0000-00000000d990' }
	});
	record(
		'client-supplied dev user id is ignored while the selector is armed',
		forged.status === 401 || forged.status === 403,
		`${forged.status}`
	);

	const zeroScope = await call(`${API}/api/v1/admin/queues`, {
		headers: { 'X-Sigap-Local-Test-Subject': 'local-zero-scope-admin' }
	});
	const zeroRows = Array.isArray(zeroScope.body?.data) ? zeroScope.body.data.length : null;
	record(
		'zero-scope admin sees no rows',
		zeroScope.status === 200 && zeroRows === 0,
		`${zeroScope.status}, rows=${zeroRows}`
	);

	const globalScope = await call(`${API}/api/v1/admin/queues`, {
		headers: { 'X-Sigap-Local-Test-Subject': 'local-global-super-admin' }
	});
	const globalRows = Array.isArray(globalScope.body?.data) ? globalScope.body.data.length : 0;
	record(
		'global admin sees rows across facilities',
		globalScope.status === 200 && globalRows >= queueRows.length,
		`${globalScope.status}, rows=${globalRows}`
	);

	// Facility scoping, checked on the facilities list rather than on queues.
	// The seeded facility admin is granted one facility, and that facility has
	// no queue tickets — so a queue-based assertion would pass on an empty set
	// and prove nothing. The facilities list is where the grant is visible.
	const scopedFacilitiesRes = await call(`${API}/api/v1/admin/facilities`, {
		headers: { 'X-Sigap-Local-Test-Subject': 'local-facility-admin' }
	});
	const scopedFacilityRows = Array.isArray(scopedFacilitiesRes.body?.data)
		? scopedFacilitiesRes.body.data
		: [];
	const globalFacilitiesRes = await call(`${API}/api/v1/admin/facilities`, {
		headers: { 'X-Sigap-Local-Test-Subject': 'local-global-super-admin' }
	});
	const globalFacilityRows = Array.isArray(globalFacilitiesRes.body?.data)
		? globalFacilitiesRes.body.data
		: [];
	record(
		'facility admin is scoped to its granted facility',
		scopedFacilitiesRes.status === 200 &&
			scopedFacilityRows.length === 1 &&
			scopedFacilityRows.length < globalFacilityRows.length,
		`scoped=${scopedFacilityRows.length}, global=${globalFacilityRows.length}`
	);

	// And the queue list must be a SUBSET of the global one, never a superset:
	// an empty result is correct for a facility with no tickets, but a row from
	// another facility would be a scoping failure.
	const scopedQueues = await call(`${API}/api/v1/admin/queues`, {
		headers: { 'X-Sigap-Local-Test-Subject': 'local-facility-admin' }
	});
	const scopedQueueRows = Array.isArray(scopedQueues.body?.data) ? scopedQueues.body.data : [];
	const scopedFacilityIds = new Set(scopedQueueRows.map((r) => r.facility_id));
	const grantedIds = new Set(scopedFacilityRows.map((f) => f.id));
	const allInScope = [...scopedFacilityIds].every((id) => grantedIds.has(id));
	record(
		'scoped queue rows all belong to granted facilities',
		scopedQueues.status === 200 && allInScope,
		`${scopedQueueRows.length} rows, ${scopedFacilityIds.size} facilities, all in scope=${allInScope}`
	);

	// Persistence proof through a second, independent read path: the appointment
	// booked above must be visible in the admin list. This replaces a GET that
	// does not exist on the public router.
	if (createdAppointmentId) {
		const adminAppointments = await call(`${API}/api/v1/admin/appointments`, {
			headers: { 'X-Sigap-Local-Test-Subject': 'local-global-super-admin' }
		});
		const rows = Array.isArray(adminAppointments.body?.data) ? adminAppointments.body.data : [];
		const seen = rows.some((r) => r.id === createdAppointmentId);
		record(
			'booked appointment persists (visible in admin list)',
			adminAppointments.status === 200 && seen,
			`${adminAppointments.status}, visible=${seen}`
		);
	}

	/* ------------------------------- report ------------------------------ */

	const failed = results.filter((r) => !r.ok);
	console.log('\n' + '='.repeat(60));
	console.log(` ${results.length - failed.length}/${results.length} checks passed`);
	if (createdAppointmentId) console.log(` disposable appointment: ${createdAppointmentId}`);
	if (createdQueueId) console.log(` disposable queue ticket: ${createdQueueId}`);
	if (failed.length) {
		console.log('\nFAILED:');
		for (const f of failed) console.log(`  - ${f.name} (${f.detail})`);
	}
	console.log('='.repeat(60) + '\n');
	process.exitCode = failed.length ? 1 : 0;
}

main().catch((err) => {
	console.error('integration run aborted:', err);
	process.exitCode = 1;
});
