# Public-Release Review Boundary

This repository is meant to become public, but it is not public yet. Nothing may be published —
no remote created, pushed, made visible or marked as a template — until every item below is
checked and recorded with the reviewer and the exact revision. Publication is an operator action
and is not something this repository automates.

## Checklist

- [ ] The repository name, owner, visibility and license are approved by the owner (spec A5,
      research R-14).
- [ ] A `LICENSE` file is present, and `docs/provenance.md` covers every vendored, adapted and
      generated path (T077).
- [ ] The neutrality scan passes over the full history (T076). It finds none of the following:
  - private product names, domain service names or schemas from any reference
  - customer or user data
  - cloud project IDs or numbers, organization IDs, service-account emails, runner pool names, or
    cache endpoints and instance names
  - personal accounts or emails, other than the commit author identity the owner approves
  - incident history or transcripts
- [ ] A secret scan (gitleaks) passes over the full history.
- [ ] The following contain no organization-specific assumptions: CI references only
      GitHub-hosted runners or generic labels, there are no deployment values, and there is no
      required account.
- [ ] Docs make no claim of live enforcement (branch protection, billing, deployment) without
      platform readback.
- [ ] A fresh clone on a clean host passes `tools/verify.sh --base=<commit>` with no credentials (T014). A rewritten public history changes every SHA, so re-anchor first (spec 007 R6).
- [ ] A repository's first hosted push has no base. Decide how the first CI run is handled (spec 007 R6).
- [ ] The review result is recorded below with the revision and the reviewer.

## Hard rule for private references

Tracked files refer to a private reference only as "a private reference implementation". Its name,
URL, revision, file paths, spec numbers, task counts, metrics and inspection notes stay in the
untracked operator record (`.private/`, excluded through `.git/info/exclude`). This is a rule, not
a reviewer option. A violation found in history must be removed before publication.

## M0 pre-check (2026-10-28)

The checks below were run before the M0 commit:

- A grep over the tracked files found no private repository or organization name and no private
  revision.
- No personal account name appears.
- The only author identity is the configured commit author.
- The Relentless CI and Spec Kit references are public, MIT-licensed sources.

| Date | Revision | Reviewer | Result |
| --- | --- | --- | --- |
| 2026-11-01 | `00a4c6b` | the author, not an independent reviewer | Executed at this revision, row by row, below. Technical readiness stands with the release gates pending: the owner identity, the LICENSE holder line, 17 unresolved licenses, and the first-push base decision are the operator's |

## Execution at `00a4c6b`

| Row | State | Evidence |
| --- | --- | --- |
| Name, owner, visibility, license approved | pending the operator: name, visibility and MIT are approved; the owner identity and the LICENSE holder line await approval | the external release packet |
| LICENSE present, provenance covers every path | yes; 17 licenses explicitly unresolved, each a listed finding | the `inventory` check at `full`; [m8](evidence/m8.md) |
| Neutrality scan over the full history | the generic scan passes with decided findings by object id; the private scan found 20 markers literally absent over the whole repository and over the export clone, by label only | [m8](evidence/m8.md) |
| Secret scan over the full history | the pinned gitleaks 8.30.1 over 115 commits with the repository's configuration, redacted: no leaks found; author-run | the retained run outside the repository |
| No organization-specific assumptions in CI | the workflow names only GitHub-hosted runners, carries no deployment values and needs no account; hosted CI has never run | `//:ci_parity_test`, [capability states](quality/capability-states.md) |
| No live-enforcement claims | none; each such capability is `no` in capability states, and the runbook names every gate | [capability states](quality/capability-states.md), [runbook](release-runbook.md) |
| Fresh clone passes with no credentials | author runs only, at weakened isolation; independent full-sandbox runs exist at M7's and M8a's revisions, not at this one | the evidence records |
| First hosted push has no base | decided: the first run is expected to exit 11, and the operator names the reviewed candidate as the base afterwards | [runbook](release-runbook.md), steps 4 and 5 |
| Result recorded with revision and reviewer | this row | this file |

## License decisions (2026-11-01)

The inventory's unresolved findings at the executed review above were decided afterwards, each from the exact
artifact: the protovalidate generated Go module from its fetched `LICENSE` (Apache-2.0), twelve Debian browser
packages from a complete read of their prose copyright files, and the two GCC runtimes from the matching Debian
source copyright record, since their fetched doc directories are symlinks to a package that is not fetched. The
decisions, with evidence hashes, scope and reviewer, are in `tools/release/license_decisions.tsv` and appear in
the generated table as `reviewed:` cells, which the `inventory` check validates and counts. A decision names a
license family as read; it is no legal guarantee, no compatibility judgment, and no clearance to redistribute the
test-time browser packages, which are in neither the Git source nor the image. The release review still decides
publication with these rows in view.

