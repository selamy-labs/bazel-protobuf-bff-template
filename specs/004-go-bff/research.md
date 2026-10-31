# Research: Go BFF for Notes (M3)

**Date**: 2026-10-31.

## R1 Two binaries, one server

**Decision**: `//bff/cmd/bff` is the production binary. `//bff/cmd/bff-dev` is a `testonly` binary
that adds test identity and fault injection. Both call the same `bff/server` package.

**Why**: FR-011 needs test identity out of the production dependency graph, and a mode flag in one
binary cannot do that. Bazel refuses a non-testonly target depending on a `testonly` one, so the
fence is enforced at analysis time.

**Consequence**: this repository ships no verifier (parent R-06), so the production binary as shipped
always refuses to start. The refusal is the intended behavior. An adopter assigns `newVerifier` in
`bff/cmd/bff/verifier.go`. The local demo and browser tests run the dev binary, so they exercise
the production server code, but not a production identity path. That is a stated limit.

## R2 connect-go v2 model

Generated handlers take and return plain messages. Server interceptors wrap the whole call and run
before payload work, so authentication happens before decompression and decoding. Three interceptors
run in this order: masking (outermost, also recovers panics), deadline, authentication.

The server applies `Connect-Timeout-Ms` to the context. The deadline interceptor adds the 2 s default
when no deadline exists and caps any deadline at 10 s.

## R3 Where validation runs

Parent T034 placed protovalidate in `bff/app`. It runs in `bff/transport` instead: it is
message-shaped, it needs Connect error details, and `app` stays free of generated types. `app` still
parses names and trims titles, because storage must never see an unparsed name.

## R4 Fault injection

`bff/testing/faults` decorates the store. A subject named `fault-unavailable`, `fault-hang`,
`fault-secret` or `fault-panic` triggers that fault. This needs no control port, keeps no shared
state, and lets parallel browser tests inject different faults. It exists only in the dev binary.

## R5 CORS

A short middleware echoes one exact origin and never sends `Access-Control-Allow-Credentials`. The
allowed headers are those Connect needs plus `Authorization`. No dependency is added.

## R6 Dependency rules

Bazel `visibility` and `testonly` are the first line. `genquery` output feeds
`tools/checks/dependency_rules.sh`, which fails on a forbidden edge. The script's defect cases run on
synthetic graphs, so the test proves it can fail.

## R7 Go quality checks

`gofmt` from the pinned SDK joins the verifier as `format-go`.

`nogo` is deferred to M5. Under rules_go 0.64.2 and Go 1.27.2 its analyzers fail with "cannot decode"
on the standard library's export data, before they reach this repository's code. rules_go 0.65.0 was
released on 2026-10-22, so it is under the 14-day pin rule on this date. **Vet does not run yet.**
M5 takes this with staticcheck, starting by rechecking 0.65.0.

## R9 Trust at the error boundary

Review of the first version found that `contractError` passed any `*connect.Error` with a
non-internal code, so a store or verifier could return private text under a public-looking code.
A regression over a real server confirmed it: 48 of 48 cases across both boundaries, direct, wrapped
and joined, leaked the planted secret in the message and in a detail.

The error's type cannot show who built it, so trust follows the boundary. Errors from `app` and the
authenticator are wrapped in `downstream` where they cross into transport. A downstream error can
map only through a sentinel, to constant text. Otherwise it is internal. Errors from transport and
the framework stay public, because they describe only what the caller sent.

## R8 Bounds

Reads are limited to 64 KiB. The HTTP server sets header, read, write and idle timeouts, and
shutdown waits at most 5 s. The memory store holds at most 10,000 notes and then reports
`unavailable`. The dev binary refuses a non-loopback address.

The server speaks HTTP/1.1. Parent `notes-api.md` also mentioned HTTP/2, which has no consumer here.
TLS and HTTP/2 belong to whatever fronts the binary in a deployment.
