# Feature Specification: Hosted example and local demo

**Feature Branch**: `012-hosted-example`
**Created**: 2026-11-01
**Status**: Specified. Current status is in the [index](../README.md)
**Input**: M10. The human directions of 2026-11-01: the whole stack is easy to bring up locally and demo, with a recording of the real frontend in the repository; the public template keeps GitHub for public CI and artifact hosting; a hosted example of the synthetic application runs on GCP, deployed by Cloud Build into a dedicated, BFF-owned GCP project, with the static frontend on GitHub Pages and the backend actually running. Parent: spec 001 FR-030 and the host-profile contract; spec 009 (the one image, by digest); spec 010 (publication, the IaC discipline, the operator gates); spec 011 (the adapters' contract suites). Nothing here makes a host mandatory for template users: the reusable core stays portable, the NixOS contract stays optional, and this feature is the template's own example.

## Clarifications

### Session 2026-11-01

- Q: GitHub Actions or GCP for deployment? → A: GCP Cloud Build for continuous deployment; GitHub keeps public CI, the container package and the downloadable artifacts; GitHub Pages may host the static frontend only, and never stands in for the backend.
- Q: Which GCP project? → A: A dedicated project owned by this template's operator, created from a reviewed plan; never an existing workload project of another product.
- Q: Which backend service? → A: Chosen by evaluation against the real server: a verifier for bearer tokens, an exact allowed origin, an in-memory store today, a refusal to start without a verifier. Cloud Run is the candidate; the evaluation is recorded before any resource exists.
- Q: Is the packaged image a deployable example? → A: No. It witnesses the production refusal: `newVerifier` is nil, so production refuses to start; the production bundle supplies no credential and says sign-in is not configured; the server wires the in-memory store unconditionally; the address defaults to `:8080` and `PORT` is not read; CORS accepts one exact origin. Each is a requirement below with its witness, through the existing ports, and the dev identity is never deployed.
- Q: Must the local demo wait for the cloud? → A: No. The local whole-stack demo is one documented command on a supported host with no GCP credential or project, and it is first-class: the same generated APIs, client and backend, the same adapter contracts as the hosted example, a usable local sign-in and synthetic data, and never a disconnected mock page or a weakened production or test identity.
- Q: What goes into public source? → A: Configuration schemas, the plan modules and synthetic example values only. Real identifiers, billing, endpoints, credentials, state, plans and evidence stay outside the repository.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An operator can stand up the example from a reviewed plan (Priority: P1)

An operator fills one typed input file, reads one plan, applies it through the recorded gates, and ends with a running backend at a known endpoint, the frontend served from Pages pointing at it, and the exact image digest that runs.

**Why this priority**: the direction requires the backend to actually run; a plan that cannot be applied end to end is documentation.

**Independent Test**: offline, the plan validates and its mock tests pass with synthetic inputs; after an authorized apply, the evidence names the endpoint, the identity that deployed, the digest served and a browser flow that created and read a note.

**Acceptance Scenarios**:

1. **Given** the typed inputs, **When** any required value is omitted or invalid, **Then** validation refuses before any plan, naming the input.
2. **Given** an authorized apply, **When** it completes, **Then** the backend answers at the recorded endpoint with the image digest from spec 009's build, not a rebuild.
3. **Given** the deployed pair, **When** the browser flow runs against the Pages frontend and the backend endpoint, **Then** a note is created and read with a real verified identity, and the dev identity is refused.

### User Story 2 - A release can be rolled back and a deployment cancelled (Priority: P1)

An operator rolls the service back to the previous digest, or cancels an in-flight deployment, through the same configuration, and the evidence shows which digest served before and after.

**Independent Test**: the plan for a rollback changes only the digest; the cancellation leaves the previous revision serving.

**Acceptance Scenarios**:

1. **Given** two known digests, **When** the rollback input is set to the earlier one and applied, **Then** the service serves it, and nothing else in the plan changes.

### User Story 3 - Costs and authority are bounded (Priority: P2)

A reader sees the per-role identities, each permission, the cost bounds and what stops a runaway, before anything exists.

**Independent Test**: the plan's identities and bindings are enumerated by a test; a binding outside the enumerated set fails it.

### User Story 4 - A real identity, a real store, a real port (Priority: P1)

A visitor signs in through a supported provider in the production bundle, the backend verifies the token against a pinned maintained verification library, and the note they create survives a revision change because the store is a managed service behind `ports.Store`.

**Why this priority**: without these the hosted example is the refusal image with a static page in front of it.

**Independent Test**: the verifier refuses a wrong issuer, a wrong audience, an expired token and a bad signature; the production bundle contains no test identity; the store adapter passes the same contract suite as the memory fake and a note is read back after the service restarts, by its owner only; the server listens on a non-default `PORT`.

**Acceptance Scenarios**:

1. **Given** a token from the configured issuer, **When** the backend verifies it, **Then** the principal is the token's subject; a wrong issuer, audience, expiry or signature is refused with the exact status.
2. **Given** the production bundle, **When** it is inspected, **Then** the test identity is absent and a sign-in path exists.
3. **Given** a note created before a restart, **When** its owner reads it after, **Then** it is returned; another owner is refused.

### User Story 5 - Anyone can bring the whole stack up locally and demo it (Priority: P1)

A developer on a supported host runs one documented command and gets the backend, the page and a usable local sign-in with synthetic data, with the working URLs printed; a port or configuration error says what to change; stopping the command stops everything it started; restart and reset semantics are stated and true.

**Why this priority**: the direction asks for an easy local bring-up and demo; it needs no cloud and must not wait for one.

**Independent Test**: from a fresh checkout on the supported Linux profile, the command starts, a real browser signs in, creates and reads a note, reloads and still reads it, the command is stopped and restarted with the stated persistence, reset clears the data, a configuration-only change (the port) takes effect, and the revision, command, startup time, logs and limitations are recorded.

**Acceptance Scenarios**:

1. **Given** a fresh checkout, **When** the documented command runs, **Then** the backend and the page start on the configured loopback ports, the URLs are printed, and the browser flow signs in, creates and reads a note.
2. **Given** a port in use or an invalid local profile value, **When** the command runs, **Then** it fails before starting anything else and names the value to change.
3. **Given** a running demo, **When** it is stopped, **Then** every process it started is gone; **When** it is restarted, **Then** the stated persistence holds; **When** reset is requested, **Then** the data is cleared and nothing else is.

### Edge Cases

- The frontend reaches a backend whose origin is not the exact allowed one: the browser flow fails, and the evidence says so; no wildcard is added.
- The image digest in the inputs is not the one spec 009's verifier recorded: the deployment is refused by the digest check, not rebuilt.
- Persistence is in-memory today: the example states that notes do not survive a revision change, and a persistent store is an adopter's port, not this feature's.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The example MUST deploy the exact OCI image spec 009 builds, identified by its content digest, without rebuilding it; the digest MUST be an input, checked against the recorded release digest.
- **FR-002**: A dedicated GCP project MUST be declared for the example alone, with its id, display name, parent, owner, billing account, region and enabled APIs as typed inputs; no existing workload project is reused.
- **FR-003**: Every input MUST be declared once in one typed composition funnel with validation, precedence and effective-value provenance; inner modules MUST take required parameters and carry no defaults.
- **FR-004**: Per-role identities (bootstrap, state, build, deploy, runtime, publisher) MUST be separate, with narrowly scoped IAM enumerated by a test; the runtime identity MUST hold no deploy or build permission.
- **FR-005**: Continuous deployment MUST run on Cloud Build from the trusted source only, authenticated through an existing sanctioned identity or workload identity federation; no GitHub Actions deployment, no long-lived key in source, no disputed or borrowed credential.
- **FR-006**: The backend service MUST be chosen by a recorded evaluation against the real server's requirements (a verifier for bearer tokens, an exact allowed origin, startup refusal without a verifier, in-memory persistence, bounded shutdown); the evaluation names what the service provides and what the example still lacks.
- **FR-007**: The frontend MAY be served from GitHub Pages as static files with its backend endpoint and base path as inputs; the Pages site MUST state that the backend runs elsewhere and MUST NOT be presented as a full-stack demo without the backend running.
- **FR-008**: The runtime configuration (endpoint, token verifier, CORS origin, persistence semantics) MUST be typed inputs with synthetic public examples; the production server's refusal without a verifier stays in force.
- **FR-009**: Rollback and cancellation MUST be inputs of the same configuration: a rollback changes only the digest; a cancellation leaves the previous revision serving; both are witnessed by evidence naming the digests before and after.
- **FR-010**: Cost bounds MUST be typed inputs (instance limits, concurrency, request timeout, budget alert) and MUST be enumerated in the plan; no bound is a monetary cap, and the evidence says so.
- **FR-015**: A verified identity adapter MUST implement the existing `identity.Verifier` port for a supported token issuer through a pinned maintained verification library, composed in production through `newVerifier` only; its tests MUST refuse a wrong issuer, a wrong audience, an expired token and a bad signature, and the dev or test identity MUST never be composed into the deployed binary.
- **FR-016**: The production bundle MUST provide a supported sign-in that acquires the bearer credential for the generated client, with the test identity absent from the bundle (witnessed by inspection), and the hosted browser journey MUST make real verified RPCs.
- **FR-017**: The hosted example's persistence MUST be stated: a managed store adapter behind `ports.Store`, passing the same contract suite as the memory fake, with tests for a read after a service restart and for owner isolation; a one-instance limit is never claimed as durability.
- **FR-018**: The container port and the server address MUST agree at the configuration funnel: the server reads `PORT` as Cloud Run's contract requires, binds `0.0.0.0` in production, keeps dev on loopback only, and a non-default port is tested.
- **FR-019**: The frontend base path and the RPC base URL MUST be independent inputs (a Pages project site has a path, and the browser origin is the host alone); the tests MUST cover the preflight, a blocked foreign origin, an authenticated call, error and status accessibility, reload and deep link, logout and cancellation, and log redaction of tokens and subjects.
- **FR-020**: The local demo MUST be one documented command (`tools/bootstrap/bazel.sh run //tools/dev`) that starts the real backend and the real page through the generated client, with a typed local profile declared once at the top boundary (ports, origin, persistence, data seed, sign-in), every needed local dependency pinned and fetched by the build, the working URLs printed, and an actionable error for a port in use or an invalid value before anything else starts; no GCP credential or project is a prerequisite.
- **FR-021**: The local sign-in MUST be usable and local (the development identity on loopback, as today, or a local issuer behind the same `identity.Verifier` port), and the production and test identities MUST NOT be weakened: production still refuses to start without a verifier and the test identity stays absent from the production bundle.
- **FR-022**: The local persistence, restart and reset semantics MUST be stated and witnessed: which store the profile selects, what survives a restart, how reset clears it; the local store adapters MUST pass the same `ports.Store` contract suite as the hosted ones; a process the command started MUST be stopped when it stops, and nothing it did not start is touched.
- **FR-023**: The local demo's evidence MUST name the revision, the command, the measured startup time, the logs and the limitations, on the supported Linux profile; macOS stays unverified until an independent native run records it, and the initial pinned acquisition may need the network, so no offline cold start is claimed.
- **FR-024**: The repository MUST carry one small animated GIF of the real local frontend working against the real local backend through the generated APIs (synthetic local sign-in, create, read, reload), labeled as the LOCAL demo and rendered inline in the README; it is recorded only after the chosen local UI is integrated, from the running stack, by a pinned maintained regeneration command whose configuration names the application revision, and it is regenerated on a material UI change; the raw capture and logs stay outside the repository; no mocked UI, no hosted claim, no stock or generated imagery, no paid tool, no private context; its frames, playback and the README rendering are verified after publication.
- **FR-011**: No private identifier, billing account, endpoint, credential, state, plan or incident detail MAY enter public source; schemas and examples use synthetic values; real tfvars, state, plans and evidence stay outside the repository.
- **FR-012**: Offline, the configuration MUST format, validate and pass mock tests with synthetic inputs and no network or credentials, through the same discipline as `governance/github`; a second project, a wildcard origin, a runtime identity with deploy permission or an unenumerated binding MUST fail a test.
- **FR-013**: Creation, apply, access, billing and spend MUST each be a recorded operator gate; nothing in CI or a script applies anything.
- **FR-014**: The evidence of a hosted run MUST name the endpoint, the deploying identity, the served digest, the browser flow's result with a verified identity and the refused dev identity, the rollback witness and the measured cost of the run.

### Key Entities

- **Input funnel**: one typed object holding every value, validated once; everything below it is required parameters.
- **Deployment record**: endpoint, identity, digest before and after, time, cost; kept outside the repository.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Offline validation and mock tests pass with synthetic inputs; each of the planted defects of FR-012 fails.
- **SC-002**: After an authorized apply, a browser flow against the Pages frontend and the backend creates and reads a note with a verified identity; the served digest equals spec 009's release digest.
- **SC-003**: A rollback to the previous digest and a cancelled deployment each leave the recorded digest serving, with evidence.
- **SC-004**: Public source carries no real identifier: the neutrality scans pass with the example's schemas and synthetic values in scope.
- **SC-006**: On a fresh checkout of the supported Linux profile, the one command brings the whole stack up; the browser flow (sign-in, create, read, reload), stop, restart, reset and a configuration-only port change are witnessed with revision, command, startup time, logs and limitations.
- **SC-005**: Every Well-Architected pillar (operations, security and privacy, reliability, cost, performance, sustainability) has a row in the acceptance matrix per component: a control with its witness or a named gap with its owner, command and revision; a document or a CI run is never cited as live IAM, recovery, availability or cost proof.

## Ownership split (proposed, to settle with the infrastructure owner)

| Concern | Owner | Paths | Tasks |
| --- | --- | --- | --- |
| Project, bootstrap, billing, APIs, IAM, state composition, apply, outputs | Infrastructure | its own repository; a versioned narrow output contract (project id, region, runtime identity, build identity, state bucket) consumed here as typed inputs | its own ids, referenced from T002 |
| Local whole-stack demo: the profile, the entry point, the local sign-in and seed, the local store adapter, the whole-stack witness, the demo recording | BFF, this repository | `tools/dev/` (entry point `//tools/dev`), `tools/dev/profile.json` and its schema, `bff/adapters/store/local/`, `web/e2e/local-stack.spec.*`, `docs/local-demo.md`, `docs/media/local-demo.gif`, `tools/dev/record.sh` and its configuration, `docs/evidence/m10.md` | T018 to T022 |
| Runtime identity adapter, store adapter, configuration funnel, service module, Cloud Build CD, frontend, browser and rollback acceptance | BFF, this repository | `bff/adapters/identity/`, `bff/adapters/store/`, `bff/cmd/bff/`, `deploy/gcp/service/`, `deploy/gcp/cloudbuild/`, `deploy/gcp/inputs/`, `web/app/`, `web/e2e/`, `docs/evidence/m10.md` | T003 to T017 |

No remote whole-state read crosses the boundary, and no root overlaps; the contract is a versioned file of outputs, not state access.

## Assumptions

- The project's bootstrap, state and IAM are owned by the infrastructure owner; the service, the frontend, CD and the deploy acceptance by this template's owner. The path split in the plan names each.
- A GCP account, access, apply and spend are approval boundaries outside this repository; this feature prepares source, tests and plans, and records one concrete gate for each.
