# DRAFT research: M8

External draft, revision 2. **Observed** comes from a read-only pass at frozen `6582ef3`. Output was counts and categories only. No marker is written here, and this file is safe to land only after a neutrality read of its own.

## R1 History as it stands (observed)

| Scan | Result |
| --- | --- |
| Pinned gitleaks, all refs, redacted | No leaks. 70 commits scanned of 71 reachable, and the other is the empty bootstrap commit |
| Service accounts, `gs://` | 0 |
| Email-shaped text | 20 lines; domains are `example.com` and `example.invalid` |
| `/home/…` | 9 lines, all fixture-shaped such as `/home/cache`. No real user path found |
| IPv4-shaped, 10+ digit numbers | Version strings and pin timestamps |
| Five known operator markers | 0, across all history |
| Words only in the untracked record | 53 shared with history. All generic English or tool names, plus this repository's own commit SHA, which came from the scan's `%H` lines and is in no blob |
| Author and committer | One identity on all 71 commits |
| Branches, tags | 8 branches, 0 tags |
| `.private/` | Excluded, and never tracked |

**Conclusion: no scan evidence requires a history rewrite.** None is planned. This pass is not T076's scan: five markers is not the operator's list, so it is a floor, not a clearance.

**One release gate is open, and it does not hold up the work.** The single commit identity is a real address on an owner domain, and the review checklist allows only "the commit author identity the owner approves". It goes in the release packet (R7). Only a scan finding or an explicit refusal justifies a rewrite.

## R2 If a rewrite is ever required

Only on a recorded finding or that refusal:

1. Preserve the originals: retain the current refs and a bundle outside any published remote.
2. Rewrite in a new clone, and record an old-to-new SHA map privately.
3. Every SHA in the tracked text moves: the anchor, the README command, evidence records, release tags.
4. **Re-anchor.** The anchor's review attached to `e3b3b34`, and a new SHA has no review. The new anchor needs an independent targeted review, as T017 did.
5. **Re-prove.** A new base means new gates. The M7 consumer pin and its upgrade evidence name commits, so they are re-proven as well. Old evidence stays as history, attributed to old SHAs, and is never relabeled.

With no rewrite, the anchor, the README command and M7's pins stay valid as they are.

## R3 Provenance and licenses

Three scopes, because a dependency's place differs by what ships:

| Scope | What it is | Examples | Inventory |
| --- | --- | --- | --- |
| Git source export | What a clone receives | `.specify/**`, `.claude/skills/speckit-*`, repository code | Path, source, revision, license. `.specify/LICENSE` ships |
| Built artifacts | What a build bundles or links | The browser bundle, with React, Connect and protobuf runtime; the Go binary, with its linked modules | Runtime dependencies actually included, each with its notice source |
| Build and test tools | Fetched, used, not shipped | ESLint, Prettier, Playwright and browser debs, Buf, prek, the Go SDK's tools | Tool, version, license |

**Runtime or tooling is decided by observation**, not by which lock file a package sits in: the built bundle's and binary's actual module lists are the source. The Go module list comes from the binary's build info, and the browser's from the bundler's output. Whether a target distributes built artifacts at all is itself recorded: this template ships source, and an adopter who ships a bundle takes on those notices, so the inventory tells them which.

**Code generation** has untracked outputs, and they still have inputs: `.proto` sources, `protoc-gen-*` pins and the output families (`*_pb.ts`, `*.pb.go`, Connect stubs), each with its generator's license, including any header it emits.

**Enumeration.** Go from the module graph, reading `LICENSE` from each fetched module. npm from the lock, reading each `license` field and the package's own license file. Debian from each package's `copyright` file.

The check reads lock files and fetched metadata offline, and `docs/provenance.md` holds the generated table, reviewed by a person. License metadata can be wrong or missing, so unknowns stay visible for review and block nothing silently. This is an inventory for reviewers, and it is no legal or compliance guarantee. No license policy is imported from another project.

## R4 Scans

Two scans with two statuses, so a missing marker file can neither masquerade as a private clearance nor fail every public CI run:

