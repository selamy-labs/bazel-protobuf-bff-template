# Notes Python CLI Reference Client (`#29`)

Thin Python reference CLI for `template.notes.v1.NoteService` built with `absl-py` and Bzlmod
`rules_python` over build-time generated Protobuf/Connect bindings (`//api:notes_py`).

All domain validation (`protovalidate`), persistence, and per-subject isolation stay authoritative
in the Go BFF (`//bff/cmd/bff-dev` locally, `//bff/cmd/bff` in production).

## Commands

Start the local development stack (same profile as `docs/local-demo.md`):

```sh
tools/bootstrap/bazel.sh run //tools/dev
```

Show CLI help:

```sh
tools/bootstrap/bazel.sh run //clients/python/notes_cli -- --help
```

Create a note as synthetic subject `ana`:

```sh
tools/bootstrap/bazel.sh run //clients/python/notes_cli -- \
  create \
  --endpoint=http://127.0.0.1:8080 \
  --subject=ana \
  --title="CLI note" \
  --body="Created from Python CLI."
```

Read a note by resource name (`notes/<id>`):

```sh
tools/bootstrap/bazel.sh run //clients/python/notes_cli -- \
  get \
  --endpoint=http://127.0.0.1:8080 \
  --subject=ana \
  --name=notes/<id>
```

Verify typed cross-subject refusal (`bob` reading `ana`'s note returns `not_found`, exit `1`):

```sh
tools/bootstrap/bazel.sh run //clients/python/notes_cli -- \
  get \
  --endpoint=http://127.0.0.1:8080 \
  --subject=bob \
  --name=notes/<id>
```

## Development Identity Boundary

`--subject` emits `Authorization: Bearer test-identity.<subject>` and is restricted fail-closed to
loopback endpoints (`127.0.0.1`, `localhost`, `::1`). Production BFF deployments refuse
`test-identity.*` tokens; use `--bearer_token` against authenticated endpoints (`https://` is
required for non-loopback `--bearer_token` endpoints, and the generated Connect client refuses HTTP
redirects so credentials never follow redirects across origins).

## Automated Verification

- Unit + live `bff-dev` CLI integration (help, invalid invocations, non-loopback plaintext bearer
  refusal, redirect refusal and credential confinement, Connect response envelope and malformed
  protobuf handling, failed-start cleanup safety, create/get round-trip, restart persistence,
  cross-subject `not_found`, server-side `invalid_argument` violations, `unauthenticated`,
  `unavailable`): `tools/bootstrap/bazel.sh test //clients/python/notes_cli:cli_test`
- Cross-client CLI ↔ Playwright browser round-trip, restart persistence, and cross-subject refusal:
  `tools/bootstrap/bazel.sh test //web/e2e:cli_web_roundtrip_e2e_test`
- Reusable web UI video evidence boundary (`#27`): `tools/checks/web_video_evidence.sh`
- Portable CLI lessons tracked against `#21` (`selamy-labs/bazel-protobuf-bff-template#21`):
  explicit `--endpoint`, bounded `--timeout_seconds`, typed `ConnectError` with decoded
  `buf.validate.Violations`, loopback-only `--subject`, zero duplicated business logic.
