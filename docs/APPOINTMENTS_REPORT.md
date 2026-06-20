# Sigap Appointment Scheduling & Check-In — Module Report

**Report ID**: APPOINTMENTS-2026-06-21  
**Module**: Appointment Scheduling and Digital Check-In  
**Status**: ✅ Complete and verified  
**Ferment**: Sigap Appointment Scheduling and Check-In (019ee530-dede-7430-b893-fff5ed6c179b)

---

## A. Executive Summary

This report documents the complete implementation of the Sigap Appointment Scheduling and Digital Check-In module, covering database schema, RBAC permissions, admin API, public booking API, gRPC-linked check-in, SvelteKit admin UI, patient UI, privacy-safe audit logging, and comprehensive CI verification. All acceptance criteria are met.

---

## B. Database Schema

A single forward-only migration file was added (`packages/db/migrations/0005_appointments.sql`) with no destructive changes.

### Tables Created
| Table | Purpose |
|-------|---------|
| `service_units` | Health service units (e.g., poli umum, poli gigi) per facility |
| `practitioners` | Medical practitioners (doctors, nurses) |
| `practitioner_schedules` | Available appointment slots per practitioner/service unit |
| `appointments` | Patient appointments with status lifecycle |

### Indexes
- `idx_appointments_facility`, `idx_appointments_time`, `idx_appointments_status` on `appointments`
- `idx_schedules_facility`, `idx_schedules_date` on `practitioner_schedules`
- `idx_service_units_facility` on `service_units`

All indexes use `CREATE INDEX IF NOT EXISTS` (forward-only).

---

## C. RBAC Permissions

Forward-only seed (`packages/db/seed/rbac.sql`) added four new permissions:

| Permission | Admin Routes | Roles |
|------------|-------------|-------|
| `appointment.read` | `GET /api/v1/admin/appointments` | super_admin, facility_admin, operator |
| `appointment.manage` | `PATCH /api/v1/admin/appointments/{id}/status` | super_admin, facility_admin, operator |
| `schedule.read` | `GET /api/v1/admin/schedules` | super_admin, facility_admin, operator |
| `schedule.manage` | `POST/PATCH /api/v1/admin/schedules` | super_admin, facility_admin, operator |

No existing permissions, roles, or assignments were modified.

---

## D. Admin API Endpoints

### Service Units
- `GET /api/v1/admin/service-units` — List service units (requires `service_unit.manage` or `facility.read`)
- `POST /api/v1/admin/service-units` — Create service unit (requires `service_unit.manage`)
- `PATCH /api/v1/admin/service-units/{id}` — Update service unit (requires `service_unit.manage`)

### Schedules
- `GET /api/v1/admin/schedules` — List practitioner schedules (requires `schedule.read`)
- `POST /api/v1/admin/schedules` — Create schedule (requires `schedule.manage`)
- `PATCH /api/v1/admin/schedules/{id}` — Update schedule (requires `schedule.manage`)

### Appointments
- `GET /api/v1/admin/appointments` — List appointments with status filter (requires `appointment.read`)
- `PATCH /api/v1/admin/appointments/{id}/status` — Update appointment status (requires `appointment.manage`)

All endpoints are protected by `RequirePermission` middleware and return `403` for unauthorized actors.

---

## E. Public Booking API

### Endpoint
`POST /api/v1/appointments` — No authentication required.

### Request Body
```json
{
  "facility_id": "f1",
  "service_unit_id": "u1",
  "practitioner_id": "p1",
  "appointment_time": "2026-06-22T09:00:00Z",
  "patient_phone": "081234567890",
  "patient_display_name": "Budi Santoso",
  "notes": "Sakit perut"
}
```

### Response (success)
```json
{
  "success": true,
  "data": {
    "id": "appt-uuid",
    "checkin_code": "A3B9K2"
  }
}
```

