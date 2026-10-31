# Acceptance / Evidence Matrix: M2

Classes: **G** deterministic guarantee, **S** review signal. Layers: **off** offline test, **loc**
local verifier, **ci** hosted CI (not run), **hum** human.

| Req | Witness | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- |
| FR-001 | `notes.proto`, `//api:compile_gate_test` | a method outside the contract → TS2339 | G | off |
| FR-002 | `//api:validate_test` | 18 boundary cases, by rule id | G | off |
| FR-003 | `workspace-policy`, `//:bzlmod_policy_test` | committed `.pb.go`, `.connect.go`, `_pb.ts` → fails | G | off, loc |
| FR-004 | `//api:*` under the sandbox with network off | no host compiler or registry used | G | loc |
| FR-005 | `//api:schema_probes_test`, `proto-format`, `proto-lint` | bad format; SERVICE_SUFFIX; comment and local-config exclusions stay inert | G | off, loc |
| FR-006 | `@bff_template_quality//proto:proto_test`, `proto-breaking` | branch name, short hash, unknown commit, non-ancestor, no schema, shallow clone → exit 4 | G | off, loc |
| FR-007 | `proto_test` (`TestBootstrapMarker`, `TestFreshHistoryCopy`), `schema_probes_test` | marker with schema history, even deleted → refused; inherited hash in a fresh-history copy → fails closed; each adopter route passes | G | off |
| FR-008 | `schema_probes_test` | six breaking probes matched as the rule's JSON type; compatible addition passes; a schema that does not compile reports `COMPILE`, which no probe expects | G | off |
| FR-009 | `//api:compile_gate_test` | four defects, each one exact diagnostic on the changed line; control clean | G | off |
| FR-010 | [docs/api-style.md](../../docs/api-style.md) | each RPC listed with rule decisions | S | hum |
| FR-011 | `required_tests.txt`, `expected` in `verify_body.sh` | a missing test or check fails the receipt | G | loc |
| FR-012 | `pnpm-lock.yaml`, `lock_fixture.sh` | a lock mismatch fails | G | loc |

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `schema_probes_test` | Local, passing |
| SC-002 | `workspace-policy` and the fresh-clone witness | Local |
| SC-003 | `validate_test` | Local, passing |
| SC-004 | check diagnostics | Local |
| SC-005 | `tools/bootstrap/fresh_clone.sh` | See [m2-m4 evidence](../../docs/evidence/m2-m4.md) |

Observed failures while building this, which are the only red results claimed: the first probe run
failed because text output named no rule; the compile gate's fourth defect reported TS2339, not the
expected TS2551; and the setup-failure witness first assumed no JSON type, when buf reports `COMPILE`.
All three exposed a wrong expectation, not a product defect.

The rename scenario (US1 AS3) was witnessed by hand on 2026-10-31: renaming `body` to `text` in the
schema alone made `//api:compile_gate_test` fail its control on `body`, with no regeneration step.
