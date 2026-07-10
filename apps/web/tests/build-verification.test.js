// Basic build verification test — runs with Node (no test framework needed)
// Usage: node tests/build-verification.test.js

import assert from 'node:assert';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(__dirname, '..');

const typesPath = path.join(root, 'src/lib/types.ts');
const typesContent = fs.readFileSync(typesPath, 'utf-8');

assert(typesContent.includes('export type Facility'), 'Facility type should be exported');
assert(typesContent.includes('export type QueueApiResponse'), 'QueueApiResponse type should be exported');
assert(typesContent.includes('export type NearbyApiResponse'), 'NearbyApiResponse type should be exported');
assert(typesContent.includes('export type MedicalRecord'), 'MedicalRecord type should be exported');

// Verify no 'any' types leak into key source files
const dashboardPath = path.join(root, 'src/lib/components/dashboard/BedAvailabilityDashboard.svelte');
const dashboardContent = fs.readFileSync(dashboardPath, 'utf-8');
assert(!dashboardContent.includes(': any'), 'Dashboard should not contain untyped any');
assert(!dashboardContent.includes('@ts-ignore'), 'Dashboard should not contain @ts-ignore');

const mapPath = path.join(root, 'src/lib/components/ReferralMap.svelte');
const mapContent = fs.readFileSync(mapPath, 'utf-8');
assert(!mapContent.includes(': any'), 'ReferralMap should not contain untyped any');

const walletPath = path.join(root, 'src/routes/wallet/+page.svelte');
const walletContent = fs.readFileSync(walletPath, 'utf-8');
assert(!walletContent.includes(': any'), 'Wallet should not contain untyped any');

// Demo polish: surface additive API response fields already returned by backend
const checkInPath = path.join(root, 'src/routes/appointments/check-in/+page.svelte');
const checkInContent = fs.readFileSync(checkInPath, 'utf-8');
assert(checkInContent.includes('appointment_id'), 'Check-in success UI should surface appointment_id');
assert(checkInContent.includes('queue_ticket_id'), 'Check-in success UI should surface queue_ticket_id');
assert(checkInContent.includes('formatted_number'), 'Check-in success UI should show formatted queue number');

const bookingPath = path.join(root, 'src/routes/appointments/new/+page.svelte');
const bookingContent = fs.readFileSync(bookingPath, 'utf-8');
assert(bookingContent.includes('checkin_code'), 'Booking success UI should surface checkin_code');
assert(bookingContent.includes('result.id'), 'Booking success UI should surface appointment id');

const adminQueuesPath = path.join(root, 'src/routes/admin/queues/+page.svelte');
const adminQueuesContent = fs.readFileSync(adminQueuesPath, 'utf-8');
assert(adminQueuesContent.includes('updated_at'), 'Admin queue status update UI should surface updated_at');

const adminApptsPath = path.join(root, 'src/routes/admin/appointments/+page.svelte');
const adminApptsContent = fs.readFileSync(adminApptsPath, 'utf-8');
assert(adminApptsContent.includes('updated_at'), 'Admin appointment status update UI should surface updated_at');

// Verify build output exists (signal that vite build succeeded)
const buildDir = path.join(root, 'build');
assert(fs.existsSync(buildDir), 'Build directory should exist after vite build');
assert(fs.existsSync(path.join(buildDir, 'index.js')), 'Build should include server entry point (index.js)');
assert(fs.existsSync(path.join(buildDir, 'client')), 'Build should include client assets directory');

console.log('✅ Build verification passed: types exported, no any types, demo polish fields, build artifacts present.');
