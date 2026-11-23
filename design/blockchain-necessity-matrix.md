# Blockchain Necessity Matrix

**Status:** PROPOSED (decision matrix — the underlying audit log is already IMPLEMENTED)
**Scope:** Sigap audit-log integrity; whether any blockchain / distributed-ledger construct is necessary.
**Owner:** Platform / Security
**Last updated:** 2026-11-08

> **Honesty markers used throughout this document**
> `[IMPLEMENTED]` — live in the current codebase today.
> `[PROPOSED]` — design recommendation, not yet built.
> `[POLICY-DECISION-REQUIRED]` — needs an explicit owner decision before it can be considered real.

---

## TL;DR / Recommendation

| Question | Answer |
|---|---|
| Do we need a blockchain? | **No.** |
| Default choice | **Option A — append-only, hash-chained, tamper-evident audit log** (already live). |
| Why | The existing PostgreSQL hash chain already delivers tamper-evidence against a single trusted operator, which matches the actual threat model; Options B and C add operational cost and privacy surface without demonstrably strengthening audit for a single-facility-hub service with no multi-organization dispute-resolution requirement. |

**Recommendation:** Adopt and harden **Option A**. Treat **Option B** (WORM + periodic cryptographic commitments) as a *gated escalation path*, not a default. **Option C** (permissioned ledger) is not justified on current evidence.

---

## 1. Context and scope

Sigap is a single-operator civic-health service: one platform operator runs the API, the queue engine, and the PostgreSQL database. There is a single logical audit participant chain — there is no set of *mutually distrusting* organizations that must each independently verify one another's records for a shared workflow.

The question under evaluation: does any blockchain / distributed-ledger technology (DLT) add demonstrable value over a plain, well-engineered audit log for this profile?

`[POLICY-DECISION-REQUIRED]` Whether Sigap is legally obligated (e.g. by a health regulator or a public-sector mandate under Indonesian law) to produce *independent* tamper-evidence — evidence that survives a fully compromised or malicious platform operator — has not been established. Until such a requirement is cited, we do not engineer for it.

---

## 2. Current state — what already exists `[IMPLEMENTED]`

The Go backend already ships an append-only, content-hash-chained audit log (`apps/api/internal/audit/service.go`, writing to the `audit_events` table):

- **Append-only insert** into PostgreSQL; events are never updated or deleted by the audit path.
- **SHA-256 hash chain:** each event's `event_hash` is `SHA-256(canonical payload + previous_hash)`, so tampering with any earlier row breaks all subsequent hashes.
- **Privacy controls already applied:**
  - IP addresses and user-agents are stored as **unsalted SHA-256 hashes**, not raw (see §8 for the low-entropy caveat).
  - Metadata is **sanitized** against a forbid-list (`patient`, `pasien`, `phone`, `telepon`, `nik`, `ktp`, `name`, `nama`, `address`, `alamat`, `email`) — matching keys are replaced with `[REDACTED]`. Matching is **shallow**: only top-level metadata keys are checked; nested objects are preserved as-is and their inner PII keys are *not* recursively scrubbed (test-verified limitation).
- **Best-effort semantics:** audit writes do not block the request's critical path; failures are logged, not returned to the caller.

**Known gaps `[IMPLEMENTED]` (accurate as of this writing):**

1. The hash chain is **best-effort** — under concurrent writers the `previous_hash` read may race, producing occasional gaps in the chain.
2. The chain has **no external anchor**: integrity is only meaningful to someone who can read the database; a privileged DB operator could, in principle, rewrite rows *and* recompute the chain.
3. There is **no retained independent commitment** (no periodically published hash root, no WORM store).

---

## 3. The three options

### Option A — Append-only audit log (signed / hash-chained)

`[IMPLEMENTED]` (hash chain) — per-event digital signatures are `[PROPOSED]` and are **not present in the current code**.

Append-only audit rows, each carrying a hash binding it to the previous row (live) and, optionally, a per-event signature (proposed). Verification requires reading all rows and recomputing the chain; today that verification is only meaningful to a party that can read the database.

### Option B — Signed events + WORM storage + periodic cryptographic commitments

`[PROPOSED]`

The same append-only audit stream, with three upgrades: (a) each event additionally carries a digital signature (event signing), (b) writes land on Write-Once-Read-Many storage, and (c) the chain root is **periodically published/anchored** outside the operator's writable scope (e.g. published to a public immutable medium, a third-party notarization service, or a trusted timestamp authority). This yields tamper-*evidence that survives the operator*, without standing up a multi-party peer network.

### Option C — Permissioned multi-organization distributed ledger

`[PROPOSED]`, **not recommended**

A permissioned ledger (e.g. a consortium chain) where several independent organizations each run a node, jointly committing an agreed record. This is only meaningful when independent, mutually-distrusting organizations share a workflow and need a common, dispute-proof record.

