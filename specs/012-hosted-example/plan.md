# Implementation Plan: Hosted example

**Branch**: `012-hosted-example` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

One typed input funnel, one isolated OpenTofu composition for the BFF-owned pieces consuming a versioned output
contract from the infrastructure-owned project, the exact spec 009 image by digest, a real verifier and a real
store behind the existing ports, Cloud Build CD, the static frontend on Pages with its backend endpoint as an input,
and evidence that names endpoint, identity, digest, browser flow, rollback and cost. Offline tests first; every live
step is a recorded gate.

## Technical Context

**Language/Version**: Go 1.27.2 (adapters, server), TypeScript 6.0.3 (sign-in, client), OpenTofu at the pin of `governance/github` (1.10.x) with the Google provider pinned by sha256 once probed
**Primary Dependencies**: a pinned maintained token verification library for the chosen issuer (probed, then pinned by go.sum); the Google provider; Cloud Build and Cloud Run through the provider only; no SDK call outside the adapters
**Testing**: contract suite for `ports.Store` over the fake and the managed adapter (the managed one behind an emulator or a recorded gate); verifier tests with synthetic keys; `tofu fmt`, `validate` and mock tests offline with synthetic inputs; Playwright journeys with the configured base path and RPC base URL
**Target Platform**: linux/amd64 container (spec 009), Cloud Run candidate, GitHub Pages static host
**Constraints**: no private value in public source; no deploy from GitHub Actions; no credential in source; the dev identity never in the deployed binary; the runtime identity never holds deploy or build permission; no resource created before its gate
**Open at planning**: the issuer and library (probe); the managed store (probe against the contract suite: the simplest managed key-value or document store with owner-scoped keys); Cloud Run's fit for the verifier, origin, port and shutdown requirements (evaluation record); the Pages base path mechanics (probe)

## Constitution Check

| Principle | Result |
| --- | --- |
| III. Pinned | the verification library, the provider and every tool by checksum; the image by digest |
| IV. One policy | the checks of this feature are registry checks and Bazel tests; no second runner |
| V. Policy cannot weaken itself | the input funnel validates once; inner parameters are required; a wildcard origin or a runtime identity with deploy permission fails a test |
| Governance: no live action | every create, apply, access, billing and spend step is a recorded operator gate |
| Public neutrality | schemas and examples hold synthetic values only; the scans run over the whole export |

## Project Structure

```text
bff/adapters/identity/<issuer>/      the verifier adapter and its negative tests
bff/adapters/store/<managed>/        the managed store adapter, the shared contract suite run against it
bff/ports/contract_test.go           the contract suite (spec 011 T010)
bff/cmd/bff/                         PORT and address at the funnel; newVerifier composed for production
web/app/signin/                      the supported sign-in; production bundle without the test identity
web/e2e/hosted.spec.*                preflight, foreign origin, authenticated call, reload, deep link, logout, redaction
deploy/gcp/inputs/                   the typed input schema and a synthetic example
deploy/gcp/service/                  the service module: Cloud Run service, runtime identity binding, config
deploy/gcp/cloudbuild/               the CD pipeline: promote by digest, rollback by digest, cancel
deploy/gcp/tests/                    offline mock tests and the planted defects
tools/dev/                           the local demo: profile, schema, entry point, whole-stack test
bff/adapters/store/local/            the file-backed local store under the same contract suite
web/e2e/local-stack.spec.*           the whole-stack witness
docs/local-demo.md                   the one command, the profile, persistence, restart, reset, limitations
docs/evidence/m10.md                 the local and hosted evidence record
specs/012-hosted-example/
```

The infrastructure owner's repository holds the project, bootstrap, billing, APIs, IAM and state composition, and
publishes a versioned output contract (`deploy/gcp/inputs/contract.schema.json` names its fields) that this
repository consumes as inputs; no state is read across the boundary.

## Steps

1. Spec, plan, research, matrix; the Well-Architected rows per component.
2. Probes: issuer and library; managed store; Cloud Run contract against the server; Pages base path.
3. Adapters behind the ports with the contract suite and the negative tests; `PORT` at the funnel; sign-in.
4. The input schema, the service and CD modules, offline tests with planted defects.
5. The evaluation record and the concrete packet of inputs and gates for the infrastructure owner and the operator.
6. After the gates: apply, deploy by digest, browser flow, rollback, cost; the evidence record.

## Gates

| Gate | Where | What it refuses |
| --- | --- | --- |
| Input validation | the funnel | an omitted or invalid input, before any plan |
| Digest binding | CD | a digest other than the recorded release digest; any rebuild |
| Identity separation | tests | a runtime identity with deploy or build permission; a binding outside the enumerated set |
| Origin | tests | a wildcard or a path in the allowed origin |
| Dev identity | bundle and binary inspection | the test identity in the production bundle or binary |
| Operator gates | the packet | create, apply, access, billing, spend without a recorded approval |

## Risks

- Cloud Run may not fit a requirement the server has (the evaluation says which, and the example states the gap rather than weakening the server).
- A managed store's semantics differ from the memory fake's (the contract suite is the arbiter; a difference is a finding, not a special case).
- Pages serves from a path while the browser origin is the host; a wrong assumption fails the preflight test, not production.

## Complexity Tracking

| Exception | Why | Owner, condition |
| --- | --- | --- |
| Two owners, two repositories | project custody is not this template's | the versioned output contract; no cross-state read |
