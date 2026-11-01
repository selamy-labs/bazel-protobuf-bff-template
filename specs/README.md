# Specifications

Spec Kit feature directories, one per milestone. [001](001-bff-template-foundation/spec.md) is the
umbrella spec (the "spec of specs") and holds the milestone map that the later specs trace to.

Each status is a separate claim. Deployed is no for every row, and hosted CI has never run. A qualified
result holds only for the revision and profile its evidence names.

| Spec | Milestone | Depends on | Status |
| --- | --- | --- | --- |
| [001-bff-template-foundation](001-bff-template-foundation/) | M0: review package (constitution, spec, research, plan, tasks, acceptance matrix, design) | None | Approved 2026-10-29 |
| [002-hermetic-skeleton](002-hermetic-skeleton/) | M1: hermetic skeleton and bootstrap | M0 | Local Linux review passed at `2ef8e2f` (2026-10-31). macOS and hosted CI remain unverified. |
| [003-notes-contract](003-notes-contract/) | M2: neutral notes contract and build-time generation | M1 | Independent local Linux verification passed at `a26b71b` (2026-10-31). |
| [004-go-bff](004-go-bff/) | M3: Go BFF for notes | M2 | Independent local Linux verification passed at `b6629b8` (2026-10-31). Vet is deferred to M5. |
| [005-browser-client](005-browser-client/) | M4: browser client and end-to-end tests | M3 | Independent local Linux review passed at `97d9e22`. Evidence is in [m2-m4](../docs/evidence/m2-m4.md). |
| [006-quality-hooks](006-quality-hooks/) | M5: registry, runner and hooks | M1 | Independent local Linux verification passed at `e0a12ac` (2026-11-01). macOS and hosted CI remain unverified. |
| [007-ci-parity-evidence](007-ci-parity-evidence/) | M6: CI parity, base-built floors and receipts | M4, M5 | Accepted at `e16718d` (2026-11-01). Evidence is split by revision in [m6](../docs/evidence/m6.md). Hosted CI has never run. |
| [008-second-consumer](008-second-consumer/) | M7: second consumer and upgrade | M6 | Accepted at `e982dd7` (2026-11-01) for the qualified local Linux profile, tagged `quality-v1.0.0`. Evidence is in [m7](../docs/evidence/m7.md). |
| [009-bff-image](009-bff-image/) | M8a: BFF container image | M7 | Implemented and tested locally at `05d2545`; the artifact executed independently in a real runtime; independent source review pending. Evidence is in [m8a](../docs/evidence/m8a.md). Not deployed. |
| [010-release-readiness](010-release-readiness/) | M8: public-release readiness and provisioning plan | M7, M8a | Implemented and tested locally; the private scan and the review run at the freeze. Evidence is in [m8](../docs/evidence/m8.md). Not published. |
| [011-quality-baseline](011-quality-baseline/) | M9: quality baseline | M8 frozen | Specified 2026-11-01. The control table lands at its measured state; closure needs every applicable control enforced and evidenced. No tool pinned yet. |
| None yet | M10, M11: optional profiles | M6, and M8a for M10 | Proposed. Each is created with `/speckit-specify` when it starts |

Start with [design.md](001-bff-template-foundation/design.md) to review the package, then read
[issue-map.md](001-bff-template-foundation/issue-map.md) and
[acceptance-matrix.md](001-bff-template-foundation/acceptance-matrix.md).
