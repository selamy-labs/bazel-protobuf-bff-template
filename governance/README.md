# Governance Configuration

This directory contains the OpenTofu declarations for repository settings and branch rulesets.

## Declared vs Live Status

- `governance/github/main.tf` declares the repository's desired target state, including rebase-only merge requirements, linear history, and commit author/committer email patterns.
- **Commit metadata rules**: The `committer_email_pattern` and `commit_author_email_pattern` blocks in the ruleset declaration represent desired target policy. They are **declared, not enforced live** on GitHub ruleset 26410174 (which currently enforces branch deletion, non-fast-forward, pull request, and required status checks).
- **Authoritative enforcement**: Commit author and committer neutrality and permitted bot merger identities are enforced in local pre-push checks and GitHub Actions CI by `tools/release/neutrality.sh`.
- **No unapproved apply**: In accordance with repository principles and the release runbook (`docs/release-runbook.md`), this repository never applies infrastructure or rulesets directly. Any updates to live GitHub rulesets require an explicit operator gate.
