# Research: M7

**Observed** is read from source at `e3b3b34`, and the couplings still stand at accepted M6 `e16718d`.

## R1 The module assumes its host (observed; the main risk)

The module is consumed by pin, so a consumer has no `quality/` directory. These do not hold for one:

| Coupling | Where | Effect on a consumer |
| --- | --- | --- |
| `evaluatorPath` = `quality/checks/coverage/cmd/floors/main.go` | `runner/base.go` | Every base refused, "the base has no evaluator" |
| `baseRegistryPath` = `quality/registry.textproto` | `runner/base.go` | Base digest unavailable, so never PASSED |
| `contract` = `quality/coverage_floors.json` | `floors` | Consumer must keep that path, or name another |
| `vendored` = `tools/speckit/scaffold.sha256` | `floors` | Spec Kit assumed |
| `@bff_template_quality//` and `bff_template_quality+` | `floors` `reportPath` | Fine for the same module name, wrong in a canonical name that differs |
| Host paths in check commands | `registry.textproto`, 11 script paths | The registry lives in the module, but its commands name host files |
| `tools/bootstrap/bazel.sh`, `tools/hooks/stage.sh` | Runner | Wrappers assumed |
| `example.com/bfftemplate/` | Host's `coverage_floors.json` | Already host-owned |

**So spec 007 R6's consumer flow cannot pass as built.** It stands as a statement of intent, and this table is the work. This does not touch the template's own anchor:
`ResolveBase` is right for a repository that vendors the module in `quality/`.

## R2 Decision: who owns the registry

| Option | For | Against |
| --- | --- | --- |
| Module ships checks; host owns its registry instance | Host can list its own scripts; matches "own policy" | Two definitions to upgrade; the diff is the host's |
| Module ships the registry; host supplies paths by variables | One definition; upgrades carry rules | Module grows a templating layer |
| **Module ships rule definitions; host registry selects and binds them** | Rule text moves by pin; host paths stay in the host | A small schema change |

Recommendation: the third. A host registry names rule ids and its own paths, and rules with no host binding come whole from the pin. It keeps "one definition per rule", and an upgrade adds or changes rules by pin alone, never by editing two copies. It stays plain data: no expressions or conditionals, and no general generator. The pin sync in R4 is the one allowed deterministic step. To confirm in a probe on the smallest consumer.

"Has an evaluator" stops being a tree-path test. The host's `MODULE.bazel` at the base names the pin, so the test is whether the base can build `@bff_template_quality//checks/coverage:floors`, which `from_base.sh` already tries. `ResolveBase` keeps the host-owned contract path, and "no evaluator" becomes a named failure of that build.

## R3 Consumer shape

The smallest real different-domain consumer: one Go package with real behavior and tests. The template already exercises Go, TypeScript, proto and browser, so this consumer has one job: to prove reusable bindings, a distinct source scope and upgrade behavior. A proto, or TypeScript, joins only if a probe shows it earns its place. No Python. Its `required_tests`, legs and floors are its own. A check with nothing to judge is not registered, the same rule as M5.

## R4 Pins and both adapters

Parent T072 requires both, against a local `file://` remote:

| Adapter | Pin | Notes |
| --- | --- | --- |
| Bazel | `git_override(module_name = "bff_template_quality", remote = "file://…", commit = "<sha>", strip_prefix = "quality")` | Bazel's docs say it forwards to `git_repository`, that it is root-only, and that `strip_prefix` is supported. Execution with the pinned Bazel still has to be shown by the probe |
| prek | `repo = "file://…"`, `rev = "<sha>"` | prek's existing remote-hook interface, with no `repo = "local"` fallback |

**One canonical choice.** The full commit SHA, in one consumer file, with a small named step that writes the second adapter from it. The second value is then a mechanical effect, shown in the diff, and not a second hand-edited pin. Whether `MODULE.bazel` or a one-line pin file is the source is for the probe.

**A remote hook repository needs real hooks.** prek reads `.pre-commit-hooks.yaml` from the root of the repository at `rev`, and the module lives in `quality/`, so the tree root must expose hooks that call the pinned runner. Those hooks are real remote adoption. A copied wrapper is not, and is not counted. This does not claim upstream #25: prek's interface is mature, and the shared stage contract stays unreleased and separately tracked.

## R5 Versions, tags and integrity

