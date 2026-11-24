# Notes Python Textual TUI Reference Client (`#34`)

Shared interactive terminal UI reference for `template.notes.v1.NoteService` built with
[Textual](https://github.com/Textualize/textual) (`textual==8.2.8`) under the repository's shared
Bzlmod `rules_python` toolchain and `python/requirements.lock`, calling the same build-time generated
Protobuf/Connect client (`//api:notes_py`) as `//clients/python/notes_cli`.

All domain validation (`protovalidate`), persistence, and per-subject isolation stay authoritative
in the Go BFF (`//bff/cmd/bff-dev` locally, `//bff/cmd/bff` in production).

## Usage

Start the local development stack (same profile as `docs/local-demo.md`):

```sh
tools/bootstrap/bazel.sh run //tools/dev
```

Create a synthetic note with the noninteractive CLI (`#29`) to obtain a `notes/<id>` resource name:

```sh
tools/bootstrap/bazel.sh run //clients/python/notes_cli -- \
  create \
  --endpoint=http://127.0.0.1:8080 \
  --subject=ana \
  --title="TUI note" \
  --body="Read from Textual TUI."
```

Launch the interactive TUI against the same service endpoint:

```sh
tools/bootstrap/bazel.sh run //clients/python/notes_tui -- \
  --endpoint=http://127.0.0.1:8080 \
  --subject=ana \
  --name=notes/<id>
```

### Keyboard Controls

- `Enter` (in the note ID input or on `Fetch`): call `GetNote` and render loading, success, or an
  actionable typed RPC error (with decoded `buf.validate.Violations` when returned by the server)
- `Tab` / `Shift+Tab`: move focus between the note ID input and `Fetch` button
- `Ctrl+L` (or `f` when input is unfocused): focus the note ID input
- `Ctrl+R` (or `r` when input is unfocused): re-fetch the current note ID
- `Escape`: unfocus the note ID input
- `Ctrl+Q` (or `q` when input is unfocused): quit

## Development Identity Boundary

`--subject` emits `Authorization: Bearer test-identity.<subject>` and is restricted fail-closed to
loopback endpoints (`127.0.0.1`, `localhost`, `::1`). Production BFF deployments refuse
`test-identity.*` tokens; use `--bearer_token` against `https://` endpoints. Invalid flag
combinations fail fast with exit code `2` before entering full-screen terminal mode.

## Adoption Pattern for BFF Apps

- BFF apps that need an interactive terminal UI reuse this Textual framework pin
  (`python/requirements.{in,lock}`), build-time generated `//api:notes_py` client, and separate
  interactive entrypoint pattern alongside their noninteractive CLI.
- Native noninteractive CLI entrypoints continue to use their own language's toolchain (`absl-py`
  for Python reference CLIs, Go/TypeScript/etc. where appropriate for downstream apps).

## Automated Verification

Run headless Textual Pilot tests (keyboard navigation, loading/success/typed-error fake controls,
live `//bff/cmd/bff-dev` read/refusal/validation integration, and CLI flag fail-closed checks)
under Bazel CI:

```sh
tools/bootstrap/bazel.sh test //clients/python/notes_tui:tui_test
```
