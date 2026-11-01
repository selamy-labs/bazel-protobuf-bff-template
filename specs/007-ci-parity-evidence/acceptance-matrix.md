# Acceptance / Evidence Matrix: M6

Classes: **G** deterministic guarantee, **S** review signal. Layers: **off** offline test, **loc** local verifier,
**hum** human. Hosted CI is never claimed: no hosted run exists. Each negative needs its positive to pass in the same setup
first.

| Req | Witness | Valid | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- | --- |
| FR-001 | `runner_test` | real registry | neither `shard` nor `postcondition`, both, unknown leg, postcondition without `after_bazel` → exit 2 | G | off |
| FR-002 | `runner_test`, `verify_body_test` | obligations derived; each leg's receipt reports all of its own | required test with no leg, or two; postcondition absent from a Bazel leg's result → not PASSED | G | off |
| FR-003 | `parity_test` | jobs and `needs` equal the derived legs | job and `needs` entry removed together, an extra leg → named rejection | G | off |
| FR-004 | `parity_test` | one runner call per step | inline `go test`, step without `--shard` → named rejection | G | off |
| FR-005 | gate run block | all `success`; leg failed then rerun `success` with others from an earlier attempt → pass | `skipped`, `cancelled`, `failure`, empty, absent; latest for a leg still `failure` → denied | G | off |
| FR-006 | Fake Bazel | `full`: one `coverage`. `pre-push`, `e2e`: one `test` | a second `test` or `coverage` in a stage → test fails | G | off |
| FR-007 | Fake Bazel, adapter review | 36 of 36 from one invocation | adapter runs `bazel test` → count fails; review | G/S | off, hum |
| FR-008 | Real Git fixtures | one `build` of the fixed target with a private output base | base build fails → exit 11; shared output base → custody fails | G | off |
| FR-009 | Analysis on the no-toolchain host | `coverage` analyzes | report generator flag removed → analysis fails on the C++ toolchain | G | off |
| FR-010 | `runner_test` | helper prints a path | helper also writes stderr → digest still correct; merged capture → red | G | off |
| FR-011 | Real Git fixtures | floor raised; unrelated update passes | evaluator replaced or base build config edited in the tested tree → still fails | G | off |
| FR-012, FR-013 | Real Git fixtures | ancestor named by SHA, by ref and by `HEAD~1` | none named, malformed, unknown, shallow, descendant, unrelated, no evaluator, no build, `--offline` → exit 11 by cause; same SHA spelled as `HEAD`, a ref and a full SHA → exit 11, own base; `--from` given with no base → still refused | G | off |
| FR-013a | Real Git fixtures | different commit, same tree → compared and passes | a weakening commit whose tree differs only by a README edit still fails on its floor, so tree difference earns nothing | G | off |
| FR-014 | `verify_body_test` | refusal beside every other check's result | other checks skipped, or result PASSED → test fails | G | off |
| FR-015 | `runner_test` | SHA and `base_source` recorded | a `reviewed` field, or base missing → rejected | G | off |
| FR-015a | `fixtures_test` | zero `--from`, setting present: first push of a topic branch passes, escalated to `full`, `hook-config` in the receipt | setting unset → exit 11 naming the install command; a moved `main` or `HEAD` is never read; installed with no value → nothing stored | G | off |
| FR-016 | Real Git fixtures | rows allowed in R8 pass | floor lowered, profile removed, metric changed, each narrowing row → fails | G | off |
| FR-016a | Real Git fixtures | delete and add within `includes` → passes | real `git rm` and add to a path outside every profile → fails, `unenrolled source`; a relocation into an existing exclude passes, asserted as the stated limit | G | off |
| FR-017 | Real Git fixtures | every measured file reported; an excludes entry with a factual reason | file absent from report → fails, never covered; an exclude with no reason, or one for a file with executable statements → rejected | G | off |
| FR-018 | Real Git fixtures | no new suppression | suppression added per language; pattern set edited → fails | G | off |
| FR-019 | Real tools | exit 0 | exit 3 with reports, empty report, truncated report → `FAILED`; removed test lowers number → fails | G | off, loc |
| FR-020 | Report fixtures, real profile | `statements` from the raw profile; Go branch shown `unmeasured` | LCOV value as Go percentage, missing `.cover`, `unmeasured` shown as met or not applicable → rejected | G | off |
| FR-020a | Real tools | a TS test yields line data for `web/client` | empty report, `(empty-report)` placeholder → `FAILED`; a removed TS test lowers the number | G | off, loc |
| FR-020b | Reconciliation in research | file-by-file table | any unexplained file → no floor set | S | hum |
| FR-021 | Real tools | both modules present | filter loses the quality module, or `external/` mapping removed → fails | G | off, loc |
| FR-022 | Doc assertions, `fresh_clone.sh` | `--base` documented | bare command passes, or a stale command in README → fails | G | off, loc |
| FR-023 | Real-registry tests | new input escalates and changes the digest | input outside the list → red | G | off |
| FR-024 | `parity_test`, `runner_test` | `if-no-files-found: error`, name with attempt | setting removed, name without attempt, personal identifier → rejected | G | off |
| FR-025 | `parity_test`, `zizmor` | read-only, no secrets | `write` permission, a `secrets.` use, `pull_request_target`, a moved `ref` → rejected | G | off |
| FR-026 | Doc assertion | sentence matches triggers | wider sentence → review | S | hum |
| FR-027 | `fixtures_test`, suppression fixtures | clean commit passes | `--no-verify` commit with a secret → CI-equivalent run fails; unbanned spelling → fails | G | off |
| SC-002 | Fresh clones | both exit 0 at the frozen revision | planted defect fails first in the hostile run | G | loc |
| SC-004 | R6 flows, logged | three flows pass | each listed negative → exit 11 by cause | G | loc |
| SC-005 | Review | claims separate | hosted claim with no run → review rejects | S | hum |
