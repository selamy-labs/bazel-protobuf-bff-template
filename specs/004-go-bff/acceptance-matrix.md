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
| FR-006 | `transport_test` | planted secret absent from the response, present in the log under the ID; panic → same shape; server keeps serving; a store or authenticator returning a Connect error with any public code, direct, wrapped or joined → internal, with no detail; validation details and framework errors stay public | G | off |
| FR-007 | `transport_test` | default, cap, client deadline, cancel, each observed in the store | G | off |
| FR-008 | `identity_test`, `transport_test` | none, cookie only, malformed → `unauthenticated`; a valid credential with a cookie is decided by the credential | G | off |
| FR-009 | `server_test`, `binaries_test` | test identity in production → refused; dev binary off loopback → refused | G | off |
| FR-010 | `binaries_test` | production binary with no verifier → exits nonzero before serving | G | off |
| FR-011 | `dependency_rules_test`; `testonly` analysis | test package in `deps(//bff/cmd/bff)` → fails; Bazel refuses the edge | G | off, hum |
| FR-012 | `transport_test` | other, prefix and suffix origins → no allow-origin; credentials never allowed | G | off |
| FR-013 | `binaries_test` | one ready event; SIGTERM → exit 0; closed stdin has no effect | G | off |
| FR-014 | `format-go`, `//:native_fixtures_test` | unformatted file → fails. Vet is not enforced yet (research R7). | G | loc |
| FR-015 | `required_tests.txt` | a missing test fails `test-inventory` | G | loc |

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `transport_test` | Local, passing |
| SC-002 | `transport_test` | Local, passing |
| SC-003 | `binaries_test`, `server_test` | Local, passing |
| SC-004 | `dependency_rules_test`; mutation sweep; M4 mapping defect | Local; M4 pending |

Limits: the production identity path has no shipped verifier, so only a test `Verifier` exercises it.

## Hand-run witnesses, 2026-10-31

These ran once and are not repeated by the verifier.

| Witness | Result |
| --- | --- |
| Mutation sweep over `bff/` | 14 of 14 caught by the intended test, none by a compile failure: not-found mapping, internal cause leak, panic masking, deadline cap, default deadline, cookie authentication, test principal in production, origin prefix match, credentials allowed, skipped validation, owner-blind store, untrimmed title, production accepting test identity, dev binding off loopback |
| Downstream error regression before the fix | 48 of 48 cases leaked over a real server |
| Production target depending on `//bff/testing/identity` | Bazel analysis refused it |
