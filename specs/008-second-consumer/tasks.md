# Tasks: M7

One writer.

- [x] T001 Spec files; recheck upstream. **Needs** M6 accepted. **Parent** T071–T075. **Witness** Review
- [x] T002 Scratch consumer probe of every R1 coupling, `git_override` execution, and prek `repo`/`rev` on `file://`. **Needs** T001. **Parent** T072. **Witness** Research updated
- [x] T003 Host-named contract, registry and vendored paths; `reportPath` from configuration. **Needs** T002. **Parent** FR-003. **Witness** Unit, this template green
- [x] T004 Rule definitions in the module; host registry binds ids and paths, as plain data. **Needs** T003. **Parent** FR-003. **Witness** `runner_test`, real registry
- [x] T005 Evaluator test by build; refusal names the failure. **Needs** T003. **Parent** FR-004. **Witness** Real Git fixtures
- [x] T006 Remote hook definitions at the tree root. **Needs** T004. **Parent** FR-002, FR-002a. **Witness** prek against a `file://` remote
- [x] T007 `quality/RELEASING.md`; notes from the single rule catalog's diff; recorded tag mapping check. **Needs** T006. **Parent** T071, FR-006, FR-008. **Witness** Fixtures: added, changed, removed, weakened; a changed mapping refused
- [x] T008 Tag N, after the interface works, in a local bare remote. **Needs** T007. **Parent** T071, FR-010. **Witness** Commit recorded
- [x] T009 Smallest consumer: one Go package, its own policy; one canonical pin, and the deterministic step that syncs both adapters. **Needs** T008. **Parent** T072, FR-002. **Witness** Separate clone; both adapters resolve one SHA
- [x] T017 Two repairs from independent review at `a988f2a`: the pin check fails closed; the notes read only inside the selected tree. **Needs** T009. **Parent** FR-002, FR-006. **Witness** `pin_test`, `release_test`; each refused prek form valid under pinned prek; red before the code
- [x] T018 A new N, never a moved tag, and a new consumer anchor on it, proven on the real consumer. **Needs** T017, root's wiring review. **Parent** T071, FR-010. **Witness** Commit recorded; anchor controls
- [x] T010 Consumer anchor C0: exit 11 control, then hold for independent targeted review. No descendant uses C0 as a base before it clears. **Needs** T018. **Parent** FR-004. **Witness** Review
- [x] T011 Consumer verifier at N, on a descendant of the reviewed C0 still pinned to N, with `--base=C0`. C0 itself refuses, as its own base. **Needs** T010. **Parent** SC-001. **Witness** Receipt
- [x] T012 One real compatible rule improvement, visible in the catalog diff, tag N+1, upgrade by the pin. **Needs** T011. **Parent** T073, FR-007, SC-002. **Witness** Diff with mechanical effects apart; clean pass; deliberate defect caught; control restored
- [x] T013 Move a tag in the remote; SHA-pinned consumer unaffected. **Needs** T011. **Parent** FR-008. **Witness** Logs
- [x] T014 Language profile document. **Needs** T012. **Parent** T074. **Witness** Review
- [x] T015 Evidence, ledgers, freeze, gates, hold. The candidate states it is proposed as 1.0.0. **Needs** T014. **Parent** all, T075, FR-010. **Witness** Receipts, independent review
- [x] T016 Tag that exact reviewed candidate as 1.0.0, with no implementation change. **Needs** T015. **Parent** T075. **Witness** Tag names the reviewed commit

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
- T015 and T016: `e982dd7` passed its independent review, and `quality-v1.0.0` names exactly that commit. The consumer's final pin is `0050f87`.
- T018: after the wiring review, `quality-v0.2.1`, `19af61b`, once this repository's full verifier passed there. The new consumer is a separate repository with one commit, and that commit is C0. T010 holds at it. No descendant uses it as a base until it clears.
