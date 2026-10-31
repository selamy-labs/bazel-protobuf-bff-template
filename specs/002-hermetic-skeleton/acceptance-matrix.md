# Acceptance / Evidence Matrix: M1

Classes: **G** deterministic guarantee, **S** review signal, **R** needs runtime or platform
readback. Layers: **off** offline test, **loc** local verifier, **ci** hosted CI, **hum** human.

| Req | Witness | Defect → expected | Class | Layer |
| --- | --- | --- | --- | --- |
| FR-001 | `tools/policy/lock_fixture.sh` | unlocked `MODULE.bazel` edit → fails naming the lockfile | G | loc |
| FR-002 | `//:bzlmod_policy_test`, `workspace-policy` | WORKSPACE file → fails | G | off, loc |
| FR-003 | `//:bazel_pin_test`, `lock_fixture.sh` | tampered or wrong-checksum Bazel or tool → refused | G | off, loc |
| FR-004 | `tools/bootstrap/fresh_clone.sh` | empty HOME, `PATH=/usr/bin:/bin`, one command | G | loc |
| FR-005 | `//:native_fixtures_test`, `suppression_test`, `//:speckit_scaffold_test` | defect and corrected fixture per guarantee | G | off |
| FR-006 | `suppression_test`, `//:verify_body_test` | failing check → named in output and receipt, nonzero exit | G | off |
| FR-007 | `sandbox-evidence`, `//:verify_body_test`, `docs/evidence/m1-isolation.md` | default on a restricted host fails; label comes from Bazel's process summary | G; R for full isolation | loc, hum |
| FR-008 | preflight output in `m1-isolation.md` | missing isolation → cause and remediations | G | loc |
| FR-009 | root `MODULE.bazel`; `REPO.bazel` ignores `quality/` | quality module has no application dependency | S | hum |
| FR-010 | `//:ci_parity_test` | 40 workflow mutations rejected for named reasons | G (shape); R (hosted run) | off, ci |
| FR-011 | `fresh_clone.sh`, `--hostile` | hostile rc files and env, planted defect → fails on that defect alone | G | loc |
| FR-012 | `tools/bench/measure.sh`, `docs/budgets/` | scenario split, exact revision | S | loc |
| FR-013 | `//:speckit_scaffold_test`, `//:normalize_json_test`, `speckit-provenance` | changed, added or removed scaffold file or digest → fails; key order alone passes | G | off, loc, ci |
| FR-014 | `inventory` and review of the file list | — | S | hum |
| T025 | `//:inventory_test`, `//:verify_body_test` | hostile path, empty category, excluded file, skip-worktree edit | G | off |
| T026 | `//:native_fixtures_test`, `//:config_policy_test` | implicit config, `.bazelrc` drift | G | off |
| T027 | `//:native_fixtures_test` | stub helpers on PATH do not change the verdict | G | off |
| T032 | `test-inventory`, receipt schema 3, `//:verify_deadline_test` | deregistered test → missing from passed set; hang → killed | G | off, loc |

## Success criteria

| SC | Witness | Status |
| --- | --- | --- |
| SC-001 | `fresh_clone.sh` | see [m1-remediation.md](../../docs/evidence/m1-remediation.md) for the revision and isolation label |
| SC-002 | fixtures above | met offline |
| SC-003 | `m1-isolation.md`; `sandbox-evidence` | met on the restricted host |
| SC-004 | `docs/budgets/linux-x86_64-20c.json` | recorded at weakened isolation |
| SC-005 | `measure.sh` no_download scenario | a downloads-disabled run, not a network-isolation claim |

Full-isolation, hosted-CI and macOS results are not claimed here. They exist only where
`m1-remediation.md` records them.
