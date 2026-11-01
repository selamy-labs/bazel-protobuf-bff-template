# Research: CI Parity, Base-Built Floors and Receipts (M6)

**Observed** means seen in a scratch probe at `e0a12ac`, or read from source. **Decision** is a choice this
plan makes. **Open** is unverified, and T002 owns it. Probe results are labeled `weakened`, and no time here
is a measurement.

## R1 Parent task paths have drifted (observed)

| Parent says | Today |
| --- | --- |
| `//quality:run --stage=full --shard=…` | The entry is root `//:quality`; no `--shard` exists |
| `quality/fixtures/floors-from-base/`, `quality/fixtures/aggregate/` | Spec 006 R11 generates fixtures at test time, away from directories that shape inputs |
| `quality/checks/ci_parity_test.sh` | `tools/ci/parity_test.sh` and `quality/checks/ciparity` exist; extend them |
| Shards `build-test`, `e2e`, `coverage`, `policy` | Three legs: `coverage` joins `build-test`, see R3 |
| `prerequisite`, `state: SUCCEEDED`, `attempt`, `base_revision` in the receipt contract | `Receipt` has none of these. It has `result: PASSED` and `stage` |

Decision: keep the parent's intent, record each change as a deviation, and reconcile the contract in the same
commit as the code. Readiness stays upstream.

## R2 Owned checks and postconditions (decision)

Every `full` check has one of two mutually exclusive forms:

```proto
string shard = 16;         // Owned by this leg. Set for every full check that is not a postcondition.
bool postcondition = 17;   // No shard. Runs after each leg's Bazel invocation. Requires after_bazel.
```

`after_bazel` keeps its meaning, *this reads the log*, so `coverage-floor` is `after_bazel` and owned by
`build-test`, while `test-inventory` and `sandbox-evidence` are postconditions.

| Leg | Owned | Postconditions | Verb |
| --- | --- | --- | --- |
| `build-test` | `tests`, `dependency-rules`, `coverage-floor` | both | `coverage` |
| `e2e` | `e2e-browser` | both | `test` |
| `policy` | All others, including `floors-from-base`, `hook-e2e`, `proto-breaking` | none: no Bazel work | none |

`required_tests.txt` gains a leg column, so it stays the one list. `test_evidence.sh inventory` takes the leg. Target
subtraction already removes targets of checks outside the selection, so `tests` stops owning `//web/e2e/...` in
`build-test` by the existing rule.

**Why a set union is not enough.** A leg can vanish with its checks' results, and every set still looks consistent
if the workflow is edited along with it. So the runner derives the obligations table from the registry, `ciparity`
compares its legs with the workflow's jobs and `needs`, and each receipt lists its own obligations. Removing a
postcondition from one leg is not expressible: a postcondition has no per-leg setting. A leg's failure to run one
leaves its obligation unreported, which denies PASSED by the rule M5 already has for an expected check.

`gate` does not read receipts (R4), so "all obligations were reported" is proven on each leg's own exit code. A
reader of a receipt can check more, and that general check is upstream #27.

## R3 One invocation for tests and coverage (observed, then decision)

**Observed.**

| Probe | Result |
| --- | --- |
| `bazel coverage --combined_report=lcov`, defaults | Analysis fails: `launcher_maker` needs a C++ toolchain. The template has none, on purpose. No JDK either |
| Merger overridden alone, `sh_binary` or Go | Same failure |
| Output and report generators both overridden by a pure Go binary | Analyzes. `--coverage_support` need not change |
| One `bazel coverage`, all targets, unpatched | 34 of 36 pass, exit 3. Two fail with exit 11 |
| Cause | An instrumented Go binary writes `warning: GOCOVERDIR not set` to stderr. `capture` merges stderr, so `bazelDigest` tried to open that text as a path |
| Same, with that path read corrected in scratch | 36 of 36 pass, exit 0, inventory equal to `required_tests.txt` |

The failure was a real, small M5 defect exposed by instrumentation, not a test difference. It failed closed, as it
should. FR-010 fixes it with a red test. The scratch patch took the last line, and the real fix captures stdout only.

