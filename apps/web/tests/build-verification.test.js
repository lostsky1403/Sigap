// Basic build verification test — runs with Node (no test framework needed)
// Usage: node tests/build-verification.test.js

import assert from 'node:assert';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');

/**
 * Type reconciliation (Phase 3B6, T-3B6-02).
 *
 * These assertions used to read `src/lib/types.ts`, the flat grab-bag that
 * mixed wire shapes with demo-only domain types. That file is gone: the wire
 * shapes now live in `src/lib/api/types/api.ts`, and the demo-only ones
 * (`Facility` with bed counts, `QueueTicket`, `QueueApiResponse`,
 * `NearbyApiResponse`) were deleted along with the demo dashboard that was
 * their only consumer.
 *
 * The assertions are REPOINTED, not dropped. Removing them would leave the
 * canonical module unguarded, and the failure they guard against is unchanged:
 * a wire type silently disappearing from the module every endpoint imports
 * from.
 */
const typesPath = path.join(root, 'src/lib/api/types/api.ts');
const typesContent = fs.readFileSync(typesPath, 'utf-8');

for (const typeName of [
	'PublicFacility',
	'PublicServiceUnit',
	'PatientStatus',
	'BookAppointmentResult',
	'CheckInResult',
	'QueueGenerateResult',
	'AdminFacility',
	'AdminQueueTicket',
	'AdminServiceUnit',
	'AdminSchedule',
	'AdminAppointment',
	'NotificationOutboxRow',
	'NotificationSummary',
	'MedicalRecord'
]) {
	assert(
		new RegExp(`export (interface|type) ${typeName}\\b`).test(typesContent),
		`${typeName} should be exported from lib/api/types/api.ts`
	);
}

// A second types module is how two competing shapes for the same endpoint
// start drifting apart, so the legacy one must not come back.
assert(
	!fs.existsSync(path.join(root, 'src/lib/types.ts')),
	'src/lib/types.ts was reconciled into lib/api/types/api.ts and must not return'
);

/**
 * The legacy demo components stay deleted (T-3B6-02).
 *
 * Asserted as absence rather than by reading the files, because the files are
 * the point: `BedAvailabilityDashboard.svelte` shipped invented facilities with
 * bed counts, a simulated availability feed and a "chaos mode" load generator,
 * and `ReferralMap.svelte` was a mock map that only it imported. Re-adding
 * either would put fabricated civic data back within reach of a route.
 */
for (const legacy of [
	'src/lib/components/dashboard/BedAvailabilityDashboard.svelte',
	'src/lib/components/ReferralMap.svelte'
]) {
	assert(
		!fs.existsSync(path.join(root, legacy)),
		`${legacy} is dead demo code and must stay deleted`
	);
}

// maplibre-gl existed only to draw that mock map. A dependency nothing imports
// is bundle weight and unused attack surface.
//
// BOTH dependency maps are checked. Asserting only `dependencies` would pass if
// the package were moved to `devDependencies`, which still installs it, still
// keeps it in the lockfile, and still ships it to anything that installs the
// workspace — so the guard would report a removal that had not happened.
const pkg = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf-8'));
for (const section of ['dependencies', 'devDependencies', 'optionalDependencies']) {
	assert(
		!pkg[section]?.['maplibre-gl'],
		`maplibre-gl was only used by the removed ReferralMap and must not be in ${section}`
	);
}

const walletPath = path.join(root, 'src/routes/wallet/+page.svelte');
const walletContent = fs.readFileSync(walletPath, 'utf-8');
assert(!walletContent.includes(': any'), 'Wallet should not contain untyped any');

// Demo polish: surface additive API response fields already returned by backend
//
// These assertions check what the citizen actually SEES, so they read the page
// together with the components it renders. Phase 3B3 moved the check-in ticket
// markup out of the page and into $lib/citizen/QueueTicket.svelte, which is
// where it belonged: the page owns fetching and the failure states, the
// component owns the success presentation. Asserting on the page file alone
// would have failed on that split while the fields were on screen the whole
// time, and "fixing" it by inlining the markup back would undo the split.
//
// So each marker is looked for across the page and the components it imports.
// A marker that is dropped from the UI still fails; one that moved does not.
const citizenTicketPath = path.join(root, 'src/lib/citizen/QueueTicket.svelte');
const checkInPath = path.join(root, 'src/routes/appointments/check-in/+page.svelte');
const checkInContent = fs.readFileSync(checkInPath, 'utf-8');
const checkInUi =
	checkInContent +
	fs.readFileSync(citizenTicketPath, 'utf-8') +
	fs.readFileSync(path.join(root, 'src/lib/citizen/CheckinForm.svelte'), 'utf-8');

