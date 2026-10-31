# Research: Neutral Notes Contract and Build-Time Generation (M2)

**Date**: 2026-10-31. Every pin is a release at least 14 days old on this date.

## R1 Generate with buf, not protoc rules

**Decision**: a small rule, `buf_generate` in `tools/proto/defs.bzl`, runs the pinned buf binary with
one plugin. rules_buf is not adopted. This revises parent T018 and T022.

**Why**: `go_proto_library` needs a protoc toolchain. Without a prebuilt protoc setup that means
compiling protoc with a host C++ toolchain, and the supported host has only Git, bash, curl and
coreutils. buf is a single static binary and already compiles schemas. It also serves format, lint
and breaking, so one pinned tool replaces two.

**Spike result**: the generation targets built under the verifier's flags on the authoring host. No
C++ action ran.

**Alternatives**: rules_buf adds gazelle and registry integration that this slice does not use, and
its breaking rule takes an image rather than a Git object.

## R2 The one schema import

`buf/validate/validate.proto` is fetched by `http_file` at protovalidate v1.2.2 (commit
`4baed765070e580226a0a30bf2a357cc027dc1f7`), checked by sha256. Actions copy it beside the schema, so
they need no registry. The Go runtime module uses its own later generated copy; the schema uses only
long-stable rules, so this skew has no effect here. It is a limit to recheck on upgrade.

`notes_pb.ts` is generated with imports, so the browser can decode `buf.validate.Violations` in M4
without a separate package.

## R3 Pins and supply chain

| Item | Pin | Notes |
| --- | --- | --- |
| buf | 1.74.0 | 1.75.0 is 9 days old |
| connect-go | v2.0.1 | v2.0.2 is 10 days old. Spike T020 passed on v2, so no v1 fallback is needed. |
| protobuf-go | 1.36.12 | |
| protovalidate-go | 1.4.0 | needs a `gazelle_override` on cel-go, whose shipped BUILD files use a different naming convention |
| gazelle | 0.54.0 | used only for `go_deps` |
| rules_js / rules_ts | 3.5.1 / 3.10.1 | 3.5.2 is 9 days old |
| rules_nodejs | 6.7.6 | 6.7.7 is 10 days old |
| Node | 24.22.0 | LTS line; sha256 per platform in `MODULE.bazel` |
| pnpm | 10.34.6 | the version rules_js pins |
| TypeScript | 6.0.3 | rules_ts 3.10.1 knows it. 7.0.2 is not evaluated. |
| protobuf-es, protoc-gen-es | 2.16.0 | |
| connect-es | 2.2.0 | 2.3.0 is 4 days old |

`pnpm-workspace.yaml` sets `minimumReleaseAge` to 14 days, which applies the pin policy to transitive
packages, and allows no install scripts.

**Telemetry**: rules_js depends on `aspect_tools_telemetry`, whose repository rule runs curl against
an external endpoint during fetch. `.bazelrc` sets `common --repo_env=DO_NOT_TRACK=1`, the documented
opt-out, and `tools/policy/bazelrc.allow` keeps that line under review.

## R4 Layout and contract corrections

- The schema lives at `api/template/notes/v1/`, not `api/notes/v1/`: STANDARD lint requires the path
  to match package `template.notes.v1`, and an exception would be worse than a longer path.
- Responses are `CreateNoteResponse` and `GetNoteResponse`, each holding a `Note`, for the same
  reason. A wrapper also leaves room to add response fields.
- The Go module path is `example.com/bfftemplate`: neutral, and an adopter renames it once.
- `quality/go.mod` keeps the quality module out of the root Go module.

## R5 Validation

Constraints use protovalidate. Title limits use CEL with `trim()`, so whitespace does not count.
Browser pre-validation is out of scope, so the Go/ES CEL parity question in parent R-04 does not
arise: the server is the single evaluator.

## R6 Baseline

`api/BASELINE` holds a full commit hash or `bootstrap`. The reader requires the commit to exist, be
an ancestor of HEAD and contain a schema, and reads blobs with `git cat-file`. `bootstrap` is valid
only in the commit that introduces the schema: if any parent has a schema, even a since-deleted one,
it is refused. A shallow clone fails closed, so CI uses `fetch-depth: 0`.

**Copies**: a copy with new history inherits a hash it does not contain, so it fails closed. If its
first commit already has the schema, the second commit declares the first commit's hash. If it adds
its schema later, it uses `bootstrap` in that commit. `api/README.md` lists every route, and
`TestFreshHistoryCopy` witnesses them. M7's second consumer is the first real use.

**Limits**: rewriting history invalidates the hash, which fails closed and needs a reviewed update.
A proposal can edit `api/BASELINE` to a later ancestor; catching that belongs to M6 and #26.
