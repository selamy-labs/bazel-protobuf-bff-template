# Feature Specification: Public-release readiness and provisioning plan

**Feature Branch**: `010-release-readiness`
**Created**: 2026-11-01
**Status**: Implemented and tested locally. Current status is in the [index](../README.md)
**Input**: M8. Parent: spec 001 US7, FR-032, FR-033, SC-009; tasks T076 to T080; upstream #31. Name, public visibility and the MIT license are already approved. Closed throughout: creation, publication, settings, Apps, access, apply, destroy, merge, paid runs. Lands after M8a (spec 009), so that its inventory and scans cover the image.

## Clarifications

### Session 2026-11-01

- Q: Does public development wait for the whole quality baseline or a stable release? → A: No. Public development of the reviewed generic core is authorized by the human direction of 2026-11-01, after the existing disclosure, provenance, licensing and publication checks; it is not a blanket approval, and the operator gates for creation, push, settings, apply and activation stay recorded gates.
- Q: Which status check does the ruleset require? → A: The workflow's aggregate job, `gate`, as `tools/ci/policy.json` names it. The configuration had said `aggregate`, which no run reports; the governance check now relates the three names and refuses a mismatch.
- Q: What becomes of the packet's pending items? → A: Each becomes a reviewed concrete decision before publication: the commit identity, the holder line and every one of the 15 unresolved licenses. None is dropped silently.
- Q: Where are artifacts hosted, and where does deployment run? → A: GitHub hosts the public CI, the repository-linked container package and the tag-bound downloadable artifacts, published later by a scoped workflow token on trusted source once permitted; GitHub Pages may serve the static frontend. Deployment is not GitHub Actions: continuous deployment runs on GCP Cloud Build into a dedicated BFF-owned project (spec 012), and the backend actually runs there.
- Q: Which templates and runners? → A: The organization's public `.github` defaults are inherited, not copied; standard GitHub-hosted Linux runners only, measured from real runs after publication.

## User Scenarios & Testing _(mandatory)_

### User Story 1 - A reviewer can see where everything came from (Priority: P1)

A reviewer opens one inventory and finds, for each vendored, adapted or generated path and each pinned dependency, its source, revision and license, in the scope it belongs to.

**Why this priority**: nothing can be published with an unaccounted third-party path.

**Independent Test**: the inventory check runs over the tracked tree, and a planted unlisted path fails it.

**Acceptance Scenarios**:

1. **Given** the tracked tree, **When** the inventory check runs, **Then** every third-party path has an entry with source, revision and license, in its scope.
2. **Given** an unknown license, **When** the inventory is read, **Then** it is shown as unresolved, and is never read as permissive.
3. **Given** the built image from spec 009, **When** the built-artifact scope is read, **Then** its contents are listed.

### User Story 2 - Nothing private ships (Priority: P1)

A reviewer runs the generic scan over the whole export, and the operator runs the private marker scan with a marker file that never enters the repository, and both report scope and result without printing a marker.

**Why this priority**: a private identifier in history is the one defect a publication cannot take back.

**Independent Test**: planted defects in a scratch repository, in contents, names, history and metadata, each found, and none echoed.

**Acceptance Scenarios**:

1. **Given** a clean export, **When** both scans run, **Then** each states its scope and literal absence.
2. **Given** a marker in a file name, a deleted blob, a commit message, a ref or an identity, **When** the private scan runs, **Then** it is found, and its text appears in no output or locator.
3. **Given** no marker file, **When** the scans run in public CI, **Then** the generic result stands alone and the private status is `not-run`, which is never a pass.

### User Story 3 - A new repository can be provisioned from a reviewed plan (Priority: P2)

An operator reads declarative settings for one new repository, validated offline against pinned tools, and applies them later by hand. Nothing here applies them.

**Why this priority**: the settings must exist and be right before the first push, and they must not act on their own.

**Independent Test**: format, validate and mock tests pass with networking and credentials unavailable, after one fetch phase.

**Acceptance Scenarios**:

1. **Given** the fetched provider and tools, **When** the offline tests run with no network and no credentials, **Then** they pass.
2. **Given** a second target, an import block or a changed owner, **When** the tests run, **Then** they fail.
3. **Given** the ruleset as declared, **When** its enforcement is read, **Then** it is disabled until an operator changes the activation input and applies.

### User Story 4 - The first hosted run is planned (Priority: P2)

A reader of the runbook sees the order of creation, first push, first green and ruleset activation, each operator gate, and what the first push's refusal means.

**Why this priority**: the first push has no base, and a runbook that hides that would invite a self-baseline.

**Independent Test**: review of the runbook against the gates in spec 001 and the refusal in spec 007.

**Acceptance Scenarios**:

1. **Given** the runbook, **When** a reader follows it, **Then** no step bypasses a gate, and the first push's refusal is expected and explained.

### User Story 5 - Public development begins on the reviewed core (Priority: P1)

