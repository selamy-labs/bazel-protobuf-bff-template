# One note, three ways in

Local Notes web, shipped Python CLI and real Textual PTY, generated Connect clients, real Go development BFF and current local store. The terminal segments replay actual terminal output with a high-contrast palette. No downstream gRPC or hosted run is asserted.

Tested main [cf5f070c1fc364d12a759812a5ff702ec506fadc](https://github.com/selamy-labs/bazel-protobuf-bff-template/tree/cf5f070c1fc364d12a759812a5ff702ec506fadc). Browser scenes show the actual styled web bundle from local candidate `ae172d5aa59a507e9331a745f7224a201d0c873d` for [#61](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/61). The Go BFF/CLI/TUI use the main source and the restart-durable file store. The first published cut under earlier recordings preserves the unstyled main browser. Compare the 1200-pixel and 390-pixel captures in this evidence bundle.

## Chapters

| Start | Scene | What the viewer sees |
|---:|---|---|
| 0:00 | Start a shift handoff | Ana needs to pass along a location. When she leaves the title empty, the web app tells her which field to fix. |
| 0:07 | Leave a handoff | Ana leaves a handoff for the next shift. The web app saves her note and returns a reference. |
| 0:14 | Find it again | After a reload, Ana enters the reference and gets back the original handoff. |
| 0:20 | Keep the handoff private | Bob tries the same reference. The service hides the note and tells him it was not found. |
| 0:26 | Work from a shell | The command-line client creates a second note. Ana reads it back. Bob gets the same refusal. |
| 0:32 | Pick it up in the browser | Ana opens the reference printed by the command-line client. The browser shows the same title and body. |
| 0:39 | Read in a terminal | The interactive terminal opens that second note for Ana. Switching to Bob keeps its contents private. |

## Feature and story coverage

| Spec requirement | Film evidence | Remaining work or boundary |
|---|---|---|
| 001 US1 / 004 US1 / 005 US1 • create and open | 0:07 Leave a handoff, 0:14 Find it again | Notes are returned by the Go BFF after browser reload. |
| 004 US2/4 / 005 US2 • typed validation and identity isolation | 0:00 Start a shift handoff, 0:20 Keep the handoff private, 0:26 Work from a shell, 0:39 Read in a terminal | The primary film shows blank-title guidance and cross-subject refusal. The companion shows invalid title length/reference, absent identity, deadline, outage and a correlation reference. Cancellation, expired credentials and oversized body still need independent evidence. |
| 005 US3/4 • browser boundary and local startup | 0:07 Leave a handoff | The running local client is visible. Cross-origin enforcement, production bundle identity exclusion and the one-command startup are verifier work. |
| 013 US1 • portable typed CLI and shared web round trip | 0:26 Work from a shell, 0:32 Pick it up in the browser | The exact CLI-created reference opens in the browser. The companion shows the reverse browser→CLI read after a real local service restart, help, non-loopback identity refusal and typed JSON errors. Unfilmed command combinations remain in the CLI contract suite. |
| 013 US2 • interactive typed read | 0:39 Read in a terminal | The shipped read-only TUI loads the CLI reference as the owner and denies the other subject. |
| 013 US3 • optional Android, iOS and desktop clients | No scene | Obtain an actual emulator, simulator, and supported-OS launch when the corresponding client ships. See [#30](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/30), [#31](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/31) and [#32](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/32). |
| Required CI and downstream fake | No scene | Land and run [#60](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/60) and [#59](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/59). The existing main CI desktop/mobile browser artifact uses its separate Connect fake. |
| Developer adoption, build, governance and deployment | No scene | The per-story appendix identifies the engineering checks. [#20](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/20) owns the hosted release exercise and [#21](https://github.com/selamy-labs/bazel-protobuf-bff-template/issues/21) the application-to-template lessons. |

## Companion review chapters

| Start | Scene | Outcome |
|---:|---|---|
| 0:00 | Fix an invalid field | A too-long title and a malformed note reference each produce a message on the field that needs attention. |
| 0:06 | Require an identity | Creating a note without an identity is refused. The page asks for authentication. |
| 0:12 | Handle storage unavailability | When the store is unavailable, the app reports the outage and invites the user to retry. |
| 0:19 | Bound a stuck request | A request that does not finish in time returns a timeout message. The form remains available for another attempt. |
| 0:26 | Keep internal failures private | An unexpected failure gives the reviewer a correlation reference without displaying the server’s internal details. |
| 0:34 | Read a web note after restart | After the local service restarts, the real command-line client opens the note created in the browser. A different user is still denied. |
| 0:42 | Use the command-line contract | The CLI explains its purpose, reports a typed JSON validation error, and refuses a test identity on a remote endpoint. |

## Source inventory


### [001-bff-template-foundation](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/001-bff-template-foundation/spec.md)

- User Story 1 - Fresh clone shows a working browser-to-BFF slice (Priority: P1) — Partial scene evidence at 0:07
- User Story 2 - Contract changes are safe and generated code cannot drift (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - Lifecycle hooks give fast, safe, actionable feedback (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - Reviewers trust exact-revision evidence and cannot be fooled by weakening (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 5 - A second project consumes the shared checks and upgrades (Priority: P3) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 6 - Optional cloud deployment profile with operator gates (Priority: P3) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 7 - Public-release review before publication (Priority: P3) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [002-hermetic-skeleton](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/002-hermetic-skeleton/spec.md)

- User Story 1 - One command produces a verified, pinned toolchain (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - The minimal verifier catches real defects with actionable output (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - Missing isolation is detected, never silently weakened (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - CI runs the same verifier at the exact revision (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 5 - Feedback cost is measured, not asserted (Priority: P3) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [003-notes-contract](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/003-notes-contract/spec.md)

- User Story 1 - One schema produces both typed clients (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - A breaking change cannot pass silently (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - Style and validation are declared once (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [004-go-bff](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/004-go-bff/spec.md)

- User Story 1 - A caller stores and reads its own notes (Priority: P1) — Partial scene evidence at 0:07, 0:14
- User Story 2 - Every failure is typed and leaks nothing (Priority: P1) — Partial scene evidence at 0:20, 0:00
- User Story 3 - Time limits reach storage (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - Identity fails closed (Priority: P1) — Partial scene evidence at 0:20
- User Story 5 - Browser access is limited to one origin (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [005-browser-client](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/005-browser-client/spec.md)

- User Story 1 - A person creates and opens a note in a browser (Priority: P1) — Partial scene evidence at 0:07, 0:14
- User Story 2 - Each failure reads as what happened (Priority: P1) — Partial scene evidence at 0:00, 0:20
- User Story 3 - The browser boundary holds (Priority: P1) — Partial scene evidence at 0:07
- User Story 4 - One command runs the app (Priority: P2) — Partial scene evidence at 0:07

### [006-quality-hooks](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/006-quality-hooks/spec.md)

- User Story 1 - One rule, one definition (P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - Fast, faithful commit feedback (P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - Push checks the commit being pushed (P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - Bounded, owned execution (P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 5 - Installation is safe and repeatable (P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 6 - Go analyzers run (P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [007-ci-parity-evidence](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/007-ci-parity-evidence/spec.md)

- US1 (P1): One registry governs local and CI. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- US2 (P1): A proposal cannot authorize its own weakening. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- US3 (P1): The aggregate cannot go green on missing work. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- US4 (P2): Coverage is real and owned. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- US5 (P2): Claims stay separate. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [008-second-consumer](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/008-second-consumer/spec.md)

- US1 (P1): A separate project adopts the checks by pin. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- US2 (P1): An upgrade is one pin and a reviewable diff. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- US3 (P2): The base contract holds for a consumer. — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [009-bff-image](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/009-bff-image/spec.md)

- User Story 1 - An adopter gets the production server as one image from the build graph (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - The image behaves as the server does today (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - One builder, stated everywhere (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - Build once, serve the browser and Connect together (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [010-release-readiness](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/010-release-readiness/spec.md)

- User Story 1 - A reviewer can see where everything came from (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - Nothing private ships (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - A new repository can be provisioned from a reviewed plan (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - The first hosted run is planned (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 5 - Public development begins on the reviewed core (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 6 - The image and the artifacts are distributed from the public repository (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [011-quality-baseline](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/011-quality-baseline/spec.md)

- User Story 1 - A reviewer sees every baseline control with its real state (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - A measured budget cannot be met by weakening (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - A second implementer can take one slice without touching another's files (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - Behavior is driven by typed configuration, with defaults in one place (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [012-hosted-example](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/012-hosted-example/spec.md)

- User Story 1 - An operator can stand up the example from a reviewed plan (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 2 - A release can be rolled back and a deployment cancelled (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 3 - Costs and authority are bounded (Priority: P2) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 4 - A real identity, a real store, a real port (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
- User Story 5 - Anyone can bring the whole stack up locally and demo it (Priority: P1) — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.

### [013-reference-clients](https://github.com/selamy-labs/bazel-protobuf-bff-template/blob/cf5f070c1fc364d12a759812a5ff702ec506fadc/specs/013-reference-clients/spec.md)

- US1 (P1 — `#29`): CLI and Web Share Notes Round-Trip, Persistence, and Refusal — Partial scene evidence at 0:26, 0:32
- US2 (P2 — `#34`): Shared Textual TUI Reference — Partial scene evidence at 0:39
- US3 (P2 — `#30`, `#31`, `#32`): Native Mobile and Desktop Reference Clients — Outside this user walkthrough; use the spec acceptance matrix or open platform issue.