**Decision.** `PlanBazel` keeps one invocation. It uses `coverage` when `coverage-floor` is selected, and `test` when it
is not. So unsharded `full` runs one `bazel coverage` over everything, satisfying `tests`, `dependency-rules` and
`e2e-browser`, and `coverage-floor` reads what that invocation left. `build-test` does the same over its targets. There is
no second policy run, and no `coverage` leg, which is why the parent's fourth shard goes away.

Consequences to accept or refuse:

- Under `full`, tests run against instrumented code in the filter. Pre-push and the `e2e` leg run uninstrumented.
  `binaries_test` and the browser tests passed instrumented in the probe.
- Configuration moves into `.bazelrc` under `coverage`, so `tools/policy/bazelrc.allow` changes, which is reviewed
  policy: both generators, `--instrumentation_filter`.
- `nogo` still judges under `coverage`: a planted `Sprintf` defect failed the build by analysis, and the clean control
  passed.

**Bootstrap is not a check invocation.** Only `floors-from-base` builds from a base, by `bazel build` of one fixed
target in a private worktree and output base. It is the analogue of `verify_body.sh` building the runner. It runs no
tests, its failure is exit 11, and a fake-Bazel test counts exactly one `test` or `coverage` per stage.

## R4 Gate semantics (hosted unobserved)

`gate` reads `needs.*.result`. GitHub's rerun of failed or selected jobs keeps the SHA and ref (GitHub's rerun guide, read 2026-11-01). That `gate` reruns as a dependent, and that `needs` then gives each leg's latest result, is
an expectation to check against the guide at T009, unobserved until a hosted run exists. The rule is "latest per leg",
not "every leg in the newest attempt", so an earlier success plus a newly successful leg must pass.

`gate` has `permissions: {}`, so it cannot read artifacts, and the contract says it does not validate contents (T068).
Artifact names carry leg and `github.run_attempt`. A rerun leaves `pull_request.base.sha` as it was, so a rerun's base can
be stale. The receipt records it, and invalidation is upstream's.

## R5 What base-built does not bind (observed from the workflow)

On `pull_request`, GitHub runs the workflow from the proposal's merge ref. A proposal can delete the `policy` job, and
the parity test that would object runs from the same proposal. Everything in `.github/` already escalates locally, but
that only changes what the proposal's own checks do. So the honest claim is: *with the workflow intact, the evaluator
cannot be replaced by the proposal*. A required workflow or ruleset is M8 and an operator gate, and #26 owns general
detection.

## R6 Base and bootstrap (decision)

**Selection is not comparison.** `--from` says what changed. It can be zero on a new topic branch, and otherwise it is
often the topic's own earlier push. The policy base says what the tested commit is compared with. They are separate
inputs, and neither fills in for the other.

**No default exists.** `main` here is `57d6ce0`, an empty commit with no files. After a merge, `HEAD` on `main` equals
`main`, so a `main`-derived default compares a commit with itself. A HEAD-only bundle has no `main` ref. And `HEAD^` on a
multi-commit branch is the proposal's own commit. So `--base` is explicit and permanent. The value may be any revision: it
is resolved once with `git rev-parse --verify`, and the resolved SHA is what is tested and recorded. Ancestry is checked,
so a named ref is never needed, and a HEAD bundle already carries every ancestor.

| Refusal, exit 11 | Named cause |
| --- | --- |
| Nothing named | `no policy base was named` |
| Empty, option-like, or whitespace or control characters | `malformed revision` |
| Well-formed, resolves to nothing | `unknown revision` |
| Shallow clone | `history is shallow` |
| Resolves to the tested commit | `a commit cannot be its own base` |
| Not an ancestor | `not an ancestor of the tested commit` |
| No evaluator at the base | `the base has no evaluator` |
| Base does not build | `the base's evaluator did not build` |
| `--offline` | `needs the network` |

**Same tree is allowed.** A different ancestor with an identical tree passes: nothing changed, so nothing weakened. Refusing
it would be false security, because any README edit makes the trees differ, and a differing tree authenticates nothing.
Whether a base was reviewed is recorded apart from this check.

