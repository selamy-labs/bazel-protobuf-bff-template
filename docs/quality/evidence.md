# What the evidence claims

Each kind of evidence below is a separate claim, and none implies another. A receipt records one run. It is
not a readiness verdict, which belongs to upstream #27.

| Claim | What backs it | State here |
| --- | --- | --- |
| Offline test | A named test target with a fixture | Real, per target |
| Local verification | A receipt from `tools/verify.sh --base=<commit>`, labeled with its isolation and `claim_layer: local` | Real, per revision, in the [evidence records](../evidence/) |
| Independent local review | A reviewer's own run of an exact revision | Named per revision in those records |
| Hosted CI | A workflow run with one receipt per leg, `claim_layer: hosted-ci` | **Never run.** No remote exists |
| Required status | A hosting platform refusing a merge without it | **No.** Not configured |
| Human approval | A person's recorded decision | Not something a receipt can say |
| Deployment and release | A deployed artifact | Not claimed |

## Reading a receipt

- `result: PASSED` means every expected check of this stage or leg passed. A leg's receipt says nothing about another leg.
- `tested_tree`, `worktree_dirty` and `test_results_cached` say what ran. A dirty tree or a cached result is a weaker claim.
- `base_revision` and `base_source` say what the tree was compared with, and how that was chosen: `argument`, `hook-config` or `ci-event`. **Neither says
  anyone reviewed that commit.** `base_policy_digest` is the base's own, by its own registry.
- `claim_layer` is a label from the environment, and it authenticates nothing.
- A check with a summary keeps `measurements`, such as each coverage profile, its metric and its floor. `unmeasured` there means no data exists. It is never met
  and never not applicable.

## What each coverage number is

| Metric | Meaning | Not |
| --- | --- | --- |
| `statements` | Go statements covered, from the raw profile, the number Go itself reports | A line count |
| `block-span-lines` | Lines inside Go coverage blocks, including comments there, from rules_go's LCOV. Used only for code that runs as a subprocess of a test | Statements |
| `compiled-js-lines` | Lines of compiled JavaScript, from Node's own runner, mapped to a `.ts` source by file name | TypeScript line numbers |
| `unmeasured` | No data: browser code, and Go branches | Covered |

Excludes in `quality/coverage_floors.json` carry a listed reason. The evaluator enforces that a listed reason is present. Whether the reason is true is a human review.

## What a base does not bind

The evaluator comes from the base, so a proposal cannot replace it. The workflow that calls it comes from the proposal on a `pull_request` run, so a proposal
could delete that job. Closing this needs a required workflow or ruleset, which is an operator decision for M8, or upstream #26. Relocating a file into a path an existing exclude already
matches is also not detected, so excludes stay narrow.

## Open gaps

| Gap | Owner |
| --- | --- |
| Shared policy-diff and trusted baseline (parent T067) | Blocked on an upstream #26 release |
| Evidence validator, independent review receipts and readiness; spec 001 FR-023 and SC-007 (parent T068) | Blocked on an upstream #27 release |
| A gate that reads receipt contents | The same. `gate` holds no permissions and reads leg results only |
| A repository's first hosted push has no base | M8 |