The reviewed generic core and its synthetic example are developed in public: one frozen candidate is exported as `main`, the README says exactly what is implemented, independently accepted, hosted-unverified and unfinished, and the open quality work is visible as open work, with no production claim.

**Why this priority**: the human direction of 2026-11-01 asks for public development on standard hosted capacity, and every publication check that exists still applies.

**Independent Test**: the candidate's revision, tree, export ref and holder are named in the packet; the private scan at that exact candidate reports the 20 markers absent by opaque output; the README's claims match the evidence records; the governance configuration's required check names the workflow's gate.

**Acceptance Scenarios**:

1. **Given** the candidate, **When** the packet is read, **Then** the proposed ref, the commit id, the tree, the holder and the exported scope are named, and the pending items are reviewed decisions.
2. **Given** the README, **When** a reader compares it with the evidence records, **Then** every milestone's state is one of implemented, independently accepted at a revision, hosted-unverified or unfinished, with no production or hosted claim.
3. **Given** the export, **When** it is compared with the working tree, **Then** it holds the generic core, the synthetic application and the public documents only: no private backups, no unpublished working files, no operator records.

### User Story 6 - The image and the artifacts are distributed from the public repository (Priority: P2)

After the first hosted gate is green, a tag on the trusted source publishes the exact image spec 009 built, by digest, to the repository-linked container package, and the release artifacts (the OCI tarball, the receipt, the inventory, the notices) as tag-bound downloads with their hashes; anyone can pull the image by digest and download the artifacts without credentials.

**Why this priority**: the hosted example (spec 012) deploys by digest from this package; it needs no rebuild and no private registry.

**Independent Test**: offline, the publishing workflow is pinned and validated against the CI-parity policy; after an authorized publication, an anonymous digest-pinned pull and the asset hashes are verified by a credentialless job.

**Acceptance Scenarios**:

1. **Given** a tag on the trusted branch, **When** the workflow runs, **Then** it builds once, publishes that output by digest with the job's own scoped token, and no fork or `workflow_run` artifact enters.
2. **Given** a published package, **When** it is pulled anonymously by digest, **Then** the digest equals the receipt's image measurement; the package's public visibility is read back, not assumed.

### Edge Cases

- A marker file that exists and is empty: the private status is `not-run`, never `passed`.
- A marker that is also a common word: hits are shown by run-local label, and no hash of it is presented as confidential.
- A third-party path with no license metadata at all: listed as unresolved. The inventory is complete when every path has an entry; it is not a legal clearance.
- The export includes the image from spec 009 and anything landed after the last scan: the final scan runs at the frozen revision, and a later change reopens it.

## Requirements _(mandatory)_

### Functional Requirements

- **FR-001**: A `LICENSE` file MUST hold the approved MIT text.
- **FR-002**: `docs/provenance.md` MUST cover every vendored, adapted and generated path. A check MUST fail on a tracked third-party path with no entry.
- **FR-003**: The inventory MUST distinguish three scopes: the Git source export, built artifacts that bundle or link dependencies, and tools used only to build or test. For built artifacts, including the image from spec 009, it MUST enumerate the runtime dependencies actually included, with their notice sources. It MUST list code-generation inputs, tool pins and output families, even though outputs are untracked. Debian packages and transitive npm packages, which are open gaps, MUST be enumerated.
- **FR-004**: An unknown or missing license MUST stay an explicit unresolved finding, and MUST NOT satisfy a claim of a complete inventory or read as permissive. Completeness is separate from legal compatibility: a known restrictive license is listed, and is not an automatic violation. The inventory is metadata for review, and is no legal or compliance guarantee.
- **FR-005**: A generic neutrality scan MUST run in the repository, with no private term in it: secret patterns, cloud identifiers, personal paths, and email domains outside an allowlist.
- **FR-006**: An operator-private scan MUST read its markers from a file outside the repository and scan the whole export: tracked contents, all reachable blobs, paths, commit messages, author and committer fields, and ref names. No output may print a marker, or any locator that contains one: a path, ref or identity with a hit is shown redacted or by an opaque run-local label, with the mapping kept privately. A marker MUST NOT enter a tracked file, a receipt or a public log.
- **FR-006a**: Its result MUST state literal absence and the exact scope scanned. No hash of a marker is presented as confidential: a short unsalted hash of a guessable word protects nothing, so displayed hits use run-local labels.
- **FR-006b**: Pull-request branches must be a single squashed commit (`ahead_count <= 1` when `GITHUB_EVENT_NAME=pull_request`) with neutral author/committer identity; this enforcement is intended so GitHub squash-merges preserve the commit's neutral author rather than falling back to the merger's personal email (pending live witness BFF-RM2-03).
- **FR-007**: The generic scan and the private release scan MUST have distinct statuses. Generic runs in public CI, and passes or fails on its own. Private is `not-run` without a marker file, or with an empty one, and a public CI run is not failed by that. `not-run` is never a private clearance, and release review requires a recorded private result.
- **FR-008**: The configured commit identity and a concrete proposed release ref MUST be listed in an external release packet. Technical and local readiness MUST NOT wait for publication decisions. The pending public identity is a listed release gate: the generic scan neither approves it nor depends on an operator-private file. History is preserved unless a scan finding or an explicit identity refusal justifies a rewrite, and none is planned. A rewrite would update references to mapped commits here, and not external dependency commits, action pins or checksums.
- **FR-009**: `governance/github/` MUST declare one configured target and its initial ruleset, with pinned tools, in fresh isolated state, with no imports or adoption. Offline tests MUST pin the allowed owner, name and resource graph. Format, validate and mock tests MUST pass with networking and credentials unavailable, after pinned tools and the provider schema are fetched into an owned cache. No apply path exists in CI or a script, and an operator reviews the plan before any apply.
- **FR-009a**: The initial ruleset MUST be declared disabled, until an activation input is changed and applied by the operator through the same configuration. No manual setting is a workaround.
- **FR-009b**: The ruleset's required status check MUST name the workflow's aggregate job as the CI-parity policy names it, and the governance check MUST relate the three and fail on a mismatch, so that activation can never require a check no run reports.
- **FR-010**: The runbook MUST order creation, first push, first green and ruleset activation, name each operator gate, and cover the first push's refusal.
- **FR-011**: `docs/public-release-review.md` MUST be executed and recorded with reviewer and revision.
- **FR-012**: Public development MUST start from one frozen exact candidate: its revision, tree, export ref and holder named in the packet, its private scan reporting the markers absent by opaque output, its README accurate about implemented, independently accepted, hosted-unverified and unfinished states, and its export holding only the generic core, the synthetic application and the public documents. Each pending packet item MUST be a reviewed decision, and the inherited organization defaults for issue and pull-request templates MUST be used unless shown insufficient. Hosted runners MUST be the standard hosted Linux runners, and their behavior MUST be measured from real runs, not claimed.