The refusal keeps every other result, and uses M5's existing exit 11 ("baseline or evidence unavailable"). This changes the
bare command's outcome, which is an intended change to parent SC-001: one documented command remains one command.

**Hook context.** A policy input in a push escalates to `full`, and so does a zero `--from`. A developer's first push of a
topic branch is routine, so it must not need `--no-verify`:

```sh
tools/bootstrap/bazel.sh run //tools/hooks:install -- --policy-base=<rev>   # stores it in local Git config
git config --get bff-template.policy-base                                   # the one declared input
```

`stage.sh` passes `--base=<value> --base-source=hook-config` at pre-push, and the receipt records that. The installer
records what it was given, unresolved, so a moving name such as `origin/main` stays a deliberate choice, and each run resolves
it once. Nothing derives this value. Unset, an escalated push refuses, and the message names this command, with `--no-verify`
as the documented bypass. A push that does not escalate never reads the setting. Naming a base here proves nothing about review.

A repository's very first CI push is different: `github.event.before` is all zeros and no ancestor exists at all. That is
bootstrap, not a topic branch, and M8 handles it explicitly.

**Anchor.** The commit that adds the evaluator has a base without it. At the anchor, `floors-from-base` refuses, exit 11, and
all other checks run. That result is recorded as the anchor's negative control, with a separate independent review of that
SHA. Every later commit can name it. The final M6 revision is therefore never the anchor. Nothing reads the anchor from the
tested tree.

**Flows.** `A` is the anchor SHA, `F` the final M6 SHA, `C0` a consumer's first reviewed commit. Add `--weak-isolation` only on a
host that needs it, and label it.

Final branch, from a HEAD-only bundle:

```sh
git bundle create m6.bundle HEAD
git clone m6.bundle c && cd c && git checkout --detach F
tools/verify.sh --base=A                     # PASSED
tools/bootstrap/fresh_clone.sh --base=A      # and --hostile --base=A
tools/verify.sh                              # exit 11: no base named
tools/verify.sh --base=HEAD                  # exit 11: own base, by resolved SHA
tools/verify.sh --base="$(git rev-parse HEAD)"  # exit 11: the same cause
tools/verify.sh --base=-x                    # exit 11: malformed revision
tools/verify.sh --base=deadbeef              # exit 11: unknown revision
git commit -q --allow-empty -m same-tree
tools/verify.sh --base=HEAD~1                # PASSED: a different commit with the same tree
```

A checkout of `main`. Real `main` has no files, so nothing can run there. No merge is authorized, so the witness is a
disposable clone where `main` points at `F`. The repository's `main` is not touched:

```sh
git clone m6.bundle m && cd m && git checkout -q -B main F
tools/verify.sh --base=A                     # PASSED, with HEAD == main
tools/verify.sh --base=main                  # exit 11: own base, by resolved SHA
tools/verify.sh --base=57d6ce0e751babe7f83d45740a2b4066b68c71a8  # exit 11: the base has no evaluator
```

The same command stays valid after any future merge, because `A` stays an ancestor. A history rewrite at publication changes
every SHA, so M8 must re-anchor and republish the command.

New consumer, M7:

```sh
# C0: the consumer's first commit, with its pin of the quality module and its own coverage_floors.json
tools/verify.sh            # exit 11, every other check run: recorded as the consumer's anchor control
# independent review of C0, then C1 descends from it
tools/verify.sh --base=C0  # PASSED
```

The consumer's evaluator comes from `C0`'s pin of the quality module, so an upgrade is itself judged by the older pin.

**Bases elsewhere.**

| Producer | Policy base | `base_source` |
| --- | --- | --- |
| Local verifier | `--base` | `argument` |
| Pre-push, when it escalates | `bff-template.policy-base`, never `--from` | `hook-config` |
| `pull_request` | `git merge-base "$BASE_SHA" "$HEAD_SHA"`, from `env`, so it is an ancestor. A stale branch is judged against its fork point. `merge_group` closes that where enabled, which is M8 | `ci-event` |
| `merge_group` | `github.event.merge_group.base_sha` | `ci-event` |
| `push` to `main` | `github.event.before`. A non-ancestor refuses. All zeros is the bootstrap case, for M8 | `ci-event` |

