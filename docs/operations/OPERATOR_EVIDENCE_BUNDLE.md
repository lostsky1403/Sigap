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
docker inspect --format '{{.Name}} image={{.Image}} restart={{.RestartCount}} started={{.State.StartedAt}}' \
  $(docker ps -q --filter "name=sigap") 2>/dev/null
docker images --format '{{.Repository}}:{{.Tag}} {{.ID}} {{.CreatedAt}}' | grep -i sigap
```

## 5. Environment — safe values only

Print **variable names and safe values only**. Never echo credentials.

```sh
# Non-secret values (safe to return):
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' $(docker ps -q --filter "name=sigap-api") \
  | grep -E '^SIGAP_ENV=|^SIGAP_AUTH_MODE=|^SIGAP_TLS_TERMINATED=|^SIGAP_TRUSTED_PROXIES='

# Presence-only (NEVER print the value).
IDS=$(docker ps -q --filter "name=sigap-api")$(docker ps -q --filter "name=sigap-web")
if [ -z "$IDS" ]; then
  echo "NO SIGAP CONTAINERS RUNNING - presence check is UNKNOWN, not ABSENT"
else
  for v in SIGAP_DEV_IDENTITY SIGAP_LOCAL_RBAC_TEST_IDENTITY SIGAP_LOCAL_E2E_ACTOR \
           SIGAP_ENGINE_FALLBACK PUBLIC_SUPABASE_URL PUBLIC_SUPABASE_ANON_KEY \
           SIGAP_API_INTERNAL SIGAP_AUTH_ISSUER SIGAP_AUTH_AUDIENCE SIGAP_AUTH_JWKS_URL \
           SIGAP_DATABASE_URL POSTGRES_PASSWORD; do
    if docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' $IDS 2>/dev/null \
         | grep -q "^${v}="; then echo "${v}=PRESENT"; else echo "${v}=ABSENT"; fi
  done
fi
```

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
# Keys only: Traefik middleware labels (e.g. *.basicauth.users) can hold
# credential material, so never print label VALUES verbatim.
docker inspect --format '{{range $k,$v := .Config.Labels}}{{println $k}}{{end}}' \
  $(docker ps -q --filter "name=sigap-web") | grep -i traefik | sort
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
| Environment presence + safe booleans | 5 |
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
- Nothing here mutates state. If a command in this document is not in your environment, skip
  it and say so rather than substituting a mutating command.
- Return the raw output; the preflight will read it, not re-derive it.
