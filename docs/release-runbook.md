# Release runbook

What happens, in order, when this repository leaves its local-only state. Every step marked **gate** is an
operator's action outside this repository, recorded in the external release packet. Nothing here is
automated, and nothing below has run.

| Step | Who | What | Proof |
| --- | --- | --- | --- |
| 0. Readiness | Author, then an independent reviewer | `tools/verify.sh --base=<accepted commit>` passes at the candidate; the private release scan ran at that exact revision with the operator's marker file and states literal absence, or each finding has a recorded decision; `docs/public-release-review.md` is executed at that revision; the inventory's unresolved licenses each have a decision | The receipt, the scan result by label, the recorded review row |
| 1. Identity | Operator, **gate** | The configured commit identity and the repository name, owner and visibility are approved | The packet |
| 2. Create | Operator, **gate** | The repository is created from `governance/github/` by a reviewed `tofu plan` and an operator's `tofu apply`, with the ruleset declared and disabled | The plan, read before apply |
| 3. First push | Operator, **gate** | The candidate is pushed as `main` | The pushed commit equals the reviewed candidate |
| 4. First run | Hosted CI | The first run has no policy base, so `floors-from-base` refuses and the aggregate is not green. That is the expected result, and it is not a defect: a repository's first commit cannot be its own base, and no self-baseline is introduced to make it green | The run's receipt shows exit 11 by name |
| 5. First base | Operator, **gate** | The operator names the reviewed candidate as the policy base for the next proposal, in the hook and CI configuration, by its full SHA | The named base in configuration |
| 6. First green | Hosted CI | A proposal on top of the candidate passes with that base | The aggregate's receipt |
| 7. Activate | Operator, **gate** | The ruleset's activation input is changed in `governance/github/` and applied through the same configuration. No setting is changed by hand | The plan shows only the enforcement change |

## What this runbook does not do

- It does not create, push, apply, activate, grant access, install Apps or start paid runs.
- It does not decide the identity: the packet lists it as pending until an operator approves it.
- It does not close the proposal-controlled workflow gap (spec 007 R5): a required check configured by the owner
  is what prevents a proposal from removing the job that judges it. The ruleset declares that; applying it is step 7.
- It does not make hosted execution evidence: steps 4 and 6 are claims only once their receipts exist.
