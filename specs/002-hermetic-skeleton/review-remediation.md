# M1 independent-review remediation

Status: REVISE. M1 has implementation and local witnesses; it is not accepted. This checklist
preserves all eleven findings from the independent review. A checkbox closes only with the
named reproduction and corrected witness, at a recorded source revision.

| Task | Finding | Required witness |
| --- | --- | --- |
| T023 | Integrate the quality-framework synthesis | Public source links, upstream ownership, milestone mappings and honest current-state limits are present; private provenance remains untracked |
| T024 | H1: CI parity accepts bypasses | Reject job/step continue-on-error, no-op shell, workflow defaults, conditional skipping, multiline unauthorized commands, wrong checkout SHA and missing aggregate dependency; valid workflow passes |
| T025 | H2: input discovery fails open | A checkout path containing spaces and regex metacharacters still scans real defects; input-discovery errors and unexpectedly empty inventories fail; corrected content passes |
| T026 | H3: implicit config disables native tools | Host/parent ShellCheck config, nested typos config and zizmor config cannot alter canonical verdicts; explicit canonical configuration still works |
| T027 | H4: actionlint uses host PATH | Pinned ShellCheck supplies identical results with and without a host ShellCheck; undeclared optional helpers cannot affect verdicts |
| T028 | H5: undeclared uv/uvx dependency | Bootstrap/provenance check works with only documented prerequisites and pinned tool downloads; missing prerequisites produce actionable failure; clean-path fresh clone is witnessed |
| T029 | M1: suppression-scan bypasses | Supported directive spellings, NUL-bearing executable source and exemption lookalikes are caught; legitimate vendored/fixture files remain narrowly identified |
| T030 | M2: ambient Bazel configuration and download identity | Host/system/environment rc injection cannot weaken policy or isolation classification; downloaded Bazel has a verified pinned identity |
| T031 | M3: canonical preflight entry point | The documented default command produces the promised sandbox remediation on a restricted host, or the documented command is corrected; explicit weak mode remains visibly weakened |
| T032 | M4: overstated evidence | Record tool versions/start time; witness checksum mismatch; measure bootstrap separately; distinguish cached and edited-code warmth; remove or truly witness network-isolation claims; bind evidence to exact tested source |
| T033 | L1: peak-memory accounting | Measurement method avoids counting shared pages twice and excludes the sampler, or reports a precise narrower metric with its limitations |
| T034 | L2: gitleaks allowlist path mismatch | Paths supplied to the scanner have the expected normalization; intended synthetic fixture allowance is exercised; an unrelated secret remains rejected |
| T035 | Re-review and final verification | All corrected witnesses and the full canonical verifier pass at the exact recorded source; fresh-clone evidence and independent re-review are terminal; unsupported matrix/isolation legs remain explicitly unverified |

For behavior changes, first reproduce the defect with a failing test, then retain a corrected
counterpart. Do not weaken a rule, widen enrollment exemptions or bypass hooks to close a task.
General policy-diff, evidence-validator and analyzer implementations remain upstream-owned.

The foundation milestone map still governs the remaining app, hooks, CI, second-consumer and
release-readiness work. This remediation checklist does not claim completion of M2–M8.
