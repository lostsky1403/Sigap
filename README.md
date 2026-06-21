# Sigap

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)
![Rust](https://img.shields.io/badge/Rust-1.78-dea584?logo=rust)
[![CI](https://github.com/lostsky1403/Sigap/actions/workflows/ci.yml/badge.svg)](https://github.com/lostsky1403/Sigap/actions/workflows/ci.yml)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)](https://github.com/lostsky1403/Sigap/blob/main/docker-compose.yml)

**Sigap** adalah kerangka kerja open-source Civic-Tech berbasis web untuk layanan informasi dan antrean kesehatan daerah (rumah sakit & puskesmas).

Tujuan: memberikan transparansi ketersediaan fasilitas kesehatan dan kemudahan pengambilan nomor antrean secara digital bagi masyarakat di daerah.

> Status: Scaffolding MVP awal (production-minded). Belum siap produksi untuk data sensitif.

## Fitur MVP Saat Ini

- Skema database lengkap (fasilitas, pasien, tiket antrean, counter harian, **service unit, jadwal praktik, janji temu**)
- Endpoint utama generate nomor antrean (Go + Rust)
- **Janji temu digital**: booking publik dengan kode check-in, validasi kapasitas slot, rate limiting by phone
- **Check-in terintegrasi**: validasi kode check-in → gRPC GenerateQueueNumber → update status `scheduled→checked_in→queued`
- **Admin jadwal & janji temu**: CRUD jadwal praktik, daftar & update status janji temu
- **Notification outbox (dev provider)**: foundation untuk appointment/check-in confirmation. Mask + SHA-256 dedup, dev/local provider only, no real vendor. Lihat [docs/NOTIFICATIONS_REPORT.md](docs/NOTIFICATIONS_REPORT.md).
- Dashboard Ketersediaan Kasur yang clean & minimalis (Svelte 5)
- Dukungan penuh dark mode & light mode dengan desain sistem yang disiplin
- Arsitektur monorepo polyglot berperforma tinggi (Go + Rust + SvelteKit)

## Tech Stack

- **Frontend**: SvelteKit (Svelte 5 runes) + Tailwind + desain sistem minimalis (headless primitives jika diperlukan)
- **Core API & Routing**: Go (net/http + gRPC client)
- **Mesin Antrean & Sinkronisasi Kasur**: Rust (tonic gRPC + sqlx)
- **Komunikasi internal**: gRPC (kontrak di `protos/sigap/queue_engine.proto`)
- **Database**: PostgreSQL + migrasi SQL murni (satu sumber kebenaran)
- **Monorepo**: pnpm (web) + Go modules + Cargo + Makefile untuk orkestrasi

## Architecture

```mermaid
flowchart TD
    subgraph Client
        User[User / Browser]
        Svelte[SvelteKit<br/>Web UI + SSE Client]
    end

    subgraph Backend
        Go[Go API<br/>HTTP Handler + Rate Limiter<br/>gRPC Client + SSE Hub]
        Rust[Rust Engine<br/>Tonic gRPC Server<br/>sqlx + Atomic Tx]
    end

    User -->|HTTP POST /generate + GET /events/beds| Svelte
    Svelte -->|Real-time SSE updates| Go
    Svelte -->|Submit queue request| Go
    Go -->|gRPC GenerateQueueNumber<br/>(with processing_time_µs)| Rust
    Rust -->|SELECT ... FOR UPDATE<br/>+ INSERT ticket| Postgres[(PostgreSQL)]
    Go -->|Direct queries<br/>(rate limiting, etc.)| Postgres
    Rust -->|Response + latency| Go
    Go -->|JSON response<br/>(incl. processing_time)| Svelte
    Go -->|SSE event: bed_updated| Svelte

    classDef go fill:#00ADD8,stroke:#00ADD8,color:white
    classDef rust fill:#dea584,stroke:#dea584,color:black
    class Go go
    class Rust rust
```

**Flow summary**:
- User interacts with SvelteKit frontend.
- Frontend talks to Go API (queue submission + real-time SSE).
- Go enforces rate limiting (phone + facility per day) then delegates the critical atomic queue generation to the Rust engine over gRPC.
- Rust engine performs the high-safety transaction (FOR UPDATE on daily counters) against PostgreSQL and returns the result **including micro-second execution time** for traceability.
- Successful queue creation triggers an SSE event that the frontend listens to for live UI updates (no page refresh).

## Struktur Direktori (Monorepo)

```
.
├── Makefile
├── protos/
│   └── sigap/
│       └── queue_engine.proto          # Kontrak gRPC (sumber utama)
├── packages/
│   └── db/
│       ├── migrations/0001_init.sql    # DDL lengkap + index + constraint
│       └── seed/dev.sql                # Data contoh 6 fasilitas
├── apps/
│   ├── api/                            # Go — gateway HTTP publik
│   │   ├── cmd/server/main.go
│   │   └── internal/handler/queue_test.go   # TDD dimulai di sini
│   ├── queue-engine/                   # Rust — mesin pemroses antrean + bed sync
│   │   └── src/
│   │       ├── main.rs
│   │       └── engine/queue.rs         # Logika transaksi kritis
│   └── web/                            # SvelteKit
│       └── src/lib/components/dashboard/
│           └── BedAvailabilityDashboard.svelte   # Komponen utama yang diminta
└── README.md
```

## Demo Ready (10 menit)

Sigap MVP siap di-demo-kan secara lokal tanpa layanan eksternal. Tiga URL
utama saat stack hidup:

| Layanan | URL |
|---------|-----|
| Web (SvelteKit) | <http://localhost:5173> |
| API (Go) | <http://localhost:8080> |
| gRPC (Rust) | `localhost:50051` |

Tiga perintah curl untuk verifikasi cepat:

```bash
# 1. Health
curl http://localhost:8080/health

# 2. Daftar fasilitas (dev identity)
curl http://localhost:8080/api/v1/admin/facilities \
  -H "X-Sigap-Dev-User-ID: dev-user-42"

# 3. Booking janji temu publik (mengembalikan checkin_code)
curl -X POST http://localhost:8080/api/v1/appointments \
  -H "Content-Type: application/json" \
  -d '{"facility_id":"<UUID>","service_unit_id":"00000000-0000-0000-0000-00000000d001","patient_display_name":"Pasien Demo","patient_phone":"+62-555-0199","appointment_time":"2026-06-22T09:00:00Z"}'
```

Untuk alur lengkap (PowerShell-friendly, dengan smoke suite otomatis dan
walkthrough UI admin + patient), lihat
[`docs/DEMO_FLOW.md`](./docs/DEMO_FLOW.md).

## Quickstart (Lokal)

1. Clone & install
   ```bash
   git clone <repo>
   cd sigap
   pnpm install --filter sigap-web
   ```

2. Siapkan PostgreSQL
   ```bash
   # Contoh pakai Docker
   docker run --name sigap-db -e POSTGRES_PASSWORD=sigap -e POSTGRES_DB=sigap -p 5432:5432 -d postgres:16
   export DATABASE_URL="postgresql://postgres:sigap@localhost:5432/sigap"
   ```

3. Migrasi & seed
   ```bash
   make db-migrate
   make db-seed
   ```

4. Jalankan layanan (gunakan 3 terminal atau `make` target)
   ```bash
   make dev-engine   # Rust gRPC (stub Phase 0)
   make dev-api      # Go HTTP (:8080)
   make dev-web      # SvelteKit (:5173)
   ```

## Testing

```bash
# Unit tests (Go + Rust)
make test

# Type checking, linting, and formatting
make lint

# Security scanning (govulncheck, cargo-audit, gitleaks)
make security
```

5. Test endpoint (setelah Phase 2 selesai)
   ```bash
   curl -X POST http://localhost:8080/api/v1/queues/generate \
     -H "Content-Type: application/json" \
     -d '{"facilityId":"f1","patient":{"fullName":"Test Pasien","phone":"081234567890"}}'
   ```

Buka http://localhost:5173 — lihat Dashboard Ketersediaan Kasur, buat janji temu di `/appointments/new`, atau check-in di `/appointments/check-in`.

**Booking janji temu (publik):**
```bash
# Buat janji temu (tanpa autentikasi)
curl -X POST http://localhost:8080/api/v1/appointments \
  -H "Content-Type: application/json" \
  -d '{"facility_id":"f1","service_unit_id":"u1","appointment_time":"2026-06-22T09:00:00Z","patient_phone":"081234567890","patient_display_name":"Budi Santoso"}'

# Response akan mengembalikan checkin_code (contoh: "A3B9K2")
```

**Check-in janji temu:**
```bash
# Check-in dengan kode (mendapat nomor antrean)
curl -X POST http://localhost:8080/api/v1/appointments/appt-id/check-in \
  -H "Content-Type: application/json" \
  -d '{"checkin_code":"A3B9K2"}'
```

## Desain Sistem UI (Strict)

- Tipografi jelas (Inter/system), tracking ketat pada judul
- Whitespace lega (px-6, py-8, gap-6)
- Satu aksen warna: emerald-600 (kesehatan & kepercayaan)
- Kartu bersih dengan border 1px, tanpa bayangan berlebih
- Progress bar tipis (h-[5px])
- Dukungan dark/light sempurna dari awal
- Tidak ada elemen generik "AI slop"

Komponen dashboard dibuat dengan Svelte 5 runes, helper murni kecil, dan kode sangat mudah dibaca.

### Admin API

Operasional fasilitas dan antrean dilakukan melalui endpoint terpisah dengan RBAC.

**Facility Administration** (`facility.read` / `facility.manage`):
- `GET /api/v1/admin/facilities` — List semua fasilitas
- `GET /api/v1/admin/facilities/{id}` — Detail fasilitas
- `POST /api/v1/admin/facilities` — Buat fasilitas baru
- `PATCH /api/v1/admin/facilities/{id}` — Update fasilitas
- `PATCH /api/v1/admin/facilities/{id}/deactivate` — Nonaktifkan fasilitas (soft delete)

Validasi: nama wajib, type enum (`rumah_sakit`, `puskesmas`), `available_beds ≤ total_beds`, dan sanitasi telepon/alamat.

**Queue Operator Console** (`queue.read` / `queue.manage`):
- `GET /api/v1/admin/queues?facility_id=` — List tiket antrean per fasilitas
- `GET /api/v1/admin/queues/{id}` — Detail tiket
- `PATCH /api/v1/admin/queues/{id}/status` — Update status antrean

State machine: `waiting→called→in_service→completed`, plus `cancelled` dan `skipped`.

**Admin UI** (`/admin`)
- `/admin/facilities` — Manajemen fasilitas (list, create, edit, deactivate)
- `/admin/queues` — Konsole operator antrean (status badge, transisi status)
- `/admin/schedules` — Manajemen jadwal praktik (list, create, edit)
- `/admin/appointments` — Daftar janji temu dengan kontrol update status

**Patient UI** (`/appointments`)
- `/appointments/new` — Form booking janji temu publik
- `/appointments/check-in` — Form check-in dengan kode check-in

Audit event untuk setiap mutasi: `facility.created`, `facility.updated`, `facility.deactivated`, `queue.status_updated`, `service_unit.created`, `service_unit.updated`, `schedule.created`, `schedule.updated`, `appointment.created`, `appointment.status_updated`, `appointment.checked_in`.

**Service Unit Administration** (`service_unit.manage`):
- `GET /api/v1/admin/service-units` — List layanan (service unit)
- `POST /api/v1/admin/service-units` — Buat layanan baru
- `PATCH /api/v1/admin/service-units/{id}` — Update layanan

**Schedule Administration** (`schedule.read` / `schedule.manage`):
- `GET /api/v1/admin/schedules` — List jadwal praktik
- `POST /api/v1/admin/schedules` — Buat jadwal praktik
- `PATCH /api/v1/admin/schedules/{id}` — Update jadwal praktik

Validasi: `start_time < end_time`, `slot_minutes ≥ 1`, `capacity_per_slot ≥ 1`.

**Appointment Administration** (`appointment.read` / `appointment.manage`):
- `GET /api/v1/admin/appointments` — List janji temu dengan filter status
- `PATCH /api/v1/admin/appointments/{id}/status` — Update status janji temu (state machine enforced)

State machine: `scheduled→checked_in→queued→completed`, plus `cancelled` dan `no_show`.

**Public Booking & Check-In** (tanpa autentikasi, rate-limited by phone):
- `POST /api/v1/appointments` — Booking janji temu. Mengembalikan `checkin_code` (6 karakter alphanumeric, uppercase). Rate-limited 2 booking/hari per nomor telepon via `limiter.DailyLimiter`. Validasi kapasitas: jumlah booking pada slot jadwal ≤ `capacity_per_slot`.
- `POST /api/v1/appointments/{id}/check-in` — Check-in dengan `checkin_code`. Memvalidasi kode, memanggil Rust gRPC `GenerateQueueNumber`, menyimpan `queue_ticket_id` di appointment, transisi status `scheduled→checked_in→queued`.

## Catatan Keamanan & Privasi (Penting)

MVP ini **hanya untuk tujuan scaffolding dan demonstrasi**.

- Jangan gunakan data pasien nyata (**never use real patient data**).
- Autentikasi tersedia (dev / JWT) tetapi OIDC discovery belum diimplementasikan penuh.
- Rate limiting aktif pada endpoint publik booking (2 per hari per nomor telepon) dan generate antrean.
- Audit event mencatat semua mutasi dengan metadata yang disanitasi (redaksi phone/patient/name/address dari metadata keys).
- Untuk produksi: audit keamanan, consent, minimization data, encryption at rest, dan logging yang sesuai regulasi kesehatan daerah wajib dilakukan.

Lihat [`SECURITY.md`](./SECURITY.md) untuk daftar lengkap **security limitation** dan panduan pengungkapan kerentanan (responsible disclosure).

## Lisensi

MIT — silakan fork, modifikasi, dan kontribusi untuk kepentingan publik.

---

## 1-Click Deploy dengan Docker (Enterprise Ready)

Seluruh stack (Postgres + Rust Engine + Go API + SvelteKit Web) bisa dijalankan dengan **satu perintah**.

```bash
# 1. Clone repo
git clone https://github.com/lostsky1403/Sigap.git
cd Sigap

# 2. Jalankan seluruh sistem (build otomatis)
docker compose up -d --build

# 3. (Sekali saja) Jalankan migrasi database
docker compose exec -T postgres psql -U sigap -d sigap -f /docker-entrypoint-initdb.d/0001_init.sql

# 4. Buka aplikasi
# Web UI:   http://localhost:3000
# API:      http://localhost:8080/health
# SSE:      http://localhost:8080/api/v1/events/beds   (EventSource)
# gRPC:     localhost:50051  (Rust engine)
```

Setelah `docker compose up`, coba buat antrean via curl:

```bash
curl -X POST http://localhost:8080/api/v1/queues/generate \
  -H "Content-Type: application/json" \
  -d '{"facilityId":"f1","patient":{"fullName":"Test Warga","phone":"081234567890"}}'
```

Anda akan melihat **processing_time** dalam response (contoh: `"processing_time":"87µs"`) dan UI web akan langsung update real-time via SSE tanpa refresh.

Untuk stop:

```bash
docker compose down -v
```

File yang relevan:
- `docker-compose.yml`
- `apps/api/Dockerfile`, `apps/queue-engine/Dockerfile`, `apps/web/Dockerfile`

---

Dibuat dengan disiplin sebagai Senior Full-Stack Engineer & Open-Source Maintainer. Semua kode mengikuti prinsip KISS, explicit error handling, named constants, dan desain yang tenang serta dapat dipercaya.

Untuk detail arsitektur lengkap dan langkah implementasi selanjutnya, lihat rencana yang telah disetujui.
