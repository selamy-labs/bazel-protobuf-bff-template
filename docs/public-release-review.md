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
- [ ] A fresh clone on a clean host passes `//:verify` with no credentials (T014).
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
| — | — | — | not yet reviewed |
