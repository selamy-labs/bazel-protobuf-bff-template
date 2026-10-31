# Feature Specification: Browser Client and End-to-End Tests (M4)

**Feature Branch**: `005-browser-client`

**Created**: 2026-10-31

**Status**: Draft

**Input**: User description: "M4: a minimal React and TypeScript form and lookup using generated
Connect clients, and a real browser driven through the real transport against the real server,
covering success, field validation, not found, cross-principal isolation, unavailable, deadlines,
unauthenticated and cookie-only behavior, and internal errors with only a correlation ID. Exact-origin
CORS with no credential cookies. A deliberately broken error mapping must fail the named scenario."

**Parent**: [001 spec of specs](../001-bff-template-foundation/spec.md), milestone M4. This spec
covers parent tasks T025, T030–T032 and T039–T042, and parent requirements FR-001, FR-002 and FR-008.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A person creates and opens a note in a browser (Priority: P1)

A person opens the page, creates a note and sees its server-assigned name, then opens it by name.

**Why this priority**: this is the working behavior the template exists to show.

**Independent Test**: a real browser loads the built page and completes the flow against the real
server, across two origins.

**Acceptance Scenarios**:

1. **Given** a valid title, **When** the form is submitted, **Then** the page shows the new note's
   name and stored title.
2. **Given** a note's name, **When** it is looked up, **Then** the page shows the note.

---

### User Story 2 - Each failure reads as what happened (Priority: P1)

Each contract error reaches the person as its own outcome. Invalid input appears on the field. A
foreign note reads exactly like an absent one. An internal failure shows only a correlation ID.

**Why this priority**: typed failures are what the contract promises the browser.

**Independent Test**: each scenario is forced through the real browser, and the visible text is
checked. A deliberately broken error mapping must fail the matching scenarios and only those.

**Acceptance Scenarios**:

1. **Given** an empty or over-long title, or an over-long body, **Then** the message appears on that
   field.
2. **Given** an absent note or another person's note, **Then** both show the same not-found text.
3. **Given** storage is unavailable or the server is stopped, **Then** the page says to retry.
4. **Given** the call takes too long, **Then** the page says so.
5. **Given** no identity, **Then** the page says authentication is required.
6. **Given** an unexpected failure, **Then** the page shows a correlation ID, and no internal text.

---

### User Story 3 - The browser boundary holds (Priority: P1)

Requests from another origin are refused by the browser, and a cookie gives no access.

**Why this priority**: the security boundary is only real if a real browser enforces it.

**Independent Test**: a page on a disallowed origin calls the server, and a page sends a credentialed
request carrying only a cookie.

**Acceptance Scenarios**:

1. **Given** a page on a disallowed origin, **Then** the browser blocks the call.
2. **Given** a cookie and no credential, **Then** a credentialed browser request is blocked, and the
   same request outside CORS is refused as unauthenticated.

---

### User Story 4 - One command runs the app (Priority: P2)

A developer runs one command and gets the app locally, with test identity clearly labeled.

**Why this priority**: an adopter's first experience is running it.

**Independent Test**: start the command and complete the flow.

**Acceptance Scenarios**:

1. **Given** a fresh clone, **When** the command runs, **Then** the page and server start on local
   addresses, and the page is labeled as using test identity.
2. **Given** the production build, **Then** it contains no test identity.

### Edge Cases

- A second submit while one is in flight does not create a second note.
- A previous outcome does not stay on screen after a new action.
- Field messages clear once the input is valid.
- A violation on an unknown field still shows a general message.
- The browser's missing host libraries fail with a clear message, never a skip.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The page MUST call both RPCs only through generated clients.
- **FR-002**: Each RPC MUST have a production browser caller and a passing browser scenario for a
  success and a failure.
- **FR-003**: Client code MUST map each contract error to one distinct UI outcome.
- **FR-004**: Field violations from the server MUST appear on their fields. The server stays the only
  validator.
- **FR-005**: Browser tests MUST run a pinned browser against the real server binary, under the
  build's test runner, across two origins.
- **FR-006**: The browser and everything it loads MUST be pinned, and MUST NOT depend on host
  libraries beyond the C library.
- **FR-007**: A broken error mapping MUST fail the matching named scenarios and only those.
- **FR-008**: The production web build MUST contain no test identity, and the dev build MUST label it
  on screen.
- **FR-009**: One command MUST start the server and page for local use.
- **FR-010**: These tests MUST join the verifier's required inventory, and the fresh-clone witness
  MUST cover the browser path.

- **FR-011**: On Linux, the verifier's preflight MUST stop before any build when glibc is below the
  documented floor, or when it cannot confirm glibc, and MUST say how to get a supported host. The
  weak-isolation opt-in MUST NOT bypass this. macOS is out of this check.

### Key Entities

- **Outcome**: what the page shows for a call: a note, field messages, or one failure kind.
- **Credential source**: where the page gets its `Authorization` value.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A fresh clone with only the documented prerequisites passes every browser scenario.
- **SC-002**: 100% of RPCs have a production caller, one success and one typed failure in a browser.
- **SC-003**: The broken-mapping variant fails exactly its expected scenarios.
- **SC-004**: 0 occurrences of test identity in the production web bundle.

## Assumptions

- The UI is deliberately minimal: one form and one lookup, with no styling system.
- A production credential source is an adopter's work. The production build reports that sign-in is
  not configured, and its calls are refused.
- The server and page run on different origins, which makes CORS real in tests.
- Chromium only. Firefox and WebKit are a later addition.
- The hermetic browser is pinned for Linux x86_64 only. Other hosts fail with a clear message.
- Limits carried from M1–M3 still apply. macOS and hosted CI remain unverified.
