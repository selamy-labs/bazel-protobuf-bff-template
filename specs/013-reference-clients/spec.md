# Feature Specification: Typed Reference Clients (CLI, Web, TUI, Mobile, Desktop)

**Feature Branch**: `013-reference-clients`

**Created**: 2026-11-23

**Status**: In progress (`#29` CLI + web round-trip merged; `#34` Textual TUI active; `#30`, `#31`, `#32` planned). Traces to [001-bff-template-foundation](../001-bff-template-foundation/spec.md) (Milestone `M12`), Epic `#28`.

## Scope and Boundaries

Make this repository a neutral starting point for thin, typed reference clients over the existing
`template.notes.v1.NoteService` Protobuf/Connect contract (`api/template/notes/v1/notes.proto`):

- **Base loop (always required, zero optional platform SDKs)**:
  - Browser web UI (`//web/app`, spec `005-browser-client`)
  - Thin Python CLI (`//clients/python/notes_cli`, `#29`) using `absl-py` and Bzlmod `rules_python`
  - Shared Python Textual TUI (`#34`, reusing the same Bzlmod `rules_python` toolchain/lock and generated `//api:notes_py` client)
- **Optional native platform reference clients (opt-in SDKs, never required by the base loop)**:
  - Native Android Kotlin/Compose (`#30`) with emulator evidence
  - Native iOS SwiftUI (`#31`) with simulator evidence
  - Shared desktop shell starting with Tauri 2 (`#32`) qualified on Windows, macOS, and Linux
- **Authoritative boundaries**:
  - The Go BFF (`//bff`) owns all domain validation (`protovalidate`), persistence, and per-subject isolation. Clients remain thin transport/presentation layers.
  - Development identity (`Bearer test-identity.<subject>`) is restricted fail-closed to loopback endpoints (`127.0.0.1`, `localhost`, `::1`). Hosted release and production identity remain owned by `#20` (`specs/012-hosted-example`).
  - Affected web UI flows reuse `#27` (`tools/checks/web_video_evidence.sh`). Portable CLI ergonomics lessons are traced to `#21`.

## User Scenarios & Testing

### US1 (P1 — `#29`): CLI and Web Share Notes Round-Trip, Persistence, and Refusal

1. **Help & invalid invocation**: Running `//clients/python/notes_cli -- --help` prints flags and usage (`absl.app` standard exit `1`); missing/unknown commands, missing `--endpoint`, missing `--name`, non-loopback `--subject`, conflicting auth flags, or non-positive `--timeout_seconds` fail with exit `2` and actionable stderr diagnostics.
2. **Cross-client round-trip**: Creating a note via the Python CLI as synthetic subject `ana` returns `notes/<id>` that opens in the existing web UI as `ana` with identical title and body; creating a note in the web UI as `ana` returns `notes/<id2>` readable by the Python CLI as `ana`.
3. **Restart persistence**: Stopping and restarting `//tools/dev` (`--store=local`) preserves both notes across both CLI and web clients.
4. **Typed cross-subject refusal & server errors**: Synthetic subject `bob` attempting to read `ana`'s note receives typed `not_found` in both CLI and web; invalid note input surfaces server-emitted `invalid_argument` with decoded `buf.validate.Violations`; unauthenticated calls return `unauthenticated`; unreachable endpoints return `unavailable` within bounded deadlines.

### US2 (P2 — `#34`): Shared Textual TUI Reference

1. **Shared toolchain & separate interactive entrypoint**: Pins `textual==8.2.8` in the same Bzlmod `rules_python` toolchain and `python/requirements.lock` as `#29`, exposing `//clients/python/notes_tui` separately from the noninteractive `//clients/python/notes_cli` entrypoint over the same build-time generated `//api:notes_py` client and configurable `--endpoint` / auth flags.
2. **Keyboard-driven `GetNote` lifecycle**: Pressing `Enter` in the note ID input (or navigating via `Tab`, `Ctrl+L`, `Ctrl+R`, `Escape`) transitions visibly from `idle` -> `loading` -> `success` (rendering `name`, `title`, `body`, `create_time`) or actionable typed `ConnectError` (clearing any stale note display, surfacing code, message, decoded `buf.validate.Violations`, and next-step hint). Domain validation, persistence, and subject isolation stay authoritative in the Go BFF.
3. **Headless Pilot verification & downstream adoption**: `//clients/python/notes_tui:tui_test` runs Textual Pilot tests headlessly under Bazel CI against both a realistic gated fake and live `//bff/cmd/bff-dev`. Downstream BFF apps needing an interactive terminal UI reuse this Textual framework and `//api:notes_py` client pattern, while native noninteractive CLI entrypoints continue to use their language's own toolchain.

### US3 (P2 — `#30`, `#31`, `#32`): Native Mobile and Desktop Reference Clients

Adds thin opt-in Android (Kotlin/Compose), iOS (SwiftUI), and Desktop (Tauri 2 evaluated first) clients over the same Notes contract without coupling the base web/CLI CI loop to optional platform SDKs.

## Functional Requirements

- **FR-001 (`#29`)**: The repository MUST provide a pinned Bzlmod `rules_python` toolchain (`3.12`) and hash-locked dependency file (`python/requirements.lock`) inventoried in `docs/provenance.md` with zero unresolved licenses.
- **FR-002 (`#29`)**: Python Protobuf message and Connect unary RPC client bindings (`//api:notes_py`) MUST be generated at Bazel build time from `api/template/notes/v1/notes.proto` without requiring a host C++ compiler and without committing generated files. The generated unary transport MUST refuse HTTP redirects, require `https://` when sending `--bearer_token` to non-loopback hosts, require HTTP `200` with `Content-Type: application/proto` on unary success responses, and surface protobuf decode failures through the typed `ConnectError` path.
- **FR-003 (`#29`)**: `//clients/python/notes_cli` MUST use `absl-py` for its entrypoint and typed flags (`--endpoint`, `--subject`, `--bearer_token`, `--timeout_seconds`, `--title`, `--body`, `--name`, `--output`), keep domain validation authoritative in the BFF, and restrict `--subject` (`test-identity.*`) fail-closed to loopback endpoints.
- **FR-004 (`#29`)**: Automated Bazel tests (`//clients/python/notes_cli:cli_test`, `//web/e2e:cli_web_roundtrip_e2e_test`) MUST prove help, invalid invocation refusal, bidirectional CLI↔web create/get by ID, persistence across server restart, typed cross-subject `not_found` refusal, server-side validation violation decoding, `unauthenticated`, and `unavailable` errors on the safe development profile.
- **FR-005 (`#34`)**: The shared Textual TUI MUST reuse `python/requirements.lock` and `//api:notes_py` in a separate interactive entrypoint verified by headless Textual Pilot tests under Bazel CI.
- **FR-006 (`#30`–`#32`)**: Optional mobile and desktop reference clients MUST keep their SDK toolchains opt-in and record exact platform blockers whenever native validation cannot execute on a host.

## Success Criteria

- **SC-001 (`#29`)**: `//clients/python/notes_cli:cli_test` and `//web/e2e:cli_web_roundtrip_e2e_test` pass inside the existing `build-test` and `e2e` CI legs alongside an exact-revision CLI transcript covering help, invalid invocation, round-trip, restart persistence, and cross-subject refusal.
- **SC-002 (`#34`)**: Headless Textual Pilot tests pass under Bazel CI with terminal capture and real-service transcript.
- **SC-003 (`#30`–`#32`)**: Each platform slice carries real emulator/simulator/OS launch proof on its target platforms without expanding base-loop prerequisites.
