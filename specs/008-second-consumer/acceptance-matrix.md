# Acceptance matrix: M7

Layers: off, loc, hum. Hosted is never claimed. Each negative needs its positive first.

| Req | Witness | Valid | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- | --- |
| FR-001 | Separate clone, path scan | no path into this repository | a relative path out → fails | G | loc |
| FR-002 | Consumer checks | both adapters resolve one full SHA from one canonical pin | `local_path_override`, `repo = "local"`, a copied wrapper, or two differing values → rejected | G | off, loc |
| FR-002a | Review | evidence says prek's remote-hook interface | a claim of adopting #25 → review rejects | S | hum |
| FR-003 | Scratch consumer, unit | each host path named by the host | a hard-coded path returns → consumer fails | G | off, loc |
| FR-004 | Consumer flow | `--base=C0` passes | bare → 11; same SHA → 11; a base that cannot build → 11 | G | loc |
| FR-005 | Review | consumer's own lists | a copied template list → review | S | hum |
| FR-006 | Notes fixture | added, changed, removed listed, from the one catalog | a rule change missing from notes, including one inside a registry entry → fails; weakening unflagged → fails | G | off |
| FR-007 | Real diff, receipts | one chosen pin with mechanical effects apart; clean consumer passes; restored control passes | a deliberate defect at N+1 → caught; a manual rule edit in the consumer → review rejects | G/S | loc, hum |
| FR-008 | Logs, release fixture | a tag moves elsewhere and the SHA-pinned consumer is unaffected, same verdict | release tooling asked to change a recorded mapping → refused | G | off, loc |
| FR-009 | Review | worked design | Python code with no consumer → rejected | S | hum |
| FR-010 | Review | N tagged after the interface works; 1.0.0 tags the exact reviewed candidate | N cut from M6's source, 1.0.0 declared before review, or an implementation change after it → rejected | S | hum |
