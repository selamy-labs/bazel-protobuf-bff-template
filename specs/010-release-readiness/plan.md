# Implementation Plan: Public-release readiness and provisioning plan

**Branch**: `010-release-readiness` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

A provenance inventory by scope with a check, two scans over the whole export with distinct statuses, one declarative repository target validated offline with its ruleset disabled, a runbook, an executed review, and an external packet. Local and offline only. Nothing is published or applied.

## Technical Context

**Language/Version**: Bash for the scans and the inventory check, under the pinned runner; Go 1.27.2 only if a scan needs structure Bash cannot give
**Primary Dependencies**: pinned gitleaks for the generic scan's secret patterns; OpenTofu and the GitHub provider at pinned versions, fetched once into an owned cache; existing registry and runner
**Testing**: scratch repositories with planted defects, run as required tests; `tofu fmt`, `validate` and `test` with mocked providers
**Target Platform**: the supported Linux profile; public CI for the generic scan only
**Constraints**: the private marker file stays outside Git at mode 0600; no marker or locator with a hit in any output; no apply path; no network after the fetch phase
**Open at planning**: whether the pinned provider's schema can be fetched into an owned cache and used fully offline is probed first (R5), and the claim is narrowed to what that probe shows

## Constitution Check

| Principle | Result |
| --- | --- |
| III. Pinned | OpenTofu, the provider and the scan tools are pinned, and the fetch phase is separate from the offline phase |
| IV. One policy | The scans are checks in the registry, run by the one runner, with the generic one in CI |
| V. Policy cannot weaken itself | The scans add no exemption path; the marker file is read only, outside the tree |
| Governance: no live action | No create, apply, settings, Apps, access, merge or paid run |
| Public neutrality | The private scan prints labels, never markers; the generic scan carries no private term |

## Project Structure

```text
LICENSE
docs/provenance.md
docs/release-runbook.md
docs/public-release-review.md          executed and recorded
tools/release/neutrality.sh            generic scan, and --markers for the private scan
tools/release/inventory.sh             the provenance check
governance/github/                     one target, disabled ruleset, offline tests
quality/registry.textproto             the two scan checks and the inventory check
specs/010-release-readiness/
```

## Steps

1. Spec files; recheck upstream #31; re-read this plan's R1 against the then-current history.
2. Probe: OpenTofu pin, provider offline story, license metadata sources.
3. `LICENSE`, provenance table, the inventory check.
4. `neutrality.sh`, generic scan, whole-export coverage, `--markers` run, each with planted defect and clean control in a scratch repository, including a marker in a file name.
5. `governance/github/`: one target, disabled ruleset, fetch phase, then offline format, validate and mock tests.
6. `docs/release-runbook.md`: order, gates, the first-push refusal.
7. With inventory, governance, runbook and the image all complete, the private release scan at the final revision. Output labels and scope only, with the mapping retained outside the repository.
8. Execute `docs/public-release-review.md` at that same revision; record reviewer and revision. Freeze, gates, hold for independent acceptance.
9. The external release packet: identity, proposed ref, activation, the open workflow choice.

Stop there. Technical readiness can pass with release gates pending. Every later step is an operator action.

## Gates

| Action | Who | In this plan |
| --- | --- | --- |
| Create the repository, push, set visibility | Operator | Runbook only |
| Apply `governance/github/` | Operator | No apply path exists |
| Activate the ruleset | Operator, by the activation input, through the same configuration | Runbook only |
| Install Apps, grant access | Operator | Runbook only |
| Merge to `main` | Operator | Not done |
| Paid runs | Operator | None |

## Risks

| Risk | Handling |
| --- | --- |
| A marker reaches a log, including through a path or ref | Labels only, with planted markers in contents, names and metadata, and a test that their text never appears |
| An empty marker file reads as a pass | Distinct statuses; `not-run` is a named control |
| Public CI fails forever for lack of markers | The generic status stands alone |
| The scan flags its own test fixtures | Fixtures generated at test time, split like existing tokens |
| Offline validate needs a network | Two-phase fetch first. Narrow the claim only after that exact path is probed, in review |
| License metadata missing or wrong | Stays visible; a person decides. No guarantee is claimed |
| Configuration reused against existing state | Fresh isolated state, no imports, offline graph tests, operator plan review |

## Complexity Tracking

None.
