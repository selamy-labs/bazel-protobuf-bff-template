# Agent Guide

- Follow the [constitution](.specify/memory/constitution.md). Work moves through Spec Kit in this
  order: specify, plan, tasks, implement. The skills are in `.claude/skills/speckit-*`.
- Every change traces to a task in `specs/*/tasks.md` and to a witness in that spec's
  `acceptance-matrix.md`.
- This repository is intended to be public. Never add private product code, schemas, data,
  identities, account or project identifiers, organization configuration, or incident history.
  Record every third-party item in `docs/provenance.md`.
  Refer to a private reference only as "a private reference implementation". Its details stay in
  the untracked `.private/` record.
- Upstream contracts are owned upstream. Consume them; do not re-implement them here:
  - the shared hook stage contract (selamy-labs/relentless-ci-python#25,
    selamy-labs/relentless-ci-typescript#20)
  - policy-diff (#26), the evidence validator (#27), the contract analyzer (#28) and the worker
    profile (#29)
  This repository implements only its own Bazel adapter. Do not
  reinstate trusted-policy issuers, and do not change branch protection.
- Never do any of the following without a recorded operator gate:
  - create or publish a remote
  - apply or destroy infrastructure
  - merge, grant access or install Apps
  - start paid runs
- Use CodeGraph (`codegraph status`/`explore`) before broad searches once source exists. Keep
  `.codegraph/` in `.git/info/exclude`.
- Verify the Spec Kit scaffold with `tools/speckit/bootstrap.sh verify` after any Spec Kit change.
