# Requirements Quality Checklist: Hosted example and local demo

**Purpose**: Unit tests for the requirements of spec 012, for an implementation reviewer at standard depth: the input funnel, the identity and store ports, the local demo's truthfulness, and the live gates
**Created**: 2026-11-01
**Feature**: [spec.md](../spec.md)

`[x]` means the reviewer judged the requirements-quality criterion satisfied; it says nothing about implementation progress. `/speckit-implement` reads this state and never changes it.

## Requirement Completeness

- [ ] CHK001 - Is every input of the funnel enumerated (project, billing, region, APIs, identities, state, endpoint, auth, origins, base path, persistence, cost bounds, rollback), so none can be added as a hidden default? [Completeness, Spec §FR-002, §FR-003, §FR-008, §FR-010]
- [ ] CHK002 - Are the per-role identities named, and is the prohibition on deploy or build permission for the runtime identity explicit? [Completeness, Spec §FR-004]
- [ ] CHK003 - Are the local profile's fields (ports, origin, persistence, seed, sign-in) enumerated? [Completeness, Spec §FR-020]
- [ ] CHK004 - Does the recording requirement say when it is recorded, from what, how it is regenerated and what it must not be? [Completeness, Spec §FR-024]

## Requirement Clarity

- [ ] CHK005 - Is "the backend actually runs" defined by an observable (a verified browser flow at a recorded endpoint and digest), not by a deployment existing? [Clarity, Spec §US1, §FR-014]
- [ ] CHK006 - Is "persistence" stated separately for the local and the hosted stores, with what survives a restart and what reset clears? [Clarity, Spec §FR-017, §FR-022]
- [ ] CHK007 - Is "supported sign-in" distinguished from the development identity, and is the production refusal kept in both? [Clarity, Spec §FR-016, §FR-021]
- [ ] CHK008 - Are "base path" and "RPC base URL" defined as independent inputs with the Pages origin fact stated? [Clarity, Spec §FR-019]

## Requirement Consistency

- [ ] CHK009 - Do FR-001 and FR-005 agree that nothing rebuilds the image and that no deployment runs on GitHub Actions? [Consistency]
- [ ] CHK010 - Do FR-011 and SC-004 agree that synthetic values are the only values in public source, and does the ownership split keep real values with the infrastructure owner? [Consistency]
- [ ] CHK011 - Do FR-021 and spec 009's refusal agree that the dev identity is never deployed and production still refuses without a verifier? [Consistency]
- [ ] CHK012 - Does FR-023 agree with the README's host truth (Linux supported, macOS unverified) and with spec 010's publication claims? [Consistency]

## Acceptance Criteria Quality

- [ ] CHK013 - Can SC-002 be verified from the evidence record alone (endpoint, identity, digest, flow result)? [Measurability, Spec §SC-002]
- [ ] CHK014 - Can SC-006 be verified from a fresh checkout by a reader following one command? [Measurability, Spec §SC-006]
- [ ] CHK015 - Does every FR in the matrix name a defect and its expected refusal, and is the layer (off, loc, hum, live) stated? [Measurability, acceptance-matrix.md]
- [ ] CHK016 - Do the Well-Architected rows name a command and an evidence location per pillar, and never a document as live proof? [Measurability, Spec §SC-005]

## Scenario and Edge Case Coverage

- [ ] CHK017 - Is a port in use, and an invalid profile value, specified as refusal before start with the value named? [Edge Case, Spec §FR-020]
- [ ] CHK018 - Is the case of a digest that is not the recorded one specified as refusal, never rebuild? [Edge Case, Spec §FR-001]
- [ ] CHK019 - Is the case of a wrong origin (a path, a wildcard, a foreign origin) specified for both the server and the journey? [Edge Case, Spec §FR-019]
- [ ] CHK020 - Is cancellation of an in-flight deployment specified with what keeps serving? [Edge Case, Spec §FR-009]

## Dependencies and Assumptions

- [ ] CHK021 - Is the output contract with the infrastructure owner versioned, and is a missing or newer version a refusal? [Dependency, research R1]
- [ ] CHK022 - Are the live gates (create, apply, access, billing, spend) each named as a recorded operator gate, with none automatic? [Assumption, Spec §FR-013]
- [ ] CHK023 - Is the macOS and offline-cold-start limitation stated as a limitation and not as a gap to be assumed closed? [Assumption, Spec §FR-023]