| Scan | Runs | Status |
| --- | --- | --- |
| Generic (`quality/checks/neutrality.sh`), no private word in it | Every run, including public CI | `passed` or `failed` |
| Private release scan, `--markers=<file outside the repository>` | By the operator, at a release candidate | `passed`, `failed`, or `not-run` with no file or an empty one. Release review needs a recorded result |

**Scope is the whole export**, stated in the result: tracked contents, every reachable blob, paths, commit messages, author and committer fields, and ref names and tags. It scans `git rev-list --all` blobs and tree paths, not `git log -p` text with `%H` headers, which caused R1's false positive.

**A locator can leak.** A marker can be part of a file name, a directory, a ref or an identity, so printing a raw path beside a hash would publish it. A hit is displayed with a run-local label (`hit-3`), and the path or ref is shown only when it holds no marker, otherwise redacted. The mapping stays in the operator's private record. A test plants a marker in a file name, a directory, a blob, a message, a ref and an identity, and asserts its text appears nowhere in the output.

**Hashes protect nothing here.** A six-character unsalted hash of a guessable word is reversible by anyone who guesses it, so results use labels and state literal absence and scope. They claim no confidentiality from hashing. The hashed lines in my own R1 pass were a convenience, and nothing rests on them.

## R5 Provisioning

`governance/github/` declares one configured target: one owner and name, and its initial ruleset.

**What is and is not claimed.** A provider resource, imported state or reused state can address an existing object, so the configuration does not make that impossible. The claim is limited to: a single configured target; fresh isolated state; no `import` blocks or adoption; offline tests that pin the allowed owner, name and resource graph; and an explicit operator review of the plan before any apply.

**Ruleset.** Declared disabled, with an activation input. The operator changes the input and applies it through the same configuration, after the first green. No manual setting stands in for it, and nothing is live now.

**Offline validation is two-phase**, like the build: fetch `tofu` and the GitHub provider, pinned with a lock file, into an owned cache; then prove `fmt`, `validate` and `tofu test` with `mock_provider`, with networking and credentials unavailable. `tofu` is not on this host, so it is pinned through the multitool lock. T002 probes that exact path before SC-004 could be weakened.

A required `gate` status does not close spec 007 R5: a proposal can still change the workflow that reports it. The required-workflow or ruleset choice stays explicitly open, and it is not decided by adding `gate`.

## R6 First hosted push

`policy_base.sh` refuses a zero `before`, so every leg stops, and `gate` is red on the first push. This is correct and honest, and the template adds no self-baseline. The plan makes it a recorded step:

| Step | What happens | Proof |
| --- | --- | --- |
| 1. Operator creates the empty repository from the reviewed plan | Nothing runs | Plan output |
| 2. Operator pushes the reviewed history | `push`, zero `before`: all legs refuse by name, and `gate` is red | Recorded as the expected bootstrap refusal. It is not a failure of the template and it is not a pass |
| 3. Operator opens a small pull request | Base is the pushed `main`, which holds the evaluator. All legs run | **The first real hosted evidence** |
| 4. Operator changes the activation input and applies the same configuration, after `gate` has reported once | A required check needs a known name | Platform readback |

A push of a branch holding a descendant of the anchor, with `main` created first, is an equivalent order. Fork pull requests get read-only tokens and no secrets, which CI needs none of. `fetch-depth: 0` and fork checkout of the base are unobserved until step 3, and rerun behavior until it is tried.

The README's command names `e3b3b34`. With history preserved it stays valid.

## R7 The external release packet

Technical and local readiness do not wait for these. Name, public visibility and MIT are approved. The packet lists what remains, for the operator, after the work is reviewable:

| Item | Listed value | Gate |
| --- | --- | --- |
| Commit identity | The one configured identity on all commits | Operator approval, or an explicit refusal, which is what would justify a rewrite |
| Proposed release ref | The accepted M8 tip, as the new repository's `main`. Local `main` is empty, and this names a ref to push, not a merge here | Operator |
| Ruleset activation | The activation input, after first green | Operator, through the configuration |
| Required-workflow choice | Open (spec 007 R5) | Operator |
| Publication | Not claimed | Operator |

Readiness says "technically ready, release gates pending", and it never says publicly approved.
