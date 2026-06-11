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

- Skema database lengkap (fasilitas, pasien, tiket antrean, counter harian)
- Endpoint utama generate nomor antrean (akan diimplementasikan penuh di Go + Rust)
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

5. Test endpoint (setelah Phase 2 selesai)
   ```bash
   curl -X POST http://localhost:8080/api/v1/queues/generate \
     -H "Content-Type: application/json" \
     -d '{"facilityId":"...","patient":{"fullName":"Test Pasien","phone":"081234567890"}}'
   ```

Buka http://localhost:5173 — lihat Dashboard Ketersediaan Kasur yang rapi.

## Desain Sistem UI (Strict)

- Tipografi jelas (Inter/system), tracking ketat pada judul
- Whitespace lega (px-6, py-8, gap-6)
- Satu aksen warna: emerald-600 (kesehatan & kepercayaan)
- Kartu bersih dengan border 1px, tanpa bayangan berlebih
- Progress bar tipis (h-[5px])
- Dukungan dark/light sempurna dari awal
- Tidak ada elemen generik "AI slop"

Komponen dashboard dibuat dengan Svelte 5 runes, helper murni kecil, dan kode sangat mudah dibaca.

## Pengembangan Selanjutnya (Sesuai Rencana)

Lihat file `plan.md` di sesi (atau jalankan fase berikutnya):
- Phase 2: Implementasi penuh Go handler + gRPC client (TDD)
- Phase 3: Rust engine dengan sqlx transaction + counter atomic + tonic server
- Phase 4: Penyempurnaan Svelte (jika perlu)
- Phase 5: README lengkap + dokumentasi kontribusi

Semua perubahan dilakukan dalam chunk <300 baris, dengan commit konvensional, dan TDD di bagian perilaku kritis.

## Catatan Keamanan & Privasi (Penting)

MVP ini **hanya untuk tujuan scaffolding dan demonstrasi**.

- Jangan gunakan data pasien nyata.
- Belum ada autentikasi, rate limiting, atau enkripsi PII.
- Untuk produksi: audit keamanan, consent, minimization data, encryption at rest, dan logging yang sesuai regulasi kesehatan daerah wajib dilakukan.

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
