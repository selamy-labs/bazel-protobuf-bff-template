# Research: Hosted example and local demo

Each entry is answered by a probe, an evaluation against the real server, or an independent run at a named
revision; none by a product's name.

## R1 The output contract with the infrastructure owner

- Two owners, two repositories, no shared state: the infrastructure owner composes the project, bootstrap, billing, APIs, IAM and state and publishes a versioned output contract; this repository consumes it as typed inputs (`deploy/gcp/inputs/contract.schema.json`: project id, region, runtime identity, build identity, state bucket, artifact registry, contract version). A missing or newer-than-supported contract version is refused by validation. The agreed task ids on the infrastructure side are recorded here when settled; until then T002 is open.

## R2 Issuer and verification library (probe at T003)

- The server's `identity.Verifier` takes a bearer token and returns an opaque subject; production composes it through `newVerifier`, which is nil today, so production refuses to start, and that refusal is what spec 009's image witnesses. The probe chooses a supported token issuer and a maintained verification library for it, pins the library by go.sum after the probe, and writes the negative tests with synthetic keys: wrong issuer, wrong audience, expired, bad signature. The development identity is never composed into the production binary (`//bff/cmd:binaries_test`).

## R3 Store (probe at T004 and T019)

- `ports.Store` has `Put` and `Get` by owner and id with `ErrNotFound` and `ErrUnavailable`; the memory adapter is bounded. The local demo takes a file-backed adapter under the same contract suite with stated persistence, restart and reset semantics; the hosted example takes the simplest managed store with owner-scoped keys, under the same suite, with a read-after-restart test and an owner-isolation test. A one-instance limit is never a durability claim.

## R4 Cloud Run against the server (evaluation at T005)

- Cloud Run's container contract: linux/amd64, listen on `0.0.0.0:$PORT`. The image is linux/amd64 and static; the server defaults to `:8080` and does not read `PORT`, and its dev mode permits loopback only, which stays. The evaluation records, per requirement, what Cloud Run provides and what the example still lacks: the verifier (R2), the exact origin (R5), bounded shutdown (`bff/server`), persistence (R3), scale-to-zero and the cost bounds. Nothing in it rebuilds the image: deployment is by the digest spec 009's verifier recorded.

## R5 Pages base path and the browser origin (probe at T006)

- A Pages project site serves under a path while the browser origin is the host alone; CORS here allows one exact origin with no path, and that stays. The frontend base path and the RPC base URL are independent inputs; the journey tests the preflight, a blocked foreign origin, an authenticated call, error and status accessibility, reload and deep link, logout and cancellation, and log redaction.

## R6 The local launcher on a host with bash 3.2 (independent Mac probe, 2026-11-01)

- An independent probe on macOS ARM at `34e1bbc` found `/bin/bash` 3.2.57, which refuses `wait -n`, the call the launcher ended with; no GNU `timeout` or `flock` on PATH either. The launcher now needs bash 3.2 and POSIX tools only: the first child's exit is detected by a `kill -0` poll with the same semantics, both children are stopped by the same trap, and a busy or invalid port is refused by name before anything starts, through bash's own `/dev/tcp` connect. The demo's host requirements are stated apart from the full verifier's. Native macOS is recorded only from an independent native run; Linux end-to-end coverage proves nothing about it.

## R7 Well-Architected

- The framework's pillars are rows in the acceptance matrix per component, each a control with its witness or a named gap with owner, command and revision; live IAM, recovery, availability and cost are proved only by live evidence behind the recorded gates, never by a document or a CI run.
