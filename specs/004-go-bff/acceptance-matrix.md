# Acceptance / Evidence Matrix: M3

Classes: **G** deterministic guarantee, **S** review signal. Layers: **off** offline test, **loc**
local verifier, **ci** hosted CI (not run), **hum** human.

| Req | Witness | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- |
| FR-001 | `transport_test` | create then read through a real server | G | off |
| FR-002 | `//bff:dependency_rules_test` | forbidden edge → fails naming it | G | off |
| FR-003 | `transport_test` | invalid input → `invalid_argument` with `Violations`; store not called | G | off |
| FR-004 | `transport_test`, `memory_test`, `app_test` | foreign and absent responses are byte-identical | G | off |
| FR-005 | `transport_test` | each condition → its code | G | off |
| FR-006 | `transport_test` | planted secret absent from the response, present in the log under the ID; panic → same shape; server keeps serving | G | off |
| FR-007 | `transport_test` | default, cap, client deadline, cancel, each observed in the store | G | off |
| FR-008 | `identity_test`, `transport_test` | none, cookie only, malformed → `unauthenticated`; a valid credential with a cookie is decided by the credential | G | off |
| FR-009 | `server_test`, `binaries_test` | test identity in production → refused; dev binary off loopback → refused | G | off |
| FR-010 | `binaries_test` | production binary with no verifier → exits nonzero before serving | G | off |
| FR-011 | `dependency_rules_test`; `testonly` analysis | test package in `deps(//bff/cmd/bff)` → fails; Bazel refuses the edge | G | off, hum |
| FR-012 | `transport_test` | other, prefix and suffix origins → no allow-origin; credentials never allowed | G | off |
| FR-013 | `binaries_test` | one ready event; SIGTERM → exit 0; closed stdin has no effect | G | off |
| FR-014 | `format-go`, nogo | unformatted file → fails; vet finding → build fails | G | loc |
| FR-015 | `required_tests.txt` | a missing test fails `test-inventory` | G | loc |

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `transport_test` | Planned |
| SC-002 | `transport_test` | Planned |
| SC-003 | `binaries_test`, `server_test` | Planned |
| SC-004 | `dependency_rules_test`; M4 mapping defect | Planned |

Limits: the production identity path has no shipped verifier, so only a test `Verifier` exercises it.
