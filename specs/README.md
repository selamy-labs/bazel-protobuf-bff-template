# Specifications

Spec Kit feature directories, one per milestone. [001](001-bff-template-foundation/spec.md) is the
umbrella spec (the "spec of specs") and holds the milestone map that the later specs trace to.

| Spec | Milestone | Status |
| --- | --- | --- |
| [001-bff-template-foundation](001-bff-template-foundation/) | M0: review package (constitution, spec, research, plan, tasks, acceptance matrix, design) | Approved 2026-10-29 |
| [002-hermetic-skeleton](002-hermetic-skeleton/) | M1: hermetic skeleton and bootstrap | Local Linux review passed at `2ef8e2f` (2026-10-31). macOS and hosted CI remain unverified. |
| [003-notes-contract](003-notes-contract/) | M2: neutral notes contract and build-time generation | Independent local Linux verification passed at `a26b71b` (2026-10-31). |
| [004-go-bff](004-go-bff/) | M3: Go BFF for notes | Independent local Linux verification passed at `b6629b8` (2026-10-31). Vet is deferred to M5. |
| [005-browser-client](005-browser-client/) | M4: browser client and end-to-end tests | Implemented; independent review pending |
| [006-quality-hooks](006-quality-hooks/) | M5: registry, runner and hooks | Independent local Linux verification passed at `e0a12ac` (2026-11-01). macOS and hosted CI remain unverified. |
| [007-ci-parity-evidence](007-ci-parity-evidence/) | M6: CI parity, base-built floors and receipts | Implemented; independent review pending |
| 008+ | M7–M10 | Each is created with `/speckit-specify` when its milestone starts |

Start with [design.md](001-bff-template-foundation/design.md) to review the package, then read
[issue-map.md](001-bff-template-foundation/issue-map.md) and
[acceptance-matrix.md](001-bff-template-foundation/acceptance-matrix.md).