---

## 4. Decision matrix

Threat model in one line: **A** addresses accidental loss, low-skill tampering, and routine misconduct (detectable by anyone who can read the DB); **B** adds tamper-evidence that survives a malicious/compromised operator; **C** additionally covers collusion among multiple operators — a concern that only exists if several mutually-distrusting organizations share the record.

| Dimension | A — append-only audit log (hash chain live; signing proposed) | B — signed events + WORM + commitments | C — permissioned multi-org DLT |
|---|---|---|---|
| **Trust requirements** | Trust the single operator's DB admin. Tampering is detectable by anyone who can read the DB, but the evidence does *not* survive a malicious operator who rewrites rows and recomputes the chain. | Trust the operator for writes plus a *minimal* anchoring party (TSA / notary / public medium) for the anchor. Survives a malicious operator for anchored portions. | Trust a *majority* of independent node operators (m-of-n). Survives a malicious or colluding minority. |
| **Independent governance** | None — single operator; no shared governance exists or is needed. | Minimal — a one-way contract with an anchoring/notarization service; no peer governance. | Full consortium governance — membership, voting, policy, join/leave. This governance is itself the main cost, and it only buys anything if several mutually-distrusting orgs share the record. |
| **Privacy** | Controlled by log policies; data stays inside the operator's own DB. (Precision: sanitization is shallow — see §2.) | Anchors are high-entropy batch roots only; no record payload leaves the operator. | **Worst** — events replicate to every independent node, widening exposure across third parties and conflicting with the §7 on-chain prohibition. |
| **Scalability** | Inherits PostgreSQL; scales with the DB; audit volume is bounded by facility throughput. | Bounded by WORM write throughput and anchor cadence; still single-writer and generally adequate. | **Worst** — every independent node must order and replicate every event, so throughput is bounded by consensus and degrades as more nodes join. |
| **Operational cost** | Lowest — already in production on existing infrastructure. | Medium — WORM media + notarization/anchoring fees + anchor operations. | Highest — node infrastructure, ops staff, and ongoing consortium governance. |
| **Key management** | Minimal — the hash chain needs no keys; if per-event signatures are added (proposed), one operator signing key + rotation. | Moderate — operator signing key + anchoring/notarization credentials + verifier/TSA keys; cadenced rotation matters. | High — per-node identity keys, consensus/PKI keys, TLS, and key custody across organizations; rotation spans the consortium. |
| **Data correction** | Easy — DB-native row correction under a controlled operator process; the chain is recomputed past the correction point. | Harder — WORM resists in-place edits; corrections need compensating records rather than rewrites. | **Very hard** — immutability plus replication makes correcting a mistaken row practically adversarial across nodes. |
| **Retention / deletion obligations** | Fully operator-controlled retention/archival; erasure straightforward and aligned with data-subject rights. | Operator-controlled but pinned to WORM media; deletion interacts with immutable media. | **Tension** — retention is consortium-governed and hard to change mid-flight; immutability conflicts with erasure/rectification obligations. |
| **Incident handling** | Rotate/revoke DB credentials, restore from backups; single-operator blast radius. | Revoke anchor/notarization credentials and rekey; manage WORM media; still single-operator coordination. | Credential rotation and remediation span consortium governance; multi-org coordination is slow. |
| **Independent verification** | Operator-side recompute only — no third-party proof. An auditor with full DB read access can recompute the chain; there is no operator-independent commitment. | **Stronger** — an external party can verify published roots against WORM/anchored commitments without trusting the operator. | Strongest — but only resolves the multi-party dispute problem; it buys nothing for a single-operator service. |

---

## 5. Evidence-based recommendation

**Recommend Option A — retain and harden the existing hash-chained append-only audit log.**

Justification:

1. **The actual threat model is a single trusted operator.** There is no multi-organization workflow with mutually-distrusting parties that would need a shared dispute-proof record. A permissioned ledger's entire value proposition assumes that exact problem, which Sigap does not have.
2. **Option A already delivers the relevant property** — content hashing makes tampering *detectable* by anyone who can read the DB — and it does so with the lowest operational burden and the lowest privacy surface.
3. **Privacy and data-subject rights (erasure/rectification) are materially easier under A.** Immutability (B/C) is a *liability* against erasure obligations for sensitive health-adjacent records; Option A keeps the control of correction where the operator already legally needs it.
4. **Cost and incident response scale badly toward C** with no commensurate assurance gain for this profile.
5. **Hardening A closes its real gap more cheaply than migrating to B/C** (see §6): close the concurrency race in the hash chain and add periodic root publication *only if* independent operator-surviving evidence is ever required.