assert(checkInUi.includes('appointment_id'), 'Check-in success UI should surface appointment_id');
assert(checkInUi.includes('queue_ticket_id'), 'Check-in success UI should surface queue_ticket_id');
assert(checkInUi.includes('formatted_number'), 'Check-in success UI should show formatted queue number');

// And the page must actually compose that component. Without this, deleting the
// <QueueTicket> usage would leave the markers present in an unreferenced file
// and every assertion above would still pass.
assert(
	checkInContent.includes('QueueTicket'),
	'Check-in page must render the QueueTicket component that surfaces those fields'
);

const bookingPath = path.join(root, 'src/routes/appointments/new/+page.svelte');
const bookingContent = fs.readFileSync(bookingPath, 'utf-8');
assert(bookingContent.includes('checkin_code'), 'Booking success UI should surface checkin_code');
assert(bookingContent.includes('result.id'), 'Booking success UI should surface appointment id');

/**
 * The admin READ pages must keep surfacing a per-row source marker — the field
 * that says when the data in front of the operator last changed.
 *
 * These two pages do NOT have the same marker, and the difference is not an
 * oversight. It is a fact about the schema, and the test is pinned to that fact
 * deliberately so nobody "fixes" one page by copying the other:
 *
 *   - `appointments` HAS an `updated_at` column, and the Go list query selects
 *     it, so the page surfaces `appointment.updated_at` directly.
 *   - `queue_tickets` has NO `updated_at` column at all. The Go list query
 *     (apps/api/internal/handler/admin.go) selects exactly
 *     `id, facility_id, queue_number, formatted_number, status, registered_at,
 *     called_at, completed_at`. There is nothing to select and nothing to show.
 *
 * So the queue board's marker is built from the three timestamps that every
 * status transition actually stamps. The assertion below therefore checks that
 * the board derives and displays a source marker, and that it does so from the
 * real fields — NOT that the string `updated_at` appears, which would be
 * asserting an inventory of a column that does not exist.
 */
const adminQueuesPath = path.join(root, 'src/routes/admin/queues/+page.svelte');
const adminQueuesContent = fs.readFileSync(adminQueuesPath, 'utf-8');
const queueBoardContent = fs.readFileSync(
	path.join(root, 'src/lib/admin/QueueBoard.svelte'),
	'utf-8'
);
const queueSourceContent = fs.readFileSync(
	path.join(root, 'src/lib/admin/queueSource.ts'),
	'utf-8'
);
assert(
	adminQueuesContent.includes('QueueBoard') && queueBoardContent.includes('sigap-queue-board__source'),
	'Admin queue read UI should render the QueueBoard that surfaces a source marker'
);
// Pinned against queueSource.ts, NOT QueueBoard.svelte. Asserting the three
// timestamp names against the component would be vacuous: the component's
// explanatory comment mentions all of them, so deleting the logic would leave
// the substrings present and the test green — proven by mutation, which missed
// exactly that version.
//
// So this block only pins the SHAPE (a marker exists, the module is the one
// holding the logic). The BEHAVIOUR is guarded by
// src/lib/admin/queueSource.test.ts, which fails the moment a stamp is dropped
// from the real list or an updated_at is ever read. Keeping the string checks
// here as well is deliberate: they catch the file being deleted or the marker
// being unhooked from the board, which the Vitest suite cannot see.
for (const stamp of ['registered_at', 'called_at', 'completed_at']) {
	assert(
		queueSourceContent.includes(stamp),
		`Admin queue source marker must derive from the real queue_timestamps (${stamp})`
	);
}

const adminApptsPath = path.join(root, 'src/routes/admin/appointments/+page.svelte');
const adminApptsContent = fs.readFileSync(adminApptsPath, 'utf-8');
assert(adminApptsContent.includes('updated_at'), 'Admin appointment status update UI should surface updated_at');

// Verify build output exists (signal that vite build succeeded)
const buildDir = path.join(root, 'build');
assert(fs.existsSync(buildDir), 'Build directory should exist after vite build');
assert(fs.existsSync(path.join(buildDir, 'index.js')), 'Build should include server entry point (index.js)');
assert(fs.existsSync(path.join(buildDir, 'client')), 'Build should include client assets directory');

console.log('✅ Build verification passed: types exported, no any types, demo polish fields, build artifacts present.');
