# M1 Remediation Evidence

Findings come from an independent adversarial review of M1 at `d8462a8`, two reviewer probes, two
independent full-isolation runs, and an adversarial re-review of the repair. Source under test:
`58b0f3891088a566dd43c1aa62e0ffc77fc78224` (tree `eb7ea7caa580ac7f398e48d7ea4b1e300b9b918c`, parent
`1965a20`). Evidence commits on top of it change only documentation and task state.

Unless a row says otherwise, results come from the authoring host at **`isolation: weakened`**.
A negative control
counted only when the input was valid, the unmodified case passed, the mutation really changed the
input, and rejection named the intended reason. Three attempts failed that bar and earned no credit:
two yq runs that picked up a `BUILD.bazel` and produced empty fixtures, and one suppression mutation
that merely failed to compile. The checksum witness also failed that bar for a while: it depended on
the network, which a dead proxy exposed.

## Findings

| Task | Finding | Red, before the fix | Green, retained witness |
| --- | --- | --- | --- |
| T024 | H1 CI parity bypasses | Old test accepted 16 of 25 mutations; the yq identity control passed, so the result was valid | `//:ci_parity_test`: 40 mutations rejected for named reasons; allowing `continue-on-error` in the validator makes the test fail |
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
| T034 | L2 gitleaks allowlist | Allowlist never matched absolute paths, and relative matching hid a tracked `bazel-config/` secret | Allowlist deleted from `.gitleaks.toml`; the scan covers a snapshot of exactly the inventory; a `bazel-config/` secret is found; no synthetic-fixture allowance is needed, because fixtures are generated at run time |

## Findings from the re-review and independent runs

| Finding | Red | Green, retained witness |
| --- | --- | --- |
| Spec Kit manifests compared as text | Full-isolation run: same path/hash map in another order reported as drift; reproduced locally | `//:normalize_json_test` (4 equivalent, 11 drifts, undeclared file refused); end to end, reordering passes and 7 real drifts fail, including a stray file with a stamped name |
| Parity test could be deregistered | `tags=["manual"]` plus a softened workflow passed | `test-inventory`: passed tests must equal `tools/checks/required_tests.txt`; cached results do not count |
| Parity accepted a scalar `env` and any step order | Mutations accepted | `ciparity` rejects non-mapping `env` and `with` and requires each job's exact ordered steps |
| `verify_body.sh` had no witness | `run_check shellcheck true` plus a defect passed | `//:verify_body_test` fails each of 16 checks in turn against the verdict and receipt; replacing a check with `true`, or recording PASSED directly, fails the test |
| `worktree_dirty` could be false for changed content | `skip-worktree` and `.git/info/exclude` passed as clean | `tested_tree` is the Git tree of what was on disk; only committed `.gitignore` rules narrow the inventory (`//:inventory_test`) |
| `.bazelrc` unpoliced; isolation inferred from a flag | Weakening lines passed | `bazelrc-policy` exact allowlist; `sandbox-evidence` reads Bazel's process summary, which the receipt records; any `local` action fails |
| ShellCheck directive forms | `# shellcheck source=/dev/null disable=SC2086` silenced the pinned tool (rc 1→0) and passed the ban | S006 covers `disable`, `enable` and `shell` anywhere in a directive; `source=` alone stays legal |
| Enrollment by extension | Extensionless and `.bash` scripts and plain `BUILD` files skipped | Shebang and name detection in `inventory.sh` |
| Stale receipt | A PASSED receipt survived a failed preflight | Receipt deleted at start; offline runs report `INCOMPLETE`, not PASSED |
| Hung descendants outlived the deadline | Nested group survived the outer timeout | `bounded()` forwards the deadline signal; `//:verify_body_test` proves a hung grandchild dies |
| Bounded execution gaps | Download, preflight, tool resolution and version calls had no bound | `curl --max-time`, per-phase limits, 2400 s overall (`//:verify_deadline_test`) |
| Pin wrapper | Empty `.bazelversion` and foreign URLs passed; `~/.curlrc` reached curl | Exact URL from a validated version; `curl -q` (`//:bazel_pin_test`) |
| `XDG_CACHE_HOME` selected unverified install state | Replaced install-base file changed a verdict | No longer passed through. See the limit below. |

