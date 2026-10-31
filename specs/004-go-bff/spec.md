# Feature Specification: Go BFF for Notes (M3)

**Feature Branch**: `004-go-bff`

**Created**: 2026-10-31

**Status**: Draft

**Input**: User description: "M3: real Go Connect BFF for CreateNote and GetNote with a memory
storage port. Field validation, not-found, cross-principal isolation, unavailable, cancellation,
deadlines, unauthenticated and cookie-only rejection, internal errors with only a correlation ID.
Exact-origin CORS with no credentials. Authenticator boundary: dev test identity visible and fenced;
production refuses test identity and fails closed without a configured verifier. No identity
provider integration."

**Parent**: [001 spec of specs](../001-bff-template-foundation/spec.md), milestone M3. This spec
covers parent tasks T027–T029 and T033–T038 and parent requirements FR-003–FR-007.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A caller stores and reads its own notes (Priority: P1)

An identified caller creates a note and reads it back by name. Another caller cannot tell that the
note exists.

**Why this priority**: this is the working behavior every other story constrains.

**Independent Test**: start the server, create a note as one caller, read it as that caller, and
read it as another caller.

**Acceptance Scenarios**:

1. **Given** a valid title, **When** a caller creates a note, **Then** it receives a server-assigned
   name and creation time, and any name it supplied is ignored.
2. **Given** a stored note, **When** its owner reads it, **Then** the stored note is returned.
3. **Given** another caller's note or an absent name, **When** a caller reads it, **Then** both give
   the same not-found result, with no difference that reveals existence.

---

### User Story 2 - Every failure is typed and leaks nothing (Priority: P1)

Each failure maps to one contract error. Invalid input names its fields. An unexpected failure gives
the caller a correlation ID and nothing else, and the detail goes to the server log.

**Why this priority**: the browser turns these into outcomes, and an internal message in a response
is a leak.

**Independent Test**: each condition is forced through a real server, and the response code and body
are checked. An unexpected failure carrying a planted secret must not show it to the caller.

**Acceptance Scenarios**:

1. **Given** invalid input, **When** a caller sends it, **Then** the error is `invalid_argument` with
   field violations, and storage was not touched.
2. **Given** storage reports it is unavailable, **Then** the error is `unavailable`.
3. **Given** an unexpected failure, **Then** the error is `internal` with a correlation ID only, and
   the server log holds the detail under that ID.
4. **Given** a handler panics, **Then** the caller sees the same `internal` shape and the server
   keeps serving.

---

### User Story 3 - Time limits reach storage (Priority: P1)

Every call has a deadline. A caller's shorter deadline is honored, a longer one is capped, and a
cancelled call stops work in storage.

**Why this priority**: an unbounded downstream call is how a small failure becomes an outage.

**Independent Test**: a storage fake blocks until its context ends and records why.

**Acceptance Scenarios**:

1. **Given** no client deadline, **Then** storage sees a deadline of about 2 seconds.
2. **Given** a client deadline above 10 seconds, **Then** storage sees a deadline of at most 10.
3. **Given** a shorter client deadline expires, **Then** the error is `deadline_exceeded`.
4. **Given** the client cancels, **Then** storage sees cancellation.

---

### User Story 4 - Identity fails closed (Priority: P1)

No identity means no access. A cookie is never identity. Test identity exists only in dev mode, is
visibly labeled, and cannot be enabled in production. Production with no configured verifier refuses
to start.

**Why this priority**: a starter is copied widely, so a default that accepts anyone would spread.

**Independent Test**: start the binary in each mode and configuration, and send requests with no
credential, a cookie only, a test credential and a malformed credential.

**Acceptance Scenarios**:

1. **Given** no credential, a cookie only, or a malformed one, **Then** the error is
   `unauthenticated`.
2. **Given** production mode with test identity requested, **Then** the process refuses to start.
3. **Given** production mode with no verifier, **Then** the process refuses to start.
4. **Given** the production binary, **Then** its dependency graph has no test identity or fault
   injection.

