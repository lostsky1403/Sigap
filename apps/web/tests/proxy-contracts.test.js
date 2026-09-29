import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');
const repoRoot = path.join(root, '..', '..');
const apiRoot = path.join(root, 'src', 'routes', 'api', 'v1');

const walk = (directory) =>
	fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
		const entryPath = path.join(directory, entry.name);
		return entry.isDirectory() ? walk(entryPath) : [entryPath];
	});

const relativeRoute = (file) => path.relative(apiRoot, file).split(path.sep).join('/');
const discoveredRoutes = walk(apiRoot)
	.filter((file) => path.basename(file) === '+server.ts')
	.map(relativeRoute)
	.sort();

const manifest = [
	{
		route: 'admin/appointments/+server.ts',
		methods: { GET: ["'/api/v1/admin/appointments'"], POST: ["'/api/v1/admin/appointments'"] }
	},
	{
		route: 'admin/appointments/[id]/status/+server.ts',
		methods: { PATCH: ["`/api/v1/admin/appointments/${encodeURIComponent(event.params.id ?? '')}/status`"] }
	},
	{
		route: 'admin/facilities/+server.ts',
		methods: { GET: ["'/api/v1/admin/facilities'"], POST: ["'/api/v1/admin/facilities'"] }
	},
	{
		route: 'admin/facilities/[id]/+server.ts',
		methods: {
			GET: ["`/api/v1/admin/facilities/${encodeURIComponent(event.params.id ?? '')}`"],
			PATCH: ["`/api/v1/admin/facilities/${encodeURIComponent(event.params.id ?? '')}`"]
		}
	},
	{
		route: 'admin/facilities/[id]/deactivate/+server.ts',
		methods: {
			PATCH: ["`/api/v1/admin/facilities/${encodeURIComponent(event.params.id ?? '')}/deactivate`"]
		}
	},
	{
		route: 'admin/notifications/+server.ts',
		methods: {
			GET: ['`/api/v1/admin/notifications${query}`'],
			POST: ["'/api/v1/admin/notifications'"]
		}
	},
	{
		route: 'admin/notifications/summary/+server.ts',
		methods: { GET: ['`/api/v1/admin/notifications/summary${query}`'] }
	},
	{
		route: 'admin/notifications/[id]/cancel/+server.ts',
		methods: { POST: ["`/api/v1/admin/notifications/${encodeURIComponent(event.params.id ?? '')}/cancel`"] }
	},
	{
		route: 'admin/notifications/[id]/retry/+server.ts',
		methods: { POST: ["`/api/v1/admin/notifications/${encodeURIComponent(event.params.id ?? '')}/retry`"] }
	},
	{
		route: 'admin/queues/+server.ts',
		methods: { GET: ["'/api/v1/admin/queues'"] }
	},
	{
		route: 'admin/queues/[id]/+server.ts',
		methods: { GET: ["`/api/v1/admin/queues/${encodeURIComponent(event.params.id ?? '')}`"] }
	},
	{
		route: 'admin/queues/[id]/status/+server.ts',
		methods: { PATCH: ["`/api/v1/admin/queues/${encodeURIComponent(event.params.id ?? '')}/status`"] }
	},
	{
		route: 'admin/schedules/+server.ts',
		methods: { GET: ["'/api/v1/admin/schedules'"], POST: ["'/api/v1/admin/schedules'"] }
	},
	{
		// Phase 3B5.0: the schedule MUTATION OPTIONS proxy. GET only — the
		// options endpoint is a read, and the schedule mutations keep their own
		// POST/PATCH handlers on the sibling routes. Adding a mutation method
		// here would give the affordance read a write path it must never have.
		route: 'admin/schedules/options/+server.ts',
		methods: { GET: ["'/api/v1/admin/schedules/options'"] }
	},
	{
		route: 'admin/schedules/[id]/+server.ts',
		methods: {
			GET: ["`/api/v1/admin/schedules/${encodeURIComponent(event.params.id ?? '')}`"],
			PATCH: ["`/api/v1/admin/schedules/${encodeURIComponent(event.params.id ?? '')}`"]
		}
	},
	{
		route: 'admin/service-units/+server.ts',
		methods: { GET: ["'/api/v1/admin/service-units'"], POST: ["'/api/v1/admin/service-units'"] }
	},
	{
		route: 'admin/service-units/[id]/+server.ts',
		methods: {
			GET: ["`/api/v1/admin/service-units/${encodeURIComponent(event.params.id ?? '')}`"],
			PATCH: ["`/api/v1/admin/service-units/${encodeURIComponent(event.params.id ?? '')}`"]
		}
	},
	{ route: 'appointments/+server.ts', methods: { POST: ["'/api/v1/appointments'"] } },
	{
		route: 'appointments/[id]/check-in/+server.ts',
		methods: { POST: ["`/api/v1/appointments/${encodeURIComponent(event.params.id ?? '')}/check-in`"] }
	},
	{ route: 'events/beds/+server.ts', methods: { GET: ['`${apiBase}/api/v1/events/beds`'] } },
	{ route: 'patient/status/+server.ts', methods: { GET: ['`/api/v1/patient/status${query}`'] } },
	{ route: 'public/facilities/+server.ts', methods: { GET: ['`${apiBase()}/api/v1/public/facilities`'] } },
	{ route: 'public/service-units/+server.ts', methods: { GET: ['`${apiBase()}/api/v1/public/service-units${query}`'] } },
	{ route: 'queues/generate/+server.ts', methods: { POST: ['`${apiBase}/api/v1/queues/generate`'] } }
];

