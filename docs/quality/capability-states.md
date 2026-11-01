# Capability states

Each capability has six separate states. A state is claimed only with its scope, its command and its
witness beside it, and no state implies another. Local evidence cannot fill the last two columns.

- **Specified**: a requirement names the behavior.
- **Implemented**: the code exists.
- **Executed**: it has actually run.
- **Passed**: it passed at a named revision. See the [evidence records](../evidence/), which name
  revisions and isolation labels.
- **Required**: a hosting platform refuses a merge without it. This repository has no remote, so
  this is `no` for every capability.
- **Verified after integration**: it was seen working on the integrated result. `no` for the same
  reason.

Each row is a local statement. An independent review, where one has happened, is named in the
evidence record. This table reuses the registry, the task ledgers and those records. It adds no
validator.

| Capability | Specified | Implemented | Executed | Passed | Required | After integration | Witness |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Pinned toolchain and hermetic bootstrap | yes | yes | yes | yes, M1 to M4 | no | no | `tools/bootstrap/fresh_clone.sh`, `toolchain-fixtures` |
| Canonical verifier | yes | yes | yes | yes, M1 to M4; M5 and M6 in the evidence records | no | no | `tools/verify.sh --base=<commit>` |
| Contract and generated code | yes | yes | yes | yes, M2 to M4 | no | no | `//api:schema_probes_test`, `proto-breaking` |
| Go BFF | yes | yes | yes | yes, M3 to M4 | no | no | `tests` |
| Browser client | yes | yes | yes | yes, M4 | no | no | `e2e-browser` |
| Registry and runner | yes | yes | yes | see [m5](../evidence/m5.md) | no | no | `@bff_template_quality//runner:runner_test` |
| Lifecycle hooks | yes | yes | yes | see [m5](../evidence/m5.md) | no | no | `//tools/hooks:fixtures_test`, `hook-e2e` |
| Go analyzers | yes | yes | yes | see [m5](../evidence/m5.md) | no | no | `analysis-fixture` in `toolchain-fixtures` |
| TypeScript lint and format | yes | yes | yes | see [m5](../evidence/m5.md) | no | no | `//web:eslint_test`, `format-ts` |
| Coverage floors, base-built policy, CI legs | yes | yes | locally; the workflow has never run | see [m6](../evidence/m6.md) | no | no | `coverage-floor`, `floors-from-base`, `//:ci_parity_test` |
| Hosted CI | yes | workflow only | no | no | no | no | Operator gate |
| Consumer path and quality-pin upgrade | yes | yes | locally, on `file://` | see [m7](../evidence/m7.md) | no | no | Spec 008 |
| Provenance and license inventory | yes | partly | no | no | no | no | M8 |
| Public release | yes | no | no | no | no | no | Operator gate |

## Profiles that do not apply

| Profile | Why not |
| --- | --- |
| Persistent storage adapter | The starter ships a memory port. An adopter brings storage |
| Identity provider | No provider ships. Production refuses to start without an adopter's verifier |
| Remote execution or a shared build cache | Not in scope. Local Bazel is the supported profile |
| Worker profile | Upstream #29 owns it, and the starter has no worker |

## Open gaps and their owners

| Gap | Owner |
| --- | --- |
| Hosted CI has never run, and no remote exists | Operator gate |
| A repository's first hosted push has no base, and a proposal controls its own workflow | M8, operator gate, upstream #26 |
| The quality interface is stable only from the `quality-v1.0.0` tag, which goes on the commit an independent review accepts. Before that tag, 1.0.0 is a proposal | M7 |
| A consumer's proposal can pin a module whose adapter judges nothing. The base's evaluator runs only if the adapter that calls it does, and that adapter comes from the proposal's pin. `floors-from-base` then passes without judging, and a person reading the pin change is the only control here | Upstream #26 |
| macOS is unverified. The browser tests cannot run there, and stock macOS has no `flock` | Open |
| The authoring host runs only with weakened isolation. Full isolation is observed in independent runs, which [m5](../evidence/m5.md) and [m6](../evidence/m6.md) name | Independent review of each final revision |
| Browser code has no coverage data, and Go has no branch data | Open, stated in `quality/coverage_floors.json` |
| Complexity is not measured. No numeric threshold is imported from another project | Open. It needs a language-qualified analyzer and a baseline of this repository's own |
| Duplication is not measured | Open, with complexity |
| Property tests exist for Go only | Open (spec 006 keeps parent T028 scope) |
| Mutation testing is not adopted. Named mutants are run by hand and recorded as evidence | Open |
| The shared hook stage contract is unreleased | Upstream (relentless-ci-python #25, relentless-ci-typescript #20) |
| Policy-diff, the evidence validator, the contract analyzer and the worker profile are unreleased | Upstream (#26, #27, #28, #29) |
| Local policy beyond coverage profiles and suppressions can be edited together with its evaluator | Upstream #26 |
| Debian package licenses and transitive npm licenses are not enumerated | M8 (parent T077) |
