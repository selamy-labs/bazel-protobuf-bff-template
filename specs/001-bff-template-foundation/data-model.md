# Data Model: Generic Bazel/Protobuf BFF Template

Two families of entities: the **example application** (deliberately tiny and neutral) and the
**quality system** (the reusable part). Field lists are design-level; schemas are fixed in M2/M5.

## Example application

### Note

| Field | Type | Rules |
| --- | --- | --- |
| `name` | string, resource name `notes/{note_id}` | output only; server-assigned |
| `title` | string | required; 1–120 characters after trimming |
| `body` | string | optional; ≤ 4,000 characters |
| `create_time` | timestamp | output only |

State: created → (immutable in the slice). No update/delete RPCs until a browser caller needs them.

### Principal (BFF boundary)

| Field | Notes |
| --- | --- |
| `subject` | opaque id from the `Authenticator` |
| `kind` | `TEST_IDENTITY` or `VERIFIED`; `TEST_IDENTITY` is rejected in production mode |

Notes are scoped to the principal: a note created by one subject is `not_found` for another.

## Quality system

### Check

| Field | Notes |
| --- | --- |
| `id` | stable, kebab-case (e.g. `proto-breaking`); never reused for a different rule |
| `tool` | Bazel label of the pinned tool; version recorded in lock |
| `selector` | glob set + language tag deciding which files are inputs |
| `stages` | subset of `pre-commit`, `commit-msg`, `pre-push`, `full` (`full` is mandatory; prek maps `full` to its `manual` stage) |
| `classification` | `GUARANTEE` (fails) or `SIGNAL` (reported, never fails) |
| `threshold` | optional typed value (e.g. floor percentage); direction (`MIN`/`MAX`) |
| `timeout` | wall-clock seconds |
| `kind` | `bazel` (composed into the stage's single Bazel invocation) or `native` (bounded pool) |
| `allow_empty` | default false; a GUARANTEE with zero inputs at `full` is an error otherwise |

Validation: unique ids; `full ∈ stages`; `SIGNAL` checks have no threshold that can fail; every
`GUARANTEE` has ≥1 defect fixture and its corrected counterpart.

### Registry

Ordered set of Checks + `version` (semver of the quality module) + `digest` (canonical hash).
Weakening relative to a baseline registry (detected by the adopted #26 contract; interim: base-built
floors only) = any removed check, removed stage, `GUARANTEE→SIGNAL`
change, widened exclusion/narrowed selector, lowered `MIN` / raised `MAX` threshold, longer
suppression allowlist, or changed tool without a version increase.

### Stage run

`stage`, `revision`, `file_set` (staged blobs or changed paths), selected checks, `started/finished`,
`concurrency_limit`, per-check `Result`.

### Result

`check_id`, `state` ∈ {`PASSED`, `FAILED`, `TIMED_OUT`, `CANCELLED`, `ERROR`, `SKIPPED_NOT_APPLICABLE`},
`diagnostics[]` (bounded), `rerun`, `duration_ms`, `peak_rss_kb`. Only `PASSED` and
`SKIPPED_NOT_APPLICABLE` are non-blocking, and `SKIPPED_NOT_APPLICABLE` is allowed at `full` only for
checks declaring `allow_empty`.

### Evidence receipt / readiness

See [contracts/evidence-receipt.md](contracts/evidence-receipt.md). Readiness semantics (attempts,
conflicts, invalidation) are owned by py#27; this repository only produces receipts and an aggregate
gate.

### Budget

`host_profile` (cpu, memory, os, bazel version), `scenario`, `cold|warm`, `n`, `p50_ms`, `p95_ms`,
`peak_rss_kb`, `measured_at`, `revision`. Declared budget = measured p95 plus a documented margin.

### Provenance entry

`path or package`, `source`, `revision`, `license (SPDX)`, `transformation` (vendored / adapted /
pattern-only), `reviewed_by`, `reviewed_at`.
