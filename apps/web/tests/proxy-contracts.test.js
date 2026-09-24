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
		methods: { GET: ["'/api/v1/admin/notifications'"], POST: ["'/api/v1/admin/notifications'"] }
	},
	{
		route: 'admin/notifications/summary/+server.ts',
		methods: { GET: ["'/api/v1/admin/notifications/summary'"] }
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
	{ route: 'public/service-units/+server.ts', methods: { GET: ['`${apiBase()}/api/v1/public/service-units`'] } },
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

assertUiCall(['apps', 'web', 'src', 'routes', 'admin', 'facilities', '+page.svelte'], '/admin/facilities/${facility.id}/deactivate', 'PATCH');
assertUiCall(['apps', 'web', 'src', 'routes', 'appointments', 'check-in', '+page.svelte'], '/api/v1/appointments/${appointmentId}/check-in', 'POST');
assertUiCall(['apps', 'web', 'src', 'routes', 'admin', 'notifications', '+page.svelte'], '/api/v1/admin/notifications/${id}/${op}', 'POST');
assertUiCall(['apps', 'web', 'src', 'routes', 'admin', 'queues', '+page.svelte'], '/admin/queues/${ticket.id}/status', 'PATCH');
assertUiCall(['apps', 'web', 'src', 'routes', 'admin', 'appointments', '+page.svelte'], '/admin/appointments/${a.id}/status', 'PATCH');

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

console.log('✅ Proxy manifest, path, and method contracts passed.');