### Key Entities

- **Inventory entry**: a path or pin, its scope (source export, built artifact, tool), source, revision and license, or an explicit unresolved mark.
- **Marker file**: the operator's private terms, outside the repository, read only by the private scan. Its contents never enter a tracked file, receipt or public log.
- **Scan result**: a status (`passed`, `failed`, `not-run`), the exact scope scanned, and hits as run-local labels.
- **Release packet**: the external record of the configured identity, the proposed release ref, the activation input and the open workflow choice.

## Success Criteria _(mandatory)_

### Measurable Outcomes

- **SC-001**: Each scan has a planted defect caught and a clean control, in a scratch repository.
- **SC-002**: Provenance covers 100% of third-party paths.
- **SC-003**: The generic scan passes over the whole export, or each finding has a decision. The private result at the release candidate states literal absence and scope, or each finding has a decision.
- **SC-004**: After the fetch phase, `tofu fmt`, `validate` and mock tests pass with no network and no credentials.
- **SC-005**: Technical readiness can pass with release gates pending, and it says so. It claims no public approval.
- **FR-013**: Distribution MUST use the public repository's own hosting: the repository-linked container package and tag-bound release assets, published by a workflow on the trusted source with its own narrowly scoped token, building once and publishing the exact output by digest; CI and fork jobs stay credentialless and without a private cache; no larger runner, no paid storage setting, no deployment from GitHub Actions. A second workflow MUST be declared in the CI-parity policy before it exists.
- **FR-014**: After an authorized publication, a credentialless job MUST pull the image by digest anonymously and verify the asset hashes, and the package's public visibility MUST be read back; a declared workflow is never live proof. No stable-release claim follows from a published pre-release.
- **SC-007**: A published pre-release is verified by an anonymous digest-pinned pull and by asset hashes, from a credentialless job, with the package's visibility read back.
- **SC-006**: The publication candidate is one named revision: its full verifier, both scans and the governance check pass at that exact revision, and the README's milestone states match the evidence records row by row.

## Assumptions

- The MIT text, the name and public visibility are decided. Nothing here revisits them.
- The private marker file exists outside Git with mode 0600, as the operator already provided it.
- Spec 009's image is complete before the final scan, so that it is in scope.

## Limits

- This feature publishes nothing. Readiness is a recorded review, and publication stays an operator action outside this repository.
- A scan finds what it knows. A marker list is only as complete as the operator's record, and the human review stays.
- A required `gate` status does not close the proposal-controlled workflow gap (spec 007 R5). The required-workflow or ruleset choice stays explicitly open.
- Provisioning and image packaging are separate: nothing here applies anything; publishing the image is the declared workflow of US6, run only on the trusted source after the first hosted gate, and deployment is spec 012's, on GCP.
- The configuration targets one repository. That is a limit on what it declares, tested offline. It is not a proof about what an operator's state or provider could address.
