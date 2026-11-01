# DRAFT acceptance matrix: M8

External draft, revision 3, unchanged in direction. Spec 010. At landing `/speckit-tasks` renders the task list in the template form, keeping these bounds. Layers: off, loc, hum. Hosted is never claimed. Each negative needs its positive first.

| Req | Witness | Valid | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- | --- |
| FR-001 | Review | approved MIT text | missing or altered → rejected | S | hum |
| FR-002 | Inventory check | every third-party path listed | a planted unlisted path → fails, named | G | off |
| FR-003 | Inventory check | three scopes; runtime contents observed; codegen inputs and output families listed | a bundled dependency or generator missing from the table → fails | G | off |
| FR-004 | Inventory check, review | known licenses shown | unknown license → stays visible, never read as permissive; a compliance claim → review rejects | G/S | off, hum |
| FR-005 | `neutrality.sh` fixtures | clean scratch repository passes | a planted identifier in a file, a deleted file and a message → fails | G | off |
| FR-006 | Marker fixtures | clean export reports scope and literal absence | a marker in a file name, directory, blob, message, ref or identity → found, and its text appears in no output or locator | G | off |
| FR-006a | Review | scope stated; run-local labels | a hash presented as confidential → review rejects | S | hum |
| FR-007 | Marker fixtures, CI config | generic passes alone in public CI | no file or an empty one → private `not-run`, never `passed`, and public CI is not failed by it; release review with `not-run` → not ready | G | off |
| FR-008 | Packet | identity and proposed ref listed; work not blocked | a rewrite with no finding or refusal → review rejects | S | hum |
| FR-009 | Offline `tofu` after the fetch phase | format, validate, mock tests pass, with network and credentials off | a second target, an `import` block, an owner or name other than the configured one, or a changed resource graph → test fails; any apply path → review rejects | G/S | off, hum |
| FR-009a | Offline `tofu` | ruleset disabled by default; the activation input enables it | enabled by default → test fails | G | off |
| FR-009b | `//:governance_test` | the required check equals the policy's gate and the workflow's job name | a context naming no job, a blank context, a renamed policy gate or a renamed workflow job → fails | G | off |
| FR-010 | Review | order and gates named | a step that bypasses a gate → rejected | S | hum |
| FR-011 | Recorded row | reviewer and the scanned revision | an empty row, or a candidate that differs from the scanned revision → not ready | S | hum |
| FR-012 | Packet, private scan at the candidate, README review | one named candidate; markers absent by opaque output; README states match the evidence; inherited templates | a README claim with no evidence row, a pending item dropped, a private backup or operator record in the export → not ready | G/S | off, hum |
| FR-013 | `//:ci_parity_test` over the declared second workflow | one build, publication by digest with the scoped token, no fork artifact, no cache, no deployment | an undeclared workflow, a broad permission, a `workflow_run` trigger, a cache step → fails | G | off |
| FR-014 | the credentialless verification job; the visibility read-back | anonymous digest-pinned pull and asset hashes match | a mismatch, a private package → not published | G | live |
| SC-007 | the same | verified pre-release | a stable claim → review rejects | S | live, hum |
| SC-006 | the full verifier, both scans, `//:governance_test` at the candidate | all pass at one revision | any run at another revision → not the candidate's evidence | G | loc, off |
