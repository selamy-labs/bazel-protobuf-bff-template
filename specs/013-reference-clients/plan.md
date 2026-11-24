# Implementation Plan: Typed Reference Clients (`013-reference-clients`, `#28`)

**Spec**: [spec.md](spec.md) · **Foundation Milestone**: `M12` ([001](../001-bff-template-foundation/spec.md))

## Architecture Decisions

1. **Hermetic Python toolchain via Bzlmod `rules_python` (`2.4.6`)**:
   - Pin Bzlmod `rules_python 2.4.6` (using Bazel's launcher-maker toolchain so zero host C++ toolchain is required on Linux/macOS) with CPython `3.12` (matching `ruff.toml` `py312`) and a shared hash-locked `python/requirements.lock` (`absl-py==2.5.1`, `protobuf==7.36.2`) so `#29` CLI and `#34` Textual TUI share one toolchain and dependency lock.
   - Inventory `python/requirements.lock` in `tools/release/inventory.sh` and `docs/provenance.md` and enroll it in `quality/registry.textproto` `escalate`.
2. **Zero-C++ build-time Python Protobuf + Connect codegen (`//api:notes_py`)**:
   - Pin official prebuilt static `protoc` `v36.2` (matching PyPI `protobuf==7.36.2`) by per-platform SHA-256 (`linux_x86_64`, `linux_aarch64`, `darwin_arm64`), consistent with `specs/003-notes-contract/research.md` R1 (no host C++ toolchain required).
   - Provide a small hermetic `protoc` plugin (`//tools/proto:protoc_gen_connect_py`) generating `template/notes/v1/notes_connect.py` (`NoteServiceClient`, `ConnectError`, `FieldViolation`) alongside `protoc --python_out` (`notes_pb2.py`, `validate_pb2.py`) in `//api:notes_py`.
   - Generated clients speak binary Protobuf over Connect HTTP/1.1 (`Content-Type: application/proto`, `Connect-Protocol-Version: 1`, `Connect-Timeout-Ms`), decode `buf.validate.Violations` on structured Connect JSON error responses, and fail closed if `test_subject` targets a non-loopback host.
3. **Thin `absl-py` CLI (`//clients/python/notes_cli`)**:
   - Exposes `create` and `get` commands with typed flags (`--endpoint`, `--subject`, `--bearer_token`, `--timeout_seconds`, `--title`, `--body`, `--name`, `--output`).
   - Applies portable CLI lessons tracked in `#21`: explicit endpoint, bounded deadline, structured typed error reporting, loopback-only dev identity, and zero duplicated domain rules.
4. **Cross-client verification without web production drift**:
   - `//clients/python/notes_cli:cli_test` (`build-test` leg) tests help, invalid invocations, create/get, `bff-dev` restart persistence, cross-subject `not_found`, server-side `invalid_argument` violations, `unauthenticated`, and `unavailable`.
   - `//web/e2e:cli_web_roundtrip_e2e_test` (`e2e` leg) launches the real `//tools/dev` stack and verifies bidirectional CLI↔Playwright browser create/read-by-ID, full stack restart persistence, and cross-subject refusal in both clients.
   - Because `web/app/` production UI is unchanged in `#29`, existing `#27` `web_video_evidence.sh` continues qualifying the web flow without redundant UI churn.
5. **Shared Textual TUI reference (`#34`)**:
   - Pin `textual==8.2.8` (MIT, pure-Python wheel graph with zero C extensions) in `python/requirements.{in,lock}` and `docs/provenance.md` under the shared `rules_python` `3.12` toolchain.
   - Add `//clients/python/notes_tui` (`notes_tui_app.py`, `main.py`, `tui_test.py`, `README.md`) as a separate interactive entrypoint alongside `//clients/python/notes_cli`, reusing `//api:notes_py` (`NoteServiceClient`, `ConnectError`, `FieldViolation`) with fail-closed flag validation before terminal launch.
   - Run `//clients/python/notes_tui:tui_test` headlessly in the `build-test` CI leg via Textual Pilot, verifying keyboard navigation (`Enter`, `Tab`, `Ctrl+L`, `Ctrl+R`, `Escape`), deterministic `loading -> success` and typed error transitions against a gated fake, and live `//bff/cmd/bff-dev` read, cross-subject `not_found`, `invalid_argument` violation, and `unauthenticated` paths.
   - Downstream BFF apps that need an interactive terminal UI reuse this Textual pin and generated client pattern; native noninteractive CLI entrypoints continue to use their language's toolchain.
6. **Follow-on native platform slices (`#30`, `#31`, `#32`)**:
   - `#30` / `#31` / `#32`: Keep Android, iOS, and desktop (Tauri 2 evaluated first) in isolated opt-in packages so base CI never depends on mobile or desktop SDKs.
