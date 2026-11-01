# Acceptance / Evidence Matrix

Each requirement maps to a witness (a test or fixture), the **evidence class** and the **claim
layer** at which it is proven.

Evidence classes:

- **G**: deterministic guarantee. It fails the build.
- **S**: review signal. It is reported but needs human architectural review.
- **R**: runtime or platform evidence. It is outside CI and needs live readback.

Claim layers:

- **off**: offline test
- **loc**: local full verifier
- **ci**: hosted CI at the exact revision
- **hum**: human approval
- **dep**: deployment

Every **G** witness has a defect case that must fail and a corrected counterpart that must pass.

| Req | Witness (task) | Defect case → expected | Class | Layer | Milestone |
| --- | --- | --- | --- | --- | --- |
| FR-001 | e2e `web/e2e/notes.spec.ts` (T032) | client bypasses generated stub → dependency rule fails (T038) | G | loc, ci | M4 |
| FR-002 | e2e per RPC (T032) + review checklist (T025); #28 profile on adoption | RPC with no browser caller → no e2e scenario → review rejects | G (e2e per RPC) / S (surface review) | loc, ci, hum | M2/M4 |
| FR-003 | handler + property tests (T027, T028) | empty title accepted → test fails | G | off | M3 |
| FR-004 | handler tests (T027) | internal error text leaks → test fails | G | off | M3 |
| FR-005 | deadline/cancel tests (T027) | port ignores ctx → test times out → fails | G | off | M3 |
| FR-006 | startup tests (T029), cookie-only e2e (T032) | test identity accepted in production, or cookie-only request authenticated → fails | G | off, loc | M3/M4 |
| FR-007 | dependency-rules test (T038) | `bff` imports `web` or testonly from cmd → fails | G | loc, ci | M3 |
| FR-008 | e2e 4 outcomes + defect witness (T032, T041) | not_found mapped to internal → e2e fails | G | loc, ci | M4 |
| FR-009 | breaking probes, baseline tests (T016, T017, T026) | field renumber → fails; missing baseline → exit 4 | G | off, ci | M2 |
| FR-010 | build-time codegen; `generated-fresh` (T022, T023) | stale committed doc → fails with regen command | G | loc, ci | M2 |
| FR-011 | #28 profile on adoption (T025); interim review checklist | unused RPC → reported, not failed | S | hum; ci after adoption | M2 / adoption |
| FR-012 | `docs/api-style.md` ledger (T019) | lint ignore not in ledger → `suppression-ban` fails | G (ledger link) / S (rule choice) | loc | M2 |
| FR-013 | CI parity test (T066) | CI step with inline lint command, or shard union ≠ full → fails | G | ci | M6 |
| FR-014 | registry validation (T045) | adapter defines its own threshold → fails | G | off | M5 |
| FR-015 | hook fixtures (T043) | secret staged → commit rejected; failing pushed commit → push rejected; corrected passes | G | off | M5 |
| FR-016 | partial-staging/untracked/rename/space fixtures (T043) | unstaged bytes changed, or untracked file changes verdict → fails | G | off | M5 |
| FR-017 | runner tests (T044); 500-file fixture | stdin reader hangs → `TIMED_OUT`; `setsid` child reaped; Bazel interrupted | G | off | M5 |
| FR-018 | diagnostic-format tests (T044) | missing rerun command → fails | G | off | M5 |
| FR-019 | suppression fixtures (T047), base-built evaluator fixtures (T060), floors (T063); #26 on adoption (T067) | lowered floor, new `//nolint`, or edited evaluator → still fails | G (bans, floors via base evaluator) / platform-required check for job removal (R) | ci; platform readback | M5/M6 |
| FR-020 | classification doc + registry field (T048, T050) | check without class → registry invalid | G | off | M5 |
| FR-021 | CI aggregate (T061, T064) | one shard skipped/cancelled → gate fails; rerun success on latest attempt → passes | G | ci | M6 |
| FR-022 | receipts (T065) | receipt missing → upload step fails (`if-no-files-found: error`) | G (presence) / tracked gap (content validation, T068) | ci | M6 |
| FR-023 | #27 validator and fixtures (T068) | truncated / contradictory / stale → not ready | G after #27 adoption | off, ci | adoption |
| FR-024 | claims doc (T069) | — | S | hum | M6 |
| FR-025 | fresh-clone test (T014) | credential prompt or unpinned fetch → fails | G | loc, ci | M1 |
| FR-026 | budget records (T015, T059, T089) | budget declared without measurement → review rejects | S (budget) / G (record presence) | loc | M1/M5 |
| FR-027 | second consumer upgrade (T072, T073) | upgrade changes rules not in release notes → rule-diff fails | G | loc | M7 |
| FR-028 | Bzlmod lock policy, registry, governance files (T006, T050, T079) | console-only setting → not representable; review | G (locks) / S (settings) | loc, hum | M1–M8 |
| FR-029 | language profile doc (T074) | placeholder service added → consumer report flags | S | hum | M7 |
| FR-030 | offline tofu tests (T084), runbook (T085) | apply without approval → pipeline config test fails | G (config) / R (live) | off / dep | M10 |
| FR-031 | worker-conformance doc (T086) | — | S | hum | M10 |
| FR-034 | spec 009 matrix | a second image builder → review rejects; a defect in the image → `image_test` fails | G / S | off, hum | M8a |
| FR-035 | T092's unchanged gates, queued, no result yet | NixOS required of a template user → review rejects | S | hum | T092 |
| FR-032 | neutrality + provenance checks (T076, T077), review (T080) | denylisted identifier → fails | G (scan) / hum (review) | loc, hum | M8 |
| FR-033 | governance module validate (T079) | — | G (validate) / R (applied state readback) | off / dep | M8 |

## Success-criteria witnesses

| SC | Witness | Layer |
| --- | --- | --- |
| SC-001 | T014 + T042 on a clean host image | loc, ci |
| SC-002 | T032 per RPC (+ #28 report on adoption) | ci |
| SC-003 | T016 fixture set | off, ci |
| SC-004 | per-check defect/corrected fixtures (T046) + T041 | off, ci |
| SC-005 | T043 partial-staging fixtures, repeated N=20 | off |
| SC-006 | T059 / T089 budget records | loc |
| SC-007 | T068 (#27 adoption) | off |
| SC-008 | T073 upgrade record | loc |
| SC-009 | T076/T077 + T080 record | loc, hum |
| SC-010 | spec 009 repeatability and refusal records | off, loc |

## M0 evidence (this package)

| Claim | Evidence | Layer |
| --- | --- | --- |
| Spec Kit scaffold is the pinned upstream output | `tools/speckit/bootstrap.sh verify` → `speckit-scaffold: OK (v1.2.3 5ea8b1cf…)` at 2026-10-28 | loc |
| Reference inspected at a recorded revision | untracked operator record (`.private/`, git-excluded); tracked files name no private identity | hum |
| No private content copied | docs/public-release-review.md M0 check | loc, hum |
| No application code exists | after the M0 commit, `git ls-files` lists only the Spec Kit scaffold, constitution, `specs/`, `docs/`, `tools/speckit/`, README and AGENTS | loc |
