# Check classification

`quality/rules.textproto` defines the reusable rules, and `quality/registry.textproto` binds them and adds this
repository's own checks. Together they are the only list of checks. Each entry carries its own class, stages and
witness, so this page explains the classes and does not repeat that list.

## Classes

| Class | Meaning | Effect on a result |
| --- | --- | --- |
| `GUARANTEE` | A deterministic rule with a named defect case and a corrected case | A failure blocks |
| `SIGNAL` | Useful evidence that is not reliable enough to block, or that needs judgment | Reported and never blocks |

A check with neither class is a configuration error, and so is a `GUARANTEE` with no witness. Every
registered check is a `GUARANTEE` today. No `SIGNAL` is registered, because none has been adopted.

`//:registry_witness_test` requires each `GUARANTEE`'s witness to exist. A shared fixture script must
hold a named defect case and corrected case for that id. Any other witness must be a required test or
a fixture check of its own.

## Enforced outside the registry

These rules hold on every build, so a check of their own would judge nothing more:

| Rule | Enforced by | Witness |
| --- | --- | --- |
| Go analyzers (vet and staticcheck) | `nogo`, in every Go build | `analysis-fixture` in `toolchain-fixtures` |
| TypeScript types | `ts_project`, in every build | `tests` |
| TypeScript lint | `//web:eslint_test` and its siblings | `tests` |
| Lockfile freshness | `--lockfile_mode=error` on every Bazel call | `toolchain-fixtures` |

## Not registered

| Rule | Decision |
| --- | --- |
| Freshness of generated files | Nothing generated is committed, so it would judge nothing |
| Line or statement coverage of the browser app | No data exists. It is a declared `unmeasured` profile in `quality/coverage_floors.json`, for review |
| Contract consumers | Waits for the upstream contract analyzer. Until then, a browser scenario per RPC is the evidence |
| Complexity, duplication | Reviewed and not adopted. No numeric policy is imported; see [capability states](capability-states.md) |
| Commit message rules | None adopted, so the stage has no check |

## Human review

No tool here can judge these, so a reviewer must:

- Whether a change belongs in a public template at all, and whether it carries private details.
- Whether a `GUARANTEE` still tests the behavior it names, and whether a new fixture fails for its
  intended reason. A check can be wrong, and a passing control is not proof that it is right.
- Whether an API change is the right design. `buf breaking` finds incompatibility, not a poor model.
- Whether a test exercises behavior instead of the implementation.
- Whether a statement in a document is still true at the revision it names.
- Whether a new third-party item is recorded in `docs/provenance.md`.
- Moving `api/BASELINE`, which a proposal can move to a later ancestor until M6 and upstream detect it.
