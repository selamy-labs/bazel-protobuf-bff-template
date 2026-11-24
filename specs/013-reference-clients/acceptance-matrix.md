# Acceptance / Evidence Matrix: `013-reference-clients` (`#28`)

Classes: **G** deterministic guarantee, **S** review signal, **R** runtime/platform readback.
Layers: **off** offline test, **loc** local verifier, **ci** hosted CI, **hum** human.

| Req | Witness | Valid | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- | --- |
| FR-001 | `//:inventory_test`, `//:bzlmod_policy_test` | `rules_python`, `protoc`, `absl-py`, `protobuf` pinned by hash and inventoried with zero unresolved licenses | unlisted `bazel_dep` or `python/requirements.lock` package → inventory fails | G | off, loc, ci |
| FR-002 | `//clients/python/notes_cli:cli_test`, `//:bzlmod_policy_test` | `//api:notes_py` generated at build time from `notes.proto` via prebuilt `protoc` + `protoc_gen_connect_py`; valid `200 application/proto` decodes cleanly | committed generated file → `bzlmod_policy_test` fails; 3xx redirect → refused without forwarding credentials; non-loopback `http://` bearer → exit `2`; non-200 / wrong media type / malformed wire bytes → typed `ConnectError` exit `1` | G | off, loc, ci |
| FR-003 | `//clients/python/notes_cli:cli_test` | `--help` prints flags/usage (`absl.app` exit `1`); valid `create`/`get` succeeds with `text` and `json` output | missing/unknown command, missing `--endpoint`, missing `--name`, non-loopback `--subject`, conflicting auth flags, `timeout_seconds <= 0` → exit `2` | G | loc, ci |
| FR-004 | `//clients/python/notes_cli:cli_test`, `//web/e2e:cli_web_roundtrip_e2e_test` | CLI→web and web→CLI create/get by ID on `//tools/dev` (`--store=local`), persistence after full stack restart | cross-subject `bob` reading `ana`'s note → typed `not_found` in CLI and web; blank title → `invalid_argument` with `note.title.required`; no auth → `unauthenticated`; closed port → `unavailable` | G | loc, ci |
| FR-005 | `//clients/python/notes_tui:tui_test`, `//:inventory_test` | `textual==8.2.8` pinned in `python/requirements.lock` and inventoried; separate `//clients/python/notes_tui` entrypoint passes headless Pilot keyboard navigation (`Enter`/`Tab`/`Ctrl+L`/`Ctrl+R`/`Escape`), `loading -> success`, live `bff-dev` read, and SVG terminal capture | missing/invalid flags → exit `2` before TUI launch; `not_found`/`invalid_argument`/`unauthenticated`/`unavailable` → stale note cleared, typed code/violations and actionable hint rendered | G / R | loc, ci |
| FR-006 | `#30`–`#32` (pending) | opt-in native emulator/simulator/desktop proofs | pending `#30`–`#32` | R | ci, hum |
