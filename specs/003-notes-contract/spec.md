# Feature Specification: Neutral Notes Contract and Build-Time Generation (M2)

**Feature Branch**: `003-notes-contract`

**Created**: 2026-10-31

**Status**: Draft

**Input**: User description: "M2: neutral notes contract and build-time generation. NoteService with
CreateNote and GetNote per the approved API contract; format, lint and breaking checks against an
immutable baseline read from Git objects, failing closed, with an explicit bootstrap marker;
declared validation; build-time server and browser code; no committed generated code; a negative
compile gate; an API-style ledger; canonical verifier integration; genuine positive and negative
cases."

**Parent**: [001 spec of specs](../001-bff-template-foundation/spec.md), milestone M2. This spec
covers parent tasks T005 and T016–T026 and parent requirements FR-009–FR-012.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One schema produces both typed clients (Priority: P1)

A contract author edits one schema. The build produces the server interface and the browser client
from it. Nobody commits generated code, and a caller that disagrees with the schema does not compile.

**Why this priority**: M3 and M4 consume this output. Without it there is no typed slice.

**Independent Test**: a clean build produces both outputs from the schema alone, with no generated
file in the repository. A compile probe using the wrong request shape fails for that reason, and its
corrected counterpart compiles.

**Acceptance Scenarios**:

1. **Given** a clean checkout, **When** the build runs, **Then** server and browser code are produced
   from the schema using only pinned tools.
2. **Given** a browser caller passes the wrong request type, **When** it is compiled, **Then**
   compilation fails on that call; the same caller with the right type compiles.
3. **Given** a schema field is renamed, **When** the build runs, **Then** both consumers of the old
   name fail, with no separate regeneration step to forget.

---

### User Story 2 - A breaking change cannot pass silently (Priority: P1)

A contract author changes the schema. A compatible addition passes. A change that would break a
deployed reader fails and names the change. If the baseline cannot be read, the check fails instead
of passing.

**Why this priority**: compatibility is the promise a shared contract makes.

**Independent Test**: each breaking probe is applied to a valid schema that first passes. Each is
rejected for its own reason, and the compatible addition passes.

**Acceptance Scenarios**:

1. **Given** a field number, field type or field name change, a wire-name change, or a removed field
   with no reservation, **When** the compatibility check runs, **Then** it fails naming the rule.
2. **Given** an added optional field, **When** the check runs, **Then** it passes.
3. **Given** the declared baseline revision is absent or the clone is shallow, **When** the check
   runs, **Then** it fails closed with a message saying what to fetch.
4. **Given** a schema with no history, **When** a reviewed bootstrap marker is present, **Then** the
   check reports that it compared nothing. **Given** schema history exists, **Then** the marker is
   refused.

---

### User Story 3 - Style and validation are declared once (Priority: P2)

Field constraints live in the schema and nowhere else. Style rules are applied by a pinned linter,
and any rule that does not apply is written in a ledger with its reason.

**Why this priority**: a second definition of a rule drifts from the first.

**Independent Test**: a defect fixture for each of format, lint and validation fails, and its
corrected counterpart passes.

**Acceptance Scenarios**:

1. **Given** an unformatted or non-conforming schema, **When** verify runs, **Then** the named check
   fails.
2. **Given** a comment-based lint exclusion or a configuration exclusion, **When** verify runs,
   **Then** it does not excuse the violation.
3. **Given** the validation rules, **When** title, body and name values at and beyond each limit are
   evaluated, **Then** the outcomes match the contract.

### Edge Cases

- A title of only whitespace is invalid; a title of 120 multi-byte characters is valid.
- A baseline value that is a branch name or a short hash is refused: only a full commit hash counts.
- A baseline hash that exists but does not contain the schema directory fails closed.
- A probe that fails for an unrelated reason, such as a compile error, earns no credit.
- A tracked generated file in the schema or client tree fails verification.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The schema MUST define exactly two RPCs, `CreateNote` and `GetNote`, in a neutral
  package, matching the approved API contract.
- **FR-002**: Field constraints MUST be declared in the schema with machine-readable validation
  annotations.
- **FR-003**: Server and browser code MUST be produced by the build graph using pinned tools, and
  MUST NOT be committed.
- **FR-004**: Generation MUST NOT need a host compiler, a schema registry or a network connection
  inside build actions.
- **FR-005**: The schema MUST pass a pinned format check and a pinned lint check, and exclusions
  through comments or configuration MUST NOT be honored.
- **FR-006**: A compatibility check MUST compare the working schema with an immutable, declared full
  commit hash read from Git objects, and MUST fail closed when it cannot.
- **FR-007**: A bootstrap marker MUST be explicit, and MUST be refused once schema history exists.
- **FR-008**: Each breaking-change class in the parent spec MUST have a probe that fails for its
  intended rule, and a compatible addition MUST pass.
- **FR-009**: A compile gate MUST show that a caller using the wrong type fails, and that its
  corrected counterpart compiles.
- **FR-010**: Each RPC MUST appear in an API-style ledger with applicable rules and justified
  exceptions.
- **FR-011**: These checks MUST run through the canonical verifier and its expected-check and
  required-test inventories.
- **FR-012**: The browser toolchain MUST be pinned by a committed lockfile with integrity hashes.

### Key Entities

- **Note**: resource with a server-assigned name, a title, a body and a creation time.
- **Breaking baseline**: a declared full commit hash, or an explicit bootstrap marker.
- **Probe**: a valid schema, one mutation, and the rule expected to reject it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of breaking probes are rejected by their intended rule, and the compatible
  addition passes.
- **SC-002**: A clean checkout generates both consumers with zero generated files committed.
- **SC-003**: 100% of validation boundary cases match the contract.
- **SC-004**: Every failure names its check and rerun command.
- **SC-005**: A fresh clone with only the documented prerequisites passes the verifier with the new
  checks included.

## Assumptions

- The first schema uses the bootstrap marker. A later commit replaces it with that schema's commit.
- A template copy has fresh history, so its first schema commit uses the marker too.
- Client-side validation is not part of this slice. The server is authoritative.
- Limits carried from M1 still apply: macOS and hosted CI remain unverified.
- This milestone adopts no upstream analyzer. The #28 consumer profile stays an open dependency.