## R7 Coverage metrics and scope (observed)

**Go.** Go's native percentage is statements covered ([go.dev/doc/build-cover](https://go.dev/doc/build-cover), read 2026-11-01).
`test.log` agrees: `coverage: 91.2% of statements`. rules_go's `convertCoverToLcov` drops `numStmt`, expands each block over
every line from start to end, and takes the maximum count per line. So `DA` is **block-span lines**: it includes comments
and blank lines inside a block, and it is neither a statement nor a source-line count. `LF`/`LH` follow from that. It emits no
`BRDA` for Go, despite a comment there mentioning branch coverage.

| Scope | Statements, raw profile | LCOV `DA` |
| --- | --- | --- |
| root | 87.9% (233/265) | 91.4% (349/382) |
| quality | 70.4% (1087/1545) | 72–75%, depending on a path match |

These are not floors. The file counts differ too (11 and 15 in the profile, 12 and 19 in LCOV). T002 reconciles them file by
file before any floor is chosen. The raw profile exists at
`coverage.dat.cover`, next to the merger's `--output_file`, and the probe's merger read it, covering 26 files. Decision: the
template's merger keeps that profile, the evaluator floors statements, summed over blocks by taking each block's maximum
across tests, and a missing `.cover` for a Go test is `FAILED`. This depends on a rules_go detail, so a fixture guards it
at the pin. Go branch coverage is `unmeasured`.

**Two modules.** The filter `^//bff[/:],^//web/client[/:],^@@bff_template_quality\+//` produced records for both modules:
`bff/…` and `external/bff_template_quality+/…`. So the canonical repository form works at this pin. The merger maps the
`external/` prefix to `quality/`. Without that entry the quality module silently disappears, so a named mutant removes it.

**Reconciliation (T002).** Each Go difference has a cause:

| Difference | Files | Cause |
| --- | --- | --- |
| LCOV only | Five `main.go` files, of `bff/cmd/bff-dev`, `ciparity/cmd`, `suppression`, `proto/cmd`, `runner/cmd` | They run as subprocesses of other tests. The raw profile holds the test process only |
| LCOV only | `registry.pb.go` under `bazel-out` | Generated and untracked, so outside any source set |
| In neither | `bff/ports/store.go` | Interfaces only |
| In neither | `bff/cmd/bff/verifier.go` | One `var` declaration, no statement |
| In neither | `api/tools.go`, `quality/analysis/tools.go` | `//go:build tools` imports |
| In neither | `quality/runner/supervise_other.go` | `//go:build !linux`, not compiled here |

The 72% and 75% quality figures differed by a path match and by those subprocess files, and neither is used. So profiles are
planned as follows, and each file is in exactly one row. Numbers come from the anchor's measurement.

| Profile | Metric | Source | Files |
| --- | --- | --- | --- |
| `go-root`, `go-quality` | `statements` | Raw profile | Go sources under `bff/` and `quality/`, except below |
| `go-entrypoints` | `block-span-lines`, labeled, never called statements | LCOV `DA` | The five subprocess `main.go` files, by path |
| `ts-client` | `compiled-js-lines`, with branches shown apart | Node's LCOV | `web/client/*.ts`, except tests |
| `web-browser` | `unmeasured`, an explicit boundary for review | None. The browser scenarios run this code, and no line data exists | `web/app/App.tsx`, `web/app/main.tsx`, `web/serve/serve.mjs` |

Excludes carry a factual reason: `interfaces-only`, `no-statements`, `build-tag-tools`, `other-platform`, `test-data`, `test-tooling`.
The evaluator enforces only that a listed reason is present. Whether it is true of a file is reviewed, because no language-specific
classifier is built for this. Lacking a test is not a reason, and absent data is never counted as covered. `web/client/transport.ts` is executable and has no
unit test, so it gets one (T013); excluding it is not an option.

**Instrumented subprocesses.** Tests that run instrumented binaries add repeated records: `ci_parity_test` has 86 `SF` entries. Merges take
the union of lines per file. Statement profiles ignore them, so a subprocess cannot inflate a statement floor.

