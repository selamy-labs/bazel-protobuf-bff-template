# Implementation Plan: Go BFF for Notes (M3)

**Branch**: `004-go-bff` | **Date**: 2026-10-31 | **Spec**: [spec.md](spec.md)

## Summary

A Go server implements the generated handler over an application layer and a storage port, with an
in-memory adapter. Interceptors authenticate, bound and mask each call. A production binary and a
`testonly` dev binary share one server package (research R1).

## Technical Context

| Item | Choice |
| --- | --- |
| Language | Go 1.27.2 |
| New dependencies | none beyond M2: connect-go, protovalidate, protobuf-go |
| Storage | in-memory adapter behind `ports.Store` |
| Testing | `go_test` over a real loopback server; `testing/quick` properties; `sh_test` on the real binaries; `genquery` for dependency rules |
| Constraints | stdin unused; bounded shutdown; no identity provider; no backend service |

## Constitution Check

| Principle | Status | Notes |
| --- | --- | --- |
| I. No vacuous surface | PASS | Both RPCs are served. The `Verifier` interface has one real consumer, `NewBearer`, and no shipped success stub. |
| II. Contracts first | PASS | Handlers are generated. |
| III. Hermetic, pinned | PASS | No new external dependency. |
| IV. One policy | PASS | `format-go` has one definition. `lint-go` moved to M5 (research R7). |
| V. Cannot weaken itself | PASS (scoped) | Dependency rules are repository files, so M6 limits still apply. |
| VI. Exact-revision evidence | PASS | New tests join the required inventory. |
| VII. Guarantees vs signals | PASS | All M3 checks are guarantees. The layer design beyond the listed edges is review. |
| VIII. Bounded, actionable | PASS | Deadlines, read limit, server timeouts, bounded shutdown. |
| IX. Public, neutral | PASS | Notes domain. No provider, account or identity value. |
| X. Smallest route | PASS | No CORS, logging or auth library. |
| Security boundaries | PASS | Test identity fenced; errors masked; deadlines propagate. |

## Project Structure

```text
bff/
├── identity/              # Principal, Authenticator, Verifier, bearer parsing
├── ports/                 # Store, Note, sentinel errors
├── adapters/memory/       # in-memory Store
├── app/                   # use cases, name parsing
├── transport/             # handlers, interceptors, CORS, error mapping
├── server/                # wiring, startup refusal, ready event, shutdown
├── cmd/bff/               # production binary
├── cmd/bff-dev/           # testonly dev binary
└── testing/{identity,faults}/  # testonly
tools/checks/dependency_rules.sh
```

## Complexity Tracking

None.