N is a real local tag, cut after the consumer interface works. M6's source cannot be N. The module is `0.1.0` today.

**What is proved, exactly:**

| Claim | Witness |
| --- | --- |
| A consumer pinned to a full SHA is unaffected by a tag moving elsewhere | Move the tag in the `file://` remote; the consumer still resolves its commit, and the verdict does not change. This is a positive |
| Release tooling refuses to change a recorded tag-to-commit mapping | A release-procedure check over a tracked record. It runs in the release step, and no consumer sees it |

An immutable SHA does not fail because a tag moved, and it should not. No issuer, authorization catalog, allowlist or authorization gate is built.

**N+1 needs one real, compatible rule improvement**, so a clean consumer upgrades by the pin alone, and the notes must show it. A new S-code alone does not: `suppression-ban` stays one unchanged registry entry, so a registry diff would be silent. Two fixes are open: represent suppression rules in the single catalog being diffed, or choose a real compatible improvement that is already registry-visible. Neither an arbitrary floor rise nor a second handwritten rule definition is acceptable as a witness. Its evidence has three parts: the clean consumer passes at N+1; a deliberate defect is caught at N+1; the restored control passes. If evidence shows a migration is unavoidable, that goes to review, and the criterion stands.

## R6 Upstream

Python #30 is the RFC. Read directly at T001, on 2026-11-01: Python #25 to #31 and TypeScript #20 are all open, with no comments and no update since 2026-10-28, and no release exists. Nothing upstream is adopted here, and each stays a named gap.

## R7 Probe results, T002

A separate repository, on the pinned Bazel 9.3.0 and prek 0.5.5, against `file://` bare remotes. Scripts and raw logs are outside this repository. The first prek attempts were setup failures and earn nothing.

| Question | Observed |
| --- | --- |
| Does `git_override` with `strip_prefix = "quality"` execute? | Yes. `//:quality`, `floors`, `suppression-ban` and `prek` built from a full SHA. A floating version is not needed: the override decides |
| Canonical name of the module | `bff_template_quality+`, as in this repository, so `from_base.sh`'s path and `reportPath` hold unchanged. The R1 row needs no change |
| Does the module's runner start in a consumer? | `pre-commit` exits 0 with everything skipped, and `full` exits 2: `tools/speckit/scaffold.sha256` is missing. The registry's host paths fail first, before any base rule |
| prek `repo`/`rev` on `file://` | A full SHA fetches into prek's own cache. `language: script` runs that copy, with the working directory and Git top level in the consumer, and hook `args` arrive after `entry`'s. A SHA that does not exist exits 2, and the restored pin passes |

So a hook's script can come from the pinned tree, and a host can name its own paths through hook arguments.

## R8 Decision: the binding

The third option of R2, kept to plain data.

- `quality/rules.textproto` is the rule catalog: the one definition of each rule a repository can bind, and the file release notes are diffed from. It has the `Registry` schema, and its checks carry no leg and no host path. A rule's banned codes are listed in the catalog, so a code added inside `suppression-ban` is visible, and a test keeps the list equal to the code.
- A host registry gains `rules`. Each entry names a rule id, its leg, and any `args`, `context` paths or Bazel `targets` that are the host's. The entry appends these to the rule. There are no placeholders, expressions or conditionals. A host's own scripts stay ordinary `checks` in its registry.
- This repository's registry stays at `quality/registry.textproto`, and its contract at `quality/coverage_floors.json`, so the accepted base `e3b3b34` can still judge it: that base's evaluator reads those paths. Those two files are this repository's host files. A consumer pins past them and reads neither.
- The scripts a rule runs move into the module: `from_base.sh` and `test_evidence.sh`, and `stage.sh` with `install.sh` behind thin local paths. Otherwise a consumer would hold a copy.
- A consumer's Bazel wrapper stays its own, because it is what fetches Bazel. It is named to the runner, and must accept startup options and `--path`.
- The evaluator test becomes: the base has the contract the host names, and the base can build `@bff_template_quality//checks/coverage:floors`. A failed build is a named refusal, exit 11.

## R9 One pin

`git_override`'s `commit` in `MODULE.bazel` is the canonical value, because Bazel can only read it there. prek's `rev` is written from it by `@bff_template_quality//hooks:pin`, and the `quality-pin` rule fails if the two differ, if the remotes differ, or if `local_path_override` or a local hook stands in.