## End-to-end witnesses at `58b0f38`

| Witness | Result |
| --- | --- |
| `tools/bootstrap/fresh_clone.sh --weak-isolation`: empty HOME, `PATH=/usr/bin:/bin`, path `fresh [clone] (a\|b)* copy` | PASSED, 16 of 16 checks; `tested_tree` equals the commit's tree; 173 files, 12 tests; `39 processwrapper-sandbox` |
| `... --hostile`: hostile `~/.bazelrc`, `BAZELRC`, `~/.shellcheckrc`, `SHELLCHECK_OPTS`, `JAVA_TOOL_OPTIONS`, `USE_BAZEL_VERSION`, `GITLEAKS_CONFIG`, `core.excludesFile`, plus a planted SC2086 defect | FAILED on `shellcheck` alone, for SC2086; PASSED once the defect was removed |
| `tools/bench/measure.sh` (`docs/budgets/linux-x86_64-20c.json`) | 0 failed runs. Bootstrap p50 36.6 s, first verify 24.1 s, unchanged 20.1 s, edited 19.4 s, downloads disabled 18.3 s; peak PSS 1.6 GB |
| Default `tools/verify.sh` on this host | Fails closed in the preflight with remediations |

## Independent full-isolation runs

An independent Linux x86_64 run from an empty HOME, under an external time cap and CPU limit, did
reach `isolation: full`:

| Source revision | Result |
| --- | --- |
| `f9a40b2` | 12 of 13 checks; provenance failed on key order, fixed since |
| `1965a20` | 12 of 13 checks; buildifier failed on an unformatted `BUILD.bazel`, fixed since |
| `58b0f38` | **PASSED**: exit 0, clean tree, 16 of 16 checks, 12 of 12 required tests, `tested_tree` equal to the source tree, `39 linux-sandbox` actions, 99.62 s on two cores |

The `58b0f38` run is one verification on one clean Linux x86_64 host, not a budget measurement, and it
covers that source revision only. Each later commit needs its own run. Its raw receipt is kept outside
the repository, so the source it verified does not contain its own result.

## Not witnessed

- **Full isolation on the authoring host.** It cannot provide namespaces, so its own results stay
  `weakened`. The budget record is from this host and is weakened too.
- **Full-isolation budgets.** The independent pass above is a single run, not a p95 figure.
- **Hosted CI.** The workflow has never run. There is no remote.
- **Other platforms.** linux-arm64 and darwin-arm64 pins are recorded but unexercised, and macOS
  lacks the GNU tools the verifier uses.
- **Pinned ESLint.** S002 spellings were observed against a host ESLint 6.4.0 only (parent T047).
- **Bazel server after a deadline.** A timed-out Bazel client is killed, but the daemon server is
  not, and it exits only on its idle timeout. The runner design covers this in M5.

## Stated limits that remain

- A suppression in `rules.go` is not caught, because that exact path is exempt.
- A hand edit that also updates the Spec Kit manifest passes offline; only provenance catches it.
- Spec Kit build constraints are version-only.
- Detection is not enforcement: a proposal can edit the workflow. The parity test catches that only
  if the test itself still runs, which needs a platform required check configured by the owner.
- `suppression-ban` uses line and block-comment heuristics, not language parsers.
- The verifier trusts its caches under `$HOME/.cache`. The Bazel binary is re-hashed on every call,
  but an extracted install base is not, so write access to that cache can change a verdict.
- `tested_tree` respects committed `.gitignore` rules, so a proposal could ignore a file it adds.
- `required_tests.txt`, `bazelrc.allow`, `policy.json` and the verifier itself are repository files.
  A proposal editing them together passes locally; catching that is the base-built evaluation in M6
  and the upstream policy-diff contract.
- `sandbox-evidence` reads a log line. It shows where actions ran, not that the sandbox held.
- Advisory: an external gocyclo run found maximum complexity 13 in `validator.job`. No threshold is
  enforced yet; this is a signal, not a guarantee.
