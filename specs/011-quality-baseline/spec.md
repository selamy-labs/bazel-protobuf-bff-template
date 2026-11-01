# Feature Specification: Quality baseline

**Feature Branch**: `011-quality-baseline`
**Created**: 2026-11-01
**Status**: Specified. Current status is in the [index](../README.md)
**Input**: M9. The quality baseline amendment of 2026-11-01, mandatory for initiative closure. Parent: spec 001 FR-019, FR-020, FR-026; constitution IV and V. Each control's state today is measured, never assumed; closure needs every applicable control enforced and evidenced. Lands after M8 (spec 010) is frozen, and nothing here changes that frozen revision.

## Clarifications

### Session 2026-11-01

- Q: May the initiative close while a control is still a gap? → A: No. The inventory may land with named gaps, but closure needs every applicable control enforced and evidenced; a gap is an owned open task, never a waiver.
- Q: Which direction is a weakening for a budget? → A: Per metric. A complexity maximum weakens by rising, a coverage or mutation minimum by falling; each metric has a positive control that moves it stricter and passes, and a negative one that moves it weaker and is refused.
- Q: Does "nothing is requalified twice" skip any check? → A: No. Only a repeat qualification of an unchanged revision is not repeated; the checks on changed code, on integrations and the final full verifier run as always, and a receipt holds for its own revision only.
- Q: Is the archive path a maintained cleanup path? → A: No. It is a primitive plus a handoff that records eligibility behind a verified archive and authorizes nothing; custody of any cleanup stays with the lifecycle owner, and the 2026-11-01 loss stays unrecovered.
- Q: Can a condition-coverage tool stand in for Go branch outcomes? → A: Only as a named sub-metric. gobco 1.3.4 has `-branch`, and on a Go 1.27 fixture it reported Condition 2/2 and Branch 2/2 while native statements were 50% and the `select` cases were never called: it does not measure `select`, so no complete Go branch claim and no waiver follows from it.
- Q: Does the archive control recover the lost object? → A: No. Prevention and recovery are separate: the controls passing changes nothing about `8ad7470`, which stays unrecovered.
- Q: Is config-driven behavior part of the baseline? → A: Yes, by the human baseline of 2026-11-01: typed, versioned configuration with defaults only once at the composition boundary, required inner parameters, defined precedence and effective-value provenance, and no bypass configuration for security, quality, spend or invariants.
- Q: Which Python is first-party here? → A: The generators in `tools/release` and `tools/pins`. Vendored Spec Kit code is not first-party.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reviewer sees every baseline control with its real state (Priority: P1)

A reviewer opens one table and reads, for each control, whether it is enforced, partial or a gap, the gate that enforces it, the measured value, the threshold with its source, and the owner of what remains.

**Why this priority**: closure is judged against this table, and a control marked enforced without a failing gate is the one failure mode this feature exists to prevent.

**Independent Test**: for each control marked enforced, a planted defect fails its named gate and the restored control passes.

**Acceptance Scenarios**:

1. **Given** the table, **When** a control says enforced, **Then** its gate, its planted defect and its restored pass are named with the revision they ran at.
2. **Given** a control that only measures, **When** it is read, **Then** it says partial, and no threshold is claimed.
3. **Given** a control with no tool, **When** it is read, **Then** it says gap, with an owner and a task.

### User Story 2 - A measured budget cannot be met by weakening (Priority: P1)

A contributor who raises a function's complexity past its budget, lowers a floor, suppresses a survivor or widens an exclusion is refused by the gate, and the refusal names the number.

**Why this priority**: budgets that can be edited past are not budgets.

**Independent Test**: each weakening is planted and refused; the budget file is a policy input the base evaluator compares.

**Acceptance Scenarios**:

1. **Given** a measured budget, **When** a change exceeds it, **Then** the gate fails and names the function and the numbers.
2. **Given** a budget file, **When** a proposal moves a budget toward leniency, **Then** the base evaluator refuses by comparison with the base. Leniency has a direction per metric: a complexity maximum weakens by rising, a coverage or mutation minimum weakens by falling. For each metric the positive control moves it stricter and passes, and the negative control moves it weaker and is refused.
3. **Given** the base evaluator, **When** its limit is read, **Then** the limit stays stated: it does not authenticate the adapter or workflow that invokes it (spec 008 R9, constitution V), and no general tamper-proof claim is made.

### User Story 3 - A second implementer can take one slice without touching another's files (Priority: P2)

A worker takes one slice, owns only its new files, delivers a measured result, and the integrator binds it.

**Why this priority**: the baseline is wide, and serial work would delay closure.

**Independent Test**: the slice table names disjoint new paths per worker and the shared files the integrator alone changes.

**Acceptance Scenarios**:

1. **Given** two slices in progress, **When** their diffs are compared, **Then** no file is touched by both.

### User Story 4 - Behavior is driven by typed configuration, with defaults in one place (Priority: P2)

A maintainer reads one versioned schema to know every configurable behavior, where its default is set, which source wins when several are given, and where an effective value came from; no inner function carries a silent tuning default.

**Why this priority**: scattered defaults are untestable behavior; the baseline of 2026-11-01 names them.

**Independent Test**: the inventory of defaults names each one with its boundary; omitted, invalid, precedence, alternative and fallback cases have tests; a behavior change made by configuration alone has a witness.

**Acceptance Scenarios**:

1. **Given** a configurable behavior, **When** its value is omitted, **Then** the default applies once at the composition or input boundary, and the inner parameter is required.
2. **Given** two sources for one value, **When** both are set, **Then** the documented precedence decides, and the effective value records its source.
3. **Given** a security, quality, spend or invariant contract, **When** configuration is read, **Then** no setting bypasses it.