assert.deepEqual(
	discoveredRoutes,
	manifest.map(({ route }) => route).sort(),
	'Proxy manifest must cover exactly the 23 API proxy files'
);

const methodBlock = (source, method) => {
	const match = new RegExp(`export const ${method}\\b[\\s\\S]*?(?=\\nexport const |$)`).exec(source);
	assert.ok(match, `Expected ${method} handler to be exported`);
	return match[0];
};

const exportedMethods = (source) => [...source.matchAll(/export const ([A-Z]+): RequestHandler/g)].map((match) => match[1]);
const literalParameter = /\/(?:ID|STATUS)(?:['"`])/u;
const encodedParameter = "encodeURIComponent(event.params.id ?? '')";

for (const entry of manifest) {
	const source = fs.readFileSync(path.join(apiRoot, entry.route), 'utf8');
	assert.deepEqual(exportedMethods(source), Object.keys(entry.methods), `${entry.route} methods must match the manifest`);
	for (const [method, pathFragments] of Object.entries(entry.methods)) {
		const block = methodBlock(source, method);
		for (const pathFragment of pathFragments) {
			assert.ok(block.includes(pathFragment), `${entry.route} ${method} must forward ${pathFragment}`);
		}
	}
}

for (const entry of manifest.filter(({ route }) => route.includes('/[id]/'))) {
	const source = fs.readFileSync(path.join(apiRoot, entry.route), 'utf8');
	assert.ok(source.includes(encodedParameter), `${entry.route} must encode event.params.id`);
	assert.ok(!literalParameter.test(source), `${entry.route} must not forward literal ID or STATUS`);
}

const read = (...segments) => fs.readFileSync(path.join(repoRoot, ...segments), 'utf8');
const between = (source, start, end) => {
	const startIndex = source.indexOf(start);
	assert.notEqual(startIndex, -1, `Expected ${start}`);
	const endIndex = source.indexOf(end, startIndex + start.length);
	assert.notEqual(endIndex, -1, `Expected ${end} after ${start}`);
	return source.slice(startIndex, endIndex);
};
const assertUiCall = (file, pathFragment, method) => {
	const source = read(...file);
	const pathIndex = source.indexOf(pathFragment);
	assert.notEqual(pathIndex, -1, `${file.join('/')} must call ${pathFragment}`);
	assert.ok(source.slice(pathIndex, pathIndex + 260).includes(`method: '${method}'`), `${file.join('/')} must use ${method}`);
};

/**
 * Asserts a call is made from the page OR from the shared API client the page
 * uses.
 *
 * The citizen check-in page used to call this endpoint with an inline
 * `fetch`. Phase 3B3 moved it into `checkInAppointment()` in
 * `$lib/api/endpoints/public.ts`, which is where it belonged: the page had no
 * business knowing the URL, and the one place that does know it is also the
 * place that documents why a 401 from it means a wrong code rather than a
 * missing session.
 *
 * So the previous assertion would have failed on a refactor that made the code
 * better, and "fixing" it by moving the call back inline would undo that. The
 * contract being protected is that this page reaches this endpoint with this
 * method, not which file spells the URL — so both are accepted, and neither
 * alone is enough: the page must actually invoke the client, and the client
 * must actually issue the call.
 */
const assertUiCallViaClient = (pageFile, clientFile, importName, pathFragment, method) => {
	const pageSource = read(...pageFile);
	const clientSource = read(...clientFile);

	assert.ok(
		pageSource.includes(importName),
		`${pageFile.join('/')} must reach the endpoint through ${importName}`
	);

	const pathIndex = clientSource.indexOf(pathFragment);
	assert.notEqual(pathIndex, -1, `${clientFile.join('/')} must call ${pathFragment}`);
	assert.ok(
		clientSource.slice(pathIndex, pathIndex + 260).includes(`method: '${method}'`),
		`${clientFile.join('/')} must use ${method}`
	);
};

/**
 * Admin MUTATION wiring is a Phase 3B5 contract, and deliberately not asserted
 * here yet.
 *
 * Until Phase 3B4 these four pages each carried a private `apiFetch` helper that
 * spelled its own URL inline and called `fetch` directly. That is the exact
 * anti-pattern Phase 3B1 removed from the citizen pages, and Phase 3B4 removed it
 * here too by routing every read through `$lib/api/endpoints/admin` — which is
 * where the URL, the method, and the error normalisation belong.
 *
 * But Phase 3B4 is scoped READ-ONLY: it must not implement or rework mutation
 * execution, because 3B5 owns queue status, appointment status, facility
 * deactivate, and notification retry/cancel. Re-adding inline `fetch` calls just
 * to satisfy an assertion about *where a URL is spelled* would reintroduce the
 * duplication 3B4 exists to remove, and would put mutation execution back into a
 * read-only phase.
 *
 * So the contract is preserved where it actually lives — the shared client — and
 * re-asserted against the pages in 3B5, once the pages legitimately call it:
 *
 *   admin.ts: deactivateFacility  PATCH /api/v1/admin/facilities/:id/deactivate
 *   admin.ts: updateQueueTicketStatus PATCH /api/v1/admin/queues/:id/status
 *   admin.ts: updateAppointmentStatus PATCH /api/v1/admin/appointments/:id/status
 *   admin.ts: retryNotification / cancelNotification POST /api/v1/admin/notifications/:id/{op}
 *
 * Note the URL is now `/api/v1/admin/...` with an `encodeURIComponent`d id,
 * because the client prefixes the version — so the old page-local fragments
 * (`/admin/facilities/${facility.id}/deactivate`, unencoded) no longer describe
 * any correct call site even once 3B5 wires the pages.
 *
 * What is NOT deferred: the backend handler, route-registry, and RequiredPolicy
 * assertions further down this file still guard the server-side mutation
 * contract, so a backend that lost its PATCH route or its `queue.manage` policy
 * still fails here.
 */
for (const [name, exportName, pathFragment, method] of [
	['facility deactivate', 'deactivateFacility', '/api/v1/admin/facilities/${encodeURIComponent(id)}/deactivate', 'PATCH'],
	['queue status', 'updateQueueTicketStatus', '/api/v1/admin/queues/${encodeURIComponent(id)}/status', 'PATCH'],
	['appointment status', 'updateAppointmentStatus', '/api/v1/admin/appointments/${encodeURIComponent(id)}/status', 'PATCH'],
	['notification retry/cancel', 'retryNotification', '/api/v1/admin/notifications/${encodeURIComponent(id)}/retry', 'POST'],
	['notification cancel', 'cancelNotification', '/api/v1/admin/notifications/${encodeURIComponent(id)}/cancel', 'POST']
]) {
	const source = read('apps', 'web', 'src', 'lib', 'api', 'endpoints', 'admin.ts');
	assert.ok(source.includes(`export function ${exportName}(`), `admin client must still export ${exportName}`);
	assert.ok(source.includes(pathFragment), `admin client ${exportName} must call ${pathFragment}`);
	assert.ok(
		source.slice(source.indexOf(pathFragment), source.indexOf(pathFragment) + 260).includes(
			`method: '${method}'`
		),
		`admin client ${exportName} must use ${method}`
	);
}

assertUiCallViaClient(
	['apps', 'web', 'src', 'routes', 'appointments', 'check-in', '+page.svelte'],
	['apps', 'web', 'src', 'lib', 'api', 'endpoints', 'public.ts'],
	'checkInAppointment',
	'/api/v1/appointments/${encodeURIComponent(appointmentId)}/check-in',
	'POST'
);

/**
 * The walk-in route is the other half of that pair, and it earns its own
 * assertion because the two are easy to confuse. They produce the same kind of
 * artefact — a queue number — from different endpoints with opposite JSON
 * conventions, so a walk-in page that reached the check-in route would
 * register a duplicate ticket instead of redeeming a code.
 */
{
	const walkIn = read('apps', 'web', 'src', 'routes', 'queues', 'new', '+page.svelte');
	assert.ok(
		walkIn.includes('generateQueueTicket'),
		'the walk-in page must reach the queue engine through generateQueueTicket'
	);
	assert.ok(
		!walkIn.includes('checkInAppointment'),
		'the walk-in page must not redeem an appointment check-in code'
	);
}

const adminHandler = read('apps', 'api', 'internal', 'handler', 'admin.go');
const facilitiesRouter = between(adminHandler, 'func (h *AdminHandler) FacilitiesRouter', '// --- Validation helpers ---');
const queuesRouter = between(adminHandler, 'func (h *AdminHandler) QueuesRouter', '// isValidQueueTransition');
const appointmentsRouter = between(adminHandler, 'func (h *AdminHandler) AppointmentsRouter', '// extractAppointmentID');
const notificationHandler = read('apps', 'api', 'internal', 'handler', 'notifications.go');
const bookingHandler = read('apps', 'api', 'internal', 'handler', 'booking.go');
const routeRegistry = read('apps', 'api', 'internal', 'router', 'router.go');

for (const [name, source, fragments] of [
	['facility deactivate', facilitiesRouter, ['case http.MethodPatch:', 'strings.HasSuffix(r.URL.Path, "/deactivate")']],
	['check-in', bookingHandler, ['case http.MethodPost:', 'parts[4] == "check-in"']],
	['notification retry/cancel', notificationHandler, ['case http.MethodPost:', 'strings.HasSuffix(rest, "/retry")', 'strings.HasSuffix(rest, "/cancel")']],
	['queue status', queuesRouter, ['case http.MethodPatch:', 'h.UpdateQueueStatus(w, r)']],
	['appointment status', appointmentsRouter, ['case http.MethodPatch:', 'h.UpdateAppointmentStatus(w, r)']]
]) {
	for (const fragment of fragments) {
		assert.ok(source.includes(fragment), `${name} backend contract must contain ${fragment}`);
	}
}

for (const fragment of [
	'{Method: http.MethodPatch, Path: "/api/v1/admin/facilities/", Prefix: true, RequiredPolicy: "facility.manage"}',
	'{Method: http.MethodPatch, Path: "/api/v1/admin/queues/", Prefix: true, RequiredPolicy: "queue.manage"}',
	'{Method: http.MethodPatch, Path: "/api/v1/admin/appointments/", Prefix: true, RequiredPolicy: "appointment.manage"}'
]) {
	assert.ok(routeRegistry.includes(fragment), `Backend route registry must contain ${fragment}`);
}

// ---------------------------------------------------------------------------
// GET query forwarding (Phase 3B1 P1 prerequisite)
//
// A proxy that drops `event.url.search` silently discards filters the Go
// handler already implements and validates. These assertions are derived from
// the backend source, not from assumption: a proxy MUST forward the query
// string exactly when its handler reads a query parameter, and MUST NOT invent
// filtering for a handler that reads none.
// ---------------------------------------------------------------------------

const queryRead = (file, start, end) => between(read(...file), start, end);
// tailOf slices from a marker to end-of-file, for the final function in a file.
const tailOf = (file, start) => {
	const source = read(...file);
	const index = source.indexOf(start);
	assert.notEqual(index, -1, `Expected ${start}`);
	return source.slice(index);
};

const backendQueryContracts = [
	{
		name: 'ListNotifications',
		readsQuery: true,
		// limit, facility_id, status, channel, and template_key are read
		// inline; created_from and created_to go through parseOptionalTime.
		params: ['limit', 'facility_id', 'status', 'channel', 'template_key'],
		helperParams: ['created_from', 'created_to'],
		source: queryRead(['apps', 'api', 'internal', 'handler', 'notifications.go'], 'func (h *NotificationsHandler) ListNotifications', 'func (h *NotificationsHandler) GetNotificationSummary')
	},
	{
		name: 'GetNotificationSummary',
		readsQuery: true,
		params: ['facility_id'],
		source: queryRead(['apps', 'api', 'internal', 'handler', 'notifications.go'], 'func (h *NotificationsHandler) GetNotificationSummary', 'func isAllowedStatus')
	},
	{
		name: 'ListPublicServiceUnits',
		readsQuery: true,
		params: ['facility_id'],
		// ListPublicServiceUnits is the final function in catalog.go, so the
		// source slice runs to end-of-file.
		source: tailOf(['apps', 'api', 'internal', 'handler', 'catalog.go'], 'func (h *CatalogHandler) ListPublicServiceUnits')
	},
	{
		name: 'PatientStatusLookup',
		readsQuery: true,
		params: ['code'],
		source: queryRead(
			['apps', 'api', 'internal', 'handler', 'patient.go'],
			'func (h *PatientHandler) PatientStatusLookup',
			'func extractIP'
		)
	},
	// Facility filtering for these collections is enforced server-side from the
	// actor's authorized read set, so forwarding a query string would be
	// inventing a filter the API does not implement.
	{ name: 'ListFacilities', readsQuery: false, source: read('apps', 'api', 'internal', 'handler', 'admin.go') },
	{ name: 'ListQueueTickets', readsQuery: false, source: read('apps', 'api', 'internal', 'handler', 'admin.go') },
	{ name: 'ListAppointments', readsQuery: false, source: read('apps', 'api', 'internal', 'handler', 'admin.go') },
	{ name: 'ListSchedules', readsQuery: false, source: read('apps', 'api', 'internal', 'handler', 'admin.go') },
	{ name: 'ListServiceUnits', readsQuery: false, source: read('apps', 'api', 'internal', 'handler', 'admin.go') },
	{ name: 'GetNotification', readsQuery: false, source: read('apps', 'api', 'internal', 'handler', 'notifications.go') }
];

for (const contract of backendQueryContracts) {
	if (!contract.readsQuery) continue;
	for (const param of contract.params) {
		assert.ok(
			contract.source.includes(`Query().Get("${param}")`),
			`${contract.name} must read the ${param} query parameter in the Go source`
		);
	}
	for (const param of contract.helperParams ?? []) {
		assert.ok(
			contract.source.includes(`parseOptionalTime(r, "${param}")`),
			`${contract.name} must read the ${param} query parameter via parseOptionalTime in the Go source`
		);
	}
}

// A proxy forwards the query string when it reads `.search` off a URL built
// from event.request.url. Two spellings are accepted: the inline
// `new URL(event.request.url).search` form and the two-step
// `const url = new URL(event.request.url); url.search` form.
const forwardsQuery = (route) => {
	const source = fs.readFileSync(path.join(apiRoot, route), 'utf8');
	return (
		source.includes('new URL(event.request.url).search') ||
		(new RegExp('new URL\\(event\\.request\\.url\\)').test(source) && /\.search\b/.test(source))
	);
};

for (const route of [
	'admin/notifications/+server.ts',
	'admin/notifications/summary/+server.ts',
	'patient/status/+server.ts',
	'public/service-units/+server.ts'
]) {
	assert.ok(forwardsQuery(route), `${route} must forward event.url.search to the API`);
}

for (const route of [
	'admin/facilities/+server.ts',
	'admin/queues/+server.ts',
	'admin/appointments/+server.ts',
	'admin/schedules/+server.ts',
	'admin/service-units/+server.ts',
	'public/facilities/+server.ts'
]) {
	assert.ok(
		!forwardsQuery(route),
		`${route} must not forward a query string; its handler reads no query parameters`
	);
}

console.log('✅ Proxy manifest, path, method, and GET query-forwarding contracts passed.');