The default is the simpler auditable solution because blockchain adds value only when it replaces *trust between parties*. Here there is one party. We therefore recommend Option B — and only Option B — *if and only if* a demonstrable operator-independent or multi-organization trust problem is cited (§6); absent that, the already-live Option A log is the sufficient, proportionate default, and no node of any kind needs to be deployed.

---

## 6. Escalation triggers — when to revisit B (and only B)

Reconsider Option B — and stop well short of C — `[POLICY-DECISION-REQUIRED]` only if one of the following is cited as an explicit requirement:

1. A regulator or public-sector mandate requires **operator-independent** tamper-evidence (proof that survives a malicious platform operator).
2. A second independent organization begins jointly committing the same workflow and needs a common dispute-proof record.
3. A documented insider-threat incident demonstrates that DB-operator tampering is both *feasible* and *going undetected* today.

Even then, the proportionate answer is almost always **Option B** (anchoring the existing hash-chain root to an external immutable medium), not standing up a multi-node permissioned ledger.

---

## 7. What MUST never go on-chain — in any form

The following **MUST NOT** be placed on any chain, in any commitment, in any form — plaintext, encrypted, or "pseudonymised" — because hashes of low-entropy identifiers are reversible by brute force/search and because replication irreversibly spreads the data:

- **NIK** (national ID number)
- **Patient name** (full or partial)
- **Phone numbers**
- **Diagnoses** (ICD codes included — they are linkable attributes)
- **Allergies**
- **Prescriptions / medication records**
- **Full medical documents** (and any full document contents)
- **Raw identifiers** (patient record IDs, appointment IDs, facility internal IDs, email addresses, address text)
- **Linkable sensitive metadata** (exact timestamps combined with facility + procedure can re-identify; exact birth dates; exact visit times paired with a facility)

Only **high-entropy, non-identifying, synthetic commitments** (randomized audit chain roots, keyed HMACs with per-secret keys, or synthetic non-identifying digests) are ever eligible — and only under the PoC rules in §9.

> The existing forbid-list in the audit service (§2) is consistent with, but weaker than, this list; it operates at the *audit metadata* layer and does **not** authorise any on-chain or anchored use of the fields above. Treat this §7 list as the stronger, authoritative ceiling.

---

## 8. Correlation and inference risks

Any cryptographic-commitment design must account for the following:

1. **Low-entropy hashing is reversibility, not anonymization.** SHA-256 of a NIK, a phone number, or a short name is vulnerable to dictionary/brute-force and search over the small identifier space. Commitments over such inputs leak the input.
2. **Chain linkage amplifies re-identification.** A commitment chain that repeatedly hashes the *same* pseudonym lets an observer cluster events by that pseudonym and correlate against external datasets (visits, facility, time) to re-identify the person.
3. **Temporal + facility correlation.** Even fully synthetic-per-event commitments, if anchored at fine-grained real timestamps with a real facility and procedure code, form a linkable side-channel that can reconstruct activity patterns.
4. **Root-level anchoring is generally safe; payload-level anchoring is not.** Anchoring only a blinded *root hash* of a large batch leaks roughly nothing about individual records; anchoring per-record commits renews the §7 risks.

**Required mitigations `[PROPOSED]`** for any future anchoring:
- Anchor only **batch roots**, never per-record commits.
- **Coarse the time** domain (e.g. daily/weekly buckets), never exact event timestamps.
- Use **salted or keyed hashing** so identical inputs do not produce identical commitments.
- Publish commitments **only via the operator's controlled channel**, never embedding record-identifying attributes.

---

## 9. Proof-of-concept guardrails

If a PoC is ever justified, it is constrained as follows `[PROPOSED]`:

- **Synthetic non-identifying commitments ONLY.** Generated random values; no production identifiers, no real patient or staff data, no live facility data.
- **No production chain.** The PoC runs in an isolated environment. It never takes writes from `audit_events` of a live deployment.
- **No smart contract.** No executable on-chain logic; the PoC is limited to publishing/verifying a synthetic commitment (e.g. a root hash), nothing more.
- **No new npm/Go/Rust runtime dependencies** unless explicitly approved — a PoC should prefer a one-shot script or existing tooling.
- The PoC's output is labeled **synthetic** and must not be presented as a production capability.

---

## 10. Open policy decisions `[POLICY-DECISION-REQUIRED]`

These are blocking decisions for any future hardening, not for keeping Option A as-is:

1. **Retention period** for audit events (durations and any legal-regulatory minimum in Indonesia).
2. **Correction/erasure framework** — the operator process for rectifying or deleting an audit row when a data-subject right is exercised, and how the hash chain is re-anchored after such a correction.
3. **Whether operator-independent tamper-evidence is a legal necessity** (the single gate that determines whether Option B is ever pursued).

None of these is required to continue operating the existing Option A log.

---

*End of decision matrix.*