### Edge Cases

- A tool that cannot be pinned by checksum is not adopted; the control stays a gap with the reason.
- A measurement that produces no output is an error, never a pass.
- A tool's own advisory report, such as the M5 complexity scan, stays advisory until it gates with a defect.
- The Go toolchain reports statements, not branch outcomes; a condition-coverage tool reports what it reports and is named as such.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every control in the baseline MUST appear in one tracked table with its state, gate, measured value, threshold source, exclusions and owner, and the table MUST be reconciled with the registry and the evidence records in the same change that alters any of them.
- **FR-002**: A control MUST be marked enforced only when a named registry check fails on a planted defect at a named revision, with the restored control passing.
- **FR-003**: Every adopted tool MUST be pinned by version and checksum through the existing module, package or tool locks, and MUST appear in the provenance inventory with its license read from the artifact.
- **FR-004**: Every numerical budget MUST be set from a measurement of this tree, recorded with the revision measured, and MUST only ratchet in the direction of strictness; no floor is lowered and no blanket exclusion is added.
- **FR-005**: Complexity MUST be measured as both cognitive and cyclomatic, per function, for Go and TypeScript, through the existing analyzer and lint routes, with budgets per FR-004.
- **FR-006**: Duplicate detection MUST be language-aware; a confirmed duplicate is deleted in a focused change, and a remaining finding is recorded with its reason rather than suppressed.
- **FR-007**: Dead-code detection MUST be reachability-based, with declared entry points, and a confirmed finding is deleted and its stale dependency removed.
- **FR-008**: Coverage MUST be measured as both lines and branch outcomes, with thresholds and changed-code expectations set from measured policy, and no blanket exclusion that drops uncovered branches. Where the toolchain cannot produce branch outcomes today, that is a blocking implementation task with an owner, not a waiver; a condition-coverage tool is recorded by the construct it measures and its stated gaps, such as `select`, and is never called complete branch coverage. No outcome is derived from statement counts. The tool is chosen by a scoped probe and reuses the existing coverage framework.
- **FR-009**: Property-based tests MUST exist for suitable behaviors in each first-party language present, including the first-party Python generators, and they MUST exercise real boundaries.
- **FR-017**: The three first-party Python scripts (`tools/pins/gen_browser_libs.py`, `tools/pins/gen_multitool_lock.py`, `tools/release/inventory_tables.py`) MUST have their own typing, lint, complexity, coverage, property and mutation applicability decided and recorded, each with a probe through the pinned uv route; a Go- or TypeScript-only gate text is never a Python waiver.
- **FR-010**: Mutation testing MUST run over the policy-bearing packages with survivors inspected and recorded; an equivalent or unreachable mutant is excluded only with a reason.
- **FR-011**: External effects MUST be tested through working fakes with a shared contract suite, and no mock library is introduced.
- **FR-012**: Escape hatches from the type systems MUST be inventoried with a justification each, or removed.
- **FR-013**: Work MUST be split into slices with disjoint new-file ownership and one integrator for the shared files.
- **FR-014**: Nothing here weakens an existing gate or introduces a second quality framework. Requalification without a change is not repeated, but the required checks on changed code, on integrations and the final independent full verifier run as always; a green result at one revision is never the receipt of another.
- **FR-015**: Git maintenance that could drop objects MUST be preceded by the archive primitive, run by the lifecycle owner: a named ref bundled, the bundle verified, and a fresh isolated fetch that resolves the exact commit and tree; a cleanup-eligibility record exists only behind a verified archive and authorizes nothing by itself, so a failed archive leaves no record and cleanup stays ineligible by construction. No cleanup step exists inside the primitive or its handoff, and neither is a maintained cleanup path: custody of any cleanup stays with the owner who reads the record.

- **FR-016**: Configurable behavior MUST be declared in a versioned typed schema with validation, precedence and effective-value provenance; a default MUST be set once at the top composition or input boundary and inner parameters MUST be required; scattered defaults MUST be inventoried and moved by focused compatibility-preserving changes, each with omitted, invalid, precedence, alternative and fallback tests and a witness that configuration alone changes the behavior; runtime evaluation MUST use OpenFeature only where it fits, with an explicit boundary fallback source; protocol zero values and absent-field semantics are schema contracts and MUST NOT serve as application configuration defaults; security, quality, spend and invariant contracts MUST NOT be bypassable by configuration.

### Key Entities

- **Control row**: control, state, gate, measured value with revision, threshold with source, exclusions with reasons, owner, task.
- **Budget file**: the numerical budgets, a policy input the base evaluator compares.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The inventory may land with named gaps, but initiative closure requires every agreed applicable control enforced and evidenced; an applicable control still lacking a tool or instrumentation is an open owned task, and never a waiver.
- **SC-002**: Every budget has a measurement older than itself, and no budget ever moves toward leniency in history.
- **SC-003**: Every slice's diff touches only its own new files plus the integrator's shared files, verified by path.
- **SC-004**: The full verifier passes at the candidate with every new gate bound, and an independent review verifies it at that exact revision.
- **SC-005**: The archive primitive has a representative failed-archive control that shows cleanup ineligible, a late-arriving destination refused, and a successful named-ref archive with a fresh restore of the exact commit and tree. These prevent a future loss; they recover nothing, and the 2026-11-01 loss stays recorded as unrecovered whatever they show.
- **SC-006**: Every configurable behavior is in the schema with one default site, and the inventory of scattered defaults reaches zero or names each remaining one with its owner.

## Assumptions

- Tool choices follow the research leads and a compatibility probe each; none is accepted by name.
- The baseline's Hypothesis requirement applies to the first-party Python generators that exist now; vendored Spec Kit code is not first-party.
