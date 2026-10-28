# Contract: Evidence Receipts and the Aggregate Gate

selamy-labs/relentless-ci-python#27 owns the shared evidence record format, the validator,
attempt semantics, independent review receipts and readiness evaluation. This repository only
produces receipts and runs one aggregate gate. It does not define readiness.

## Receipt (one per CI shard per attempt)

| Field | Meaning |
| --- | --- |
| `schema_version` | receipt schema version (aligned to #27 on adoption) |
| `prerequisite` | shard id, e.g. `ci/full:build-test`, `ci/full:e2e`, `ci/full:policy` |
| `revision` | full SHA of the commit under test |
| `base_revision` | full SHA of the comparison baseline |
| `base_policy_digest` | digest of the base revision's registry, i.e. the policy that judged this run (R-09) |
| `producer` | workflow, job, run id and attempt; no personal identifiers |
| `attempt` | attempt number |
| `state` | `SUCCEEDED`, `FAILED`, `CANCELLED`, `TIMED_OUT` or `ERROR` (terminal states only are written) |
| `inventory` | expected and observed counts of checks and tests |
| `location` | artifact path of the full result |
| `claim_layer` | always `hosted-ci` for these receipts |

## Aggregate gate (interim)

The aggregate job runs with `if: always()` and passes only if every expected shard's **latest
attempt** for the checked-out head SHA is `success`. It is the single required check name.

The gate does **not** validate receipt contents. Receipt validation is a tracked gap (T068) that
closes when #27 is adopted. Readiness is computed when it is evaluated. Nothing here pushes
invalidation when the base moves; that belongs to the #27 validator.
