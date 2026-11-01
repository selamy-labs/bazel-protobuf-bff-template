# Generic Bazel/Protobuf BFF Template Constitution

## Core Principles

### I. Real Vertical Slice, No Vacuous Surface

The reference application MUST demonstrate one real browser-to-BFF behavior end to end: a
browser client calls a backend-for-frontend through generated Protocol Buffer/Connect clients
and observes both a success and a typed failure. Every published contract (RPC, message, check,
adapter, hook) MUST have at least one production consumer, observable behavior and an acceptance
scenario. Deferred design lives in tracked specifications, never in success-returning stubs,
placeholder services or empty language profiles. A language or service is added only when a real
consumer needs it.

Rationale: a starter that advertises unused surface teaches the wrong lesson and hides drift.

### II. Contracts First and Compatibility-Checked

Protobuf schemas are the single source of truth for client/BFF interfaces. Schemas MUST be
formatted, linted and checked for breaking changes against a declared baseline with pinned native
tooling. Generated code MUST be produced by the build graph and MUST fail a freshness check when it
diverges from its inputs. Wire-incompatible changes require a new package version, not an edit.

Rationale: generated clients are only trustworthy if their inputs are versioned and their outputs
cannot silently drift.

### III. Hermetic, Pinned, Reproducible

Every tool that can change a verdict (Bazel, compilers, protoc plugins, linters, prek, Spec Kit,
browsers used in tests) MUST be pinned to an exact version or immutable digest and recorded in a
committed lockfile or manifest. A fresh clone on a supported host MUST bootstrap with one
documented command and no developer-specific accounts, credentials, caches or cloud services.
Stable configuration MUST be declarative and versioned; ad hoc or console-only setup is not
acceptable.

Rationale: a template that only works on its author's machine is not a template.

### IV. One Policy, Thin Adapters, Local/CI Parity

Each quality rule has exactly one canonical definition. Local hooks (prek), the full local
verifier and required CI MUST invoke the same canonical tool at the same pinned version with the
same configuration. Language and build adapters translate file sets and invocation only; they MUST
NOT re-implement or fork policy. Fast local stages MAY run a subset, but a fast profile is never
evidence that the full gate passed, and required CI MUST independently enforce the full policy
because local hooks are bypassable.

Rationale: two definitions of one rule always diverge.

### V. Policy Cannot Weaken Itself

A proposal MUST NOT be able to authorize its own weakened thresholds, removed checks, widened
exclusions, new suppressions or replaced evaluators. Weakening MUST be detected and reported by
comparison against an independently trusted, pinned baseline, using an evaluator built from that
baseline rather than from the proposal. That evaluator does not authenticate the adapter or workflow
that invokes it, and documentation MUST say so. PR prose or in-branch approval files cannot authorize
themselves. Detection is not enforcement: blocking a proposal that removes the detecting job itself
requires a platform-side required check configured by the owner, and documentation MUST say so
rather than claim enforcement. This template MUST NOT reinstate the custom trusted-policy issuers removed from the
Relentless CI templates, require a GitHub App, or change live branch protection; it documents
deployable options and their limits for maintainers to adopt explicitly.

Rationale: a gate that its subject can edit is advisory.

### VI. Exact-Revision, Terminal Evidence

Completion claims MUST be backed by evidence bound to the exact source revision and policy
revision, from a terminal producer, covering the complete expected inventory of checks, tests and
matrix legs. Pending, skipped, cancelled, truncated, empty, stale or contradictory evidence MUST
NOT satisfy readiness, and readiness MUST be invalidated when its inputs change. A worker's own
assertion is distinct from an independent review receipt. Offline tests, local verification,
hosted CI, human approval and deployment are separate claims and are reported separately.

Rationale: a green exit code is not proof of completion.

### VII. Deterministic Guarantees vs Review Signals

Every check MUST be classified as either a deterministic guarantee (fails the build) or an
advisory architectural review signal (reported, never silently promoted or ignored). Static tools
cannot prove SOLID/DRY/YAGNI, contractual usefulness, live billing or live platform enforcement,
and documentation MUST NOT claim they do. Claims of live enforcement require platform readback.

