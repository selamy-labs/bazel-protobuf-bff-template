# Implementation Plan: Generic Bazel/Protobuf BFF Template — Foundation

**Branch**: `001-bff-template-foundation` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-bff-template-foundation/spec.md`

## Summary

Build a public, neutral starter that shows a real browser → BFF slice over generated
Protobuf/Connect clients, with Bazel owning toolchains, code generation and tests. Quality checks
ship as a versioned shared Bazel module with one declarative registry. Thin adapters run that
registry through prek hooks, the full verifier and CI. Evidence is bound to the exact revision.
Delivery is milestone-by-milestone (M1–M11 in the spec). This plan covers the whole roadmap at
design depth and the first slice (M1–M4 plus minimal M5/M6 gates) at task depth. M0 (this package)
stops before any application code.

## Technical Context

**Language/Version**: Go (latest stable at M1 pin time, ≥ 1.26.x via rules_go SDK); TypeScript 5.x
on Node (pinned via rules_nodejs, reference uses 26.12.0); Starlark; Protocol Buffers (proto3/
editions per buf default).

**Primary Dependencies**: Bazel 9.x (Bzlmod), rules_go + gazelle, aspect_rules_js/rules_ts,
protobuf, rules_buf (buf CLI), protovalidate, connect-go (v2 line, R-04), protobuf-es v2 +
connect-es v2 + connect-web, React 19 + Vite, Playwright, rules_multitool (gitleaks, typos,
shellcheck, actionlint, prek), Spec Kit v1.2.3 (pinned).

**Storage**: In-memory adapter behind a storage port (example only). No database in the template.

**Testing**: `go test` via rules_go (unit, property-based with a pinned library, handler tests
over an in-process Connect server); Vitest for client units; Playwright browser end-to-end against
the real BFF binary under `bazel test`; buf breaking probes; hook fixtures in temporary Git
repositories; evidence/registry fixtures (golden diagnostics).

**Target Platform**: Linux x86_64 (primary, CI) with unprivileged user namespaces; macOS arm64
secondary for local use. Browser: pinned Chromium (Firefox/WebKit optional later).

**Project Type**: Web application (browser client + BFF) inside a Bazel monorepo, plus a reusable
quality module.

**Performance Goals**: Example BFF responds in well under its 2 s default deadline locally; template
budgets are feedback-time budgets (R-11), measured not asserted.

**Constraints**: Only Git, bash, curl and coreutils on the host (Bazel is fetched by verified pin); no credentials or network
services needed for verification beyond fetching pinned artifacts; no remote cache/RBE; sandboxed
actions with network disabled by default; all hooks bounded and non-interactive.

**Scale/Scope**: 1 schema package (2 RPCs), 1 BFF binary, 1 browser app, 1 quality module,
~20 canonical checks, 3 prek hooks plus the `full` stage, 1 CI workflow sharded by registry, 1 second consumer (M7).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Pre-research | Post-design | Notes |
| --- | --- | --- | --- |
| I. Real slice, no vacuous surface | PASS | PASS | Two RPCs, both with browser callers and e2e scenarios; no backend service until a consumer exists (R-06); Python profile documented, not stubbed (R-12). |
| II. Contracts first, compatibility-checked | PASS | PASS | buf lint/format/breaking vs immutable SHA, fail closed, build-time codegen (R-04). |
| III. Hermetic, pinned, reproducible | PASS | PASS | Bazel fetched by verified pin, nothing else needed from the host beyond Git, bash, curl and coreutils; Bzlmod lock in error mode; multitool digests; Spec Kit commit pin verified (R-03, R-15). |
| IV. One policy, thin adapters, parity | PASS | PASS | Single registry; one runner used by prek, `tools/verify.sh` and CI (R-08). |
| V. Policy cannot weaken itself | PASS | PASS (detect-and-report, as worded) | Floors and suppression bans judged by an evaluator built from the base revision; #26 owns the general contract; enforcement against removing the CI job needs a platform required check (owner) — stated, not claimed (R-09). No issuers, Apps or protection changes. |
| VI. Exact-revision terminal evidence | PASS | PASS w/ tracked gap | Aggregate gate on latest attempts + receipts bound to revision and base policy digest; validator and readiness owned by #27 (R-10, T068). |
| VII. Guarantees vs signals | PASS | PASS | Registry `classification` field; acceptance matrix labels each witness. |
| VIII. Bounded, measured, actionable | PASS | PASS | Runner bounds, closed stdin, process-group kill; measurement harness before budgets (R-11). |
| IX. Public, neutral, provenance-tracked | PASS | PASS | Neutral "notes" domain; provenance inventory and release review created in M0. |
| X. Smallest viable route | PASS | PASS | Hybrid distribution, no generator; GCP and worker profiles optional (R-07, R-13). |

No violations. Complexity Tracking is empty.

## Architecture (see [design.md](design.md) for diagrams)

- **Dependency direction (enforced)**: `web/app` → `web/client` (generated TS) → *wire* →
  `bff/transport` → `bff/app` → `bff/ports` ← `bff/adapters/*`. Generated Go/TS targets depend only
  on `api/`. Nothing in `bff/` imports `web/`; `testonly` fakes and test identity are invisible to
  production binaries (`visibility` + a dependency-rule test).
- **Quality module** `quality/` (`bff_template_quality`) is a separate Bazel module with no dependency on the application;
  the application depends on it, never the reverse.
- **Runner contract**: [contracts/check-registry.md](contracts/check-registry.md) and
  [contracts/hook-stages.md](contracts/hook-stages.md).
- **Evidence**: [contracts/evidence-receipt.md](contracts/evidence-receipt.md).
- **Example API**: [contracts/notes-api.md](contracts/notes-api.md).

## Project Structure

### Documentation (this feature)

```text
specs/001-bff-template-foundation/
├── spec.md               # spec of specs (milestones M0–M11)
├── plan.md               # this file
├── research.md           # Phase 0 decisions R-01…R-16
├── data-model.md         # Phase 1 entities
├── quickstart.md         # Phase 1 target adopter flow (validated in M1/M4)
├── contracts/            # Phase 1 interface contracts
│   ├── notes-api.md
│   ├── check-registry.md
│   ├── hook-stages.md
│   └── evidence-receipt.md
├── design.md             # concise design for review
├── issue-map.md          # eight upstream issues → requirements, milestones, witnesses, deferrals
├── acceptance-matrix.md  # requirement → witness → evidence class → claim layer
├── checklists/requirements.md
└── tasks.md              # dependency-ordered tasks
```

### Source Code (repository root, target layout after M1–M6)

```text
MODULE.bazel  MODULE.bazel.lock  .bazelversion  .bazelrc  BUILD.bazel  prek.toml
api/notes/v1/                 # neutral schema, buf config, breaking baseline marker
bff/
├── cmd/bff/                  # main: config, mode (dev|production), listener, shutdown
├── transport/                # Connect handlers, CORS, deadlines, Authenticator boundary
├── app/                      # use cases, validation, error mapping
├── ports/                    # storage interface
├── adapters/memory/          # in-memory store
└── testing/                  # testonly identity, fault injection (not visible to cmd/)
web/
├── app/                      # React components, Vite config
├── client/                   # generated-client wrapper, transport setup, error presentation
└── e2e/                      # Playwright specs: success, invalid, not found, unavailable
quality/                      # SEPARATE Bazel module (shared, versioned)
├── MODULE.bazel
├── proto/registry.proto      # registry + receipt schemas
├── registry.textproto        # canonical checks
├── runner/                   # Go: stage selection, bounded pool, diagnostics, receipts
├── adapters/{go,ts,proto,docs,shell}/
└── fixtures/                 # defect + corrected fixtures per check
tools/
├── bench/                    # measurement harness
└── speckit/                  # pinned Spec Kit bootstrap/verify
.github/workflows/ci.yml      # local file; never pushed in M0
docs/  (provenance.md, public-release-review.md, budgets/, extending/)
deploy/gcp/                   # M10 optional profile
governance/github/            # M8 declarative repository settings (not applied)
```

**Structure Decision**: Web application in one Bazel workspace with a nested, separately
versioned `quality/` module. Top-level `api/`, `bff/`, `web/` make ownership and dependency
direction visible; `quality/` can be extracted to its own repository after M7 without path changes
for consumers (they reference `@bff_template_quality//…` either way).

## Phase 2 approach (tasks)

[tasks.md](tasks.md) is ordered by milestone and user story. Each task names files and its witness;
test tasks precede implementation within a story; each story ends with a checkpoint that runs its
independent test and records evidence at the exact revision.

## Complexity Tracking

No constitution violations to justify.