**TypeScript (T002).** An earlier empty report was partly this probe's own fault, because its merger read only `*.dat`. After that was fixed:

| Attempt | Outcome |
| --- | --- |
| rules_js c8 path, `js_test` default | Runs. Its merger is the rule's own, unaffected by Bazel's flags, but its manifest lists `.ts` and V8 reports `.js`, so c8 keeps nothing. `eslint_test` shows `(empty-report)` |
| Same, with a `js_library` of the compiled `.js` | Manifest stays `.ts`-only |
| Node's `run({coverage: true})`, with source maps | Node cannot find the mapped `.ts` in runfiles, and the test exits 1 |
| Node's `run({coverage: true})`, no source maps | Works. `errors.js`: 75 of 75 lines, 31 of 32 branches, 5 of 5 functions |

Decision: a small test entry calls Node's own runner and writes LCOV to `COVERAGE_OUTPUT_FILE`, which rules_js lets a test own. No new pin,
no `tsconfig` change. The metric is lines of compiled JS, `compiled-js-lines`, so it maps to a `.ts` file by name and not by line number. Node's
`run` coverage is the pinned Node 24.22's API, and a fixture guards it at that pin. No mature third-party tool is needed.

**S005 gap found.** Node's own marker, `node:coverage ignore`, is not in S005's pattern. Adopting Node's coverage makes that spelling real, so
T014 adds it to the pattern set, with defect and corrected cases, before the anchor.

**After failures.** Whole3 left 17 non-empty reports beside 2 failed tests, so a report is not a pass (FR-019).

No number is chosen here. Floors come from the first honest measurement at the anchor, with no imported policy.

## R8 Scope terminology (decision)

"Widening scope" was ambiguous, because scope has two directions. Measuring *more* source is good, and excluding *more* is
weakening. So the rule uses defined terms: the measured source set and narrowing, computed from Git for
each tree, plus enrollment, which looks at one tree and needs no history. Worked cases:

| Change | Verdict |
| --- | --- |
| New file matched by `includes` | Allowed; it counts |
| File deleted from the tree | Allowed; the percentage must still meet the floor |
| File moved within `includes` | Allowed; the old path is gone and the new path is in the set |
| File moved outside `includes`, by delete and add | The old path is simply gone. The new path fails as `unenrolled source` |
| File moved to a path an existing `excludes` entry matches | **Not detected.** Stated limit: entries stay narrow and reasoned |
| File moved to a language with no profile | Not judged. `.go` and the TypeScript extensions all have profiles |
| `excludes` gains a path matching a surviving file | Narrowing, fails |
| `includes` loses a path, surviving files drop | Narrowing, fails |
| `includes` gains a path | Allowed |
| Floor raised | Allowed |
| Floor lowered, profile removed, `metric` or `language` changed | Fails |
| Anything outside `coverage_floors.json` | Not judged here. That is #26 |

There is deliberately no move detector: Git's rename guess is not consulted anywhere. The evaluator reads two profiles and two
file lists. It is not a general comparator, and it knows nothing of checks, stages or
authorizations.

## R9 Triggers (observed; a small fix)

`ci.yml` runs on `pull_request`, `push` to `main` and `merge_group`. `hook-stages.md` says "CI checks every ref", which is false for a
branch with no pull request. That sentence has been inaccurate since M5, with no effect yet because hosted CI has never run.
Narrow it to "CI judges pull requests, merge groups and `main`", and do not widen the triggers to match the words.

## R10 Upstream (read 2026-11-01)

python #25–#29 and typescript #20 are open, with 0 comments, last updated 2026-10-28, and no release was listed. T067 and T068
stay blocked, and T070 scopes M6's exit without them.

## R11 Costs and order

`coverage` replaces `test` in `full` rather than adding a run. Each coverage probe of the whole set took about 50 s on warm
actions, which says only that nothing obviously doubled. `floors-from-base` adds one evaluator build. The full record predates
`hook-e2e`, so M6 re-measures once, at the end. Formatter and lint checks run before any clone gate.