Rationale: overstated guarantees are worse than none.

### VIII. Bounded, Measured, Actionable Feedback

Hooks, verifiers and tests MUST run with declared wall-clock, concurrency and resource bounds,
MUST NOT wait on stdin, and MUST own and clean up their process trees. Feedback-time and resource
budgets MUST be measured (cold and warm) and recorded, never asserted; thresholds MUST NOT be
weakened to meet a budget. Failures MUST exit nonzero with the named check, a bounded actionable
diagnostic and a rerun command usable by humans and repair loops. Hooks MUST NOT stage unrelated
work and MUST be safe under partial staging, renames, deletions and filenames with spaces.

Rationale: unbounded or opaque feedback is skipped, and skipped feedback is no feedback.

### IX. Public, Neutral, Provenance-Tracked

This repository is intended to become public. It MUST NOT contain private product code, domain
schemas, customer data, secrets, account/project identifiers, organization-specific configuration
or incident history from any reference. Patterns may be learned from references; implementations
are rebuilt from public specifications. Private references are named in tracked files only as "a
private reference implementation"; their identity, revisions, paths, metrics and inspection notes
stay in an untracked operator record. Every borrowed pattern, dependency and generated asset MUST
be recorded in a provenance and license inventory, and publication requires an explicit
public-release review.

Rationale: a starter is copied widely; anything private in it leaks widely.

### X. Smallest Viable Route, Optional Providers

Prefer the smallest mechanism that delivers the guarantee: shared versioned dependencies over
copies, copies over generators, and no new remote cache, build-execution service, multiworker
orchestrator, agent platform or cloud control plane for the reference. Cloud deployment is an
optional, provider-specific profile; when offered it MUST be fully declarative including bootstrap,
with CI/CD responsibilities and explicit operator gates. Optional worker conformance is a contract,
not a runtime.

Rationale: every component a template ships is a component every consumer maintains.

## Security and Identity Boundaries

- Test-mode identity MUST be visibly distinct from production authentication and MUST NOT be
  accepted by a production-configured BFF.
- The BFF MUST validate inputs at its boundary, map internal errors to typed contract errors
  without leaking internals, and propagate deadlines and cancellation to downstream calls.
- Evaluators that inspect proposals MUST NOT run proposal-controlled code with privileged
  credentials.
- Receipts and logs MUST NOT contain secret values or personal account identifiers. Commit
  attribution, execution principal and billing/provider identity are explicit configuration; a
  fallback MUST stay within pre-authorized policy and MUST NOT select another person's account.
- No action in this repository creates or publishes repositories, applies or destroys
  infrastructure, grants access, installs Apps, merges, or starts paid runs without an explicit,
  separately recorded operator gate.

## Development Workflow and Quality Gates

- Work follows Spec Kit: constitution → specification → research and plan → dependency-ordered
  tasks → implementation. Each implementation change traces to a task, a requirement and an
  acceptance witness in the acceptance/evidence matrix.
- Changes are small, single-concern and independently revertible. Tests that witness behavior are
  written before or with the behavior and include at least one negative (deliberately failing)
  case and its corrected counterpart.
- A change is done only when the full canonical verifier passes at the exact revision and the
  evidence record for that revision is complete and terminal.
- Spec 001 is the one spec of specs, and `specs/README.md` is its index. A change to a feature's status
  reconciles them in the same change.
- Upstream Relentless CI issues own their implementations; this repository consumes their outputs
  and MUST NOT duplicate active issue work.

## Governance

This constitution supersedes conflicting practice in this repository. Amendments are made by pull
request that updates this file, states the version bump and rationale, and updates dependent
specifications and checks in the same change. Versioning is semantic: MAJOR for removing or
redefining a principle, MINOR for adding a principle or materially expanding guidance, PATCH for
clarification. Reviews verify compliance with each principle; any justified exception is recorded
in the plan's Complexity Tracking table with its owner and expiry. Weakening a principle is itself a
policy change subject to Principle V.

**Version**: 1.1.0 | **Ratified**: 2026-10-28 | **Last Amended**: 2026-11-01
