# Research: Hosted example and local demo

Each entry is answered by a probe, an evaluation against the real server, or an independent run at a named
revision; none by a product's name.

## R1 The output contract with the infrastructure owner

- Two owners, two repositories, no shared state: the infrastructure owner composes the project, bootstrap, billing, APIs, IAM and state and publishes a versioned output contract; this repository consumes it as typed inputs (`deploy/gcp/inputs/contract.schema.json`: project id, region, runtime identity, build identity, state bucket, artifact registry, contract version). A missing or newer-than-supported contract version is refused by validation. The agreed task ids on the infrastructure side are recorded here when settled; until then T002 is open.

## R2 Issuer and verification library (probe at T003)

- The server's `identity.Verifier` takes a bearer token and returns an opaque subject; production composes it through `newVerifier`, which is nil today, so production refuses to start, and that refusal is what spec 009's image witnesses. The probe chooses a supported token issuer and a maintained verification library for it, pins the library by go.sum after the probe, and writes the negative tests with synthetic keys: wrong issuer, wrong audience, expired, bad signature. The development identity is never composed into the production binary (`//bff/cmd:binaries_test`).

## R3 Store (probe at T004 and T019)

- `ports.Store` has `Put` and `Get` by owner and id with `ErrNotFound` and `ErrUnavailable`; the memory adapter is bounded. The local demo takes a file-backed adapter under the same contract suite with stated persistence, restart and reset semantics; the hosted example takes the simplest managed store with owner-scoped keys, under the same suite, with a read-after-restart test and an owner-isolation test. A one-instance limit is never a durability claim.

## R4 Cloud Run against the server (evaluation, T005)

From the pinned source and the service's published container contract; nothing was run against the service, and no resource exists.

| Requirement of the server | The server today | Cloud Run's contract | What the example adds | Gap or caveat |
| --- | --- | --- | --- | --- |
| Listen address | `BFF_ADDR` defaults to `:8080`; `PORT` is not read; dev mode is loopback only | the container listens on `0.0.0.0:$PORT`, linux/amd64 | FR-018: production reads `PORT`, binds `0.0.0.0`; dev stays loopback; a non-default port tested | none once T009 lands |
| Image | one static linux/amd64 image, no base, identified by content digest (spec 009) | an image from a registry the service can read | the image promoted by digest into the contract's registry; deployment by that digest only | the digest check in CD (FR-001); nothing rebuilds |
| Startup | refuses to start without a verifier (`newVerifier` nil) | a container that exits is a failed revision; the previous revision keeps serving | FR-015: a real verifier composed through `newVerifier`; the refusal stays for a missing one | the example's first deployment cannot be green before T007 |
| Identity | bearer tokens through `identity.Verifier`; no provider shipped | the service can require a platform identity for ingress, or allow unauthenticated ingress and let the application verify | the application verifies the issuer's tokens (issuer, audience, keys as inputs); ingress stays open to the browser | the issuer is an operator input, not a decision made here |
| Origin | one exact origin, no path, no credentials | nothing; CORS is the application's | `runtime.allowed_origin` is the Pages host origin; the base path is separate (FR-019) | none |
| Persistence | in-memory store, bounded, per process | instances start, stop and scale; nothing on an instance survives | FR-017: a managed store behind `ports.Store`, under the contract suite, with the persistence statement in the inputs | the store's choice is T004's probe; a one-instance limit is never durability |
| Shutdown | bounded shutdown on SIGTERM (`bff/server`) | the service sends SIGTERM and allows a bounded grace period | nothing new; the bound is measured in the live record | none |
| Health | no health endpoint; the ready event on stdout | the service considers a container healthy when it accepts connections on the port | nothing new; process survival is not health (A05) | a request-level check in the journey, not a claim of availability |
| Logs | JSON logs on stderr; secrets redacted by test | stdout and stderr are collected | nothing new; redaction verified in the journey (FR-019) | none |
| Scale and cost | n/a | instance bounds, concurrency and request timeout per revision; billing per use | `bounds` inputs enumerated by a test; a budget alert, never a cap (FR-010) | the alert is not a monetary cap, and the evidence says so |
| Rollback and cancel | n/a | traffic can target a previous revision; a deployment can be cancelled | `release.serving_digest` and `cancel_in_flight` (FR-009) | witnessed only live |

Conclusion for the plan: Cloud Run fits the server's contract once the three application gaps (verifier, store,
`PORT`) are closed by the adapters the spec already requires; nothing about the service weakens the server's
refusals or its exact origin. The decision stays provisional until the adapters exist and an authorized live run
records the endpoint, the identity, the digest and the flow.

## R5 Pages base path and the browser origin (probe at T006)

- A Pages project site serves under a path while the browser origin is the host alone; CORS here allows one exact origin with no path, and that stays. The frontend base path and the RPC base URL are independent inputs; the journey tests the preflight, a blocked foreign origin, an authenticated call, error and status accessibility, reload and deep link, logout and cancellation, and log redaction.

## R6 The local launcher on a host with bash 3.2 (independent Mac probe, 2026-11-01)

- An independent probe on macOS ARM at `34e1bbc` found `/bin/bash` 3.2.57, which refuses `wait -n`, the call the launcher ended with; no GNU `timeout` or `flock` on PATH either. The launcher now needs bash 3.2 and POSIX tools only: the first child's exit is detected by a `kill -0` poll with the same semantics, both children are stopped by the same trap, and a busy or invalid port is refused by name before anything starts, through bash's own `/dev/tcp` connect. The demo's host requirements are stated apart from the full verifier's. Native macOS is recorded only from an independent native run; Linux end-to-end coverage proves nothing about it.

## R7 Well-Architected

- The framework's pillars are rows in the acceptance matrix per component, each a control with its witness or a named gap with owner, command and revision; live IAM, recovery, availability and cost are proved only by live evidence behind the recorded gates, never by a document or a CI run.
