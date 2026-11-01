# Tasks: Hosted example

**Input**: Design documents from `specs/012-hosted-example/`
**Prerequisites**: plan.md, spec.md, research.md, acceptance-matrix.md
**Tests**: required by the spec. Offline tests with planted defects before any live step; every live step is a recorded gate.

## Phase 1: Setup

- [x] T001 Land spec, plan, research, matrix and the Well-Architected rows in `specs/012-hosted-example/`
- [ ] T002 (the schema half landed: `contract.schema.json` published, referenced by the inputs schema and checked against the struct; the owner's task ids stay open until the infrastructure owner settles them) The versioned output contract with the infrastructure owner: `deploy/gcp/inputs/contract.schema.json` (project id, region, runtime identity, build identity, state bucket, artifact registry) and the agreed task ids on their side, recorded in research R1

## Phase 2: Probes (research)

- [ ] T003 [P] Issuer and verification library probe: a maintained library for the chosen issuer, pinned by go.sum after the probe, with synthetic keys for the negative tests; recorded in research R2
- [ ] T004 [P] Managed store probe: the simplest managed store with owner-scoped keys, run against the `ports.Store` contract suite behind an emulator or a recorded gate; recorded in research R3
- [x] T005 [P] (research R4) Cloud Run evaluation against the server: `PORT`, `0.0.0.0`, the verifier, the exact origin, bounded shutdown, in-memory versus managed persistence; what it provides and what the example still lacks; recorded in research R4
- [ ] T006 [P] Pages base path probe: project-site path versus host origin; recorded in research R5

## Phase 3: User Story 4 - A real identity, a real store, a real port (Priority: P1)

- [ ] T007 [US4] The verifier adapter in `bff/adapters/identity/<issuer>/` implementing `identity.Verifier`, composed in production through `bff/cmd/bff/verifier.go`; tests refuse a wrong issuer, a wrong audience, an expired token and a bad signature; the dev identity stays out of the production binary (`//bff/cmd:binaries_test`)
- [ ] T008 [US4] The managed store adapter in `bff/adapters/store/<managed>/` behind `ports.Store`, passing `bff/ports/contract_test.go`, with a read-after-restart test and an owner-isolation test
- [x] T009 [US4] (the flag, then BFF_ADDR, then PORT on every interface, then 8080; dev stays loopback-only by its own refusal; a non-default port tested) `PORT` and the address at the funnel in `bff/cmd/bff/config.go`: production binds `0.0.0.0:$PORT`, dev stays loopback-only, a non-default port is tested
- [ ] T010 [US4] Supported sign-in in `web/app/signin/` acquiring the bearer credential for the generated client; the production bundle inspected for the absent test identity (`//web/app:bundle_test`)

## Phase 4: User Story 1 - Stand up from a reviewed plan (Priority: P1)

- [x] T011 [US1] (`inputs.Resolve`: a base file and an operator's overlays merged leaf by leaf with the last file winning, a leaf given by no file refused by name, the source of every effective value printed, the effective document written and read by the plan in the pipeline and the offline witness) The typed input funnel `deploy/gcp/inputs/` with validation, precedence and effective-value provenance, and a synthetic example; inner modules take required parameters only
- [x] T012 [US1] (the service module and the pipeline landed offline; the Cloud Build trigger, its connection to the public repository and its approval setting are the infrastructure owner's and the operator's gates) The service module `deploy/gcp/service/` (Cloud Run service by digest, runtime identity, config, exact origin) and the CD pipeline `deploy/gcp/cloudbuild/` (promote by digest from the trusted source, rollback by digest, cancel), authenticated through the sanctioned identity or WIF; no GitHub Actions deployment
- [x] T013 [US1] (twelve cases; a planted identity with deploy permission is not a resource of this module and stays with the infrastructure owner's tests) Offline tests `deploy/gcp/tests/`: format, validate, mock plan; planted defects fail: a second project, a wildcard origin, a runtime identity with deploy permission, an unenumerated binding, a digest that is not the recorded one; bound as a required Bazel test with the pinned tofu and provider
- [ ] T014 [US1] (landed locally: the page makes no root-absolute request, the static server serves under a required base path, and `web/e2e/hosted.spec.mjs` runs the page under the example's base path with the backend on another origin: the preflight and the call, a foreign origin blocked, errors and status announced, a reload, the backend's log without credential or subject; open with the sign-in (T010): the authenticated call with a verified identity, logout, and a deep link, since the page has one screen today) The frontend inputs: base path and RPC base URL independent; `web/e2e/hosted.spec.*` covers the preflight, a blocked foreign origin, an authenticated call, error and status accessibility, reload and deep link, logout and cancellation, and log redaction

## Phase 5: User Story 2 and 3 - Rollback, cancellation, bounds (Priority: P1, P2)

- [x] T015 [US2] (offline: the rollback inputs serve the previous digest and change nothing else, the cancel inputs change nothing in the declared service and the pipeline's cancel step keeps every later step from promoting, planning or applying, each with a planted defect; the digests before and after are in the evidence record; the live witness is T017's) Rollback and cancellation as inputs of the same configuration, witnessed by the digests before and after in the evidence record
- [x] T016 [US3] (the bounds and the identities enumerated by `inputs_test`; the packet with its inputs and gates for the operator and the infrastructure owner is outside the repository, in the M10 packet) Cost bounds as typed inputs enumerated by a test; the identities and bindings enumerated by a test; the evaluation and the concrete packet of inputs and gates (project id availability, creation authority, billing choice, region, auth provider, cost bound, apply approval) for the operator and the infrastructure owner, outside the repository

## Phase 6: User Story 5 - The local whole-stack demo (Priority: P1; needs no cloud)

- [x] T018 [P] [US5] The typed local profile `tools/dev/profile.json` with its schema (ports, origin, persistence, seed, sign-in), validated once by `tools/dev/dev.sh` before anything starts, an actionable error for a port in use or an invalid value, and the URLs printed; a configuration-only port change tested in `tools/dev/dev_test.sh`
- [x] T019 [P] [US5] The local store behind `ports.Store` in `bff/adapters/store/local/` (file-backed, owner-scoped), passing `bff/ports/contract_test.go`; persistence, restart and reset semantics stated in `docs/local-demo.md` and witnessed: a note survives a restart, reset clears it, the stop kills only what the command started
- [x] T020 [US5] (the development identity on loopback, as today, with the seed subject in the profile; production refusal and the bundle inspection unchanged) Local sign-in and synthetic seed: the development identity on loopback as today, or a local issuer behind `identity.Verifier`; production refusal and the bundle inspection unchanged
- [x] T021 [US5] (Linux; macOS only from the independent run) The whole-stack witness `web/e2e/local-stack.spec.*`: fresh checkout, the one command, sign-in, create, read, reload, stop, restart, reset; `docs/evidence/m10.md` records revision, command, measured startup time, logs and limitations on the supported Linux profile; macOS recorded only from an independent native run

- [x] T022 [US5] (recorded at `03204b8`; GitHub rendering verified after publication) The demo recording: `tools/dev/record.sh` with its configuration naming the application revision, driving the real local stack through the whole-stack journey with the pinned browser and producing one small GIF at `docs/media/local-demo.gif` with a pinned maintained encoder; the README embeds it labeled "local demo"; raw capture and logs stay outside the repository; recorded only after T018 to T021, regenerated on a material UI change; frames, playback and the README rendering verified after publication (spec 010 US5)

## Phase 7: Live, behind the gates

- [ ] T017 After the recorded gates: apply, deploy by digest, the browser flow with a verified identity and the refused dev identity, rollback, cancellation, cost; `docs/evidence/m10.md` with endpoint, identity, digest, flow result and measured cost; the Well-Architected rows updated from live evidence, never from documents

## Dependencies

- T001, T002 first. T003 to T006 together. T007 to T010 after their probes. T011 to T014 after T002. T015 and T016 after T012. T018 to T021 need no cloud and run beside everything; T022 after them. T017 last, and only behind the gates.