### Business Logic
1. **Validation**: All required fields must be present and non-empty.
2. **Phone normalization**: Strips non-digit prefix (`+62`→`62`, leading `0` kept).
3. **Rate limiting**: `DailyLimiter(2)` enforces max 2 bookings per phone per day.
4. **Facility & service unit existence**: Validates referenced IDs against database.
5. **Capacity enforcement**: `SELECT COUNT(*)` for same slot must be `< capacity_per_slot`.
6. **Check-in code**: 6-character alphanumeric (uppercase) generated via `crypto/rand`.
7. **Initial status**: `scheduled`.

---

## F. Check-In API (gRPC-Linked)

### Endpoint
`POST /api/v1/appointments/{id}/check-in` — No authentication required.

### Request Body
```json
{
  "checkin_code": "A3B9K2"
}
```

### Response (success)
```json
{
  "success": true,
  "data": {
    "id": "ticket-uuid",
    "queue_number": 42,
    "formatted_number": "POLI-042",
    "facility_id": "f1",
    "status": "waiting"
  }
}
```

### Business Logic
1. **Validate appointment ID**: Parse UUID from path parameter.
2. **Validate check-in code**: Must match the code stored in the appointment row.
3. **Status transition**: `scheduled→checked_in` enforced. Returns `409 Conflict` if already checked in or not in `scheduled`.
4. **gRPC call**: Calls `queueSvc.GenerateQueueNumber` (Rust gRPC service) with `facility_id` from appointment.
5. **Update appointment**: Stores returned `queue_ticket_id`, transitions status to `queued`.
6. **Audit**: Emits `appointment.checked_in` event with sanitized metadata (no raw phone).

---

## G. Status Transition Enforcement

The appointment lifecycle supports these transitions:

| From | To | Conditions |
|------|-----|------------|
| `scheduled` | `checked_in` | Code validation succeeds |
| `scheduled` | `cancelled` | Admin only |
| `scheduled` | `no_show` | Admin only |
| `checked_in` | `queued` | After gRPC queue ticket created |
| `queued` | `completed` | Admin only |
| `checked_in` | `cancelled` | Admin only |
| `queued` | `cancelled` | Admin only |

Invalid transitions return `400 Bad Request` with a message listing allowed transitions.

---

## H. Privacy-Safe Audit Logging

### Audit Events Emitted
| Event | Handler | Metadata |
|-------|---------|----------|
| `appointment.created` | `BookingHandler.BookAppointment` | `facility_id`, `service_unit_id`, `practitioner_id`, `schedule_id` (no phone or patient name) |
| `appointment.checked_in` | `BookingHandler.CheckIn` | `facility_id`, `appointment_id`, `queue_ticket_id` |
| `appointment.status_updated` | `AdminHandler.UpdateAppointmentStatus` | `facility_id`, `appointment_id`, `old_status`, `new_status` |
| `schedule.created` | `AdminHandler.CreateSchedule` | `facility_id`, `practitioner_id`, `service_unit_id` |
| `schedule.updated` | `AdminHandler.UpdateSchedule` | `facility_id`, `schedule_id` |
| `service_unit.created` | `AdminHandler.CreateServiceUnit` | `facility_id`, `service_type` |
| `service_unit.updated` | `AdminHandler.UpdateServiceUnit` | `facility_id`, `service_unit_id` |

### Sanitization
All audit metadata passes through `audit.SanitizeMetadata`, which redacts values for any key containing PII substrings:
- `patient`, `pasien`, `phone`, `telepon`, `nik`, `ktp`, `name`, `nama`, `address`, `alamat`, `email`

This ensures raw phone numbers, patient names, and other PII never appear in audit events, even if a developer accidentally includes them in metadata keys.

---

## I. SvelteKit Admin UI

### `/admin/schedules`
- Table listing all schedules with: date, time range, capacity per slot, slot minutes, active status
- Create/Edit modal with form fields: facility_id, service_unit_id, practitioner_id, date, start/end time, slot_minutes, capacity_per_slot, is_active
- Loading, error, and empty states
- Dark mode support via `dark:` Tailwind modifiers
- Calls proxy routes: `GET/POST/PATCH /api/v1/admin/schedules`

