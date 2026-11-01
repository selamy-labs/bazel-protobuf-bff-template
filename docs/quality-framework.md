# BFF template: quality framework synthesis

Prepared 2026-10-31. This is a proposed integration brief, not evidence that the template already implements these guarantees.

The reusable unit is a working application plus an upgradeable quality dependency. Bazel owns the build, toolchains and generated contracts. Prek connects the canonical checks to normal Git actions. CI independently verifies the complete policy. A neutral browser → generated Connect client → Go BFF → storage-port slice demonstrates what adopters actually receive.

## What is verified now

The local template has an approved foundation design and a repaired M1 skeleton. Its first independent review returned REVISE: five high, four medium and two low findings. Those and a later re-review are repaired, and the source at `58b0f38` passed one independent full-isolation run on Linux x86_64 (see [the evidence record](evidence/m1-remediation.md)). There is no application yet, and no origin is configured. Local runs on the authoring host stay explicitly weakened. Hosted CI and macOS have not been witnessed.

The public upstream hook, policy-diff, evidence, contract-consumer and worker-profile issues are still open. They are design inputs, not released dependencies. The template must record their adoption points without inventing parallel implementations or claiming their guarantees already exist.

Two earlier Relentless CI reviews have since completed their fixes and merged:

- [TypeScript #18](https://github.com/selamy-labs/relentless-ci-typescript/pull/18), head `6826345534633156928edde75b8c1964476488ca`, merged 2026-10-20: recognize assertions through the real test callback and resolved assertion types. Zero assertions, arbitrary methods on `expect`, and assertions in titles do not earn credit.
- [Python #21](https://github.com/selamy-labs/relentless-ci-python/pull/21), head `14bc214a5047d5e259ceaa898b49158095b8df1d`, merged 2026-10-24: selective mutation commands require a pristine passing witness. The coordinator establishes credit before trials; trial code cannot write or alter it. Uncertain selection falls back to the full suite.

## Principles to turn into executable witnesses

| Principle | Required behavior | Negative witness | Owning milestone |
| --- | --- | --- | --- |
| One definition of each rule | Hooks, verifier and CI consume identical pinned tools, config and registry definitions | Change a CI command or omit a required shard; parity fails | M1 repair, M5–M6 |
| The environment cannot alter a verdict | Isolate Bazel system/home/environment rc inputs; make every native tool's config explicit; pin transitive helper tools too | Inject hostile home rc, nested config or PATH helper; verdict and isolation classification remain correct | M1 repair |
| Empty checks cannot pass as complete | Validate input discovery and complete expected inventories; distinguish legitimate not-applicable from accidental emptiness | Regex characters in root path, narrowed enrollment, deleted input, empty receipt or omitted matrix leg fails meaningfully | M1 repair, M5–M6 |
| Gates must be tested as adversarial subjects | Parse workflow structure rather than match selected lines; reject alternative shells, conditional skips and softened failures | `continue-on-error`, a no-op shell, workflow defaults, wrong checkout SHA or hidden multiline command is rejected | M1 repair, M6 |
| A negative control must test the intended defect | First prove the valid fixture passes, the mutation changes it, and the named violation causes rejection | Missing executables, empty generated fixtures, setup failures or identical mutants invalidate the witness instead of earning rejection credit | M1 repair, M5–M6 |
| Compare the meaning the contract defines | Parse structured manifests and ignore only explicitly declared volatile fields; preserve inventories, value types and array order | Equivalent key order passes; a changed hash, missing path, removed timestamp field or reordered meaningful array still fails | M1 provenance, M6 upstream adoption |
| Performance preserves the same obligation | Selection and caching may avoid redundant work only when identity, scope and results remain valid | A selected command fails on pristine code; no mutation kill is awarded and full-suite fallback applies | M5–M6, upstream adoption |
| A subject cannot award itself passing evidence | Protect baseline evaluators, input maps and credit against proposal/trial writes | Replace the evaluator or forge a cached pristine pass; readiness fails | M6, upstream #26/#27 |
| An absence needs a control that could have been present | Before asserting that something was not sent or not found, show in the same setup that it would be seen when present | A no-cookie assertion with no cookie seeded proves nothing; a same-origin control shows the check sees it. A scan that reads no files passes too; a positive control catches that | M4 |
| A runtime closure includes data and configuration | Pin what a program reads as well as what it links, and exercise the feature that reads it. Prove the claim on a host that lacks the ambient copy | A library-only closure passes on a developer machine because system fonts exist, and crashes on a minimal host; a script-only smoke never reaches text rendering |
| Ambient discovery can change what a build produces | Disable automatic host tool discovery, and witness the output rather than the flag | With a compiler on the host, Go silently linked dynamically and required libc; on a compiler-free host analysis failed. The fix must leave a static binary on both | M4, M5 |
| The acceptance criteria can be wrong too | Keep valid and invalid examples reviewed independently of the code; name the oracle; challenge impossible preconditions, always-success behavior and missing side effects. A filled spec or a ticked task is a record, not the evidence | An always-true check passes every positive case, so it needs a rejected invalid example; an oracle that cannot fail earns no credit | M5–M8 |
| Tests assert observable behavior | Genuine assertion calls occur in the real test callback; deliberate defects fail the named scenario. Where a fake is shipped, run the same named contract cases against fake and real implementations | `expect.assertions(0)`, `expect(1).toString()` or an assertion in a title does not count | M4–M6, TS policy adoption |
| Resource bounds include descendants and locks | Close stdin; bound wall time and concurrency; own process trees; keep slot locks in the waiting parent and out of daemon children | A surviving child cannot hold capacity after the foreground command exits; active foreground work still holds its slot | M1 repair where relevant, M5 |
| Every contract has a real consumer | Each RPC maps to browser behavior, typed errors and an acceptance test; legitimate fakes remain test-only consumers of actual behavior | Generated-only references or success-returning speculative stubs do not count as a production caller | M2–M4, upstream #28 |
| A dependency cannot authorize its own disclosure | Decide what a caller may read from where an error came from, never from its type or status. Downstream failures map only through named contract sentinels, to fixed text. Test masking and legitimate public detail together | A store or verifier returns a public-looking status with planted private text, direct or wrapped: the response stays internal with only a correlation ID, while real field violations still reach the caller | M3, FR-006 |
| Evidence names the precise thing tested | Record source and policy revision, expected inventory, producer, attempt, terminal outcome, tool versions and isolation | Stale SHA, pending producer, skipped/cancelled leg or truncated/conflicting record cannot satisfy readiness | M1 honest records, M6, upstream #27 |
| Local hooks are fast feedback with explicit limits | Staged bytes at pre-commit; pushed commit at pre-push; full policy in CI; install reproducibly in worktrees | Partial staging, untracked files, spaces, renames, deletion and multiple pushed refs have defined outcomes | M5 |
| A template must support upgrades | Shared checks stay versioned; copied application code may diverge; prove one upgrade in a fresh, separate Git repository, with repeatable commands, precise prerequisites and a small handoff of supported and unresolved behavior | A new policy release reaches the consumer through a pin change and catches its deliberate defect. An in-tree fixture or a no-op version change does not count | M7 |
| Feedback cost needs edited input | Measure edited-input samples, failed attempts, resource envelopes and review rework under a named profile; targets stay targets until then | A no-change cache hit, a single success or a token price proves no productivity gain | M1, M5, M6 |
| A metric measures what it measures | Complexity counts selected control-flow properties, not architecture or necessity. Qualify a tool's native semantics and thresholds with negative controls before claiming enforcement; adopt no number from elsewhere, add no waiver, and do not split coherent code to improve a score | A threshold that never rejected a seeded case enforces nothing | M5 review; adoption gap below |

## Capability states

Each applicable capability has six separate states: specified, implemented, executed, passed at a named revision, required by the hosting platform, and verified after integration. Scope, command, witness and gap sit beside the state. Local evidence cannot fill the last two. A missing upstream release stays an adoption gap, and an optional profile that does not apply says why, without hiding an unmet core requirement. The table lives in [capability-states.md](quality/capability-states.md), which reuses the registry, the task ledgers and the evidence records. It adds no validator and no second specification.

## M1 repair comes first

Record the independent review as an explicit task/acceptance checklist. Preserve the existing findings and reproduce each important failure before changing its behavior.

1. Replace CI parity's line filtering with a pinned structured parser. Validate the entire permitted workflow shape, exact checkout revision, all `run` bodies, job dependencies, shell/default/condition/error settings and the aggregate verdict.
2. Discover authored inputs as relative NUL-delimited paths. Avoid interpolating repository paths into regexes, preserve filenames, propagate discovery failures, and reject unexpected empty inventories.
3. Disable implicit native-tool configuration. Supply pinned ShellCheck to actionlint; disable undeclared helpers. Check nested and parent configs as well as user config. Do not allow a config file to silently switch off a check.
4. Remove the undeclared uv/uvx dependency or pin it through the supported bootstrap. A fresh-clone witness must use a controlled prerequisite set, rather than inheriting the author's tool-filled PATH.
5. Close suppression-scan escape paths: exact exemption identities, narrow vendored enrollment, executable-source NUL cases and all supported suppression spellings. A broad binary-content exemption must not hide executable source.
6. Isolate Bazel configuration and pin the Bazel download identity. Prove an ambient option cannot relabel a weakened run as full isolation.
7. Correct the preflight entry point and evidence claims. Record what actually ran, distinguish cached no-change warmth from edited-code feedback, add the missing checksum-mismatch witness, and measure bootstrap separately from verification.
8. Repair RSS accounting and path matching, or explicitly narrow their claims. Re-run the canonical verifier and fresh-clone witness at the final source revision, then independently re-review the repaired M1.

Fixes must not lower floors, widen exclusions, suppress diagnostics, bypass hooks or substitute claimed acceptance for a test. Weak isolation remains explicitly labeled until a full-isolation host actually passes.

## Finish the existing route

- **M2:** neutral notes schema, immutable compatibility baseline, build-time Go/TS generation, validation and breaking/compatible probes.
- **M3–M4:** real Go BFF, React caller and browser tests. Include invalid input, not-found, cross-principal isolation, unauthenticated/cookie-only requests, unavailable, deadlines and cancellation. A deliberately broken error mapping must fail the appropriate browser scenario.
- **M5:** one check registry and runner, one Bazel invocation per stage, pinned prek, safe staged/pushed snapshots, bounded process ownership and measured edited-code feedback. Commit-message policy remains opt-in.
- **M6:** local/CI inventory parity, complete aggregate gate, base-built ratchets and receipts. General policy-diff and readiness validation are consumed from upstream when released; missing upstream work stays visibly open.
- **M7:** second neutral consumer and a real quality-dependency upgrade before declaring the interface stable.
- **M8:** complete public-release review, license/provenance inventory and offline repository-provisioning plan. Creation, publication and live settings require the separately recorded operator gate.

Cloud deployment and worker integration remain optional profiles. No remote cache, paid build service, Batch runtime or multiworker platform is necessary to prove this starter. An optional GCP profile uses declarative bootstrap and Cloud Build CD; actual deployment and IAM changes remain operator actions.

## Sources and boundaries

The original engineering motivation was to make prerequisites executable, keep speculative surface out of production code, and give repair loops a clear failure signal at normal commit/push boundaries. These are design goals; they do not make bypassable hooks a security boundary or let static analysis prove architectural necessity.

Public sources:

- [Shared backlog and tracker #31](https://github.com/selamy-labs/relentless-ci-python/issues/31), [template RFC #30](https://github.com/selamy-labs/relentless-ci-python/issues/30).
- [Python hooks #25](https://github.com/selamy-labs/relentless-ci-python/issues/25), [TypeScript hooks #20](https://github.com/selamy-labs/relentless-ci-typescript/issues/20).
- [Policy weakening #26](https://github.com/selamy-labs/relentless-ci-python/issues/26), [revision-bound evidence #27](https://github.com/selamy-labs/relentless-ci-python/issues/27), [contract consumers #28](https://github.com/selamy-labs/relentless-ci-python/issues/28), [bounded worker profile #29](https://github.com/selamy-labs/relentless-ci-python/issues/29).
- The two merged PRs above supply concrete regression cases; their historical review chat is superseded for merge status.
- [Prek configuration reference](https://prek.j178.dev/reference/configuration/): `require_serial` controls batching within a hook; hook priority controls ordering/concurrency among hooks. Configure both deliberately.
- [Randal Schwartz's workflow reference](https://gist.github.com/RandalSchwartz/598a1f2b5760a23c1d70b57e9e85393f): useful inspiration for expressing prerequisites as executable dependencies. Its surrounding orchestration and approval machinery are not adopted by reference.

Private meeting/reference details stay outside tracked public files. A separate operator-only source map retains the private provenance. This brief is a targeted synthesis of the original discussion, BFF review, current upstream issues and merges, and relevant active project lessons; it is not an exhaustive audit of every repository or proof of a finished framework.
