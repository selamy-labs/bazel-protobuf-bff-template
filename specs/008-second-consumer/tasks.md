# Tasks: M7

One writer.

| Task | After | Work | Parent | Witness |
| --- | --- | --- | --- | --- |
| T001 | M6 accepted | Spec files; recheck upstream | T071–T075 | Review |
| T002 | T001 | Done, R7. Scratch consumer probe of every R1 coupling, `git_override` execution, and prek `repo`/`rev` on `file://` | T072 | Research updated |
| T003 | T002 | Host-named contract, registry and vendored paths; `reportPath` from configuration | FR-003 | Unit, this template green |
| T004 | T003 | Rule definitions in the module; host registry binds ids and paths, as plain data | FR-003 | `runner_test`, real registry |
| T005 | T003 | Evaluator test by build; refusal names the failure | FR-004 | Real Git fixtures |
| T006 | T004 | Remote hook definitions at the tree root | FR-002, FR-002a | prek against a `file://` remote |
| T007 | T006 | `quality/RELEASING.md`; notes from the single rule catalog's diff; recorded tag mapping check | T071, FR-006, FR-008 | Fixtures: added, changed, removed, weakened; a changed mapping refused |
| T008 | T007 | Tag N, after the interface works, in a local bare remote | T071, FR-010 | Commit recorded |
| T009 | T008 | Smallest consumer: one Go package, its own policy; one canonical pin, and the deterministic step that syncs both adapters | T072, FR-002 | Separate clone; both adapters resolve one SHA |
| T010 | T009 | Consumer anchor C0: exit 11 control, then hold for independent targeted review. No descendant uses C0 as a base before it clears | FR-004 | Review |
| T011 | T010 | Consumer verifier at N, on a descendant of the reviewed C0 still pinned to N, with `--base=C0`. C0 itself refuses, as its own base | SC-001 | Receipt |
| T012 | T011 | One real compatible rule improvement, visible in the catalog diff, tag N+1, upgrade by the pin | T073, FR-007, SC-002 | Diff with mechanical effects apart; clean pass; deliberate defect caught; control restored |
| T013 | T011 | Move a tag in the remote; SHA-pinned consumer unaffected | FR-008 | Logs |
| T014 | T012 | Language profile document | T074 | Review |
| T015 | T014 | Evidence, ledgers, freeze, gates, hold. The candidate states it is proposed as 1.0.0 | all, T075, FR-010 | Receipts, independent review |
| T016 | T015 | Tag that exact reviewed candidate as 1.0.0, with no implementation change | T075 | Tag names the reviewed commit |
