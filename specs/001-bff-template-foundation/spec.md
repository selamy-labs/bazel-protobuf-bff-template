# Feature Specification: Generic Bazel/Protobuf BFF Template — Foundation and Spec of Specs

**Feature Branch**: `001-bff-template-foundation`

**Created**: 2026-10-28

**Status**: Living umbrella. M0 was approved 2026-10-29. Delivery status is in the [index](../README.md)

**Input**: Owner request to start a generic Bazel/Protobuf BFF template, generalizing the
architecture of a private reference implementation. The request asks for the Spec Kit workflow to
produce a constitution, a spec of specs, research and a technical plan, dependency-ordered tasks,
an acceptance/evidence matrix and a concise design for review, and to stop before any application
implementation. Public backlog: selamy-labs/relentless-ci-python#30 (RFC) and #31
(tracker), with #25–#29 and selamy-labs/relentless-ci-typescript#20.

## Scope of this specification

This is the umbrella specification ("spec of specs") for the template. It defines the user value,
the requirements every milestone must satisfy and the milestone decomposition (see
[Milestone Map](#milestone-map-spec-of-specs)). Each milestone after M0 gets its own Spec Kit
feature directory when it starts; this document is the parent those specs trace to.

Three responsibilities stay separate throughout (RFC #30):

1. **Shared quality checks**: versioned, reusable check definitions and evidence contracts.
2. **Thin adapters**: language/build adapters and lifecycle hooks that invoke the canonical pinned
   tools locally, through hooks and in CI.
3. **Reference application**: an opinionated, real browser → BFF vertical slice.

**Product outcome**: an adopter clones one neutral template, gets a working browser-to-BFF slice and a
quality gate that runs the same way locally and in CI, and can adopt those checks in another repository
by one pin.

**Concern boundaries and owners**:

| Concern | Owner | Authoritative feature |
| --- | --- | --- |
| Application builds, tests and toolchain pins | Bazel | [002](../002-hermetic-skeleton/) |
| Quality rules and their release | `bff_template_quality` | [006](../006-quality-hooks/), [008](../008-second-consumer/) |
| Application images | rules_oci, through Bazel | [009](../009-bff-image/) |
| Repository settings, as a reviewed plan | OpenTofu, applied only by an operator | [010](../010-release-readiness/) |
| Infrastructure lifecycle | OpenTofu | M10, optional |
| Workloads, where they apply | GitOps | Outside this repository |
| Managed runtimes | The provider, as supported | Outside this repository |
| NixOS on self-managed hosts | Shared host operations | Outside this repository; the consumer side is [host-profile](contracts/host-profile.md) |
| Hook stage contract, policy-diff, evidence validator, contract analyzer, worker profile | Upstream #25 to #29, TS #20 | Consumed, not implemented here |

**Capabilities, and who consumes them.** This template is a shared foundation, and its reusable value is
the set below. Each row says what is delivered here and what a downstream product still owns. Three
downstream products are known to its operator and are named only by neutral labels here. What is known
of each is a kind of work, and no more: routing and custody operations for a service; role-based
recruiting for a small team's idea; continuous operations across several interfaces for a platform.
Each would reuse typed contracts, and none has chosen this framework. Their domain models, business
rules, identities, permissions, integrations, data and deployment approvals are **unknown to this
specification**, stay with their product owners, and are not invented here. None of them is started by
this template, and none of them requires NixOS.

| Capability | Delivered here | A downstream product still owns |
| --- | --- | --- |
| Typed schema and generated code | One Protobuf contract, generated Go and TypeScript, drift and breaking-change checks | Its own schema, package versions and compatibility policy |
| Typed backend and frontend boundary | A Connect BFF with ports and adapters, and a browser client that observes success and typed failure | Its domain, storage, identity provider and authorization |
| Declarative, reproducible build and quality | Pinned toolchains, one rule catalog, hooks, verifier and receipts, consumable by one pin | Its own floors, required tests, policy base and review |
| Packaging | The production server as one OCI image from the build graph, with the exact startup refusal (M8a) | A real verifier, runtime assets, registry and deployment |
| Fresh-clone and app flow | Clone to a passing verifier and a working slice on the supported profile | Its product flows, their acceptance tests and CI |
| Independent consumer upgrade | A second consumer moved N to N+1 by one pin, with notes and negative controls | Its own upgrade cadence and the review of each pin change |

Hooks give early, enforceable checks. They are bypassable and prove no business correctness. A product's
acceptance tests, negative controls, exact-source independent review and its own CI still matter.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fresh clone shows a working browser-to-BFF slice (Priority: P1)

A developer who has never seen the project creates a copy, runs one documented bootstrap command on
a supported host with only the documented prerequisite installed, opens the example in a browser,
performs the example action and sees it succeed; then submits invalid input and sees a clear,
typed failure message. No personal account, credential, cache service or cloud resource is needed.

**Why this priority**: this is the template's reason to exist; everything else supports it.

**Independent Test**: on a clean machine image, clone, run the bootstrap/verify command and the
browser end-to-end test; both the success and the failure scenario pass, and the run records the
exact revision tested.

**Acceptance Scenarios**:

1. **Given** a fresh clone and only the documented prerequisite, **When** the developer runs the
   bootstrap command, **Then** all pinned tools are fetched and verified and the full verifier
   passes without prompting for input or credentials.
2. **Given** the example is running locally, **When** the user submits a valid item, **Then** the
   browser shows the stored item returned by the BFF.
3. **Given** the example is running, **When** the user submits an invalid item, **Then** the
   browser shows a field-specific validation message derived from the typed contract error, and no
   internal detail is exposed.
4. **Given** the item does not exist, **When** the user requests it, **Then** the browser shows a
   "not found" outcome distinct from a validation or transport failure.
5. **Given** the BFF is unreachable or exceeds its deadline, **When** the user acts, **Then** the
   browser shows a retryable "unavailable" outcome within the declared deadline.
6. **Given** a deliberately broken change (for example the BFF returns the wrong error class), **When**
   the end-to-end test runs, **Then** it fails naming the scenario; the corrected change passes.

---

### User Story 2 - Contract changes are safe and generated code cannot drift (Priority: P1)

A contract author edits the schema. A compatible addition passes and is immediately usable by both
the browser client and the BFF; a wire-breaking change fails before review, naming the field and
rule; generated code is always derived from the current schema.

**Why this priority**: generated clients are only trustworthy if schema evolution is checked.

**Independent Test**: apply a synthetic breaking edit and a synthetic compatible addition to the
schema; the first fails the compatibility check against the declared baseline, the second passes
and both consumers build.

**Acceptance Scenarios**:

1. **Given** a declared immutable baseline revision, **When** a field number or type is changed,
   **Then** the compatibility check fails with the rule and location.
2. **Given** a compatible field addition, **When** the full verifier runs, **Then** it passes and
   both consumers compile against the new contract.
3. **Given** any committed generated artifact differs from what the build produces, **When** the
   freshness check runs, **Then** it fails and prints the regeneration command.
4. **Given** a contract element with no production consumer, **When** the contract-consumer report
   runs, **Then** it lists the element as a review signal (not a hard failure) separately from
   test-only and generated references.

---

### User Story 3 - Lifecycle hooks give fast, safe, actionable feedback (Priority: P2)

A developer or coding agent commits and pushes. Cheap checks run on staged content at commit time,
broader checks at push time, and failures name the check, the file and the rerun command. Partially
staged work is never altered or staged by a hook. Bypassing hooks locally is still caught by
required CI running the same canonical rules.

**Why this priority**: checks that are not in the normal Git lifecycle get skipped (#25, TS #20).

**Independent Test**: in a temporary clone, install hooks, stage a bad change alongside an unstaged
unrelated edit; the commit fails for the intended reason and the unstaged edit is byte-identical
afterward; the corrected change commits; a bypassed commit fails in CI-equivalent full verification.

**Acceptance Scenarios**:

1. **Given** a staged file containing a secret or a banned suppression, **When** committing,
   **Then** the commit is rejected with the check name, path and rerun command.
2. **Given** partial staging, untracked files, renames, deletions and filenames containing spaces,
   **When** the commit hook runs, **Then** only staged blob content is checked and the working tree
   and index are unchanged.
3. **Given** a push of a commit that breaks a test, **When** the push hook runs, **Then** the pushed
   commit (not the working tree) is checked and the push is rejected; the corrected commit pushes.
4. **Given** many staged files, **When** hooks run, **Then** whole-project checks run once per
   stage, not once per filename batch, within the declared concurrency bound.
5. **Given** hooks are installed twice or alongside an existing unrelated hook, **When** installing,
   **Then** the result is identical and the unrelated hook is preserved.
6. **Given** a malformed hook or registry configuration, **When** hooks run, **Then** they fail
   with a configuration diagnostic, never a silent pass.
7. **Given** a commit made with hooks bypassed, **When** the full verifier runs, **Then** the same
   violation fails.

---

### User Story 4 - Reviewers trust exact-revision evidence and cannot be fooled by weakening (Priority: P2)

A reviewer (human or automated) decides readiness from an evidence record bound to the exact
revision. Missing, pending, skipped, truncated, stale or contradictory evidence blocks readiness. A
change that weakens a threshold, removes a check, widens an exclusion or adds a suppression is
reported as weakening against a baseline the proposal cannot edit.

**Why this priority**: green exit codes and self-edited policy are not proof (#26, #27).

**Independent Test**: feed the evidence validator synthetic fixtures (complete, running, truncated,
omitted leg, old revision, contradictory, moved branch) and the weakening detector synthetic
proposals; each produces the documented verdict and an actionable diagnostic.

**Acceptance Scenarios**:

1. **Given** a complete terminal evidence set for revision R, **When** validated against R,
   **Then** readiness is granted and lists each satisfied prerequisite.
2. **Given** any prerequisite pending, skipped, cancelled, empty or for a different revision,
   **When** validated, **Then** readiness is denied naming the prerequisite and next action.
3. **Given** a proposal lowering a floor or adding an exclusion, **When** compared with the trusted
   baseline, **Then** it is reported as weakening; a non-weakening tool update passes.
4. **Given** PR text or an in-branch file claiming approval, **When** evaluated, **Then** it does not
   authorize weakening.

---

### User Story 5 - A second project consumes the shared checks and upgrades (Priority: P3)

A maintainer of a different, neutral project adopts the shared quality checks at one version, then
upgrades to the next version with a documented, reviewable change, and sees which consumer policy
changed.

**Why this priority**: a copied snapshot with no update story is insufficient (RFC #30); the
reusable interface is not declared stable before this works.

**Independent Test**: create a second neutral consumer pinned to version N of the shared checks;
upgrade to N+1 by changing one pin; the upgrade diff shows exactly the policy changes and the
consumer's verifier passes (or fails for a documented, intended new rule).

**Acceptance Scenarios**:

1. **Given** the consumer pins version N, **When** it runs the verifier, **Then** the checks of
   version N run with N's pinned tools.
2. **Given** an upgrade to N+1, **When** the pin changes, **Then** the release notes and the
   consumer diff identify every added, changed or removed rule, and weakening rules are flagged.

---

### User Story 6 - Optional cloud deployment profile with operator gates (Priority: P3)

An operator who chooses to deploy follows a provider-specific optional profile that bootstraps all
infrastructure declaratively, with continuous delivery that stops at explicit operator approval
gates. Users who do not deploy are unaffected.

**Why this priority**: deployment is optional for a generic starter (RFC #30); it must not be
implied as required.

**Independent Test**: the profile's configuration validates and plans offline against fixtures;
no apply occurs without an operator gate. (Live deployment evidence is a separate claim.)

**Acceptance Scenarios**:

1. **Given** the profile is not enabled, **When** the full verifier runs, **Then** no provider
   tooling or credential is required.
2. **Given** the profile is enabled, **When** CD is triggered, **Then** it halts before any
   environment change until an operator approves the exact revision.

---

### User Story 7 - Public-release review before publication (Priority: P3)

Before the repository is published, a reviewer can see a complete provenance and license inventory
and a checklist proving no private product code, schemas, data, identities, configuration or
incident history is present.

**Why this priority**: the reference is private and proprietary; the template is intended to be
public.

**Independent Test**: run the provenance/neutrality scan; every third-party asset has a recorded
source and license and no denylisted private identifier appears in tracked files or history.

**Acceptance Scenarios**:

1. **Given** a tracked file derived from a third-party source, **When** the inventory check runs,
   **Then** the file has a provenance entry with source, revision and license.
2. **Given** a private identifier from the reference's denylist appears, **When** the scan runs,
   **Then** it fails naming the file.

### Edge Cases

- Host without unprivileged user namespaces (sandboxing unavailable): bootstrap fails with an
  actionable message rather than silently weakening isolation.
- First commit of a schema (no baseline yet): an explicit, reviewed bootstrap marker is required;
  absence of a baseline never passes silently.
- Base revision unavailable (shallow clone): compatibility and ratchet checks fail closed with the
  fetch command.
- Hook installation in a Git worktree or with a custom hooks path: respects worktree-local config.
- Hooks running on a commit that changes the verifier or hook configuration itself: invalidates
  cached results and runs the full stage.
- Browser test on a host without the pinned browser: the build fetches the pinned build; if that is
  impossible offline, the test fails, it never skips green.
- Process that waits on stdin or leaks children: terminated at the declared deadline and reported
  as a timeout, not a check failure.

## Requirements *(mandatory)*

### Functional Requirements

Reference application (responsibility 3)

- **FR-001**: The template MUST include one real browser client and one BFF communicating only
  through generated clients/servers from a versioned schema.
- **FR-002**: The example contract MUST be neutral (no product domain) and every RPC in it MUST have
  a production browser caller, observable behavior and an end-to-end acceptance scenario.
- **FR-003**: The BFF MUST validate inputs at its boundary and return typed, field-specific errors.
- **FR-004**: The BFF MUST map internal failures to a fixed set of contract error codes without
  exposing internal details.
- **FR-005**: The BFF MUST honor client deadlines and cancellation and bound every downstream call.
- **FR-006**: The BFF MUST define an authentication/session boundary; a test-mode identity MUST be
  labeled as such and MUST be rejected when the BFF runs in production mode. The BFF MUST NOT
  authenticate from ambient credentials (cookies) unless CSRF protection is added with it.
- **FR-007**: Allowed dependency directions (browser → generated client → BFF → backends; no reverse
  or cross-layer imports; test fakes not reachable from production targets) MUST be declared and
  checked deterministically.
- **FR-008**: Browser end-to-end tests MUST cover success, validation failure, not-found and
  unavailable/deadline outcomes against the real BFF, plus one deliberately failing variant.

Contracts and generation

- **FR-009**: Schemas MUST be format-, lint- and compatibility-checked against an immutable declared
  baseline revision with pinned tools; checks MUST fail closed when the baseline is unavailable.
- **FR-010**: Generated code MUST be produced by the build graph; any committed generated artifact
  MUST have a freshness check.
- **FR-011**: A contract-consumer report MUST separate production consumers, test-only callers,
  fakes, generated references and declared dynamic entry points; unused surface is a review signal.
- **FR-012**: The applicable API-style rules and each justified exception MUST be documented in a
  versioned ledger.

Shared checks and adapters (responsibilities 1 and 2)

- **FR-013**: Each quality rule MUST have exactly one canonical definition consumed by hooks, the
  full verifier and CI at the same pinned tool versions.
- **FR-014**: Adapters MUST only translate file sets and invocation; they MUST NOT redefine policy.
- **FR-015**: Lifecycle hooks MUST provide pre-commit (staged-file hygiene, syntax, secrets,
  suppressions), commit-msg (only if adopted) and pre-push (changed-code checks and affected tests)
  stages, plus a documented full-verification command.
- **FR-016**: Hooks MUST be safe under partial staging, renames, deletions and spaces in filenames,
  MUST NOT stage unrelated work, and MUST install idempotently without silently overwriting
  unrelated hooks.
- **FR-017**: Hooks and verifiers MUST bound concurrency and wall-clock time, never wait on stdin,
  own their process trees, and run whole-project checks at most once per stage.
- **FR-018**: Every failure MUST exit nonzero with the check name, bounded diagnostic and rerun
  command; output MUST be consumable by humans and repair loops.
- **FR-019**: Suppressions, exclusions and thresholds MUST be protected: bans on inline suppression,
  floors that only rise relative to the base revision, evaluated by checks built from the base
  revision; broader weakening detection is adopted from the shared upstream contract.
- **FR-020**: Each check MUST be classified as deterministic guarantee or advisory review signal,
  and documentation MUST state which findings need human architectural review.

Evidence and CI

- **FR-021**: Required CI MUST independently run the full canonical policy at the exact revision and
  aggregate results into one required verdict.
- **FR-022**: Evidence records MUST bind each prerequisite to source revision, policy/base revision,
  producer, terminal result and location, and readiness MUST be invalidated when inputs change.
- **FR-023**: Pending, skipped, cancelled, truncated, empty, stale or contradictory evidence MUST NOT
  satisfy readiness; a worker's own assertion MUST be distinct from an independent review receipt.
- **FR-024**: Claims for offline tests, local verification, hosted CI, human approval and deployment
  MUST be reported separately.

Bootstrap, budgets and distribution

- **FR-025**: A fresh clone MUST bootstrap with one documented command, with every verdict-affecting
  tool pinned and locked, and without developer-specific accounts, credentials, caches or services.
- **FR-026**: Cold and warm latency and peak resource use for each hook stage, the full verifier and
  CI MUST be measured and recorded with the host profile; declared budgets MUST derive from
  measurements and thresholds MUST NOT be weakened to meet them.
- **FR-027**: The shared checks MUST be consumable as a versioned dependency with release notes that
  list rule changes; a second neutral consumer MUST demonstrate adoption and one upgrade before the
  interface is declared stable.
- **FR-028**: Stable configuration (tool pins, check registry, policy floors, hook stages,
  repository settings, deployment) MUST be declarative and versioned.
- **FR-029**: Additional languages (including Python) MUST be addable through a documented adapter
  profile; no language, service or contract is added without a real consumer.

Optional profiles, provenance and governance

- **FR-030**: An optional cloud deployment profile, if offered, MUST include declarative bootstrap,
  CI/CD responsibilities and explicit operator gates, and MUST NOT be required by the core verifier.
- **FR-031**: An optional worker-integration conformance profile MAY define bounded execution,
  failure taxonomy and identity/billing evidence; it MUST remain a contract, not a runtime.
- **FR-032**: The repository MUST keep a provenance and license inventory and a public-release
  review checklist, and MUST pass a neutrality scan before publication.
- **FR-033**: Repository ownership, name, visibility and settings MUST be planned as declarative
  provisioning; nothing is created or applied without an operator gate.
- **FR-034**: Application images, if offered, MUST be produced only by the Bazel graph through rules_oci.
  A pipeline MAY invoke that target and publish its digest, and MUST NOT rebuild the image.
- **FR-035**: No template user MUST need NixOS. Support stays Linux only, as qualified. An optional host
  profile MUST be a stated contract of measured requirements, and MUST be qualified by the existing
  commands unchanged.

### Key Entities

- **Contract**: a versioned schema package with RPCs and messages; has consumers and a baseline.
- **Consumer**: a production caller, test caller, fake or generated reference of a contract element.
- **Check**: one canonical rule — identifier, tool and pinned version, inputs, classification
  (guarantee/signal), stages, thresholds.
- **Check registry**: the versioned set of checks a consumer adopts.
- **Hook stage**: pre-commit, commit-msg, pre-push or full; selects checks and file sets.
- **Policy baseline**: the trusted, pinned registry and thresholds a proposal is compared against.
- **Evidence record**: per-revision list of prerequisites with producer, result, state and
  location.
- **Budget**: measured latency/resource figure for a stage on a declared host profile.
- **Provenance entry**: source, revision, license and transformation of a third-party asset.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a clean supported host, a new adopter goes from clone to a passing full verification
  and a working example in one documented command, with zero credentials entered. That command names a
  policy base, `tools/verify.sh --base=<reviewed commit>` (spec 007 FR-022). With no base it runs every
  other check and exits 11, naming the fix.
- **SC-002**: 100% of example RPCs have a production browser caller and at least one success and one
  failure end-to-end scenario.
- **SC-003**: 100% of the synthetic breaking schema changes in the fixture set are rejected and
  100% of the compatible ones accepted.
- **SC-004**: Each named defect fixture (secret, suppression, threshold lowering, stale generated
  code, broken end-to-end behavior, bypassed hook) fails for its intended reason and its corrected
  counterpart passes.
- **SC-005**: Partial-staging fixtures leave unstaged content byte-identical in 100% of runs.
- **SC-006**: Commit-stage and push-stage feedback times are measured (cold and warm, p50/p95) on a
  documented host profile and recorded; the declared budgets are met on that profile without
  weakening any threshold.
- **SC-007** (after adopting the shared evidence validator): 100% of the evidence fixtures (complete, running, truncated, omitted leg, old revision,
  contradictory, moved branch) produce the documented readiness verdict.
- **SC-008**: A second neutral consumer adopts the shared checks and completes one upgrade by
  changing a single version pin plus documented reviewed edits.
- **SC-009**: The neutrality scan finds zero private identifiers and the provenance inventory covers
  100% of third-party assets before any publication.
- **SC-010**: The image's manifest digest repeats across two clean builds, and its entrypoint shows the
  server's exact refusal.

## Milestone Map (spec of specs)

Each milestone becomes its own Spec Kit feature (`specs/NNN-*`) when started. Dependencies are
strict; parallel-safe pairs are marked.

| ID | Milestone | Depends on | Delivers (requirements) | Upstream issues consumed |
| --- | --- | --- | --- | --- |
| M0 | Spec Kit review package (this feature) | — | constitution, this spec, research, plan, tasks, acceptance matrix, design, provenance | #30, #31 |
| M1 | Hermetic skeleton and bootstrap | M0 | FR-025, FR-028 (pins), FR-026 (measurement harness) | — |
| M2 | Neutral contract and generation | M1 | FR-009–FR-012 | #28 (schema profile) |
| M3 | Go BFF | M2 | FR-003–FR-007 | #28 (consumers) |
| M4 | Browser client and end-to-end tests | M3 | FR-001, FR-002, FR-008 | #28 |
| M5 | Shared check registry, adapters and prek hooks | M1 (parallel with M2–M4) | FR-013–FR-020 | #25, TS #20 (stage/diagnostic contract) |
| M6 | CI parity, aggregate gate, receipts, base-built ratchets | M4, M5 | FR-019 (bans, floors), FR-021, FR-022 (receipts), FR-024 | #26, #27 (consumed; FR-023 and SC-007 complete when #27 is adopted) |
| M7 | Second neutral consumer and upgrade | M6 | FR-027, FR-029 | #30 |
| M8a | BFF container image | M7 | FR-034, SC-010 | #30 |
| M8 | Public-release readiness and repository provisioning plan | M7, M8a | FR-032, FR-033 | #31 |
| M9 | Quality baseline: every control measured, budgets that only ratchet stricter, gates with planted defects; mandatory for initiative closure (amendment of 2026-11-01) | M8 frozen | FR-019, FR-020, FR-026 | — |
| M10 | Hosted example: a dedicated BFF-owned GCP project, Cloud Build CD, the spec 009 image by digest, a real verifier and store behind the ports, the static frontend on Pages; required by the direction of 2026-11-01, and never a requirement on template users | M8, M8a, and spec 010 US6 for the package | FR-030, spec 012 | #30 |
| M11 | Optional worker-integration conformance | M6 (optional) | FR-031 | #29 |

The **first implementation slice** is M1 → M4 with the minimal M5/M6 gates needed to keep them
honest (pins, schema checks, suppression ban, single CI aggregate).

## Explicitly Out of Scope / Deferred

- Remote cache, remote build execution or any hosted build service.
- Host configuration of any kind, and a healthy production deployment until a real verifier exists.
- Multiworker orchestration, Batch runtime, agent platform or cloud control plane.
- Domain services, background workflows, data aggregation and migration/cutover. These are
  product-specific.
- Reinstating custom trusted-policy issuers, requiring a GitHub App, or changing branch protection.
- Creating or publishing a repository, applying infrastructure, merging, granting access,
  installing Apps, paid cloud runs.
- Implementing the upstream #25/#20 prek adapters, #26 policy-diff contract or #27 evidence format
  inside this repository; this repository consumes them when released (see issue map).

## Assumptions

- **A1 (validated)**: The architectural reference is a private, unlicensed (proprietary) monorepo.
  Its identity and the inspected revision are kept in the operator's untracked inspection record.
  Nothing is copied from it, and tracked files call it only "the private reference implementation".
- **A2 (validated, material)**: The reference implements toolchain, contract kernel and quality
  gates. Its production BFF, browser frontend and browser end-to-end path are planned only.
  M3 and M4 therefore design these from public specifications rather than generalizing existing
  code.
- **A3 (material)**: Git, bash, curl and coreutils are the only host prerequisites; Go, Node,
  pnpm, protoc, buf and browsers come through the build graph. The reference's hermetic toolchain shows this is
  feasible in principle. M1 must prove it on a clean host.
- **A4 (material, approved 2026-10-29)**: The shared checks are distributed as a versioned dependency
  (a Bazel module plus a prek hook repository at the same version tag) and the reference app as a
  copyable template; no generator. See research R-07.
- **A5 (material, approved 2026-10-29 as target)**: Repository `selamy-labs/bazel-protobuf-bff-template`,
  public, GitHub template repository, MIT license (matching the Relentless CI templates). Stays
  local until the M8 publication gate.
- **A6**: Supported host profile for budgets and CI: Linux x86_64 with unprivileged user
  namespaces; macOS arm64 is a secondary profile measured separately in M1.
- **A7**: The Relentless CI templates are copy-only today (no packages, tags or reusable
  workflows); their upstream `main` is python `7dffef99` and typescript `9c18c555`. Reuse is of
  rules, pins and contracts, not imported code.
