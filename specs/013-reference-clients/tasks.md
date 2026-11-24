# Tasks: Typed Reference Clients (`013-reference-clients`, Epic `#28`)

## Phase 1: Python Toolchain, Generated Bindings & Thin CLI (`#29`)

- [x] T001 Pin Bzlmod `rules_python` (`2.4.6`), Python `3.12` toolchain, shared `python/requirements.{in,lock}` (`absl-py==2.5.1`, `protobuf==7.36.2`), and prebuilt per-platform `protoc` `v36.2` in `MODULE.bazel`, `python/`, `docs/provenance.md`, `tools/release/inventory.sh`, and `quality/registry.textproto`. **Witness**: `//:inventory_test`, `//:bzlmod_policy_test`.
- [x] T002 Add build-time Python Protobuf/Connect codegen (`tools/proto/protoc_gen_connect_py.py`, `tools/proto/defs.bzl`, `tools/proto/BUILD.bazel`, `api/BUILD.bazel` `//api:notes_py`) with zero committed generated files and zero host C++ dependency. **Witness**: `//clients/python/notes_cli:cli_test`, `//:bzlmod_policy_test`.
- [x] T003 Implement thin `absl-py` Notes CLI (`clients/python/notes_cli/{main.py,BUILD.bazel,README.md}`) with configurable endpoint, bounded timeout, loopback-only `--subject`, `--bearer_token`, and typed `ConnectError` / `buf.validate.Violations` output. **Witness**: `//clients/python/notes_cli:cli_test`.
- [x] T004 Add live `bff-dev` CLI integration + negative controls (`//clients/python/notes_cli:cli_test`) and bidirectional CLI↔web E2E round-trip + restart persistence + cross-subject refusal (`//web/e2e:cli_web_roundtrip_e2e_test`), enrolled in `tools/checks/required_tests.txt`. **Witness**: `//clients/python/notes_cli:cli_test`, `//web/e2e:cli_web_roundtrip_e2e_test`.
- [x] T005 Link `013-reference-clients` (`M12`) from `specs/001-bff-template-foundation/spec.md` and `specs/README.md`, referencing `#27` for web video evidence, `#21` for portable CLI lessons, and `#20` for hosted release; capture exact-revision CLI transcript (help, invalid invocation, round-trip, restart, refusal). **Witness**: PR evidence + hosted CI `4/4` green.

## Phase 2: Shared Textual TUI Reference (`#34`, depends on `#29`)

- [x] T006 Pin `textual==8.2.8` and its pure-Python transitive graph in `python/requirements.{in,lock}` and `docs/provenance.md` under the shared `rules_python` toolchain. **Witness**: `//:inventory_test`, `//:bzlmod_policy_test`.
- [x] T007 Add interactive Textual TUI entrypoint (`//clients/python/notes_tui`) reusing `//api:notes_py` with keyboard-driven `GetNote` (loading, success, actionable typed RPC error), headless Textual Pilot + live `bff-dev` tests (`//clients/python/notes_tui:tui_test` enrolled in `tools/checks/required_tests.txt`), terminal capture, real-service transcript, and adoption docs (`clients/python/notes_tui/README.md`, `README.md`, `specs/013-reference-clients/`). **Witness**: `//clients/python/notes_tui:tui_test`.

## Phase 3: Optional Native Mobile & Desktop Clients (`#30`, `#31`, `#32`)

- [ ] T008 (`#30`) Add opt-in Kotlin/Compose Android Notes client over generated Connect bindings with real emulator evidence.
- [ ] T009 (`#31`) Add opt-in SwiftUI iOS Notes client over generated Connect bindings with real simulator evidence.
- [ ] T010 (`#32`) Evaluate Tauri 2 first and qualify one shared desktop Notes shell reusing the web UI/typed client on Windows, macOS, and Linux (signing/distribution out of scope).
