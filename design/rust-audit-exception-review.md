# Rust audit exception review — PR #96 (read-only verification)

PR #96 makes `cargo audit` blocking in CI (`.github/workflows/ci.yml`: removed both
`|| true` suppressions) and keeps two documented exceptions in
`apps/queue-engine/.cargo/audit.toml`:

```
ignore = ["RUSTSEC-2023-0071", "RUSTSEC-2026-0459"]
```

**This is not a clean advisory database.** `cargo audit` exits 0 only because those two
advisories are explicitly allowed. The RustSec database itself contains many advisories and
a current run also emits an unmaintained-crate warning for `chrono` (RUSTSEC-2026-0543),
which is not an error.

## Verified scope of each exception

| Advisory | Crate in lockfile | Build-graph evidence (PR head) | Scope verdict |
| --- | --- | --- | --- |
| `RUSTSEC-2023-0071` | `rsa 0.9.10` via optional `sqlx-mysql` | `cargo tree -i rsa` and `cargo tree --target all -i rsa` → *"nothing to print"*; no `sqlite`/`mysql` reference in `src/`, `build.rs`, or `Cargo.toml`; `sqlx` pinned `default-features = false`, features `postgres` only. | Narrow and currently valid: excluded MySQL dependency path is unused. |
| `RUSTSEC-2026-0459` | `libsqlite3-sys 0.30.1` via optional `sqlx-sqlite` | `cargo tree -i libsqlite3-sys` and `cargo tree --target all -i libsqlite3-sys` → *"nothing to print"*; `sqlx-mysql`/`sqlx-sqlite` likewise absent from the resolved graph; all four crates exist only as `Cargo.lock` entries. | Narrow and currently valid: the production queue-engine build does not activate or link SQLite. |

`cargo audit` on the exact PR head in a disposable checkout: exit 0, *"1 allowed warning
found"* (`chrono` unmaintained), no unhandled advisory. The CI status for all 7 jobs on
`036a1aa9…` is `success`.

## Future-change risk — OPEN

Both exceptions are **reachability assumptions**, not fixed facts. Enabling the `sqlite` or
`mysql` feature on `sqlx` (directly, via a workspace feature unification, or by adding a
dependency that selects those features) would pull `libsqlite3-sys` / `rsa` into the build
graph while `cargo audit` would keep ignoring the advisories. Nothing in the current CI
detects that transition: the gate checks *new findings*, not the continued non-reachability
of an ignored crate.

## Proposed regression check (not implemented here — would change the PR head)

Add a CI step in the `Rust Vulnerability Scan` job, immediately after `cargo audit`, that
fails if an ignored advisory becomes reachable:

```sh
cd apps/queue-engine
# Each ignored advisory must remain absent from the resolved build graph.
for crate in rsa libsqlite3-sys; do
  if cargo tree --target all -i "$crate" 2>/dev/null | grep -q "$crate"; then
    echo "FAIL: ignored-advisory crate '$crate' is now in the build graph; remove the ignore and remediate"
    exit 1
  fi
done
# Feature guard: the sqlx declaration must stay Postgres-only.
grep -q 'default-features = false' Cargo.toml || { echo 'FAIL: sqlx default features enabled'; exit 1; }
if grep -E 'features.*(sqlite|mysql)' Cargo.toml; then
  echo 'FAIL: sqlite/mysql feature enabled; re-review both advisory ignores'
  exit 1
fi
```

Because this is a source/CI change it belongs in a separate, owner-approved follow-up PR;
it would require re-running the affected tests and re-verifying the new PR head. It is not
part of PR #96's verified baseline.
