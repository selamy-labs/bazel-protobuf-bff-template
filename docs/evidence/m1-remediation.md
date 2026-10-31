# M1 Remediation Evidence

Findings come from an independent adversarial review of M1 at `d8462a8`, plus two reviewer probes
added during remediation. Repair source: `f9a40b2f9697b4f1ed31a0afc06960fa607f3139`
(tree `5f69203490bb66e9b22097baf82bcaf5b7eba9eb`, parent `58a308b`).

Every result below comes from the authoring host at **`isolation: weakened`**. A negative control
counted only when the input was valid, the unmodified case passed, the mutation really changed the
input, and rejection named the intended reason. Three attempts failed that bar and earned no credit:
two yq runs that picked up a `BUILD.bazel` and produced empty fixtures, and one suppression mutation
that merely failed to compile.

## Findings

| Task | Finding | Red, before the fix | Green, retained witness |
| --- | --- | --- | --- |
| T024 | H1 CI parity bypasses | Old test accepted 16 of 25 mutations; the yq identity control passed, so the result was valid | `//:ci_parity_test`: 32 mutations rejected for named reasons; allowing `continue-on-error` in the validator makes the test fail |
| T025 | H2 input discovery failed open | Checkout path `x[1`: `grep: Unmatched [`, then `typos` and `shellcheck` PASS over planted defects and `verify: PASSED` | `//:inventory_test` under a path with spaces and regex metacharacters; empty categories, a missing manifest entry and a non-repository fail; fresh clones use such a path |
| T026 | H3 implicit config | `.shellcheckrc` 1→0, nested `.typos.toml` 2→0, `zizmor.yml` 13→12, and **new** `.gitleaksignore` 1→0 | `//:native_fixtures_test`, `//:config_policy_test`; the buildifier case has a control showing its config file really would disable the check |
| T027 | H4 actionlint host PATH | Same workflow: rc 0 without shellcheck on PATH, 1 with it | Identical verdict with no helper, a no-op `shellcheck` stub and a failing `pyflakes` stub on PATH |
| T028 | H5 undeclared uv | Preflight passed, then `uvx: command not found` (rc 127) | Pinned uv 0.13.1, managed CPython 3.12.15, hash-locked dependencies; fresh clone with only `/usr/bin:/bin` |
| T029 | M1 suppression gaps | 16 spellings, 6 exemption lookalikes and the NUL case failed as tests | `suppression_test`: all pass; reverting span matching and hash comparison each fails the test |
| T029 | Probe: multi-line ESLint config | Reviewer probe failed; host ESLint 6.4.0 honors the form; neighbors (quoted rule, leading newline, `global`, `globals`, `eslint-env`) were also missed | Fixtures with expected start lines; prose and `//` forms not flagged; the reviewer's `probe_test.go` passes on the repo-pinned Go 1.27.2 |
| T029 | Probe: fixture lookalike | `testdata/S999_good.sh` was exempt | Exemptions need path and content hash; `TestFixtureManifestIsExact` keeps the inventory equal to disk |
| T030 | M2 ambient Bazel config | `BAZELRC` was read by Bazel (`Unrecognized option`) | `//:bazel_pin_test` (stub binary, checksum, env) and `tools/policy/lock_fixture.sh` (real Bazel: raw binary obeys the hostile rc as a control, the wrapper does not) |
| T031 | M3 entry point | `bazel run //:verify` died in Bazel before the preflight | `tools/verify.sh` reaches the preflight and prints four remediations, exit 1; `//:verify` is removed |
| T032 | M4 overstated evidence | Receipt lacked tools and start time; no checksum test; bootstrap unmeasured; cached runs; proxy-only "no network" claim | Receipt schema 2; `--nocache_test_results`; checksum witnesses for Bazel and tools; separate scenarios; SC-005 reworded to what `--repository_disable_download` shows |
| T033 | L1 peak memory | Summed RSS, counted shared pages twice and the sampler | Sampled PSS over the verifier and server trees, with the sampler outside them; sampling limit stated |
| T034 | L2 gitleaks allowlist | Allowlist never matched absolute paths, and relative matching hid a tracked `bazel-config/` secret | Allowlist deleted; the scan covers a snapshot of exactly the inventory; a `bazel-config/` secret is found; no synthetic-fixture allowance is needed, because fixtures are generated at run time |

## End-to-end witnesses at `f9a40b2`

| Witness | Result |
| --- | --- |
| `tools/bootstrap/fresh_clone.sh --weak-isolation`: empty HOME, `PATH=/usr/bin:/bin`, path `fresh [clone] (a\|b)* copy` | PASSED, 13 of 13 checks, clean tree, 157 files, 9 test targets |
| `... --hostile`: hostile `~/.bazelrc`, `BAZELRC`, `~/.shellcheckrc`, `SHELLCHECK_OPTS`, `JAVA_TOOL_OPTIONS`, `USE_BAZEL_VERSION`, `GITLEAKS_CONFIG`, `core.excludesFile`, plus a planted SC2086 defect | FAILED on `shellcheck` alone, for SC2086; PASSED once the defect was removed |
| Default `tools/verify.sh` on this host | Fails closed in the preflight with remediations |

## Not witnessed

- **Full isolation.** No run at `isolation: full` exists on any host. An independent full-sandbox
  fresh-clone run is expected from the reviewer after this repair is frozen; it is not claimed here.
- **Hosted CI.** The workflow has never run. There is no remote.
- **Other platforms.** linux-arm64 and darwin-arm64 pins are recorded but unexercised, and macOS
  lacks the GNU tools the verifier uses.
- **Pinned ESLint.** S002 spellings were observed against a host ESLint 6.4.0 only (parent T047).
- **Bounded descendants.** `timeout` bounds each check and kills its process; a Bazel server outliving
  a timed-out client is not witnessed. The runner design covers this in M5.

## Stated limits that remain

- A suppression in `rules.go` is not caught, because that exact path is exempt.
- A hand edit that also updates the Spec Kit manifest passes offline; only provenance catches it.
- Spec Kit build constraints are version-only.
- Detection is not enforcement: a proposal can edit the workflow. The parity test catches that only
  if the test itself still runs, which needs a platform required check configured by the owner.
- `suppression-ban` uses line and block-comment heuristics, not language parsers.
