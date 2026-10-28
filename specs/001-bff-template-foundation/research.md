# Research: Generic Bazel/Protobuf BFF Template

**Date**: 2026-10-28 · **Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md)

## Method and recorded revisions

| Source | Revision | License | Notes |
| --- | --- | --- | --- |
| Private reference implementation (a Bazel/Protobuf monorepo) | recorded in the operator's private inspection record (outside this repository) | proprietary | Inspected read-only for architecture patterns. Nothing is copied: no code, schema, path, identifier, metric or history. |
| [selamy-labs/relentless-ci-python](https://github.com/selamy-labs/relentless-ci-python) | `main` @ `7dffef993b54019ddd86762c4a3b66ae17c6d05f` | MIT | Check registry, strictness controls, CI aggregate. |
| [selamy-labs/relentless-ci-typescript](https://github.com/selamy-labs/relentless-ci-typescript) | `main` @ `9c18c5552604a1a2a87efe7062b42bfc2b341012` | MIT | Same, for TypeScript. |
| [github/spec-kit](https://github.com/github/spec-kit) | `v1.2.3` = `5ea8b1cf0a38699340a38e00d02770308997d8c0` | MIT | Pinned scaffold (R-15). |
| Issues py#25–#31, ts#20 | read 2026-10-28 | — | Mapped in [issue-map.md](issue-map.md). |

Both Relentless CI templates are copy-only today: they have no packages, tags or reusable
workflows. Their custom trusted-policy issuers were removed upstream (python PR #18,
typescript PR #16) and are not reinstated here.

## R-01 Reference architecture: what is implemented, what generalizes

| Pattern | Status in the private reference | Generalizes? |
| --- | --- | --- |
| Hermetic toolchain: Bazel 9, Bzlmod only, committed lockfile in error mode, no WORKSPACE, prebuilt tools pinned by digest | Implemented and gated | **Yes**. This is M1. |
| One aggregate local check over many gates (lint, format, types, complexity, clones, test structure, suppression ban with stable codes, source enrollment, dependency allowlist, CI policy, secrets, spelling, links) | Implemented | **Pattern yes, mechanism no.** Its imperative dispatcher needs two edits per gate. Replace it with a declarative registry (R-08). |
| Floors that may only rise vs the base revision (coverage, mutation) | Implemented | **Yes**. This is the interim weakening protection (R-09). |
| Proto gates: format/lint, AIP linting with an exception ledger, breaking check against an immutable full SHA read from git objects (fail closed, explicit bootstrap marker), breaking probes | Implemented and gated | **Yes**. This is M2. |
| Code generated at build time (Go Connect via rules_go, TypeScript via protoc-gen-es); only API docs committed, with a freshness test | Implemented | **Yes** |
| Generated TS client compiled by test callers; negative compile gates | Implemented (test callers only) | **Yes** (informs #28) |
| Test-only Connect fake: readiness event on stdout, loopback control port, exact-origin CORS, graceful shutdown | Implemented (testonly) | **Partly** |
| Production BFF (auth, deadlines, error mapping) | **Planned only** | Design input |
| Browser frontend and browser end-to-end tests | **Planned only**: no app code, no browser tests | Design input |
| Domain services | Server binaries with no methods registered | **No**. This is vacuous surface to avoid. |
| Spec Kit vendored with pinned upstream, hashes and an offline reproduction test | Implemented | **Yes** (R-15) |
| Lifecycle hooks | None | New (M5) |
| Remote cache / remote execution | Optional / design | **Excluded** (constitution X) |
| Deployment and repository settings as code | Planned only | Pattern only (M8/M9) |
| Measured feedback budgets | None | New (M1, M5) |

**Implication (material)**: the reference is strongest at hermeticity, contracts and quality gates.
It is weakest exactly where the template must be real: the BFF runtime and the browser slice. The
template generalizes the former. It designs the latter from public documentation (Connect,
protovalidate, Playwright, Vite).

## R-02 Validate the proposed first slice (Go BFF + TypeScript browser + Connect + Bazel)

**Decision**: Adopt.

**Rationale**:

- The same Bazel module family is already proven in a real monorepo: rules_go, aspect_rules_js,
  connect-go, protoc-gen-es v2 and connect-es v2.
- It matches the direction that was planned but not yet built for the frontend and BFF there.
- Go gives a single static BFF binary.
- TypeScript is the only realistic browser language.

**Alternatives**:

- **TypeScript BFF (Node).** Fewer languages, but it loses the "polyglot through contracts"
  property the RFC asks for, and it puts server and client in one dependency graph.
- **Python BFF.** The slice has no need for it.

**Caveat**: no generalizable BFF or browser code exists to learn from. M3 and M4 carry the most
design risk, so they get spike tasks.

## R-03 Bazel module set and version policy

**Decision**:

- Bazel 9.x, set in `.bazelversion`.
- Bzlmod only, with `MODULE.bazel.lock` committed.
- `.bazelrc` sets `--lockfile_mode=error`.
- Sandbox network is off by default.

The table lists the starting candidates. M1 fixes the exact pins and records them in the lockfile.
Only releases at least 14 days old at pin time are used.

| Module | Latest stable release ≥ 14 days old on 2026-10-28 |
| --- | --- |
| bazel | 9.3.x |
| rules_go | 0.64.2 |
| gazelle | 0.54.0 (2.0.0 is beta) |
| aspect_rules_js / rules_ts | 3.5.1 / 3.10.1 |
| protobuf | 36.2 |
| rules_buf | 0.5.4 |
| rules_multitool | 1.11.x (provides gitleaks, typos, shellcheck, actionlint, zizmor, prek, buildifier) |

**Not adopted yet**:

- **OCI image rules.** No image is needed until M9.
- **rules_python.** The slice has no Python consumer (R-12).
- **googleapis.** Only adopted if an AIP type is actually used.
- **Remote cache configuration.** Excluded.

## R-04 Contract tooling and Connect version line

**Decisions**:

- **buf (via rules_buf).**
  - Used for format, lint (`STANDARD`) and breaking-change checks (`FILE`).
  - AIP rules are applied through a documented ledger. The full api-linter is not enforced; it can
    be adopted through the #28 profile.
- **Breaking baseline.**
  - The baseline is an immutable full commit SHA, read through `git` object commands.
  - The check fails closed when the baseline is absent.
  - The first schema needs an explicit, reviewed bootstrap marker.
- **Validation.**
  - protovalidate annotations state each field constraint once.
  - The BFF enforces them and is authoritative.
  - The browser may pre-validate for UX.
  - M2 spike T020 verifies that the CEL functions used (for example trimming) behave the same in
    Go and ES. If they don't, the rule is simplified so both sides agree.
- **Code generation at build time.**
  - Go: rules_go `go_proto_compiler` with protoc-gen-connect-go.
  - TypeScript: protoc-gen-es v2. Connect-ES v2 needs no separate plugin.
  - No generated code is committed, except optional API docs that have a freshness test.
- **connect-go version line (material).**
  - Upstream released v2.0.0 on 2026-10-07 and v2.0.2 on 2026-10-21. v1.21.1 is still maintained,
    and a v2 migration tool exists.
  - A new template should not start on a superseded major version, but v2 is only three weeks old.
  - **Decision**: spike T020 builds against the newest v2.x release that is at least 14 days old.
    It falls back to v1.21.x only if rules_go or gazelle integration fails, and the reason is
    recorded.

## R-05 Browser client

**Decision**:

- TypeScript, React 19 and Vite, built and served by Bazel through aspect_rules_js.
- Transport: `@connectrpc/connect-web`.
- End-to-end tests: Playwright against the real BFF binary.
- Browsers are pinned through the build. Spike T030 also tests whether Chromium's own sandbox
  works inside Bazel's linux-sandbox, or whether it needs a documented, test-only `--no-sandbox`.

**Rationale**:

- The RFC asks for an opinionated app, and React is the most common baseline for adopters.
- The UI is deliberately minimal: one form and one lookup.

**Alternatives**:

- **Framework-free TypeScript.** Smaller, but adopters would add a framework immediately, and the
  template would not show component testing.
- **Tailwind.** Dropped: styling is not a template guarantee.

## R-06 BFF shape, boundaries, identity and session

**Decisions**:

- **Example domain.** Neutral **notes**: `CreateNote` covers the validation-failure path and
  `GetNote` covers the not-found path. Both RPCs have browser callers.
- **Layers.**
  - `transport` → `app` → `ports` ← `adapters/memory`.
  - There is no backend service in the slice.
  - The storage port is where an adopter attaches a real backend.
- **Errors.** A fixed mapping: `invalid_argument` with field violations, `not_found`,
  `unavailable`, `deadline_exceeded`, `unauthenticated`, and `internal` (correlation id only).
- **Deadlines.** Each RPC has a server-side default and maximum. Port calls receive the request
  context, and cancellation is tested.
- **Identity.**
  - An `Authenticator` sits at the transport boundary.
  - `dev` mode accepts a header identity labeled `test-identity`.
  - `production` mode refuses `test-identity` at startup. It requires a configured verifier and
    fails closed without one.
  - No identity-provider integration ships. That choice is left to the adopter, and the docs say
    so.
- **Session boundary.**
  - The browser sends a bearer credential in the `Authorization` header.
  - The BFF never authenticates from cookies, and CORS does not allow credentials. With no ambient
    credentials, the BFF has no CSRF surface.
  - A test proves that a request carrying only a cookie gets `unauthenticated`.
  - The docs state what an adopter must add before switching to cookie sessions: `SameSite`,
    CSRF tokens and origin checks.
- **Dependency directions.** Enforced deterministically by Bazel `visibility`, `testonly` and a
  dependency-rule test.

## R-07 Distribution: copy vs shared dependency vs generator

| Criterion | Template copy | Shared versioned dependency | Generator |
| --- | --- | --- | --- |
| Update propagation | None after the copy (Relentless CI today) | Change one pin; release notes list the rule changes | Regenerate, then resolve merge conflicts |
| Review of consumer policy | Drift is invisible | The pin-bump diff is the unit of review | Diff of the regenerated files |
| Build cost | None | A versioned artifact and a release process | Generator code, templates and their own tests |
| Fits app code | **Yes** (apps diverge by design) | No | Yes, but app code diverges anyway |
| Fits quality checks | Poor (drift) | **Yes** | Overkill |

**Decision (smallest viable route)**: a hybrid.

1. **Quality checks are a versioned shared dependency.**
   - They live in a Bazel module under `quality/` with its own `MODULE.bazel`.
   - The module has a neutral name, `bff_template_quality`, so it does not pre-empt Relentless CI
     naming.
   - This repository consumes it with `local_path_override`.
   - Other repositories use `git_override` at an immutable tag commit. The same tag is the prek
     `rev`.
   - Moving the module into its own repository is an M8 ownership decision.
2. **The reference application is a template copy.** Applications are meant to diverge.
3. **No generator.** Revisit only if M7 shows that copy-and-rename is error-prone.

## R-08 Check registry, runner and hooks

**Registry**

- `quality/registry.textproto`, with its schema in `quality/proto/registry.proto`.
- Each check records: id, tool label, pinned version, selector, stages, classification
  (`GUARANTEE` or `SIGNAL`), timeout, `allow_empty`, and execution kind (`bazel` or `native`).
- The registry is validated at build time.

**Runner**

`bazel run //quality:run -- --stage=… [--shard=…]`.

- **One Bazel invocation per stage.** All `bazel` checks are composed into a single `bazel test`
  or `bazel build` invocation, which avoids output-base lock contention.
- **Bounded pool.** `native` checks run in a pool of at most `min(4, ncpu/2)` workers.
- **Deadlines.**
  - When a deadline passes, the runner sends SIGINT to the Bazel client, then runs
    `bazel shutdown` if needed.
  - Native tools run in process groups under a Linux child subreaper, so a child started with
    `setsid` is still reaped.
  - stdin is closed.
- **Exit codes.** The runner's codes are distinct from Bazel's. Bazel exit codes are translated
  explicitly; Bazel's 4 ("no tests") on an affected-test set becomes `NOT_APPLICABLE`.
- **Empty input.** A `GUARANTEE` check with zero inputs at the `full` stage is an error unless the
  check has `allow_empty`. Narrowing a selector to nothing therefore cannot pass silently.

**Stages and prek mapping**

`full` is not a prek stage. It is `//:verify`, and is also exposed as prek's `manual` stage.

| Stage | Hook | What it checks | Notes |
| --- | --- | --- | --- |
| `pre-commit` | `pre-commit` | **staged blob content** (`git diff --cached --name-only -z`, then `git show :path`) | Content checks only: secrets, suppression ban, typos, format-check. No Bazel-glob builds, so untracked files cannot leak in. |
| `commit-msg` | `commit-msg` | the commit message | Off by default; must be explicitly adopted. |
| `pre-push` | `pre-push` | the commit at `PRE_COMMIT_TO_REF`, checked out in a reusable detached worktree under `$(git rev-parse --git-common-dir)/bff-template/push-worktree` with its own warm output base | Changed-package build, lint and affected tests. prek runs once for the first pushed ref; other refs in a multi-ref push are not checked locally (documented limit; CI covers them). |
| `full` | `manual` | everything | `//:verify` = the union of CI shards. |

**Hook rules**

- **Escalation.** At `pre-push`, a change to `quality/**`, `prek.toml`, `.bazel*`, `MODULE.bazel*`
  or the tool locks escalates the stage to `full`. `pre-commit` never escalates, which keeps it
  within budget.
- **Installation.**
  - Hooks are installed with the pinned prek's own install. It handles existing hooks by moving
    them to `.legacy`, which fixtures verify.
  - A thin wrapper only pins the prek version and checks `core.hooksPath` and worktrees.
  - There is no custom chaining code.

**Upstream alignment**

- Stage semantics, exit codes and the diagnostic line follow the shared contract from py#25 and
  ts#20. Task T056 cites the upstream revision it follows.
- Only the Bazel adapter is implemented here.

## R-09 Suppression, threshold and weakening protection

The first slice ships two deterministic local mechanisms:

- **Inline-suppression bans** with stable codes.
- **Floors that may only rise relative to the base revision.**
  - The CI `policy` job **builds the evaluator from the base revision's `quality/` module**. A
    proposal therefore cannot replace the evaluator that judges it.
  - It runs that evaluator against the proposal tree with read-only permissions and no secrets.

A general policy-diff comparator (removed checks, narrowed selectors, self-authorization) and the
trusted-baseline authorization contract are **owned by py#26**. This repository adopts them on
release (T067) and does not build its own.

This repository does not:

- reinstate trusted-policy issuers
- require a GitHub App
- change branch protection or required-review settings

**Stated limit**: a proposal can still edit the CI workflow to drop the policy job. Only a
required check, configured on the platform by the owner, closes that gap. Constitution V is
therefore worded "detect and report", with enforcement stated separately as requiring platform
configuration.

## R-10 Exact-revision evidence

**Decision**:

- CI checks out the exact head SHA.
- CI jobs are **shards of the registry's `full` stage**, and only shards: `//quality:run
  --stage=full --shard=<name>`. A parity test (T066) asserts that the union of shards equals
  `full` and that no workflow step runs policy inline.
- Each shard writes a receipt with: `revision`, `base_revision`, `base_policy_digest` (the policy
  that judged the run, taken from the base revision), `producer`, `attempt` and `state`.
- One aggregate job (`if: always()`) requires every expected shard to succeed on the latest
  attempt for that SHA.
- The **evidence record format and validator are owned by py#27**, including attempt semantics,
  independent review receipts and readiness evaluation. Readiness is recomputed whenever it is
  evaluated; it is not pushed when inputs change.
- M6 delivers the aggregate gate and receipts only. Evidence-fixture validation (SC-007) is
  blocked on #27.
- No scheduler, signing service or App.

## R-11 Budgets and measurement

**Decision**: measure first, then declare.

**Harness**: `tools/bench/measure.sh` records the following:

| Aspect | Detail |
| --- | --- |
| Cold runs | fresh output base, empty disk cache, pinned downloads pre-fetched |
| Warm runs | repeats of the same run |
| Statistics | N=10 runs, p50/p95 wall time, peak RSS |
| Scenarios | bootstrap; `pre-commit` on 1, 50 and 500 files; `pre-push` on a one-package change; `full`; browser e2e |

Results are stored in `docs/budgets/<host-profile>.json`.

**Provisional targets**: these are hypotheses on Linux with 8 vCPU / 32 GB. Measurement replaces
them; checks are never loosened to meet them.

| Scenario | Target |
| --- | --- |
| warm `pre-commit` p95 | ≤ 5 s |
| warm `pre-push` p95 | ≤ 90 s |
| warm `full` | ≤ 10 min |
| cold bootstrap | ≤ 20 min |

## R-12 Polyglot extension, including Python

**Decision**: the template ships Go and TypeScript only, because only they have consumers.

A **language adapter profile** (`docs/extending/language-profile.md`) defines what a new language
must supply:

- Bazel rules and a toolchain pin
- a generated-client target
- registry entries for format, lint, types and tests
- suppression patterns
- a real consumer with an end-to-end scenario

Python is the worked design example. It uses rules_python and protobuf/Connect Python code
generation, and reuses the Relentless CI Python rule choices. It is implemented only when a real
consumer exists. No placeholder service is added.

## R-13 Optional GCP deployment profile (design only, M9)

The profile is optional and lives in `deploy/gcp/`. `//:verify` never requires it.

**OpenTofu scope**:

1. **Bootstrap.**
   - A versioned state bucket.
   - The bucket is created with local state, which is then migrated (an operator step).
2. **Platform resources.**
   - Project services and Artifact Registry.
   - A Cloud Run service with a least-privilege runtime service account.
   - A Cloud Build service account and triggers.
3. **Cloud Build pipelines.**
   - Build and push by digest.
   - Deploy only through a trigger with `approvalRequired`.

**Offline CI**: `tofu fmt -check`, `validate`, `tflint`, and `tofu test` with mocked providers.

**Operator gates**: every apply, every approval and every IAM change.

**Constraints**:

- No project, account, billing or identity values are committed.
- Nothing is applied in M0.

## R-14 Repository ownership, name, visibility and provisioning

**Proposal (owner decision; it blocks M8 only)**:

- Repository `selamy-labs/bazel-protobuf-bff-template`.
- Public, a GitHub template repository, MIT license.
- One required aggregate check.

**Governance**:

- Settings for **this new repository only** are declared in `governance/github/` with the
  OpenTofu GitHub provider: repository, template flag, ruleset and merge settings.
- They are validated offline and applied only by an operator after explicit approval.
- This is a carve-out from "no branch-protection changes". It concerns the new repository's own
  initial ruleset. It never touches the Relentless CI repositories or any existing protection.

Drafts stay local, and no origin is configured.

## R-15 Spec Kit bootstrap

**Decision**: pin the official repository by commit (`v1.2.3` → `5ea8b1cf…`) and run it through
`uvx`, with no global install.

- `tools/speckit/bootstrap.sh verify` re-initializes into a temp directory and diffs the vendored
  scaffold, ignoring install timestamps. It passed at M0.
- Globally installed `specify` versions are ignored.
- M1 moves the check into Bazel, using an offline, hash-pinned archive.

## R-16 Licensing and provenance

**Decision**: provenance inventory in [docs/provenance.md](../../docs/provenance.md); public-release
boundary in [docs/public-release-review.md](../../docs/public-release-review.md). The reference is
proprietary — only architectural observations (no code, schema, fixture, configuration value,
identifier or incident text) inform this repository. Relentless CI and Spec Kit are MIT; any adapted
file records source, revision and license.

## Open questions for the reviewer

1. Approve R-07 hybrid distribution (shared checks in-repo as a Bazel module first, extracted later)?
2. Approve repository name/visibility/license proposal (R-14 / A5)?
3. Accept connect-go v2 as the starting line subject to the M2 spike (R-04)?
4. Accept React (vs framework-free TypeScript) for the opinionated browser client (R-05)?