---

### User Story 5 - Browser access is limited to one origin (Priority: P2)

Only the configured web origin may call the server from a browser, and no ambient credentials are
allowed.

**Why this priority**: M4 calls this from a browser, and a permissive policy would be copied.

**Independent Test**: send preflight and simple requests from allowed and other origins and inspect
the response headers.

**Acceptance Scenarios**:

1. **Given** the configured origin, **Then** that exact origin is echoed and credentials are not
   allowed.
2. **Given** any other origin, including a prefix or a suffix of the allowed one, **Then** no
   allow-origin header is sent.

### Edge Cases

- A cookie and a valid credential together: the cookie is ignored and the credential decides.
- A note with a title of surrounding whitespace is stored trimmed.
- A request body over the size limit is rejected before it is parsed.
- A client deadline of zero or a malformed one follows the transport's defined behavior and is
  tested.
- Shutdown lets in-flight calls finish within a bound, and new calls are refused.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The server MUST implement `CreateNote` and `GetNote` over the generated contract.
- **FR-002**: Dependencies MUST point from transport to application to ports, and adapters MUST
  implement ports. The dependency direction MUST be checked deterministically.
- **FR-003**: Inputs MUST be validated by the schema's rules before any port call, and failures MUST
  carry field violations.
- **FR-004**: Notes MUST be scoped to the principal, and absent and foreign notes MUST be
  indistinguishable to a caller.
- **FR-005**: Failures MUST map to `invalid_argument`, `not_found`, `unauthenticated`, `unavailable`,
  `deadline_exceeded` or `internal`.
- **FR-006**: `internal` responses MUST contain only a generic message and a correlation ID, and the
  detail MUST reach the server log under that ID. A panic MUST be handled the same way. A store or
  authenticator MUST NOT be able to label its own failure as public, whatever error type or code it
  returns, wrapped or not.
- **FR-007**: Every RPC MUST have a default deadline of 2 seconds and a maximum of 10, and the context
  MUST reach the port.
- **FR-008**: Identity MUST come from an `Authenticator` boundary reading only the `Authorization`
  header. Cookies MUST NOT authenticate.
- **FR-009**: Test identity MUST be labeled, MUST need explicit dev mode, and MUST NOT be accepted or
  linkable in a production configuration.
- **FR-010**: Production mode MUST refuse to start without a configured verifier. No
  success-returning production stub and no identity provider integration may ship.
- **FR-011**: Test identity and fault injection MUST be absent from the production binary's
  dependency graph, checked deterministically.
- **FR-012**: CORS MUST allow only an exact configured origin and MUST NOT allow credentials.
- **FR-013**: The binary MUST print one ready event on stdout, MUST NOT read stdin, and MUST shut down
  gracefully within a bound.
- **FR-014**: Go code MUST pass the pinned format check in the verifier. The vet analyzers join in M5
  (research R7).
- **FR-015**: These tests MUST join the verifier's required inventory.

### Key Entities

- **Note**: as in M2. Stored with its owner.
- **Principal**: a subject and a kind (`TEST_IDENTITY` or `VERIFIED`).
- **Store**: the port for saving and reading notes, taking the request context.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of contract error conditions are produced through a real server and checked for
  code and body.
- **SC-002**: A planted secret in an unexpected failure appears in 0 responses and in the server log.
- **SC-003**: 100% of disallowed start-up configurations exit nonzero before serving a request.
- **SC-004**: A deliberately weakened dependency rule, error mapping or identity fence fails its
  named witness.

## Assumptions

- A verifier is an interface an adopter implements. This repository ships no implementation of it.
- The test credential is `Authorization: Bearer test-identity.<subject>`, accepted only in dev mode.
- The in-memory store is the only adapter, and data does not survive a restart.
- Request bodies are limited to 64 KiB, which fits a maximum note.
- Limits carried from M1 and M2 still apply. macOS and hosted CI remain unverified.
