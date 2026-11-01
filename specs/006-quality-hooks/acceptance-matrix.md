# Acceptance / Evidence Matrix: M5

Classes: **G** deterministic guarantee, **S** review signal. Layers: **off** offline test, **loc**
local verifier, **ci** hosted CI (not run), **hum** human. Every negative row keeps its positive
control: the same setup must pass before a rejection earns credit.

| Req | Witness | Valid example | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- | --- |
| FR-001 | `runner_test`; `verify.sh` | a stage runs exactly its entries | an entry removed → absent from the stage and the receipt; no second list exists | G | off, loc |
| FR-002 | `runner_test` | the real registry validates | duplicate id, no `full`, failing `SIGNAL`, no witness, no class, unknown field → exit 2 | G | off |
| FR-003 | `native_fixtures_test`; review | adapter passes tools their config | an adapter threshold → review | G/S | off, hum |
| FR-004 | `runner_test`, `hook-e2e` | two Bazel checks | fake Bazel counts exactly one test invocation | G | off, loc |
| FR-005 | `runner_test` | a child exits 0 | stdin reader sees EOF; a hung, SIGTERM-ignoring `setsid` tree → `TIMED_OUT`, nothing left | G | off |
| FR-006 | `runner_test` | exit 0 | each table row of both code tables | G | off |
| FR-006 | `verify_body_test`, `preflight_test` | a stale receipt is removed once its owner has released, and a holder that releases within the bound is waited for | a wrapper started beside a running runner → exit 10 with the receipt intact; no `flock` → `bootstrap ERROR`, exit 2, nothing removed; lock held across a Bazel call → test fails | G | off |
| FR-007 | `runner_test` | format parsed back | 51 lines → 50 and `… 1 more`; a failure with no rerun → test fails | G | off |
| FR-008 | `runner_test` | `allow_empty` → not applicable | zero inputs at `full` → 12 | G | off |
| FR-009 | `fixtures_test` | clean staged commit | bad staged blob with clean working copy → rejected; bad working copy with clean staged blob → passes; `commit -a` and `commit <path>` read their own index | G | off |
| FR-009a | `fixtures_test` | an untracked bad file in an ordinary checked package → the commit passes, bytes unchanged | a broken file in the runner's dependency graph → `bootstrap ERROR`, exit 2, no check named, and a staged-defect rejection is never claimed | G | off |
| FR-010 | `fixtures_test`, `hook-e2e` | corrected push | pushed commit fails while the working tree holds the fix → rejected | G | off, loc |
| FR-011 | `runner_test` | an ordinary edit stays `pre-push` | a policy path or unknown base → `full`; pre-commit never | G | off |
| FR-012 | `runner_test`, `fixtures_test` | spaces, newline, rename, deletion | a NUL-unsafe split loses the file → fails | G | off |
| FR-013 | `runner_test` | second run proceeds after the first ends | holder active → waits, bound → 10; close-on-exec cleared → the daemon control shows the slot stuck | G | off |
| FR-014 | `runner_test`, `hook-e2e` | owned output base, proved process → server stops | an independent producer on the checkout's server keeps running and finishes when this runner times out, same server PID; without custody no server is signaled; a PID file naming an unrelated process → that process lives, and the run reports it | G | off, loc |
| FR-015 | `fixtures_test` | one hook per stage | double install identical; foreign hook runs; worktree; `core.hooksPath`; malformed config refuses; prek moved off `PATH` route still pinned | G | off |
| FR-016 | `analysis-fixture` in `toolchain-fixtures` | corrected code builds | vet finding and staticcheck finding fail the build | G | off |
| FR-017 | `eslint_test`, `eslint_witness_test` | clean TS passes | a lint finding fails; each S002 spelling really suppresses under pinned ESLint, and the unsuppressed control reports | G | off |
| FR-018 | `runner_test` | every `GUARANTEE` names both cases | a missing case fails | G | off |
| FR-019 | `docs/budgets/` | a record with edited input | a budget without a measurement → review rejects | S | loc, hum |
| FR-020 | `binaries_test`, `browser_smoke_test`, `toolchain-fixtures` | static binary; pinned fonts | a dynamic control binary is detected; empty font root fails | G | off, loc |
| FR-021 | `fixtures_test` | documented variables | a bypassed commit fails `full` | G | off |
| FR-022 | Registry field; review | every check has a class | missing class → exit 2 | G/S | off, hum |

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `fixtures_test` | Passed locally, `weakened`; review pending |
| SC-002 | `fixtures_test`, 20 runs | Passed locally, `weakened`, 20 of 20 runs; review pending |
| SC-003 | `runner_test` | Passed locally; review pending |
| SC-004 | `docs/budgets/` | Measured at `7366549`, `weakened`; see `docs/evidence/m5.md` |
| SC-005 | `verify.sh` receipt | See `docs/evidence/m5.md` |
