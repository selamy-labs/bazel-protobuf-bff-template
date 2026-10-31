# Acceptance / Evidence Matrix: M4

Classes: **G** deterministic guarantee, **S** review signal. Layers: **off** offline test, **loc**
local verifier, **ci** hosted CI (not run), **hum** human.

| Req | Witness | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- |
| FR-001 | `//api:compile_gate_test`, `notes_e2e_test` | a wrong caller does not compile | G | off |
| FR-002 | `notes_e2e_test` | each RPC: a success and typed failures in a browser | G | off |
| FR-003 | `errors_test` | each code → one distinct outcome; an unknown code → a general failure | G | off |
| FR-004 | `errors_test`, `notes_e2e_test` | a violation lands on its field; an unknown field stays visible | G | off |
| FR-005 | `notes_e2e_test` | pinned browser, real binary, two origins | G | off |
| FR-006 | `browser_smoke_test` | the browser starts with only pinned libraries | G | off |
| FR-007 | `defect_e2e_test` | broken mapping → exactly the not-found scenarios fail | G | off |
| FR-008 | `bundle_test` | test identity in the production bundle → fails | G | off |
| FR-009 | `dev_test` | the runner serves the page and the server | G | off |
| FR-010 | `required_tests.txt`, `fresh_clone.sh` | a missing test fails the inventory | G | loc |

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `fresh_clone.sh` | Planned |
| SC-002 | `notes_e2e_test` | Planned |
| SC-003 | `defect_e2e_test` | Planned |
| SC-004 | `bundle_test` | Planned |
