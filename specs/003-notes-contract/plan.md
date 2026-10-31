# Implementation Plan: Neutral Notes Contract and Build-Time Generation (M2)

**Branch**: `003-notes-contract` | **Date**: 2026-10-31 | **Spec**: [spec.md](spec.md)

## Summary

One schema, `api/template/notes/v1/notes.proto`, defines `CreateNote` and `GetNote`. A small Starlark
rule runs the pinned `buf generate` with one pinned plugin per output, so Go and TypeScript code
exist only in the build graph. Schema checks run through `tools/checks/native.sh` and are witnessed
by probes on the real schema. The compatibility baseline is read from Git objects by a Go tool in
the quality module.

## Technical Context

| Item | Choice |
| --- | --- |
| Languages | Protobuf (proto3), Go 1.27.2, TypeScript 6.0.3 on Node 24.22.0, Starlark, bash |
| New tools | buf 1.74.0, protoc-gen-go and connect-go 2.0.1 from `go.mod`, protoc-gen-es 2.16.0 from `pnpm-lock.yaml`, gazelle 0.54.0, rules_js 3.5.1, rules_ts 3.10.1, rules_nodejs 6.7.6 |
| Testing | `go_test`, `js_test` with the real compiler, `sh_test` probes in a real Git repository |
| Constraints | No host compiler, no schema registry, no network inside actions |

## Constitution Check

| Principle | Status | Notes |
| --- | --- | --- |
| I. No vacuous surface | PASS | Connect-Web and browser packages wait for M4. The Go client exists only because the plugin emits it and M3 consumes the handler. |
| II. Contracts first | PASS | This milestone. |
| III. Hermetic, pinned | PASS | Tools pinned by sha256 or lockfile integrity; `DO_NOT_TRACK=1` stops the reporter rules_js pulls in. |
| IV. One policy | PASS | One `buf.yaml` in the quality module; a nearby config is neither read nor allowed. |
| V. Cannot weaken itself | PASS (scoped) | Comment and config exclusions are inert. Editing the canonical file with its verifier still needs M6 base-built evaluation. |
| VI. Exact-revision evidence | PASS | Four more required tests and three more expected checks. |
| VII. Guarantees vs signals | PASS | Every check here is a guarantee. The API-style ledger is a human review signal. |
| VIII. Bounded, actionable | PASS | The new checks run under `run_check`, and each failure names its rule. |
| IX. Public, neutral | PASS | Notes domain; `example.com` module path. |
| X. Smallest route | PASS | No rules_buf, protobuf compiler module or generator. See research R1. |

## Project Structure

```text
api/
├── BASELINE                      # bootstrap marker, then a full commit hash
├── BUILD.bazel                   # generation, go_library, ts_project, tests
├── template/notes/v1/notes.proto
├── tools.go                      # records what generated code imports, for go mod tidy
└── testdata/{breaking/probes.tsv,negative/}
quality/proto/                    # buf.yaml, baseline reader and its tests
tools/proto/                      # buf_generate rule, probes test
docs/api-style.md                 # API-style ledger
```

## Complexity Tracking

None.
