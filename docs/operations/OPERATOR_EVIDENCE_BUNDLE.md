# Sigap — Operator Evidence Bundle (read-only)

Run these commands **on the production host** and return the output. Every command is
**read-only**: nothing here starts, stops, recreates, pulls, migrates, seeds, or writes.
Do not edit any file. Do not run the application's protected routes (they write audit rows).

Set the deployment directory once, then run the blocks in order.

```sh
# 0. Deployment directory (REQUIRED - do not guess)
SIGAP_DEPLOY_DIR="${SIGAP_DEPLOY_DIR:?set SIGAP_DEPLOY_DIR to the compose directory}"
cd "$SIGAP_DEPLOY_DIR" || exit 1
```

## 1. Deployment directory + repo identity

```sh
pwd
git rev-parse --show-toplevel 2>/dev/null || echo "not a git repo"
git rev-parse --abbrev-ref HEAD 2>/dev/null
git rev-parse HEAD 2>/dev/null
git status --porcelain 2>/dev/null | head -20
```

## 2. Compose + tooling

```sh
docker compose version
docker --version
ls -1 docker-compose*.yml
```

## 3. Services in use (resolved config)

```sh
docker compose -f docker-compose.yml -f docker-compose.prod-ports.yml -f docker-compose.prod-edge.yml config --services
```

## 4. Containers: status, images, digests

```sh
docker ps --filter "name=sigap" --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
docker images --format '{{.Repository}}:{{.Tag}} {{.ID}} {{.CreatedAt}}' | grep -i sigap
# Authoritative per-container identity. Inspected by EXACT NAME, one at a time:
# `--filter name=` is a substring match and can select an unrelated container
# (e.g. "sigap-api-old"), and a single combined invocation hides which container
# failed. A failure here must be visible, never swallowed.
for c in sigap-postgres sigap-rust-engine sigap-api sigap-web; do
  docker inspect --format '{{.Name}} image={{.Image}} restart={{.RestartCount}} started={{.State.StartedAt}}' "$c" 2>&1 \
    || printf '%s=INSPECT_FAILED_OR_MISSING\n' "$c"
done
```

## 5. Environment — safe values only

Print **variable names and safe values only**. Never echo credentials.

```sh
# Use the committed helper. It inspects each container by EXACT NAME and reports
# a THREE-STATE result: PRESENT / ABSENT / UNKNOWN. It exits non-zero if any
# container is missing or its inspect fails, so a failed check can never be
# recorded as "the variable is unset".
sh scripts/ops/evidence-env-presence.sh sigap-api sigap-web
```

Do **not** hand-roll this as a single combined `docker inspect $(...) $(...)`
invocation. That form concatenates the two command substitutions into one
over-long token, `docker inspect` fails with "no such object", and — if the
error is swallowed — every variable is reported ABSENT, including
`SIGAP_DEV_IDENTITY`. A false ABSENT is a false PASS. The regression controls
for this live in `scripts/ops/test-operator-evidence-bundle.sh`.

Report back, as literal values:

- `SIGAP_ENV` = <exact value>   (must be non-local)
- `SIGAP_AUTH_MODE` = <exact value>   (must be `jwt`)
- `SIGAP_DEV_IDENTITY` = PRESENT/ABSENT (and if present, its literal value)
- everything else = PRESENT/ABSENT

## 6. Port bindings + network membership

```sh
docker ps --filter "name=sigap" --format '{{.Names}}' \
  | xargs -r docker inspect --format '{{.Name}} ports={{json .NetworkSettings.Ports}} networks={{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}'
```

Expected: postgres/engine/api bound to `127.0.0.1` only; web bound to `127.0.0.1:3005`; web
also attached to the shared edge network.

## 7. Reverse proxy metadata (keys only — do not change anything, do not dump values)

```sh
# KEYS ONLY. Traefik middleware labels (e.g. *.basicauth.users) can hold
# credential material, so never print label VALUES verbatim.
#
# Do NOT substitute `--format '{{json .Config.Labels}}'` or `docker inspect
# <name>` without a format: both emit every label VALUE. A prior collection
# deviated this way; no credential label exists today, but the guarantee is
# only real if the command is.
docker inspect --format '{{range $k,$v := .Config.Labels}}{{$k}}{{println}}{{end}}' \
  sigap-web | grep -i traefik | sort
docker network ls | grep -i traefik
```

If a specific routing value is needed, read the single key you need
(`traefik.http.routers.sigap-web.rule`, `…entrypoints`, `…tls.certresolver`) — never the
whole label set.

## 8. Release identity

```sh
curl -fsS https://<SIGAP_PUBLIC_HOST>/_app/version.json || echo "unreachable"
```

## 9. Health (public, unauthenticated routes only)

```sh
curl -fsS -o /dev/null -w 'web / -> %{http_code}\n'      https://<SIGAP_PUBLIC_HOST>/
curl -fsS -o /dev/null -w 'catalog -> %{http_code}\n'    https://<SIGAP_PUBLIC_HOST>/api/v1/public/facilities
```

> **Do NOT probe `/api/v1/admin/*`.** Those routes are RBAC-protected and every refusal
> writes an `audit_events` row (`internal/identity/authz.go` → `audit.Service.LogEvent`).
> They are excluded from read-only preflight by policy (runbook §8a).

## 10. API liveness/readiness (from inside the host only)

```sh
curl -fsS http://127.0.0.1:18080/health || echo "unreachable"
curl -fsS http://127.0.0.1:18080/readyz || echo "unreachable"
```

---

### What this bundle establishes

| Preflight requirement | Command block |
|---|---|
| Deployment directory | 1 (`pwd`, `SIGAP_DEPLOY_DIR`) |
| Deployed commit (if repo-based) | 1 (`git rev-parse HEAD`) |
| Compose version | 2 |
| Compose files in use | 2, 3 |
| Container status | 4 |
| Image IDs / digests / tags | 4 |
| Environment presence + safe booleans | 5 (`evidence-env-presence.sh`) |
| `SIGAP_ENV` exact non-secret value | 5 |
| `SIGAP_DEV_IDENTITY` safe state | 5 |
| `SIGAP_AUTH_MODE` non-secret mode | 5 |
| Port bindings | 6 |
| Network membership | 6 |
| Reverse-proxy labels/network | 7 |
| Release identifier | 8 |
| Health status | 9, 10 |

### Handling rules

- Never paste secret values. `POSTGRES_PASSWORD`, `SIGAP_DATABASE_URL`, `SIGAP_AUTH_*` tokens
  and any key are reported PRESENT/ABSENT only.
- Presence is **three-state**: PRESENT, ABSENT, or UNKNOWN. A missing container or a failed
  `docker inspect` yields UNKNOWN and a non-zero exit from `evidence-env-presence.sh` — never
  ABSENT. Recording UNKNOWN as ABSENT is a false PASS on the dev-only-flag control.
- Nothing here mutates state. If a command in this document is not in your environment, skip
  it and say so rather than substituting a mutating command.
- Return the raw output; the preflight will read it, not re-derive it.
- Container identity is read by **exact name** and by **image ID**, never by the mutable
  `latest` tag and never inferred from image creation time or the host Git checkout.

### Regression controls

`scripts/ops/test-operator-evidence-bundle.sh` proves, locally and with no Docker daemon,
that: the concatenated-id form cannot resolve a container; a missing container is reported
MISSING rather than ABSENT; an inspect failure is never converted to ABSENT; no secret value
is printed; and a decoy container (`sigap-api-old`) is not selected by exact-name inspection.
Run it whenever this document's commands change.