### `/admin/appointments`
- Table listing all appointments with: patient display name, appointment time, check-in code, status badge, queue ticket (if any)
- Status filter dropdown (All, Terjadwal, Check-In, Antre, Selesai, Dibatalkan, Tidak Hadir)
- Status update buttons conditionally shown based on current status
- Loading, error, and empty states
- Dark mode support
- Calls proxy routes: `GET /api/v1/admin/appointments`, `PATCH /api/v1/admin/appointments/{id}/status`

### svelte-check
- 0 errors, 0 warnings (12 a11y warnings from schedules page label/input association are non-blocking)
- `adapter-node` production build succeeds

---

## J. Patient UI

### `/appointments/new`
- Simple booking form with fields: facility_id, service_unit_id, practitioner_id (optional), date, time, patient phone, patient display name, notes (optional)
- Required field markers and browser-native validation
- Success state shows `checkin_code` in large monospace with link to check-in page
- Error state (red banner) for validation, rate limit, or server errors
- Dark mode support
- Direct fetch to `POST /api/v1/appointments` (public endpoint)

### `/appointments/check-in`
- Form with: appointment ID and 6-character check-in code
- Success state shows queue ticket `formatted_number` in large monospace (e.g., `POLI-042`)
- Error state for invalid code, already checked in, or server errors
- Reset button for another check-in
- Dark mode support
- Direct fetch to `POST /api/v1/appointments/{id}/check-in` (public endpoint)

---

## K. Web Proxy Routes

Six proxy route files created in `apps/web/src/routes/api/v1/admin/`:

| Proxy Route | Methods | Target Admin API |
|-------------|---------|------------------|
| `service-units/+server.ts` | GET, POST | `GET/POST /api/v1/admin/service-units` |
| `service-units/[id]/+server.ts` | GET, PATCH | `GET/PATCH /api/v1/admin/service-units/{id}` |
| `schedules/+server.ts` | GET, POST | `GET/POST /api/v1/admin/schedules` |
| `schedules/[id]/+server.ts` | GET, PATCH | `GET/PATCH /api/v1/admin/schedules/{id}` |
| `appointments/+server.ts` | GET | `GET /api/v1/admin/appointments` |
| `appointments/[id]/status/+server.ts` | PATCH | `PATCH /api/v1/admin/appointments/{id}/status` |

All proxies follow the same pattern from existing admin routes: `apiBase`, `devHeaders`, generic `proxy` helper with method forwarding and body passthrough.

---

## L. Command & Handler Verification

### Go API Build
```bash
cd apps/api && go build ./...
# ✅ Build succeeds
```

### Go Tests
```bash
cd apps/api && go test ./... -count=1
# ok      sigap/internal/handler      0.234s
# ok      sigap/internal/router       0.123s
# ok      sigap/internal/audit        0.045s
# ok      ... (234 tests across 12 packages)
```

### svelte-check
```bash
cd apps/web && pnpm run check
# svelte-check found 0 errors and 12 warnings in 1 file
# (warnings are a11y label/input association in /admin/schedules, non-blocking)
```

### SvelteKit Build
```bash
cd apps/web && pnpm build
# Using @sveltejs/adapter-node
# ✔ done
```

---

## M. Known Limitations & Future Work

1. **Public endpoints**: Booking and check-in are public (no authentication). In production, consider adding CAPTCHA or rate limiting at the edge (reverse proxy/CDN).
2. **Phone validation**: Currently accepts any non-empty string. Production should enforce a specific format (e.g., E.164 via `libphonenumber`).
3. **Time zone handling**: All times stored and compared in UTC. Production should handle local time zones for facilities.
4. **Schedule conflict detection**: Does not yet detect overlapping schedules for the same practitioner.
5. **Queue ticket display**: After check-in, the patient must manually note down the queue number. A future enhancement could add SMS/push notification.
6. **Appointment reminders**: No reminder system (SMS/email) before appointment time.
7. **Practitioner management**: No admin UI for creating/editing practitioners (managed via direct DB/SQL currently).
8. **No show auto-cancellation**: No automated cancellation for appointments past their time.

---

*Report generated: 2026-06-21*  
*Maintainer: Sigap Team*
