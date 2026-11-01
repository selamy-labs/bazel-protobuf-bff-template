# Requirements Quality Checklist: Quality baseline

**Purpose**: Unit tests for the requirements of spec 011, for an implementation reviewer at standard depth: measurement before policy, the ratchet, the enforced state, and the archive primitive
**Created**: 2026-11-01
**Feature**: [spec.md](../spec.md)

`[x]` means the reviewer judged the requirements-quality criterion satisfied; it says nothing about implementation progress. `/speckit-implement` reads this state and never changes it.

## Requirement Completeness

- [ ] CHK001 - Are the columns of the control table enumerated so that a row cannot omit the measured revision or the threshold's source? [Completeness, Spec §FR-001]
- [ ] CHK002 - Is the set of controls the table must cover tied to a named source, so a control cannot be left out silently? [Completeness, Spec §FR-001, Gap]
- [ ] CHK003 - Are the three states (enforced, partial, gap) defined so that each row has exactly one? [Completeness, Spec §US1]
- [ ] CHK004 - Is the owner of a gap required to be a task, not a person's name alone? [Completeness, Spec §SC-001]

## Requirement Clarity

- [ ] CHK005 - Is "enforced" defined by an observable event (a named gate failing on a planted defect at a named revision) rather than by intent? [Clarity, Spec §FR-002]
- [ ] CHK006 - Is the direction of strictness stated per metric, so that "ratchet" is not ambiguous between a maximum and a minimum? [Clarity, Spec §FR-004, Clarifications]
- [ ] CHK007 - Is "measured" defined to exclude a tool's run that produced no output? [Clarity, Edge Cases]
- [ ] CHK008 - Is the branch-coverage requirement precise about what a condition-coverage tool may be called and what it may not? [Clarity, Spec §FR-008]
- [ ] CHK009 - Is "first-party Python" bounded by path, so vendored code is excluded without a judgment call? [Clarity, Clarifications]

## Requirement Consistency

- [ ] CHK010 - Do FR-004 and US2 agree that the base evaluator, not the adapter, compares the budget with the base? [Consistency, Spec §FR-004, §US2]
- [ ] CHK011 - Does FR-014 agree with SC-004 that the full verifier always runs at the candidate, and that no receipt crosses revisions? [Consistency, Spec §FR-014, §SC-004]
- [ ] CHK012 - Does FR-015 agree with SC-005 and the Clarifications that the archive is a primitive and a handoff, never a cleanup path? [Consistency, Spec §FR-015, §SC-005]
- [ ] CHK013 - Does the stated limit of the base evaluator (it does not authenticate its caller) match spec 008 R9 and constitution V, with no stronger claim elsewhere in the spec? [Consistency, Spec §US2]

## Acceptance Criteria Quality

- [ ] CHK014 - Can SC-002 be checked from history alone, with no judgment about intent? [Measurability, Spec §SC-002]
- [ ] CHK015 - Can SC-003 be checked by path comparison, and is the integrator's shared-file set enumerated? [Measurability, Spec §SC-003, Plan §Ownership]
- [ ] CHK016 - Does every FR in the acceptance matrix name both a valid case and a defect with its expected refusal? [Measurability, acceptance-matrix.md]

## Scenario and Edge Case Coverage

- [ ] CHK017 - Is the case of a tool that cannot be pinned by checksum specified as a gap with a reason rather than a pin by name? [Edge Case, Edge Cases]
- [ ] CHK018 - Is the case of a confirmed duplicate that is not deleted specified (recorded with a reason, never suppressed)? [Edge Case, Spec §FR-006]
- [ ] CHK019 - Is the case of an equivalent or unreachable mutant specified with the reason requirement? [Edge Case, Spec §FR-010]
- [ ] CHK020 - Is the case of a failed archive specified as leaving no eligibility record, and of a destination that changes between restore and publication as a refusal? [Edge Case, Spec §FR-015, Plan §Risks]
- [ ] CHK021 - Is the recovery expectation for the 2026-11-01 loss stated honestly as unrecovered until both controls run? [Edge Case, Spec §SC-005]

## Dependencies and Assumptions

- [ ] CHK022 - Is the assumption that each tool is accepted only by a probe stated, with the probe's record location? [Assumption, Assumptions, research.md]
- [ ] CHK023 - Are the upstream boundaries (policy-diff #26 for authorizing a weakening) kept, so that this spec claims no tamper-proof property? [Dependency, Spec §US2]
