# Tasks: M7

One writer.

| Task | After | Work | Parent | Witness |
| --- | --- | --- | --- | --- |
| [x] T001 | M6 accepted | Spec files; recheck upstream | T071–T075 | Review |
| [x] T002 | T001 | Scratch consumer probe of every R1 coupling, `git_override` execution, and prek `repo`/`rev` on `file://` | T072 | Research updated |
| [x] T003 | T002 | Host-named contract, registry and vendored paths; `reportPath` from configuration | FR-003 | Unit, this template green |
| [x] T004 | T003 | Rule definitions in the module; host registry binds ids and paths, as plain data | FR-003 | `runner_test`, real registry |
| [x] T005 | T003 | Evaluator test by build; refusal names the failure | FR-004 | Real Git fixtures |
| [x] T006 | T004 | Remote hook definitions at the tree root | FR-002, FR-002a | prek against a `file://` remote |
| [x] T007 | T006 | `quality/RELEASING.md`; notes from the single rule catalog's diff; recorded tag mapping check | T071, FR-006, FR-008 | Fixtures: added, changed, removed, weakened; a changed mapping refused |
| [x] T008 | T007 | Tag N, after the interface works, in a local bare remote | T071, FR-010 | Commit recorded |
| [x] T009 | T008 | Smallest consumer: one Go package, its own policy; one canonical pin, and the deterministic step that syncs both adapters | T072, FR-002 | Separate clone; both adapters resolve one SHA |
| [x] T017 | T009 | Two repairs from independent review at `a988f2a`: the pin check fails closed; the notes read only inside the selected tree | FR-002, FR-006 | `pin_test`, `release_test`; each refused prek form valid under pinned prek; red before the code |
| [x] T018 | T017, root's wiring review | A new N, never a moved tag, and a new consumer anchor on it, proven on the real consumer | T071, FR-010 | Commit recorded; anchor controls |
| [x] T010 | T018 | Consumer anchor C0: exit 11 control, then hold for independent targeted review. No descendant uses C0 as a base before it clears | FR-004 | Review |
| [x] T011 | T010 | Consumer verifier at N, on a descendant of the reviewed C0 still pinned to N, with `--base=C0`. C0 itself refuses, as its own base | SC-001 | Receipt |
| [x] T012 | T011 | One real compatible rule improvement, visible in the catalog diff, tag N+1, upgrade by the pin | T073, FR-007, SC-002 | Diff with mechanical effects apart; clean pass; deliberate defect caught; control restored |
| [x] T013 | T011 | Move a tag in the remote; SHA-pinned consumer unaffected | FR-008 | Logs |
| [x] T014 | T012 | Language profile document | T074 | Review |
| [ ] T015 | T014 | Evidence, ledgers, freeze, gates, hold. The candidate states it is proposed as 1.0.0 | all, T075, FR-010 | Receipts, independent review |
| [ ] T016 | T015 | Tag that exact reviewed candidate as 1.0.0, with no implementation change | T075 | Tag names the reviewed commit |

## Execution record

- T001: `af77ec6`. T002: `8957aa3`, results in R7.
- T003 to T006: `a988f2a`. The first real consumer commit found that `quality-pin` could not see its files in a staged snapshot: `14d3199`.
- T007: `ee82490`. Module-wide analysis then failed on one target, `8626b63`, and the coverage floor refused the untested release command, `8f66f7a`. Both failed full runs are kept outside this repository, and earn nothing.
- T008: `quality-v0.2.0`, `8f66f7a`, after this repository's full verifier passed there with `--base=e3b3b34`.
- T009: the consumer, in its own repository. Its first commit is C0.
- T010: was to hold at C0 for independent targeted review. T011 needs a descendant of the reviewed C0.
- Review of `a988f2a` found two defects that `quality-v0.2.0` carries, so that tag and its C0 are withdrawn as a baseline. The tag and its record stay as they are. T017 repairs both, and T018 replaces T008 and T009's result.
- T017: `8b57e2c`, `b71f78f`.
- T010: C0 `8b76a2f` cleared by independent targeted review. T011: C1 `05884b1`.
- T012: `quality-v0.3.0`, `9284009`, and the consumer's `8c9b3aa`. T013: same verdict with the tag moved, and the fixture restored. T014: the language profile.
- T015: the candidate carries 1.0.0 as a proposal, so nothing but the tag changes after review. The consumer pins it before the final review, and the evidence that names both follows in a later commit, because a file cannot name the commit that contains it.
- T018: after the wiring review, `quality-v0.2.1`, `19af61b`, once this repository's full verifier passed there. The new consumer is a separate repository with one commit, and that commit is C0. T010 holds at it. No descendant uses it as a base until it clears.
