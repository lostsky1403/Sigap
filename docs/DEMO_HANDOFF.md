# Sigap Demo Handoff Checkpoint

Operational checkpoint for the current **demo-green** state on `main`. Use this to reproduce the local demo without re-discovering setup.

For full walkthroughs see [DEMO_FLOW.md](./DEMO_FLOW.md) and [LOCAL_DEMO_RUNBOOK.md](./LOCAL_DEMO_RUNBOOK.md).

---

## Green checkpoint (2026-07)

| Item | Detail |
|------|--------|
| **Main** | Green after PR **#28** (admin queue / appointment response demo polish) and PR **#29** (frontend demo flow polish) |
| **Go toolchain** | `apps/api/go.mod` at **1.25.12** (CI `govulncheck`; was 1.25.11) |
| **Tags** | `local-demo-green-2026-07-09`, `frontend-demo-green-2026-07-10` |

### Latest validation (this checkpoint)

| Check | Result |
|-------|--------|
| `pr-autopilot -VerifyOnly` | **PASS** |
| `sigap-demo-smoke.ps1` | **PASS 8/8** |
| `sigap-full-local-demo.ps1` | **FULL LOCAL DEMO PASS** |
| → demo smoke | PASS 8/8 |
| → notification smoke | PASS 9/9 |
| → patient portal smoke | PASS 5/5 |

---

## Required local env vars

Set in the **same shell** that starts the API (and smoke scripts if they rely on overrides):

```powershell
$env:DATABASE_URL          = "postgresql://<user>:<password>@localhost:5432/sigap"
$env:SIGAP_DATABASE_URL    = $env:DATABASE_URL
$env:SIGAP_AUTH_MODE       = "dev"
$env:SIGAP_DEV_IDENTITY    = "true"
$env:SIGAP_ENGINE_FALLBACK = "dev"   # fast demo: no Rust queue engine
```

Optional: `SIGAP_API_BASE` (default smoke target is typically `http://[::1]:8080`).

Use placeholders only — put real credentials in local `.env` / shell, never in commits.

---

## Fast local demo (API)

With Postgres migrated + seeded and env vars set:

```powershell
cd apps/api
go run ./cmd/server
```

`SIGAP_ENGINE_FALLBACK=dev` uses the in-memory fake queue while still writing real `queue_tickets` rows, so check-in / queue / status flows work without `cargo` / `protoc`.

Web (optional for UI):

```powershell
pnpm --filter sigap-web dev
```

---

## Full local demo validation

Repo root, API already running (or as required by the script), env available:

```powershell
pwsh -NoProfile -File .\scripts\smoke\sigap-full-local-demo.ps1
```

Skip re-seed if DB already has demo data:

```powershell
pwsh -NoProfile -File .\scripts\smoke\sigap-full-local-demo.ps1 -SkipSeed
```

### Expected pass criteria

- Console ends with **`FULL LOCAL DEMO: PASS`**
- Demo smoke **8/8**
- Notification smoke **9/9**
- Patient portal smoke **5/5**

PR gate only:

```powershell
pwsh -NoProfile -File .\scripts\dev\pr-autopilot.ps1 -VerifyOnly -CommandTimeoutSeconds 300
```

Expect **PASS** (no app code changes required for this handoff doc).

---

## Troubleshooting

| Symptom | Likely cause / fix |
|---------|-------------------|
| Smoke: connection refused / network error | **API not running** on the smoke base URL. Start `go run ./cmd/server` with env set, then re-run smoke. |
| Auth / DB errors after opening a new terminal | **Env vars lost** on shell restart. Re-export `DATABASE_URL`, `SIGAP_*`, and `SIGAP_ENGINE_FALLBACK` (or re-source your local setup). |
| `psql` connects as Windows username / wrong DB | **`DATABASE_URL` unset**. Always pass `$env:DATABASE_URL` (or set it first); bare `psql` uses OS user defaults. |
| CI **`govulncheck`** fails after local green | **Go toolchain patch mismatch**. Align local + `apps/api/go.mod` with the version main uses for govulncheck (checkpoint: **1.25.12**). |

---

## Out of scope for this handoff

- No screenshots, binaries, or scratch trees
- Do not commit `.grok`, `.omo`, `.trae`, or AI attribution files
- Staging/production: never enable `SIGAP_DEV_IDENTITY` or `SIGAP_ENGINE_FALLBACK=dev`
