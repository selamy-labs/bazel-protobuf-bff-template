# Packaging Requirements Quality Checklist: BFF container image

**Purpose**: Unit tests for the requirements on image identity, startup scope and the one-builder boundary
**Created**: 2026-11-01
**Feature**: [spec.md](../spec.md)

**Note**: `[x]` means a reviewer judged the requirement's quality. It says nothing about the implementation. `/speckit-implement` reads these marks and does not set them.

## Requirement Completeness

- [ ] CHK001 Is every property that identifies the image stated, including how its identity is computed? [Completeness, Contract]
- [ ] CHK002 Are the file list, owner, mode and timestamps of the one layer each specified? [Completeness, Spec §FR-002, Contract]
- [ ] CHK003 Are the ELF properties that define "statically linked" named? [Completeness, Spec §FR-004]
- [ ] CHK004 Is the exact refusal text, and its stream and exit code, written down in one place? [Completeness, Spec §FR-005, Contract]

## Requirement Clarity

- [ ] CHK005 Is "no filename or reported label is trusted" specific about what is compared with what? [Clarity, Spec §FR-007]
- [ ] CHK006 Is "repeatable" defined by what differs between the two builds and what must match? [Clarity, Spec §FR-006]
- [ ] CHK007 Does the spec separate the extracted-entrypoint claim from the real-runtime claim? [Clarity, Spec §SC-004]

## Requirement Consistency

- [ ] CHK008 Do the spec, contract and spec 001 FR-034 state one image builder in the same terms? [Consistency, Spec §FR-001, §FR-009]
- [ ] CHK009 Are the words "no base" and the base-related edge cases consistent across spec, research and contract? [Consistency, Spec §FR-002]
- [ ] CHK010 Does the stated limit on a healthy deployment agree with the production server's own contract? [Consistency, Spec §FR-010]

## Acceptance Criteria Quality

- [ ] CHK011 Can each success criterion be judged from a named artifact? [Measurability, Spec §SC-001 to §SC-004]
- [ ] CHK012 Does each negative in the matrix follow a stated positive? [Measurability, Acceptance Matrix]

## Scenario and Edge Case Coverage

- [ ] CHK013 Are hosts that cannot build this image addressed, including what a required test does there? [Coverage, Edge Case]
- [ ] CHK014 Is the compiler-free host addressed as a requirement, and not only in research? [Coverage, Edge Case]
- [ ] CHK015 Is a later runtime asset, such as CA roots, stated as out of scope with a named owner? [Coverage, Assumption]

## Dependencies and Assumptions

- [ ] CHK016 Is the reason for the rules_oci pin stated, given that upstream states no Bazel version? [Assumption, Research R2]
- [ ] CHK017 Is the dependency between this feature's inventory requirement and spec 010 stated? [Dependency, Spec §FR-008]

## Notes

- Items stay open until a reviewer marks them.
