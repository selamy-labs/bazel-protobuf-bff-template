# Contract: Evidence Receipts and the Aggregate Gate

selamy-labs/relentless-ci-python#27 owns the shared evidence record format, the validator,
attempt semantics, independent review receipts and readiness evaluation. This repository only
produces receipts and runs one aggregate gate. It does not define readiness.

## Receipt (one per CI leg per attempt)

`quality/runner` writes the receipt, and each leg uploads it as
`verify-receipt-<job>-<attempt>`. Field names follow the runner, and they are aligned to #27 on adoption.

| Field | Meaning |
| --- | --- |
| `schema_version` | receipt schema version |
| `shard` | the leg: `build-test`, `e2e` or `policy`. Absent on a whole-stage run |
| `revision`, `tree`, `tested_tree` | full SHA of the commit under test, and what was tested |
| `policy_digest` | digest of the policy inputs in the tested tree |
| `base_revision`, `base_source` | the policy base and how it was chosen, once the leg has one. Naming a base claims no review |
| `base_policy_digest` | digest of the base's policy inputs, once the leg has a base |
| `producer` | workflow, job, run id and attempt; no personal identifiers |
| `attempt` | hosted run attempt |
| `result` | `PASSED`, `FAILED`, `INCOMPLETE` and other terminal results. Only terminal receipts are written |
| `checks`, `expected_checks` | one result per obligation of this leg |
| `inventory` | input and test counts |
| `claim_layer` | `local` or `hosted-ci`, from the environment that produced the receipt. A label, not an authentication |

## Aggregate gate (interim)

The aggregate job runs with `if: always()` and passes only if exactly the expected number of legs
reported `success` in `needs`, which is the latest result per leg. That is not "every leg in the newest
attempt": an earlier success beside a rerun success passes. `ciparity` compares the jobs and `needs` with the
registry's legs. It is the single required check name. Hosted behavior is unobserved.

The gate holds no permissions, and it does **not** validate receipt contents. Receipt validation is a tracked gap (T068) that
closes when #27 is adopted. Readiness is computed when it is evaluated. Nothing here pushes
invalidation when the base moves; that belongs to the #27 validator.
