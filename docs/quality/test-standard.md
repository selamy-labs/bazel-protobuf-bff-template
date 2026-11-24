# Canonical Testing Standard

This document is the canonical testing reference for this repository, paired with
[Constitution § Testing Standard](../../.specify/memory/constitution.md#testing-standard) and
[CONTRIBUTING.md](../../CONTRIBUTING.md) (tracked in issue `#10` and `specs/011-quality-baseline/`).

## 1. AAA and One Assertion per Test Case

Every test case MUST structure its body into visibly separated **Arrange**, **Act**, and
**Assert** phases and check **exactly one assertion per test case**.

- Separate facts belong in separate named test cases rather than multi-assertion blocks.
- Table-driven subtests are encouraged: each table entry runs as its own named subtest with one
  assertion on the observed outcome for that case.
- Failure messages MUST identify the case name, expected value, and observed value so a failure is
  actionable without stepping through a debugger.

## 2. Property-Based Testing by Default

Wherever a module defines an invariant, tests MUST prefer property-based testing over hand-picked
examples alone. Typical invariants include:

- **Round-trip**: encode/decode, serialize/parse, create/read back.
- **Idempotence**: repeating an operation leaves the result unchanged.
- **Ordering and determinism**: sorting, canonicalization, selection precedence.
- **Totality**: every valid or malformed input maps cleanly without panics or unhandled exceptions.
- **Fail-closed rejection**: malformed, untrusted, or out-of-policy inputs are always rejected.
- **Model equivalence**: an optimized or production implementation agrees with a simple reference
  model or fake contract.

Language libraries (adopted only after a pinned checksum probe per `specs/011-quality-baseline`
`FR-003`):

| Language   | Library                                 | Current repository state                                                        |
| ---------- | --------------------------------------- | ------------------------------------------------------------------------------- |
| Go         | `testing/quick` (or `rapid` / `gopter`) | `testing/quick` in use (`bff/app`, `quality/runner`); partial coverage          |
| TypeScript | `fast-check`                            | `fast-check` 4.10.2 pinned in `pnpm-lock.yaml` (`web/client`); partial coverage |
| Python     | `Hypothesis`                            | Open gap; awaits pinned `uv` lock adoption (`specs/011-quality-baseline` T009/T011) |

Generators MUST cover the real input space (including boundary, empty, malformed, and unicode
values where applicable), and every property suite MUST record or support reproducible seeds so
any counterexample can be replayed deterministically.

## 3. All Mutants Killed and Ratcheting Floor

Mutation testing proves that tests actually fail when production behavior changes.

- **Target vs. measured module floor**: 100% of non-equivalent mutants killed is the repository
  target. Each module's enforced floor starts at its measured baseline once mutation tooling is
  bound, ratchets up to the achieved score in each change, and NEVER moves downward (Constitution
  Principle V, `specs/011-quality-baseline` `FR-004`).
- **Equivalent mutants only**: the only permitted surviving mutants are proven *equivalent*
  mutants (proven unreachability qualifies only when it establishes behavioral equivalence under
  this same proof requirement). Each survivor MUST be recorded in a reviewed equivalents file with:
  1. a stable mutant ID,
  2. exact file path and line/span location, and
  3. a concise one-line proof of behavioral equivalence.
- **No blanket exclusions or disabled mutators**: do not disable mutator classes or exclude
  production packages to hit a score.
- **Pristine pass first**: a mutation run earns credit only after the unmodified suite passes on
  the exact same code (`docs/quality-framework.md`).

## 4. Retrofit Slice Rules

Existing modules are retrofitted incrementally under issue `#10`:

- Keep every retrofit PR small, single-module, and independently reviewable.
- Never mix test-standard retrofit work with feature changes.
- Preserve production behavior and the existing passing test set.
- Once mutation testing is configured for the target module, every retrofit PR MUST state the
  before/after mutation score and the resulting floor change.

## 5. Current Enforcement vs. Open Gaps

Recording this standard does NOT claim automated enforcement for controls that are still open.
The authoritative measured status of each control lives in [baseline.md](baseline.md):

| Control                                   | Baseline state | What exists today                                                            | Open work                                                        |
| ----------------------------------------- | -------------- | ---------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| AAA & one assertion per test case         | `partial`      | Review standard recorded in constitution, `CONTRIBUTING.md`, and this doc    | Per-module retrofit slices (`#10`)                               |
| Property-based testing (Go, TypeScript)   | `partial`      | `testing/quick` (`bff/app`, `quality/runner`), `fast-check` (`web/client`)   | Expand invariant coverage per module (`#10`, `T009`)             |
| Property-based testing (Python)           | `gap`          | Probed only; not pinned in CI                                                | Pin `Hypothesis` and add generator/client invariant tests (`T009`, `T011`) |
| Mutation testing & ratcheting floor       | `partial`      | `gremlins` 0.6.0 probed on `checks/command`; hand-planted mutants at M5–M6   | Configure mutation tooling, baseline, ratcheting floor, and equivalents file (`T008`, `T013`, `#10`) |
